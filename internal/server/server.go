package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"reviews/internal/reviewjson"
	"reviews/internal/store"
)

type Config struct {
	Addr               string
	StaticDir          string
	ProductURLTemplate string
	ProductLinks       map[string]string
	// ProductLinksPath is where the crawled product-link list is persisted so
	// the in-admin "refresh products" action can rewrite it.
	ProductLinksPath string
	// SitemapURL is the shop sitemap crawled by the refresh action.
	SitemapURL string
	SessionTTL time.Duration
	SecureCookies      bool
	TriggerSync        func(marketplaces []string)
	Marketplaces       []MarketplaceStatus
	// AllowedOrigins lists shop origins permitted to fetch public reviews
	// data cross-origin (the embedding site). Empty disables CORS.
	AllowedOrigins []string
}

type Server struct {
	store  *store.Store
	cfg    Config
	logger *slog.Logger
	server *http.Server

	// linksMu guards cfg.ProductLinks, which the refresh-products action swaps
	// at runtime while request handlers read it.
	linksMu sync.RWMutex
}

// productLinks returns the current article→URL map under a read lock.
func (s *Server) productLinks() map[string]string {
	s.linksMu.RLock()
	defer s.linksMu.RUnlock()
	return s.cfg.ProductLinks
}

// setProductLinks atomically swaps the in-memory article→URL map.
func (s *Server) setProductLinks(links map[string]string) {
	s.linksMu.Lock()
	defer s.linksMu.Unlock()
	s.cfg.ProductLinks = links
}

func New(store *store.Store, cfg Config, logger *slog.Logger) *Server {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8080"
	}
	if cfg.StaticDir == "" {
		cfg.StaticDir = "web/reviews-widget"
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 24 * time.Hour
	}
	return &Server{
		store:  store,
		cfg:    cfg,
		logger: logger,
	}
}

// handler builds the full public + admin route tree wrapped in the middleware
// chain. Exposed for tests so the CORS/security middleware is exercised.
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/reviews", s.handleReviews)
	mux.HandleFunc("GET /api/showcase", s.handleShowcase)
	mux.HandleFunc("GET /api/widget-config", s.handlePublicWidgetConfig)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.Handle("/admin/", s.adminMux())
	mux.Handle("/", http.FileServer(http.Dir(s.cfg.StaticDir)))

	return securityHeaders(s.cors(s.logRequests(mux)))
}

func (s *Server) Run(ctx context.Context) error {
	s.server = &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           s.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		s.logger.Info("server listening", "addr", s.cfg.Addr, "static_dir", s.cfg.StaticDir)
		errs <- s.server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// adminMux builds the admin API + SPA routes. Setup and login are public;
// everything below /admin/api/ requires a valid session.
func (s *Server) adminMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/setup-status", s.handleSetupStatus)
	mux.HandleFunc("POST /admin/api/setup", s.handleSetup)
	mux.HandleFunc("POST /admin/api/login", s.handleLogin)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /admin/api/me", s.handleMe)
	protected.HandleFunc("GET /admin/api/csrf", s.handleCSRFToken)
	protected.HandleFunc("GET /admin/api/reviews", s.handleAdminReviews)
	protected.Handle("POST /admin/api/reviews/publish", requireCSRF(http.HandlerFunc(s.handlePublishReviewsData)))
	protected.Handle("POST /admin/api/reviews/bulk", requireCSRF(http.HandlerFunc(s.handleAdminReviewsBulkModerate)))
	protected.Handle("PATCH /admin/api/reviews/{id}", requireCSRF(http.HandlerFunc(s.handleAdminReviewModerate)))
	protected.Handle("DELETE /admin/api/reviews/{id}", requireCSRF(http.HandlerFunc(s.handleAdminReviewDelete)))
	protected.Handle("POST /admin/api/reviews/{id}/restore", requireCSRF(http.HandlerFunc(s.handleAdminReviewRestore)))
	protected.Handle("PUT /admin/api/reviews/{id}/reply", requireCSRF(http.HandlerFunc(s.handleAdminReviewReply)))
	protected.HandleFunc("GET /admin/api/articles/{article}/pins", s.handleListArticlePins)
	protected.Handle("PUT /admin/api/articles/{article}/pins", requireCSRF(http.HandlerFunc(s.handleReplaceArticlePins)))
	protected.Handle("DELETE /admin/api/articles/{article}/pins/{reviewID}", requireCSRF(http.HandlerFunc(s.handleRemoveArticlePin)))
	protected.HandleFunc("GET /admin/api/dashboard", s.handleDashboard)
	protected.HandleFunc("GET /admin/api/marketplaces", s.handleMarketplaces)
	protected.Handle("PUT /admin/api/marketplaces/{id}/credentials", requireCSRF(http.HandlerFunc(s.handleSaveMarketplaceCredentials)))
	protected.Handle("POST /admin/api/sync", requireCSRF(http.HandlerFunc(s.handleTriggerSync)))
	protected.Handle("POST /admin/api/site-links/refresh", requireCSRF(http.HandlerFunc(s.handleRefreshSiteLinks)))
	protected.HandleFunc("GET /admin/api/showcase-rule", s.handleGetShowcaseRule)
	protected.Handle("PUT /admin/api/showcase-rule", requireCSRF(http.HandlerFunc(s.handlePutShowcaseRule)))
	protected.HandleFunc("GET /admin/api/widget-config/{context}", s.handleGetWidgetConfig)
	protected.HandleFunc("GET /admin/api/widget-config/{context}/versions", s.handleListWidgetConfigVersions)
	protected.Handle("POST /admin/api/widget-config/{context}", requireCSRF(http.HandlerFunc(s.handlePublishWidgetConfig)))
	protected.Handle("POST /admin/api/widget-config/{context}/rollback/{version}", requireCSRF(http.HandlerFunc(s.handleRollbackWidgetConfig)))
	protected.Handle("POST /admin/api/logout", requireCSRF(http.HandlerFunc(s.handleLogout)))
	mux.Handle("/admin/api/", s.requireSession(protected))

	mux.Handle("/admin/", s.adminSPAHandler())
	return mux
}

func (s *Server) handleReviews(w http.ResponseWriter, r *http.Request) {
	filter, err := parseReviewFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	filter.Visibility = "visible"

	var reviews []store.Review
	defaults, ranking, useWidgetRules := s.reviewRulesForRequest(r)
	if useWidgetRules {
		reviews, err = s.store.ListReviewsByWidgetRules(r.Context(), filter, defaults, ranking)
	} else {
		reviews, err = s.store.ListReviews(r.Context(), filter)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	mapper := reviewjson.Mapper{
		ProductURLTemplate: s.cfg.ProductURLTemplate,
		ProductLinks:       s.productLinks(),
	}
	items := make([]reviewjson.Review, 0, len(reviews))
	for _, review := range reviews {
		items = append(items, mapper.ToReview(review))
	}
	writeJSON(w, http.StatusOK, reviewsResponse{Reviews: items, Count: len(items)})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Debug("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func parseReviewFilter(r *http.Request) (store.ReviewListFilter, error) {
	query := r.URL.Query()
	filter := store.ReviewListFilter{
		Marketplace: query.Get("marketplace"),
		Limit:       parseInt(query.Get("limit"), 100),
		Offset:      parseInt(query.Get("offset"), 0),
		SortBy:      query.Get("sort"),
	}

	if rating := query.Get("rating"); rating != "" && rating != "all" {
		parsed, err := strconv.Atoi(rating)
		if err != nil {
			return store.ReviewListFilter{}, fmt.Errorf("rating must be a number")
		}
		filter.Rating = parsed
	}

	return filter, nil
}

type widgetRulesPayload struct {
	Defaults *store.WidgetReviewDefaults `json:"defaults"`
	Ranking  []store.ReviewRankingRule   `json:"ranking"`
}

func (s *Server) reviewRulesForRequest(r *http.Request) (store.WidgetReviewDefaults, []store.ReviewRankingRule, bool) {
	query := r.URL.Query()
	if query.Get("apply_config") == "0" || query.Get("applyConfig") == "0" {
		return store.WidgetReviewDefaults{}, nil, false
	}
	contextName := query.Get("context")
	if contextName == "" {
		contextName = "product"
	}
	if err := validateWidgetContext(contextName); err != nil {
		return store.WidgetReviewDefaults{}, nil, false
	}
	cfg, err := s.store.GetActiveWidgetConfig(r.Context(), contextName)
	if err != nil {
		return store.WidgetReviewDefaults{}, nil, false
	}
	var payload widgetRulesPayload
	if err := json.Unmarshal([]byte(cfg.Payload), &payload); err != nil {
		return store.WidgetReviewDefaults{}, nil, false
	}
	defaults := store.DefaultWidgetReviewDefaults()
	if payload.Defaults != nil {
		defaults = *payload.Defaults
	}
	ranking := payload.Ranking
	if len(ranking) == 0 {
		ranking = store.DefaultReviewRanking()
	}
	return defaults, ranking, true
}

func parseInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

type reviewsResponse struct {
	Reviews []reviewjson.Review `json:"reviews"`
	Count   int                 `json:"count"`
}

func StaticDirExists(path string) error {
	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}
