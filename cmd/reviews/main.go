package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"reviews/internal/collector"
	"reviews/internal/config"
	"reviews/internal/export"
	"reviews/internal/marketplace"
	"reviews/internal/marketplace/wb"
	"reviews/internal/marketplace/ym"
	"reviews/internal/reviewjson"
	"reviews/internal/scheduler"
	"reviews/internal/server"
	"reviews/internal/site"
	"reviews/internal/store"
)

const (
	exitOK          = 0
	exitRunError    = 1
	exitConfigError = 2
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return exitConfigError
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return exitConfigError
	}

	logger := newLogger(cfg.Log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch args[0] {
	case "migrate":
		return runMigrate(ctx, cfg, logger)
	case "sync":
		return runSync(ctx, args[1:], cfg, logger)
	case "serve":
		return runServe(ctx, args[1:], cfg, logger)
	case "discover-site-urls":
		return runDiscoverSiteURLs(ctx, args[1:], cfg, logger)
	case "export":
		return runExport(ctx, args[1:], cfg, logger)
	case "-h", "--help", "help":
		usage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		usage()
		return exitConfigError
	}
}

func runMigrate(ctx context.Context, cfg config.Config, logger *slog.Logger) int {
	db, err := store.Open(cfg.DB)
	if err != nil {
		logger.Error("open database", "error", err)
		return exitConfigError
	}

	if err := db.Migrate(ctx); err != nil {
		logger.Error("migrate database", "error", err)
		return exitRunError
	}

	logger.Info("migrations applied", "driver", cfg.DB.Driver)
	return exitOK
}

func runSync(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) int {
	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	once := flags.Bool("once", false, "run one sync and exit")
	marketplace := flags.String("marketplace", "", "sync only one marketplace")
	if err := flags.Parse(args); err != nil {
		return exitConfigError
	}

	if !*once {
		fmt.Fprintln(os.Stderr, "sync currently requires --once")
		return exitConfigError
	}

	if *marketplace != "" && !config.IsKnownMarketplace(*marketplace) {
		fmt.Fprintf(os.Stderr, "unknown marketplace: %s\n", *marketplace)
		return exitConfigError
	}
	if err := cfg.ValidateMarketplaceCredentials(*marketplace); err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return exitConfigError
	}

	db, err := store.Open(cfg.DB)
	if err != nil {
		logger.Error("open database", "error", err)
		return exitConfigError
	}
	if err := db.Migrate(ctx); err != nil {
		logger.Error("migrate database", "error", err)
		return exitRunError
	}

	runner := collector.NewRunner(db, cfg.Sync, logger, buildAdapters(cfg))
	marketplaces := cfg.EnabledMarketplaces()
	if *marketplace != "" {
		marketplaces = []string{*marketplace}
	}

	results := runner.RunOnce(ctx, marketplaces)
	var failed bool
	for _, result := range results {
		if result.Error != nil {
			failed = true
			logger.Error("sync marketplace failed", "marketplace", result.Marketplace, "error", result.Error)
			continue
		}
		logger.Info("sync marketplace ok", "marketplace", result.Marketplace, "seen", result.Seen, "upserted", result.Upserted)
	}
	if failed {
		return exitRunError
	}
	return exitOK
}

func runServe(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	addr := flags.String("addr", "127.0.0.1:8080", "HTTP listen address")
	staticDir := flags.String("static-dir", "web/reviews-widget", "static widget directory")
	productURLTemplate := flags.String("product-url-template", cfg.Web.ProductURLTemplate, "seller product URL template")
	withSync := flags.Bool("with-sync", false, "run periodic review sync inside the server process")
	if err := flags.Parse(args); err != nil {
		return exitConfigError
	}

	if err := server.StaticDirExists(*staticDir); err != nil {
		logger.Error("static directory", "path", *staticDir, "error", err)
		return exitConfigError
	}

	db, err := store.Open(cfg.DB)
	if err != nil {
		logger.Error("open database", "error", err)
		return exitConfigError
	}
	if err := db.Migrate(ctx); err != nil {
		logger.Error("migrate database", "error", err)
		return exitRunError
	}

	runner := collector.NewRunner(db, cfg.Sync, logger, buildAdapters(cfg))
	triggerSync := func(marketplaces []string) {
		if len(marketplaces) == 0 {
			marketplaces = cfg.EnabledMarketplaces()
		}
		for _, result := range runner.RunOnce(ctx, marketplaces) {
			if result.Error != nil {
				logger.Error("manual sync marketplace failed", "marketplace", result.Marketplace, "error", result.Error)
				continue
			}
			logger.Info("manual sync marketplace ok", "marketplace", result.Marketplace, "seen", result.Seen, "upserted", result.Upserted)
		}
	}

	if *withSync {
		if err := cfg.ValidateMarketplaceCredentials(""); err != nil {
			logger.Error("sync credentials invalid", "error", err)
			return exitConfigError
		}
		sched := scheduler.New(
			syncRunnerAdapter{runner: runner, logger: logger},
			cfg.Sync.Interval,
			cfg.EnabledMarketplaces(),
			logger,
		)
		go sched.Run(ctx)
		logger.Info("in-process sync scheduler started", "interval", cfg.Sync.Interval.String())
	}

	httpServer := server.New(db, server.Config{
		Addr:               *addr,
		StaticDir:          *staticDir,
		ProductURLTemplate: *productURLTemplate,
		ProductLinks:       loadProductLinks(cfg.Web.ProductLinksPath, logger),
		SessionTTL:         24 * time.Hour,
		SecureCookies:      os.Getenv("REVIEWS_INSECURE_COOKIES") == "",
		TriggerSync:        triggerSync,
		Marketplaces:       marketplaceStatuses(cfg),
	}, logger)
	if err := httpServer.Run(ctx); err != nil {
		logger.Error("server stopped with error", "error", err)
		return exitRunError
	}

	logger.Info("shutdown complete")
	return exitOK
}

// syncRunnerAdapter adapts collector.Runner to scheduler.Runner and logs the
// per-marketplace results that the scheduler intentionally ignores.
type syncRunnerAdapter struct {
	runner *collector.Runner
	logger *slog.Logger
}

func (a syncRunnerAdapter) RunOnce(ctx context.Context, marketplaces []string) {
	for _, result := range a.runner.RunOnce(ctx, marketplaces) {
		if result.Error != nil {
			a.logger.Error("scheduled sync marketplace failed", "marketplace", result.Marketplace, "error", result.Error)
			continue
		}
		a.logger.Info("scheduled sync marketplace ok", "marketplace", result.Marketplace, "seen", result.Seen, "upserted", result.Upserted)
	}
}

func runDiscoverSiteURLs(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) int {
	flags := flag.NewFlagSet("discover-site-urls", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	sitemapURL := flags.String("sitemap", "https://shegida.ru/sitemap.xml", "sitemap URL")
	out := flags.String("out", cfg.Web.ProductLinksPath, "output JSON path")
	timeout := flags.Duration("timeout", 2*time.Minute, "scan timeout")
	if err := flags.Parse(args); err != nil {
		return exitConfigError
	}

	scanCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	links, err := site.DiscoverShegidaProductLinks(scanCtx, *sitemapURL, nil)
	if err != nil {
		logger.Error("discover site URLs", "error", err)
		return exitRunError
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		logger.Error("create output dir", "path", filepath.Dir(*out), "error", err)
		return exitRunError
	}
	file, err := os.Create(*out)
	if err != nil {
		logger.Error("create output file", "path", *out, "error", err)
		return exitRunError
	}
	defer file.Close()

	if err := site.EncodeProductLinks(file, links); err != nil {
		logger.Error("write product links", "path", *out, "error", err)
		return exitRunError
	}

	logger.Info("site URLs discovered", "count", len(links), "out", *out)
	return exitOK
}

func runExport(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) int {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	outDir := flags.String("out", "web/reviews-data", "output directory for static JSON")
	productURLTemplate := flags.String("product-url-template", cfg.Web.ProductURLTemplate, "seller product URL template")
	if err := flags.Parse(args); err != nil {
		return exitConfigError
	}

	db, err := store.Open(cfg.DB)
	if err != nil {
		logger.Error("open database", "error", err)
		return exitConfigError
	}

	reviews, err := db.ListAllReviews(ctx)
	if err != nil {
		logger.Error("list reviews", "error", err)
		return exitRunError
	}

	mapper := reviewjson.Mapper{
		ProductURLTemplate: *productURLTemplate,
		ProductLinks:       loadProductLinks(cfg.Web.ProductLinksPath, logger),
	}
	bundles := export.BuildBundles(reviews, mapper)

	if err := export.Write(*outDir, bundles, time.Now().UTC()); err != nil {
		logger.Error("write export", "out", *outDir, "error", err)
		return exitRunError
	}

	logger.Info("export complete", "articles", len(bundles), "reviews", len(reviews), "out", *outDir)
	return exitOK
}

func newLogger(cfg config.LogConfig) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}
	if strings.EqualFold(cfg.Format, "json") {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func emptyAsAll(value string) string {
	if value == "" {
		return "all"
	}
	return value
}

func buildAdapters(cfg config.Config) []marketplace.Adapter {
	var adapters []marketplace.Adapter
	if cfg.Marketplaces.WB.Enabled {
		adapters = append(adapters, wb.New(cfg.Marketplaces.WB))
	}
	if cfg.Marketplaces.YM.Enabled {
		adapters = append(adapters, ym.New(cfg.Marketplaces.YM))
	}
	return adapters
}

func marketplaceStatuses(cfg config.Config) []server.MarketplaceStatus {
	return []server.MarketplaceStatus{
		{
			ID:         config.MarketplaceWB,
			Enabled:    cfg.Marketplaces.WB.Enabled,
			Configured: cfg.Marketplaces.WB.Token != "",
		},
		{
			ID:      config.MarketplaceYM,
			Enabled: cfg.Marketplaces.YM.Enabled,
			Configured: cfg.Marketplaces.YM.BusinessID != "" &&
				(cfg.Marketplaces.YM.APIKey != "" || cfg.Marketplaces.YM.OAuthToken != ""),
		},
		{
			ID:         config.MarketplaceOzon,
			Enabled:    cfg.Marketplaces.Ozon.Enabled,
			Configured: cfg.Marketplaces.Ozon.ClientID != "" && cfg.Marketplaces.Ozon.APIKey != "",
		},
	}
}

func loadProductLinks(path string, logger *slog.Logger) map[string]string {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		logger.Warn("open product links", "path", path, "error", err)
		return nil
	}
	defer file.Close()
	links, err := site.LoadProductLinkMap(file)
	if err != nil {
		logger.Warn("load product links", "path", path, "error", err)
		return nil
	}
	logger.Info("product links loaded", "path", path, "count", len(links))
	return links
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage:
  reviews migrate
  reviews sync --once [--marketplace wb|ym|ozon]
  reviews serve [--addr 127.0.0.1:8080] [--with-sync]
  reviews discover-site-urls
  reviews export [--out web/reviews-data]

Environment:
  REVIEWS_DB_DRIVER=sqlite|postgres
  REVIEWS_DB_DSN=./reviews.db
`)
}
