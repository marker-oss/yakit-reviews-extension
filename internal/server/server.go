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
	"strings"
	"sync"
	"time"

	"reviews/internal/config"
	"reviews/internal/marketplace"
	"reviews/internal/mediaproxy"
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
	SitemapURL    string
	SessionTTL    time.Duration
	SecureCookies bool
	TriggerSync   func(marketplaces []string)
	Marketplaces  []MarketplaceStatus
	// AllowedOrigins lists shop origins permitted to fetch public reviews
	// data cross-origin (the embedding site). Empty disables CORS.
	AllowedOrigins []string
	// Media configures the face-blur image proxy (/media route).
	Media config.MediaConfig
	// Review submission settings.
	UploadDir      string
	PrivacyURL     string
	ReviewTermsURL string
	// ReplyPublishers maps marketplace id → publisher for posting seller
	// replies back to the marketplace. Marketplaces absent here are treated
	// as "unsupported".
	ReplyPublishers map[string]marketplace.ReplyPublisher
	// QuestionAnswerPublishers maps marketplace id → publisher for posting
	// seller answers to product questions back to the marketplace.
	QuestionAnswerPublishers map[string]marketplace.QuestionAnswerPublisher
}

type Server struct {
	store                    *store.Store
	cfg                      Config
	logger                   *slog.Logger
	server                   *http.Server
	submissions              *submissionLimiter
	replyPublishers          map[string]marketplace.ReplyPublisher
	questionAnswerPublishers map[string]marketplace.QuestionAnswerPublisher

	// linksMu guards cfg.ProductLinks, which the refresh-products action swaps
	// at runtime while request handlers read it.
	linksMu sync.RWMutex

	// siteLinksMu guards siteLinksJob, the status snapshot of the background
	// catalog refresh (single job slot).
	siteLinksMu  sync.Mutex
	siteLinksJob siteLinksStatus
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
		store:                    store,
		cfg:                      cfg,
		logger:                   logger,
		replyPublishers:          cfg.ReplyPublishers,
		questionAnswerPublishers: cfg.QuestionAnswerPublishers,
	}
}

// handler builds the full public + admin route tree wrapped in the middleware
// chain. Exposed for tests so the CORS/security middleware is exercised.
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/reviews", s.handleReviews)
	mux.HandleFunc("GET /api/showcase", s.handleShowcase)
	mux.HandleFunc("GET /api/widget-config", s.handlePublicWidgetConfig)
	mux.HandleFunc("GET /api/review-submission-config", s.handleReviewSubmissionConfig)
	mux.HandleFunc("POST /api/review-submissions", s.handleCreateReviewSubmission)
	mux.HandleFunc("GET /api/questions", s.handlePublicQuestions)
	mux.HandleFunc("GET /api/question-submission-config", s.handleQuestionSubmissionConfig)
	mux.HandleFunc("POST /api/questions", s.handleCreateQuestionSubmission)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /user-media/{token}", s.handleUserMedia)
	if mediaHandler, err := mediaproxy.NewHandler(s.cfg.Media, nil); err == nil {
		mux.Handle("GET /media", mediaHandler)
	} else {
		s.logger.Error("media proxy disabled", "error", err)
	}
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
	protected.Handle("DELETE /admin/api/reviews/{id}/purge", requireCSRF(http.HandlerFunc(s.handleAdminReviewPurge)))
	protected.Handle("POST /admin/api/reviews/{id}/restore", requireCSRF(http.HandlerFunc(s.handleAdminReviewRestore)))
	protected.Handle("PUT /admin/api/reviews/{id}/reply", requireCSRF(http.HandlerFunc(s.handleAdminReviewReply)))
	protected.Handle("POST /admin/api/reviews/{id}/reply/retry", requireCSRF(http.HandlerFunc(s.handleAdminReviewReplyRetry)))
	protected.HandleFunc("GET /admin/api/articles/{article}/pins", s.handleListArticlePins)
	protected.Handle("PUT /admin/api/articles/{article}/pins", requireCSRF(http.HandlerFunc(s.handleReplaceArticlePins)))
	protected.Handle("DELETE /admin/api/articles/{article}/pins/{reviewID}", requireCSRF(http.HandlerFunc(s.handleRemoveArticlePin)))
	protected.HandleFunc("GET /admin/api/dashboard", s.handleDashboard)
	protected.HandleFunc("GET /admin/api/marketplaces", s.handleMarketplaces)
	protected.Handle("PUT /admin/api/marketplaces/{id}/credentials", requireCSRF(http.HandlerFunc(s.handleSaveMarketplaceCredentials)))
	protected.HandleFunc("GET /admin/api/settings", s.handleGetSettings)
	protected.Handle("PUT /admin/api/settings", requireCSRF(http.HandlerFunc(s.handlePutSettings)))
	protected.Handle("POST /admin/api/sync", requireCSRF(http.HandlerFunc(s.handleTriggerSync)))
	protected.Handle("POST /admin/api/site-links/refresh", requireCSRF(http.HandlerFunc(s.handleRefreshSiteLinks)))
	protected.HandleFunc("GET /admin/api/site-links/refresh", s.handleSiteLinksRefreshStatus)
	protected.HandleFunc("GET /admin/api/showcase-rule", s.handleGetShowcaseRule)
	protected.Handle("PUT /admin/api/showcase-rule", requireCSRF(http.HandlerFunc(s.handlePutShowcaseRule)))
	protected.HandleFunc("GET /admin/api/widget-config/{context}", s.handleGetWidgetConfig)
	protected.HandleFunc("GET /admin/api/widget-config/{context}/versions", s.handleListWidgetConfigVersions)
	protected.Handle("POST /admin/api/widget-config/{context}", requireCSRF(http.HandlerFunc(s.handlePublishWidgetConfig)))
	protected.Handle("POST /admin/api/widget-config/{context}/rollback/{version}", requireCSRF(http.HandlerFunc(s.handleRollbackWidgetConfig)))
	protected.Handle("POST /admin/api/logout", requireCSRF(http.HandlerFunc(s.handleLogout)))
	protected.HandleFunc("GET /admin/api/questions", s.handleAdminQuestions)
	protected.Handle("PUT /admin/api/questions/{id}/answer", requireCSRF(http.HandlerFunc(s.handleAdminQuestionAnswer)))
	protected.Handle("POST /admin/api/questions/{id}/answer/retry", requireCSRF(http.HandlerFunc(s.handleAdminQuestionAnswerRetry)))
	protected.HandleFunc("GET /admin/api/dsr/lookup", s.handleDSRLookup)
	protected.HandleFunc("GET /admin/api/dsr/export", s.handleDSRExport)
	protected.Handle("POST /admin/api/dsr/delete", requireCSRF(http.HandlerFunc(s.handleDSRDelete)))
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
	filter.Status = "public"

	var reviews []store.Review
	defaults, ranking, marketplacePolicy, useWidgetRules := s.reviewRulesForRequest(r)
	filter.ExcludedMarketplaces = marketplacePolicy.ExcludedMarketplaces()
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
		MarketplacePolicy:  marketplacePolicy,
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

// publicQuestion is the minimal public shape returned by GET /api/questions.
// Static export of questions is deferred (live API only for now).
type publicQuestion struct {
	Question string    `json:"question"`
	Answer   string    `json:"answer"`
	Date     time.Time `json:"date"`
}

// handlePublicQuestions returns answered+visible questions for an article.
// Questions are served live-API-only; static bundle inclusion is deferred.
func (s *Server) handlePublicQuestions(w http.ResponseWriter, r *http.Request) {
	article := strings.TrimSpace(r.URL.Query().Get("article"))
	questions, err := s.store.ListQuestions(r.Context(), store.QuestionFilter{
		Visibility:    "visible",
		SellerArticle: article,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := make([]publicQuestion, 0, len(questions))
	for _, q := range questions {
		if q.AnswerText == nil || *q.AnswerText == "" {
			continue
		}
		date := q.CreatedAtMP
		if q.AnswerAt != nil {
			date = *q.AnswerAt
		}
		items = append(items, publicQuestion{
			Question: q.Text,
			Answer:   *q.AnswerText,
			Date:     date,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"questions": items})
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
	Defaults          *store.WidgetReviewDefaults    `json:"defaults"`
	Ranking           []store.ReviewRankingRule      `json:"ranking"`
	MarketplacePolicy reviewjson.MarketplacePolicies `json:"marketplacePolicy"`
}

func (s *Server) reviewRulesForRequest(r *http.Request) (store.WidgetReviewDefaults, []store.ReviewRankingRule, reviewjson.MarketplacePolicies, bool) {
	query := r.URL.Query()
	if query.Get("apply_config") == "0" || query.Get("applyConfig") == "0" {
		return store.WidgetReviewDefaults{}, nil, nil, false
	}
	contextName := query.Get("context")
	if contextName == "" {
		contextName = "product"
	}
	if err := validateWidgetContext(contextName); err != nil {
		return store.WidgetReviewDefaults{}, nil, nil, false
	}
	cfg, err := s.store.GetActiveWidgetConfig(r.Context(), contextName)
	if err != nil {
		return store.WidgetReviewDefaults{}, nil, nil, false
	}
	var payload widgetRulesPayload
	if err := json.Unmarshal([]byte(cfg.Payload), &payload); err != nil {
		return store.WidgetReviewDefaults{}, nil, nil, false
	}
	defaults := store.DefaultWidgetReviewDefaults()
	if payload.Defaults != nil {
		defaults = *payload.Defaults
	}
	ranking := payload.Ranking
	if len(ranking) == 0 {
		ranking = store.DefaultReviewRanking()
	}
	return defaults, ranking, payload.MarketplacePolicy.Normalized(), true
}

func (s *Server) activeMarketplacePolicy(ctx context.Context, widgetContext string) reviewjson.MarketplacePolicies {
	if err := validateWidgetContext(widgetContext); err != nil {
		return nil
	}
	cfg, err := s.store.GetActiveWidgetConfig(ctx, widgetContext)
	if err != nil {
		return nil
	}
	return reviewjson.ParseMarketplacePolicies(cfg.Payload)
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
