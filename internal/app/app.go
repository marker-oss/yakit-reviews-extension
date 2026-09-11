package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"reviews/internal/config"
	"reviews/internal/marketplace/apihttp"
	"reviews/internal/secrets"
	"reviews/internal/server"
	"reviews/internal/store"
	"reviews/internal/syncer"
)

// Options customizes the runtime for closed-source overlays. The zero value
// reproduces the plain open-source binary.
type Options struct {
	// Version stamped into the binary (release tag or "dev").
	Version string
	// ExtraMigrate adds overlay tables (payments, operator data) inside the
	// same migration run.
	ExtraMigrate func(ctx context.Context, db *store.Store) error
	// ExtraAdminRoutes registers protected operator routes: mounted under
	// /admin/api/saas/ inside requireSession.
	ExtraAdminRoutes func(s *server.Server) *http.ServeMux
	// ExtraPublicRoutes registers public routes (payment webhooks): mounted
	// under /billing/.
	ExtraPublicRoutes func(s *server.Server) *http.ServeMux
	// OnServerStart runs right after the server is built, before Run: the
	// overlay starts its own background loops here (subscription expiry).
	OnServerStart func(ctx context.Context, db *store.Store, s *server.Server)
}

// OpenStore opens the database, installs the credentials cipher (SaaS:
// REVIEWS_CREDENTIALS_KEY seals marketplace tokens at rest; empty keeps
// plaintext for single-tenant installs), runs core migrations, then the
// overlay migration hook, then the plaintext-credentials upgrade.
func OpenStore(ctx context.Context, cfg config.Config, logger *slog.Logger, opts Options) (*store.Store, error) {
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
	if opts.ExtraMigrate != nil {
		if err := opts.ExtraMigrate(ctx, db); err != nil {
			return nil, fmt.Errorf("overlay migration: %w", err)
		}
	}
	if err := db.MigrateCredentials(ctx); err != nil {
		return nil, fmt.Errorf("migrate credentials encryption: %w", err)
	}
	if cipher != nil {
		logger.Info("marketplace credentials sealed at rest (AES-256-GCM)")
	}
	return db, nil
}

// Serve builds and runs the HTTP server plus the background loops, blocking
// until ctx is cancelled or the listener fails.
func Serve(ctx context.Context, cfg config.Config, logger *slog.Logger, opts Options, addr, staticDir, productURLTemplate string, withSync bool) (*store.Store, *server.Server, error) {
	if err := server.StaticDirExists(staticDir); err != nil {
		return nil, nil, err
	}

	db, err := OpenStore(ctx, cfg, logger, opts)
	if err != nil {
		return nil, nil, err
	}

	executor := apihttp.NewExecutor()
	coordinator := syncer.NewCoordinator()
	operations := NewMarketplaceOperations(ctx, db, cfg, logger, executor, coordinator)
	var httpServer *server.Server
	effectiveCfg := operations.EffectiveConfig(ctx)

	listTenants := func() ([]store.Tenant, error) {
		tenants, err := db.ListTenants(ctx)
		if err != nil {
			logger.Error("list tenants", "error", err)
		}
		return tenants, err
	}
	afterTenant := func(tenantID uint) {
		if httpServer == nil {
			return
		}
		tCtx := store.WithTenant(context.Background(), tenantID)
		httpServer.RetryPendingReplies(tCtx)
		httpServer.RetryPendingQuestionAnswers(tCtx)
	}
	triggerSync := func(reqCtx context.Context, marketplaces []string) (server.SyncDispatch, error) {
		tenantID := store.TenantIDFromCtx(reqCtx)
		return operations.DispatchSync(reqCtx, marketplaces, func() { afterTenant(tenantID) })
	}

	httpServer = server.New(db, server.Config{
		Addr:                     addr,
		StaticDir:                staticDir,
		ProductURLTemplate:       productURLTemplate,
		ProductLinks:             LoadProductLinks(cfg.Web.ProductLinksPath, logger),
		ProductLinksPath:         cfg.Web.ProductLinksPath,
		SitemapURL:               cfg.Web.SitemapURL,
		SessionTTL:               24 * time.Hour,
		SecureCookies:            os.Getenv("REVIEWS_INSECURE_COOKIES") == "",
		TriggerSync:              triggerSync,
		Marketplaces:             MarketplaceStatuses(effectiveCfg),
		AllowedOrigins:           cfg.Web.ShopOrigins,
		Media:                    cfg.Media,
		UploadDir:                cfg.Web.UploadDir,
		PrivacyURL:               cfg.Web.PrivacyURL,
		ReviewTermsURL:           cfg.Web.ReviewTermsURL,
		Version:                  opts.Version,
		LatestReleaseURL:         UpdateCheckURL(),
		ResolveReplyPublisher:    operations.ResolveReplyPublisher,
		ResolveQuestionPublisher: operations.ResolveQuestionPublisher,
		ExtraAdminRoutes:         opts.ExtraAdminRoutes,
		ExtraPublicRoutes:        opts.ExtraPublicRoutes,
		OzonProductsProbe: func(probeCtx context.Context) error {
			probeCtx, cancel := context.WithTimeout(probeCtx, 10*time.Second)
			defer cancel()
			return operations.CheckOzonProducts(probeCtx)
		},
	}, logger)

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

	if withSync {
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

	autoPublishEvery := EnvDuration("REVIEWS_AUTOPUBLISH_INTERVAL", 5*time.Minute, logger)
	httpServer.StartAutoPublish(ctx, autoPublishEvery)
	catalogRefreshEvery := EnvDuration("REVIEWS_CATALOG_REFRESH_INTERVAL", 24*time.Hour, logger)
	httpServer.StartCatalogAutoRefresh(ctx, catalogRefreshEvery)
	httpServer.StartTrialExpiry(ctx)
	logger.Info("continuous publish enabled", "publish_interval", autoPublishEvery.String(), "catalog_interval", catalogRefreshEvery.String())

	if opts.OnServerStart != nil {
		opts.OnServerStart(ctx, db, httpServer)
	}

	if err := httpServer.Run(ctx); err != nil {
		return db, httpServer, err
	}
	logger.Info("shutdown complete")
	return db, httpServer, nil
}
