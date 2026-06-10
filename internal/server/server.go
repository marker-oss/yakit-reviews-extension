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
	"time"

	"reviews/internal/reviewjson"
	"reviews/internal/store"
)

type Config struct {
	Addr               string
	StaticDir          string
	ProductURLTemplate string
	ProductLinks       map[string]string
	SessionTTL         time.Duration
	SecureCookies      bool
	TriggerSync        func(marketplaces []string)
	Marketplaces       []MarketplaceStatus
}

type Server struct {
	store  *store.Store
	cfg    Config
	logger *slog.Logger
	server *http.Server
}

func New(store *store.Store, cfg Config, logger *slog.Logger) *Server {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8080"
	}
	if cfg.StaticDir == "" {
		cfg.StaticDir = "web/reviews-widget"
	}
	if cfg.ProductURLTemplate == "" {
		cfg.ProductURLTemplate = "https://shegida.ru/search?query={seller_article_url}"
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

func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/reviews", s.handleReviews)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.Handle("/admin/", s.adminMux())
	mux.Handle("/", http.FileServer(http.Dir(s.cfg.StaticDir)))

	s.server = &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           securityHeaders(s.logRequests(mux)),
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
	protected.Handle("PATCH /admin/api/reviews/{id}", requireCSRF(http.HandlerFunc(s.handleAdminReviewModerate)))
	protected.HandleFunc("GET /admin/api/dashboard", s.handleDashboard)
	protected.HandleFunc("GET /admin/api/marketplaces", s.handleMarketplaces)
	protected.Handle("POST /admin/api/sync", requireCSRF(http.HandlerFunc(s.handleTriggerSync)))
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
	reviews, err := s.store.ListReviews(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	mapper := reviewjson.Mapper{
		ProductURLTemplate: s.cfg.ProductURLTemplate,
		ProductLinks:       s.cfg.ProductLinks,
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
