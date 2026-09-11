package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"reviews/internal/collector"
	"reviews/internal/config"
	"reviews/internal/marketplace"
	"reviews/internal/marketplace/apihttp"
	"reviews/internal/marketplace/ozon"
	"reviews/internal/marketplace/wb"
	"reviews/internal/marketplace/ym"
	"reviews/internal/server"
	"reviews/internal/store"
	"reviews/internal/syncer"
)

// errReplyPublishUnsupported and errQuestionPublishUnsupported are stable
// sentinel errors so callers (and their persisted "unsupported" state) don't
// depend on adapter-specific wording.
var (
	errReplyPublishUnsupported    = errors.New("marketplace does not support reply publishing")
	errQuestionPublishUnsupported = errors.New("marketplace does not support question answer publishing")
)

// adapterFactory constructs a marketplace.Adapter from effective config, one
// marketplace id, and the shared executor. Production uses newLiveAdapter;
// tests inject a recording/fake factory.
type adapterFactory func(cfg config.Config, marketplaceID string, executor *apihttp.Executor) (marketplace.Adapter, error)

// newLiveAdapter constructs the real client for marketplaceID.
func newLiveAdapter(cfg config.Config, marketplaceID string, executor *apihttp.Executor) (marketplace.Adapter, error) {
	switch marketplaceID {
	case config.MarketplaceWB:
		return wb.New(cfg.Marketplaces.WB, executor), nil
	case config.MarketplaceYM:
		return ym.New(cfg.Marketplaces.YM, executor), nil
	case config.MarketplaceOzon:
		return ozon.New(cfg.Marketplaces.Ozon, executor), nil
	default:
		return nil, fmt.Errorf("unknown marketplace: %s", marketplaceID)
	}
}

// ozonProductChecker is implemented by the Ozon adapter to verify the
// configured Api-Key can list the seller's products.
type ozonProductChecker interface {
	CheckProductsAccess(ctx context.Context) error
}

// marketplaceOperations resolves marketplace credentials from the database
// at call time rather than at process startup, so admin-panel credential
// edits take effect on the next sync or publish without a restart. It owns
// the single apihttp.Executor and syncer.Coordinator shared across every
// marketplace adapter constructed in the process.
//
// On SaaS one process serves many tenants: every operation runs with the
// tenant stamped into its ctx, and the coordinator slot key includes the
// tenant so tenant A's in-flight sync never blocks tenant B.
type MarketplaceOperations struct {
	// ctx is the server-lifetime context passed at construction. DispatchSync
	// uses it for background work launched after it has already returned to
	// its caller (an HTTP handler); RunSync always uses its own explicit
	// context instead.
	ctx         context.Context
	db          *store.Store
	base        config.Config
	logger      *slog.Logger
	executor    *apihttp.Executor
	coordinator *syncer.Coordinator
	newAdapter  adapterFactory
}

func NewMarketplaceOperations(ctx context.Context, db *store.Store, base config.Config, logger *slog.Logger, executor *apihttp.Executor, coordinator *syncer.Coordinator) *MarketplaceOperations {
	return &MarketplaceOperations{
		ctx:         ctx,
		db:          db,
		base:        base,
		logger:      logger,
		executor:    executor,
		coordinator: coordinator,
		newAdapter:  newLiveAdapter,
	}
}

// tenantCtx stamps the tenant from parent into a fresh child of the
// server-lifetime context, so background work (DispatchSync goroutines)
// carries the tenant even though it detaches from the caller's request.
func (o *MarketplaceOperations) tenantCtx(parent context.Context) context.Context {
	return store.WithTenant(o.ctx, store.TenantIDFromCtx(parent))
}

// slotKey namespaces a coordinator slot by tenant.
func slotKey(tenantID uint, marketplace string) string {
	return fmt.Sprintf("%d:%s", tenantID, marketplace)
}

// EffectiveConfig overlays admin-saved marketplace credentials onto the base
// config. It re-reads the database on every call.
func (o *MarketplaceOperations) EffectiveConfig(ctx context.Context) config.Config {
	return applyStoredMarketplaceCredentials(ctx, o.db, o.base, o.logger)
}

// Runnable returns the enabled marketplaces whose current credentials pass
// validation, evaluated fresh against the database.
func (o *MarketplaceOperations) Runnable(ctx context.Context) []string {
	effective := o.EffectiveConfig(ctx)
	var ids []string
	for _, id := range effective.EnabledMarketplaces() {
		if err := effective.ValidateMarketplaceCredentials(id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// resolvedAdapter validates marketplaceID against the current effective
// config and constructs a fresh adapter, returning the effective config
// alongside so callers needing more than the adapter (e.g. Sync settings)
// don't re-query the database.
func (o *MarketplaceOperations) resolvedAdapter(ctx context.Context, marketplaceID string) (marketplace.Adapter, config.Config, error) {
	effective := o.EffectiveConfig(ctx)
	if err := effective.ValidateMarketplaceCredentials(marketplaceID); err != nil {
		return nil, config.Config{}, err
	}
	a, err := o.newAdapter(effective, marketplaceID, o.executor)
	if err != nil {
		return nil, config.Config{}, err
	}
	return a, effective, nil
}

// adapter validates one enabled marketplace and constructs a fresh adapter
// for it using the shared executor.
func (o *MarketplaceOperations) adapter(ctx context.Context, marketplaceID string) (marketplace.Adapter, error) {
	a, _, err := o.resolvedAdapter(ctx, marketplaceID)
	return a, err
}

// runOne resolves a fresh adapter for marketplaceID and runs one sync
// against it via a one-adapter collector.Runner.
func (o *MarketplaceOperations) runOne(ctx context.Context, marketplaceID string) collector.Result {
	a, effective, err := o.resolvedAdapter(ctx, marketplaceID)
	if err != nil {
		return collector.Result{Marketplace: marketplaceID, Error: err}
	}
	runner := collector.NewRunner(o.db, effective.Sync, o.logger, []marketplace.Adapter{a})
	results := runner.RunOnce(ctx, []string{marketplaceID})
	return results[0]
}

// resolveIDs implements the shared validation/resolution rules for
// DispatchSync and RunSync: an empty requested list resolves to the
// currently runnable marketplaces (invalid enabled ones silently skipped);
// an explicit list is validated in full before any acquisition or work
// starts, so a single bad id rejects the whole request.
func (o *MarketplaceOperations) resolveIDs(ctx context.Context, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return o.Runnable(ctx), nil
	}
	effective := o.EffectiveConfig(ctx)
	for _, id := range requested {
		if !config.IsKnownMarketplace(id) {
			return nil, fmt.Errorf("unknown marketplace: %s", id)
		}
		if err := effective.ValidateMarketplaceCredentials(id); err != nil {
			return nil, err
		}
	}
	return requested, nil
}

// DispatchSync resolves the tenant from ctx (an admin request or the SaaS
// sync loop), then resolves requested marketplaces (or the tenant's
// Runnable list when requested is empty), acquires the per-tenant
// coordinator slot for each, and returns immediately with which ids
// started versus which were already busy. Work for started marketplaces
// runs in background goroutines carrying the same tenant; after (if
// non-nil) runs exactly once, after every marketplace started by this
// dispatch finishes.
func (o *MarketplaceOperations) DispatchSync(ctx context.Context, requested []string, after func()) (server.SyncDispatch, error) {
	tenantID := store.TenantIDFromCtx(ctx)
	ctx = store.WithTenant(o.ctx, tenantID)
	ids, err := o.resolveIDs(ctx, requested)
	if err != nil {
		return server.SyncDispatch{}, err
	}

	var dispatch server.SyncDispatch
	var wg sync.WaitGroup
	for _, id := range ids {
		release, ok := o.coordinator.TryAcquire(slotKey(tenantID, id))
		if !ok {
			dispatch.Busy = append(dispatch.Busy, id)
			continue
		}
		dispatch.Started = append(dispatch.Started, id)
		wg.Add(1)
		go func(id string, release func()) {
			defer wg.Done()
			defer release()
			result := o.runOne(ctx, id)
			if result.Error != nil {
				o.logger.Error("dispatched sync marketplace failed", "tenant", tenantID, "marketplace", id, "error", result.Error)
				return
			}
			o.logger.Info("dispatched sync marketplace ok", "tenant", tenantID, "marketplace", id, "seen", result.Seen, "upserted", result.Upserted)
		}(id, release)
	}

	if after != nil {
		go func() {
			wg.Wait()
			after()
		}()
	}

	return dispatch, nil
}

// RunSync is the synchronous CLI sync path: it applies the same
// validation, fresh-adapter, and coordinator rules as DispatchSync, but
// blocks until every requested marketplace finishes before calling after
// (if non-nil) once and returning.
func (o *MarketplaceOperations) RunSync(ctx context.Context, requested []string, after func()) ([]collector.Result, error) {
	ids, err := o.resolveIDs(ctx, requested)
	if err != nil {
		return nil, err
	}

	results := make([]collector.Result, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		release, ok := o.coordinator.TryAcquire(slotKey(store.TenantIDFromCtx(ctx), id))
		if !ok {
			results[i] = collector.Result{Marketplace: id, Error: fmt.Errorf("marketplace %s sync already in progress", id)}
			continue
		}
		wg.Add(1)
		go func(i int, id string, release func()) {
			defer wg.Done()
			defer release()
			results[i] = o.runOne(ctx, id)
		}(i, id, release)
	}
	wg.Wait()

	if after != nil {
		after()
	}
	return results, nil
}

// ResolveReplyPublisher resolves a fresh adapter for marketplaceID and
// asserts it supports publishing seller replies.
func (o *MarketplaceOperations) ResolveReplyPublisher(ctx context.Context, marketplaceID string) (marketplace.ReplyPublisher, error) {
	a, err := o.adapter(ctx, marketplaceID)
	if err != nil {
		return nil, err
	}
	pub, ok := a.(marketplace.ReplyPublisher)
	if !ok {
		return nil, errReplyPublishUnsupported
	}
	return pub, nil
}

// ResolveQuestionPublisher resolves a fresh adapter for marketplaceID and
// asserts it supports publishing seller answers to product questions.
func (o *MarketplaceOperations) ResolveQuestionPublisher(ctx context.Context, marketplaceID string) (marketplace.QuestionAnswerPublisher, error) {
	a, err := o.adapter(ctx, marketplaceID)
	if err != nil {
		return nil, err
	}
	pub, ok := a.(marketplace.QuestionAnswerPublisher)
	if !ok {
		return nil, errQuestionPublishUnsupported
	}
	return pub, nil
}

// CheckOzonProducts resolves a fresh Ozon adapter and verifies the
// configured Api-Key can list the seller's products.
func (o *MarketplaceOperations) CheckOzonProducts(ctx context.Context) error {
	a, err := o.adapter(ctx, config.MarketplaceOzon)
	if err != nil {
		return err
	}
	checker, ok := a.(ozonProductChecker)
	if !ok {
		return fmt.Errorf("ozon adapter does not support product access checks")
	}
	return checker.CheckProductsAccess(ctx)
}
