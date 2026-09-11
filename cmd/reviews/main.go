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

	"reviews/internal/app"
	"reviews/internal/auth"
	"reviews/internal/config"
	"reviews/internal/export"
	"reviews/internal/installer"
	"reviews/internal/marketplace/apihttp"
	"reviews/internal/reviewjson"
	"reviews/internal/secrets"
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

	logger := app.NewLogger(cfg.Log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// One process serves one tenant in variant A (container per tenant) and
	// all tenants in variant B (shared SaaS process): serve backgrounds run
	// per tenant, and CLI subcommands below default to the default tenant.
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

	db, err := openStore(ctx, cfg, logger)
	if err != nil {
		logger.Error("open database", "error", err)
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
	db, err := openStore(ctx, cfg, logger)
	if err != nil {
		logger.Error("open database", "error", err)
		return exitRunError
	}

	// Overlay credentials saved through the admin panel so CLI sync behaves
	// the same as `serve --with-sync` and manual syncs.
	operations := app.NewMarketplaceOperations(ctx, db, cfg, logger, apihttp.NewExecutor(), syncer.NewCoordinator())

	var marketplaces []string
	if *marketplace != "" {
		marketplaces = []string{*marketplace}
	}

	// SaaS: sync every tenant in sequence. Single-tenant installs have one
	// row (the implicit default tenant), so the loop degenerates to the
	// previous behavior.
	tenants, err := db.ListTenants(ctx)
	if err != nil {
		logger.Error("list tenants", "error", err)
		return exitRunError
	}
	var failed bool
	for _, t := range tenants {
		tCtx := store.WithTenant(ctx, t.ID)
		results, err := operations.RunSync(tCtx, marketplaces, nil)
		if err != nil {
			logger.Error("sync tenant failed", "tenant", t.ID, "error", err)
			failed = true
			continue
		}
		for _, result := range results {
			if result.Error != nil {
				failed = true
				logger.Error("sync marketplace failed", "tenant", t.ID, "marketplace", result.Marketplace, "error", result.Error)
				continue
			}
			logger.Info("sync marketplace ok", "tenant", t.ID, "marketplace", result.Marketplace, "seen", result.Seen, "upserted", result.Upserted)
		}
	}
	if failed {
		return exitRunError
	}
	return exitOK
}

// openStore opens the database, installs the credentials cipher (SaaS:
// REVIEWS_CREDENTIALS_KEY seals marketplace tokens at rest; empty keeps
// plaintext for single-tenant installs), runs migrations, and upgrades any
// legacy plaintext credential rows.
func openStore(ctx context.Context, cfg config.Config, logger *slog.Logger) (*store.Store, error) {
	db, err := store.Open(cfg.DB)
	if err != nil {
		return nil, err
	}
	cipher, err := secrets.New(os.Getenv("REVIEWS_CREDENTIALS_KEY"))
	if err != nil {
		return nil, err
	}
	db.SetCredentialsCipher(cipher)
	if err := db.Migrate(ctx); err != nil {
		return nil, err
	}
	if err := db.MigrateCredentials(ctx); err != nil {
		return nil, fmt.Errorf("migrate credentials encryption: %w", err)
	}
	if cipher != nil {
		logger.Info("marketplace credentials sealed at rest (AES-256-GCM)")
	}
	return db, nil
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

	db, err := openStore(ctx, cfg, logger)
	if err != nil {
		logger.Error("open database", "error", err)
		return exitRunError
	}

	executor := apihttp.NewExecutor()
	coordinator := syncer.NewCoordinator()
	operations := app.NewMarketplaceOperations(ctx, db, cfg, logger, executor, coordinator)
	var httpServer *server.Server
	effectiveCfg := operations.EffectiveConfig(ctx)
	// Tenants are listed fresh on every scheduled sync tick: a tenant
	// registered after startup is picked up without a restart.
	listTenants := func() ([]store.Tenant, error) {
		tenants, err := db.ListTenants(ctx)
		if err != nil {
			logger.Error("list tenants", "error", err)
		}
		return tenants, err
	}
	// afterTenant runs the post-sync publication retries for one tenant,
	// detached from the request that triggered the sync.
	afterTenant := func(tenantID uint) {
		if httpServer == nil {
			return
		}
		tCtx := store.WithTenant(context.Background(), tenantID)
		httpServer.RetryPendingReplies(tCtx)
		httpServer.RetryPendingQuestionAnswers(tCtx)
	}
	triggerSync := func(reqCtx context.Context, marketplaces []string) (server.SyncDispatch, error) {
		// Admin-triggered sync: the request ctx carries the admin's tenant.
		tenantID := store.TenantIDFromCtx(reqCtx)
		return operations.DispatchSync(reqCtx, marketplaces, func() { afterTenant(tenantID) })
	}

	httpServer = server.New(db, server.Config{
		Addr:                     *addr,
		StaticDir:                *staticDir,
		ProductURLTemplate:       *productURLTemplate,
		ProductLinks:             app.LoadProductLinks(cfg.Web.ProductLinksPath, logger),
		ProductLinksPath:         cfg.Web.ProductLinksPath,
		SitemapURL:               cfg.Web.SitemapURL,
		SessionTTL:               24 * time.Hour,
		SecureCookies:            os.Getenv("REVIEWS_INSECURE_COOKIES") == "",
		TriggerSync:              triggerSync,
		Marketplaces:             app.MarketplaceStatuses(effectiveCfg),
		AllowedOrigins:           cfg.Web.ShopOrigins,
		Media:                    cfg.Media,
		UploadDir:                cfg.Web.UploadDir,
		PrivacyURL:               cfg.Web.PrivacyURL,
		ReviewTermsURL:           cfg.Web.ReviewTermsURL,
		Version:                  version,
		LatestReleaseURL:         app.UpdateCheckURL(),
		ResolveReplyPublisher:    operations.ResolveReplyPublisher,
		ResolveQuestionPublisher: operations.ResolveQuestionPublisher,
		OzonProductsProbe: func(probeCtx context.Context) error {
			probeCtx, cancel := context.WithTimeout(probeCtx, 10*time.Second)
			defer cancel()
			return operations.CheckOzonProducts(probeCtx)
		},
	}, logger)

	// SaaS: scope the static reviews-data export per tenant (by public key)
	// so many tenants share one instance without colliding. In compat
	// (single-tenant) mode TenantScope returns "" and the shared legacy
	// directory is used, keeping existing installs byte-identical.
	httpServer.SetTenantExportScope(func(ctx context.Context) (string, error) {
		if !store.StrictTenantMode() {
			return "", nil
		}
		tenant, err := db.TenantByID(ctx)
		if err != nil {
			return "", err
		}
		return tenant.PublicKey, nil
	})

	if *withSync {
		// SaaS sync loop: every tick walks all tenants and dispatches their
		// runnable marketplaces. Tenant N+1's slow sync never blocks tenant
		// N's dispatch (per-tenant coordinator slots).
		interval := effectiveCfg.Sync.Interval
		go func() {
			runOneTick := func() {
				tenants, err := listTenants()
				if err != nil {
					return
				}
				for _, t := range tenants {
					tCtx := store.WithTenant(ctx, t.ID)
					if _, err := operations.DispatchSync(tCtx, nil, func() { afterTenant(t.ID) }); err != nil {
						logger.Error("scheduled sync dispatch failed", "tenant", t.ID, "error", err)
					}
				}
			}
			runOneTick()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					runOneTick()
				}
			}
		}()
	}

	// Continuous publish: the static export regenerates itself after data
	// changes, and the catalog re-crawls daily — no manual «Опубликовать» /
	// «Обновить каталог» needed in steady state.
	autoPublishEvery := app.EnvDuration("REVIEWS_AUTOPUBLISH_INTERVAL", 5*time.Minute, logger)
	httpServer.StartAutoPublish(ctx, autoPublishEvery)
	catalogRefreshEvery := app.EnvDuration("REVIEWS_CATALOG_REFRESH_INTERVAL", 24*time.Hour, logger)
	httpServer.StartCatalogAutoRefresh(ctx, catalogRefreshEvery)
	// SaaS: pause trials that ended; hourly tick, idempotent store method.
	httpServer.StartTrialExpiry(ctx)
	logger.Info("continuous publish enabled", "publish_interval", autoPublishEvery.String(), "catalog_interval", catalogRefreshEvery.String())

	if err := httpServer.Run(ctx); err != nil {
		logger.Error("server stopped with error", "error", err)
		return exitRunError
	}

	logger.Info("shutdown complete")
	return exitOK
}

// envDuration reads a duration env var; "0"/"off" disable the feature (zero).

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
		ProductLinks:       app.LoadProductLinks(cfg.Web.ProductLinksPath, logger),
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


func emptyAsAll(value string) string {
	if value == "" {
		return "all"
	}
	return value
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
