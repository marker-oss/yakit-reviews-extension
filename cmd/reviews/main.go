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

	"reviews/internal/auth"
	"reviews/internal/config"
	"reviews/internal/export"
	"reviews/internal/installer"
	"reviews/internal/marketplace/apihttp"
	"reviews/internal/marketplace/wbtoken"
	"reviews/internal/reviewjson"
	"reviews/internal/scheduler"
	"reviews/internal/server"
	"reviews/internal/site"
	"reviews/internal/store"
	"reviews/internal/syncer"
)

const (
	exitOK          = 0
	exitRunError    = 1
	exitConfigError = 2
)

// version is stamped by the release workflow via
// -ldflags "-X main.version=vX.Y.Z"; local builds stay "dev".
var version = "dev"

// latestReleaseURL is the feed the admin update-banner checks once a day.
const latestReleaseURL = "https://api.github.com/repos/marker-oss/yakit-reviews-extension/releases/latest"

// updateCheckURL resolves the release feed for the update banner.
// REVIEWS_UPDATE_CHECK=false disables the daily lookup entirely; any other
// non-empty value overrides the feed URL (useful for mirrors and tests).
func updateCheckURL() string {
	switch value := os.Getenv("REVIEWS_UPDATE_CHECK"); value {
	case "":
		return latestReleaseURL
	case "false", "0", "off":
		return ""
	default:
		return value
	}
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return exitConfigError
	}

	if args[0] == "version" || args[0] == "--version" {
		fmt.Println(version)
		return exitOK
	}

	if args[0] == "install" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return runInstall(ctx, args[1:])
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return exitConfigError
	}

	logger := newLogger(cfg.Log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// One process serves one tenant (variant A: container per tenant); every
	// job, CLI subcommand, and background loop inherits it from here.
	ctx = store.WithTenant(ctx, store.DefaultTenantID)

	switch args[0] {
	case "admin":
		return runAdmin(ctx, args[1:], cfg, logger)
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

func runInstall(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return exitConfigError
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "install does not accept positional arguments: %s\n", strings.Join(flags.Args(), " "))
		return exitConfigError
	}
	if err := installer.RunTUI(ctx, installer.TUIOptions{}); err != nil {
		fmt.Fprintf(os.Stderr, "installer error: %v\n", err)
		return exitRunError
	}
	return exitOK
}

func runAdmin(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "admin requires a subcommand")
		return exitConfigError
	}
	switch args[0] {
	case "reset-password":
		return runAdminResetPassword(ctx, args[1:], cfg, logger)
	default:
		fmt.Fprintf(os.Stderr, "unknown admin subcommand: %s\n", args[0])
		return exitConfigError
	}
}

func runAdminResetPassword(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) int {
	flags := flag.NewFlagSet("admin reset-password", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	login := flags.String("login", "admin", "admin login")
	password := flags.String("password", "", "new password")
	if err := flags.Parse(args); err != nil {
		return exitConfigError
	}
	if strings.TrimSpace(*login) == "" {
		fmt.Fprintln(os.Stderr, "login is required")
		return exitConfigError
	}
	if len(*password) < 8 {
		fmt.Fprintln(os.Stderr, "password must be at least 8 characters")
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
	user, err := db.GetAdminUserByLogin(ctx, strings.TrimSpace(*login))
	if err != nil {
		logger.Error("admin user not found", "login", *login, "error", err)
		return exitRunError
	}
	hash, err := auth.HashPassword(*password)
	if err != nil {
		logger.Error("hash password", "error", err)
		return exitRunError
	}
	if err := db.UpdateAdminPassword(ctx, user.ID, hash); err != nil {
		logger.Error("update password", "login", *login, "error", err)
		return exitRunError
	}
	if err := db.DeleteSessionsByUser(ctx, user.ID); err != nil {
		logger.Error("delete sessions", "login", *login, "error", err)
		return exitRunError
	}
	logger.Info("admin password reset", "login", *login)
	return exitOK
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
	db, err := store.Open(cfg.DB)
	if err != nil {
		logger.Error("open database", "error", err)
		return exitConfigError
	}
	if err := db.Migrate(ctx); err != nil {
		logger.Error("migrate database", "error", err)
		return exitRunError
	}

	// Overlay credentials saved through the admin panel so CLI sync behaves
	// the same as `serve --with-sync` and manual syncs.
	operations := newMarketplaceOperations(ctx, db, cfg, logger, apihttp.NewExecutor(), syncer.NewCoordinator())

	var marketplaces []string
	if *marketplace != "" {
		marketplaces = []string{*marketplace}
	}

	results, err := operations.RunSync(ctx, marketplaces, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return exitConfigError
	}

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

	executor := apihttp.NewExecutor()
	coordinator := syncer.NewCoordinator()
	operations := newMarketplaceOperations(ctx, db, cfg, logger, executor, coordinator)
	effectiveCfg := operations.EffectiveConfig(ctx)

	var httpServer *server.Server
	afterSync := func() {
		if httpServer == nil {
			return
		}
		httpServer.RetryPendingReplies(context.Background())
		httpServer.RetryPendingQuestionAnswers(context.Background())
	}
	triggerSync := func(marketplaces []string) (server.SyncDispatch, error) {
		return operations.DispatchSync(marketplaces, afterSync)
	}

	httpServer = server.New(db, server.Config{
		Addr:                     *addr,
		StaticDir:                *staticDir,
		ProductURLTemplate:       *productURLTemplate,
		ProductLinks:             loadProductLinks(cfg.Web.ProductLinksPath, logger),
		ProductLinksPath:         cfg.Web.ProductLinksPath,
		SitemapURL:               cfg.Web.SitemapURL,
		SessionTTL:               24 * time.Hour,
		SecureCookies:            os.Getenv("REVIEWS_INSECURE_COOKIES") == "",
		TriggerSync:              triggerSync,
		Marketplaces:             marketplaceStatuses(effectiveCfg),
		AllowedOrigins:           cfg.Web.ShopOrigins,
		Media:                    cfg.Media,
		UploadDir:                cfg.Web.UploadDir,
		PrivacyURL:               cfg.Web.PrivacyURL,
		ReviewTermsURL:           cfg.Web.ReviewTermsURL,
		Version:                  version,
		LatestReleaseURL:         updateCheckURL(),
		ResolveReplyPublisher:    operations.ResolveReplyPublisher,
		ResolveQuestionPublisher: operations.ResolveQuestionPublisher,
		OzonProductsProbe: func(probeCtx context.Context) error {
			probeCtx, cancel := context.WithTimeout(probeCtx, 10*time.Second)
			defer cancel()
			return operations.CheckOzonProducts(probeCtx)
		},
	}, logger)

	if *withSync {
		sched := scheduler.New(
			schedulerRunnerAdapter{operations: operations, logger: logger, after: afterSync},
			effectiveCfg.Sync.Interval,
			logger,
		)
		go sched.Run(ctx)
	}

	// Continuous publish: the static export regenerates itself after data
	// changes, and the catalog re-crawls daily — no manual «Опубликовать» /
	// «Обновить каталог» needed in steady state.
	autoPublishEvery := envDuration("REVIEWS_AUTOPUBLISH_INTERVAL", 5*time.Minute, logger)
	httpServer.StartAutoPublish(ctx, autoPublishEvery)
	catalogRefreshEvery := envDuration("REVIEWS_CATALOG_REFRESH_INTERVAL", 24*time.Hour, logger)
	httpServer.StartCatalogAutoRefresh(ctx, catalogRefreshEvery)
	logger.Info("continuous publish enabled", "publish_interval", autoPublishEvery.String(), "catalog_interval", catalogRefreshEvery.String())

	if err := httpServer.Run(ctx); err != nil {
		logger.Error("server stopped with error", "error", err)
		return exitRunError
	}

	logger.Info("shutdown complete")
	return exitOK
}

// envDuration reads a duration env var; "0"/"off" disable the feature (zero).
func envDuration(key string, fallback time.Duration, logger *slog.Logger) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if value == "0" || strings.EqualFold(value, "off") || strings.EqualFold(value, "false") {
		return 0
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		logger.Warn("invalid duration, using default", "env", key, "value", value, "default", fallback.String())
		return fallback
	}
	return parsed
}

// schedulerRunnerAdapter adapts marketplaceOperations to scheduler.Runner.
// It ignores the marketplaces list the scheduler was constructed with (a
// startup snapshot) and re-resolves Runnable(ctx) on every tick instead, so
// admin-panel credential edits take effect on the next tick without a
// restart, and an invalid enabled marketplace never blocks the others. On
// completion it fires the same post-sync publication callback as a manual
// trigger.
type schedulerRunnerAdapter struct {
	operations *marketplaceOperations
	logger     *slog.Logger
	after      func()
}

func (a schedulerRunnerAdapter) RunOnce(ctx context.Context) {
	_ = ctx // operations holds the server-lifetime context from runServe.
	if _, err := a.operations.DispatchSync(nil, a.after); err != nil {
		a.logger.Error("scheduled sync dispatch failed", "error", err)
	}
}

func runDiscoverSiteURLs(ctx context.Context, args []string, cfg config.Config, logger *slog.Logger) int {
	flags := flag.NewFlagSet("discover-site-urls", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	sitemapURL := flags.String("sitemap", "", "sitemap URL of the shop (required), e.g. https://myshop.example/sitemap.xml")
	out := flags.String("out", cfg.Web.ProductLinksPath, "output JSON path")
	timeout := flags.Duration("timeout", 30*time.Minute, "scan timeout (a 4500-product shop takes about ten minutes)")
	if err := flags.Parse(args); err != nil {
		return exitConfigError
	}
	if *sitemapURL == "" {
		logger.Error("discover-site-urls requires --sitemap (the shop sitemap URL)")
		return exitConfigError
	}

	scanCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	links, scanErr := site.DiscoverKitProductLinks(scanCtx, *sitemapURL, nil)
	if scanErr != nil && len(links) == 0 {
		logger.Error("discover site URLs", "error", scanErr)
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

	if scanErr != nil {
		logger.Warn("scan incomplete: partial catalog written, re-run to continue", "count", len(links), "out", *out, "error", scanErr)
		return exitOK
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

	reviews, err := db.ListVisibleReviews(ctx)
	if err != nil {
		logger.Error("list reviews", "error", err)
		return exitRunError
	}

	mapper := reviewjson.Mapper{
		ProductURLTemplate: *productURLTemplate,
		ProductLinks:       loadProductLinks(cfg.Web.ProductLinksPath, logger),
		MarketplacePolicy:  activeExportMarketplacePolicy(ctx, db, logger),
	}
	pins, err := db.AllShowcasePins(ctx)
	if err != nil {
		logger.Error("list showcase pins", "error", err)
		return exitRunError
	}
	bundles := export.BuildBundles(reviews, mapper, pins)

	generatedAt := time.Now().UTC()
	if err := export.Write(*outDir, bundles, generatedAt); err != nil {
		logger.Error("write export", "out", *outDir, "error", err)
		return exitRunError
	}

	linkIndex := export.BuildLinkIndex(loadProductCatalog(cfg.Web.ProductLinksPath, logger), generatedAt)
	if err := export.WriteLinks(*outDir, linkIndex); err != nil {
		logger.Error("write links index", "out", *outDir, "error", err)
		return exitRunError
	}

	logger.Info("export complete", "articles", len(bundles), "reviews", len(reviews),
		"linkPaths", len(linkIndex.ByPath), "linkIDs", len(linkIndex.ByID), "out", *outDir)
	return exitOK
}

func activeExportMarketplacePolicy(ctx context.Context, db *store.Store, logger *slog.Logger) reviewjson.MarketplacePolicies {
	cfg, err := db.GetActiveWidgetConfig(ctx, "product")
	if err != nil {
		return nil
	}
	policy := reviewjson.ParseMarketplacePolicies(cfg.Payload)
	if len(policy) > 0 {
		logger.Info("export marketplace policy loaded", "context", "product")
	}
	return policy
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

func marketplaceStatuses(cfg config.Config) []server.MarketplaceStatus {
	wbConfigured := cfg.Marketplaces.WB.Token != ""
	var wbWarning string
	if wbConfigured {
		if err := wbtoken.ValidatePersonalTokenMetadata(cfg.Marketplaces.WB.Token, time.Now()); err != nil {
			wbConfigured = false
			wbWarning = err.Error()
		}
	}
	return []server.MarketplaceStatus{
		{
			ID:         config.MarketplaceWB,
			Enabled:    cfg.Marketplaces.WB.Enabled,
			Configured: wbConfigured,
			Fields: map[string]bool{
				"token": cfg.Marketplaces.WB.Token != "",
			},
			Warning: wbWarning,
		},
		{
			ID:      config.MarketplaceYM,
			Enabled: cfg.Marketplaces.YM.Enabled,
			Configured: cfg.Marketplaces.YM.BusinessID != "" &&
				(cfg.Marketplaces.YM.APIKey != "" || cfg.Marketplaces.YM.OAuthToken != ""),
			Fields: map[string]bool{
				"api_key":     cfg.Marketplaces.YM.APIKey != "",
				"oauth_token": cfg.Marketplaces.YM.OAuthToken != "",
				"business_id": cfg.Marketplaces.YM.BusinessID != "",
				"campaign_id": cfg.Marketplaces.YM.CampaignID != "",
			},
		},
		{
			ID:         config.MarketplaceOzon,
			Enabled:    cfg.Marketplaces.Ozon.Enabled,
			Configured: cfg.Marketplaces.Ozon.ClientID != "" && cfg.Marketplaces.Ozon.APIKey != "",
			Fields: map[string]bool{
				"client_id": cfg.Marketplaces.Ozon.ClientID != "",
				"api_key":   cfg.Marketplaces.Ozon.APIKey != "",
			},
		},
	}
}

func applyStoredMarketplaceCredentials(ctx context.Context, db *store.Store, cfg config.Config, logger *slog.Logger) config.Config {
	creds, err := db.ListMarketplaceCredentials(ctx)
	if err != nil {
		logger.Warn("load marketplace credentials", "error", err)
		return cfg
	}
	for _, cred := range creds {
		values := cred.PayloadMap()
		switch cred.Marketplace {
		case config.MarketplaceWB:
			cfg.Marketplaces.WB.Enabled = cred.Enabled
			if values["token"] != "" {
				cfg.Marketplaces.WB.Token = values["token"]
			}
		case config.MarketplaceYM:
			cfg.Marketplaces.YM.Enabled = cred.Enabled
			if values["api_key"] != "" {
				cfg.Marketplaces.YM.APIKey = values["api_key"]
			}
			if values["oauth_token"] != "" {
				cfg.Marketplaces.YM.OAuthToken = values["oauth_token"]
			}
			if values["business_id"] != "" {
				cfg.Marketplaces.YM.BusinessID = values["business_id"]
			}
			if values["campaign_id"] != "" {
				cfg.Marketplaces.YM.CampaignID = values["campaign_id"]
			}
		case config.MarketplaceOzon:
			cfg.Marketplaces.Ozon.Enabled = cred.Enabled
			if values["client_id"] != "" {
				cfg.Marketplaces.Ozon.ClientID = values["client_id"]
			}
			if values["api_key"] != "" {
				cfg.Marketplaces.Ozon.APIKey = values["api_key"]
			}
		}
	}
	return cfg
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

func loadProductCatalog(path string, logger *slog.Logger) []site.ProductLink {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		logger.Warn("open product catalog", "path", path, "error", err)
		return nil
	}
	defer file.Close()
	links, err := site.LoadProductLinks(file)
	if err != nil {
		logger.Warn("load product catalog", "path", path, "error", err)
		return nil
	}
	return links
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage:
  reviews migrate
  reviews admin reset-password --login admin --password NEW_PASSWORD
  reviews install
  reviews sync --once [--marketplace wb|ym|ozon]
  reviews serve [--addr 127.0.0.1:8080] [--with-sync]
  reviews discover-site-urls
  reviews export [--out web/reviews-data]

Environment:
  REVIEWS_DB_DRIVER=sqlite|postgres
  REVIEWS_DB_DSN=./reviews.db
`)
}
