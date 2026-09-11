package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"reviews/internal/config"
	"reviews/internal/marketplace"
	"reviews/internal/marketplace/apihttp"
	"reviews/internal/marketplace/wbtoken"
	"reviews/internal/store"
	"reviews/internal/syncer"
)

// --- test fixtures -----------------------------------------------------

func newOpsTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "reviews.db")
	db, err := store.Open(config.DBConfig{Driver: "sqlite", DSN: dbPath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newOps(t *testing.T, base config.Config) (*marketplaceOperations, *store.Store) {
	t.Helper()
	db := newOpsTestStore(t)
	return newMarketplaceOperations(context.Background(), db, base, testLogger(), apihttp.NewExecutor(), syncer.NewCoordinator()), db
}

// wbJWT builds an unsigned JWT-shaped fixture; validation is metadata-only
// (no signature verification), so tests construct these directly.
func wbJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	encode := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return encode(map[string]string{"alg": "none"}) + "." + encode(claims) + ".sig"
}

func personalWBToken(t *testing.T) string {
	return wbJWT(t, map[string]any{"acc": 3, "exp": time.Now().Add(time.Hour).Unix()})
}

func baseWBToken(t *testing.T) string {
	return wbJWT(t, map[string]any{"acc": 1, "exp": time.Now().Add(time.Hour).Unix()})
}

func saveWBCredential(t *testing.T, db *store.Store, token string, enabled bool) {
	t.Helper()
	if _, err := db.SaveMarketplaceCredential(context.Background(), store.MarketplaceCredentialPatch{
		Marketplace: config.MarketplaceWB,
		Enabled:     &enabled,
		Values:      map[string]string{"token": token},
	}); err != nil {
		t.Fatalf("save wb credential: %v", err)
	}
}

func saveYMCredential(t *testing.T, db *store.Store, enabled bool) {
	t.Helper()
	if _, err := db.SaveMarketplaceCredential(context.Background(), store.MarketplaceCredentialPatch{
		Marketplace: config.MarketplaceYM,
		Enabled:     &enabled,
		Values: map[string]string{
			"api_key":     "ym-key",
			"business_id": "ym-biz",
		},
	}); err != nil {
		t.Fatalf("save ym credential: %v", err)
	}
}

// fakeAdapter is a minimal marketplace.Adapter that returns no reviews and
// implements neither publisher capability interface.
type fakeAdapter struct{ id string }

func (a fakeAdapter) Marketplace() string { return a.id }
func (a fakeAdapter) FetchReviews(ctx context.Context, since time.Time, cursor string) ([]marketplace.Review, string, error) {
	return nil, "", nil
}

// errorAdapter always fails FetchReviews, exercising the coordinator's
// release-on-error path.
type errorAdapter struct {
	id  string
	err error
}

func (a errorAdapter) Marketplace() string { return a.id }
func (a errorAdapter) FetchReviews(ctx context.Context, since time.Time, cursor string) ([]marketplace.Review, string, error) {
	return nil, "", a.err
}

// blockingAdapter blocks inside FetchReviews until unblock is closed, so
// tests can observe DispatchSync returning before background work
// completes. entered is closed the instant FetchReviews is called.
type blockingAdapter struct {
	id      string
	entered chan struct{}
	unblock chan struct{}
}

func newBlockingAdapter(id string) *blockingAdapter {
	return &blockingAdapter{id: id, entered: make(chan struct{}), unblock: make(chan struct{})}
}

func (a *blockingAdapter) Marketplace() string { return a.id }
func (a *blockingAdapter) FetchReviews(ctx context.Context, since time.Time, cursor string) ([]marketplace.Review, string, error) {
	close(a.entered)
	select {
	case <-a.unblock:
		return nil, "", nil
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
}

// recordingFactory records the WB token each build call observed, proving
// callers see freshly-read credentials rather than a captured snapshot.
type recordingFactory struct {
	mu     sync.Mutex
	tokens []string
}

func (f *recordingFactory) build(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
	f.mu.Lock()
	f.tokens = append(f.tokens, cfg.Marketplaces.WB.Token)
	f.mu.Unlock()
	return fakeAdapter{id: id}, nil
}

// countingFactory records how many times it was invoked, proving rejected
// dispatches never launch a goroutine that constructs an adapter.
type countingFactory struct {
	mu    sync.Mutex
	calls int
}

func (f *countingFactory) build(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return fakeAdapter{id: id}, nil
}

func (f *countingFactory) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// --- Step 1: dynamic-config tests --------------------------------------

func TestMarketplaceOperationsRunnableReflectsCredentialsSavedAfterConstruction(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: false}}}
	o, db := newOps(t, base)

	if got := o.Runnable(context.Background()); len(got) != 0 {
		t.Fatalf("Runnable before save = %v, want empty", got)
	}

	saveWBCredential(t, db, personalWBToken(t), true)

	got := o.Runnable(context.Background())
	if len(got) != 1 || got[0] != config.MarketplaceWB {
		t.Fatalf("Runnable after save = %v, want [wb]", got)
	}

	if _, err := o.adapter(context.Background(), config.MarketplaceWB); err != nil {
		t.Fatalf("adapter(wb) after save = %v, want success", err)
	}
}

func TestMarketplaceOperationsAdapterUsesLatestStoredToken(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: false}}}
	o, db := newOps(t, base)
	factory := &recordingFactory{}
	o.newAdapter = factory.build

	token1 := personalWBToken(t)
	saveWBCredential(t, db, token1, true)
	if _, err := o.adapter(context.Background(), config.MarketplaceWB); err != nil {
		t.Fatalf("adapter(wb) with token1 = %v", err)
	}

	token2 := personalWBToken(t)
	saveWBCredential(t, db, token2, true)
	if _, err := o.adapter(context.Background(), config.MarketplaceWB); err != nil {
		t.Fatalf("adapter(wb) with token2 = %v", err)
	}

	if len(factory.tokens) != 2 {
		t.Fatalf("factory calls = %d, want 2", len(factory.tokens))
	}
	if factory.tokens[0] != token1 {
		t.Fatalf("first adapter token = %q, want token1", factory.tokens[0])
	}
	if factory.tokens[1] != token2 {
		t.Fatalf("second adapter token = %q, want token2, not token1", factory.tokens[1])
	}
}

func TestMarketplaceOperationsInvalidBaseWBTokenExcludedButYMStillWorks(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{
		WB: config.WBConfig{Enabled: true, Token: baseWBToken(t)}, // acc=1, rejected
	}}
	o, db := newOps(t, base)
	saveYMCredential(t, db, true)

	got := o.Runnable(context.Background())
	if len(got) != 1 || got[0] != config.MarketplaceYM {
		t.Fatalf("Runnable = %v, want [ym] (wb excluded by invalid base token)", got)
	}

	_, err := o.adapter(context.Background(), config.MarketplaceWB)
	if !errors.Is(err, wbtoken.ErrPersonalTokenRequired) {
		t.Fatalf("adapter(wb) error = %v, want ErrPersonalTokenRequired", err)
	}

	if _, err := o.adapter(context.Background(), config.MarketplaceYM); err != nil {
		t.Fatalf("adapter(ym) = %v, want success", err)
	}
}

func TestMarketplaceOperationsDisablingWBRejectsSubsequentResolution(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: false}}}
	o, db := newOps(t, base)
	saveWBCredential(t, db, personalWBToken(t), true)

	if _, err := o.adapter(context.Background(), config.MarketplaceWB); err != nil {
		t.Fatalf("adapter(wb) while enabled = %v, want success", err)
	}

	saveWBCredential(t, db, personalWBToken(t), false)

	if _, err := o.adapter(context.Background(), config.MarketplaceWB); err == nil {
		t.Fatal("adapter(wb) after disabling = nil error, want rejection")
	}
	if _, err := o.ResolveReplyPublisher(context.Background(), config.MarketplaceWB); err == nil {
		t.Fatal("ResolveReplyPublisher after disabling = nil error, want rejection")
	}
	if got := o.Runnable(context.Background()); len(got) != 0 {
		t.Fatalf("Runnable after disabling = %v, want empty", got)
	}
}

// --- Step 4: coordinated dispatch tests ---------------------------------

func TestMarketplaceOperationsDispatchSyncStartedReturnsBeforeBackgroundCompletes(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: true, Token: "any"}}}
	o, _ := newOps(t, base)
	wbAdapter := newBlockingAdapter("wb")
	o.newAdapter = func(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
		return wbAdapter, nil
	}
	o.base.Marketplaces.WB.Token = personalWBToken(t)

	dispatch, err := o.DispatchSync(context.Background(), []string{"wb"}, nil)
	if err != nil {
		t.Fatalf("DispatchSync: %v", err)
	}
	if len(dispatch.Started) != 1 || dispatch.Started[0] != "wb" || len(dispatch.Busy) != 0 {
		t.Fatalf("dispatch = %+v, want Started:[wb]", dispatch)
	}

	select {
	case <-wbAdapter.entered:
	case <-time.After(time.Second):
		t.Fatal("background FetchReviews was never called")
	}
	close(wbAdapter.unblock)
}

func TestMarketplaceOperationsDispatchSyncBusyOnSecondCallMakesNoCollectorCall(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: true, Token: personalWBToken(t)}}}
	o, _ := newOps(t, base)
	wbAdapter := newBlockingAdapter("wb")
	calls := &countingFactory{}
	o.newAdapter = func(cfg config.Config, id string, executor *apihttp.Executor) (marketplace.Adapter, error) {
		calls.build(cfg, id, executor)
		return wbAdapter, nil
	}

	first, err := o.DispatchSync(context.Background(), []string{"wb"}, nil)
	if err != nil || len(first.Started) != 1 {
		t.Fatalf("first dispatch = %+v, err=%v", first, err)
	}
	<-wbAdapter.entered // first goroutine is now mid-flight

	second, err := o.DispatchSync(context.Background(), []string{"wb"}, nil)
	if err != nil {
		t.Fatalf("second DispatchSync: %v", err)
	}
	if len(second.Started) != 0 || len(second.Busy) != 1 || second.Busy[0] != "wb" {
		t.Fatalf("second dispatch = %+v, want Busy:[wb]", second)
	}
	if got := calls.count(); got != 1 {
		t.Fatalf("adapter constructed %d times, want 1 (busy dispatch must not build an adapter)", got)
	}

	close(wbAdapter.unblock)
}

func TestMarketplaceOperationsDispatchSyncYMStartsWhileWBBlocked(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{
		WB: config.WBConfig{Enabled: true, Token: personalWBToken(t)},
		YM: config.YMConfig{Enabled: true, APIKey: "k", BusinessID: "b"},
	}}
	o, _ := newOps(t, base)
	wbAdapter := newBlockingAdapter("wb")
	ymAdapter := newBlockingAdapter("ym")
	o.newAdapter = func(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
		if id == "wb" {
			return wbAdapter, nil
		}
		return ymAdapter, nil
	}

	if _, err := o.DispatchSync(context.Background(), []string{"wb"}, nil); err != nil {
		t.Fatalf("dispatch wb: %v", err)
	}
	<-wbAdapter.entered

	dispatch, err := o.DispatchSync(context.Background(), []string{"ym"}, nil)
	if err != nil {
		t.Fatalf("dispatch ym: %v", err)
	}
	if len(dispatch.Started) != 1 || dispatch.Started[0] != "ym" {
		t.Fatalf("ym dispatch = %+v, want Started:[ym]", dispatch)
	}

	close(wbAdapter.unblock)
	close(ymAdapter.unblock)
}

func TestMarketplaceOperationsDispatchSyncReleasesAfterCollectorError(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: true, Token: personalWBToken(t)}}}
	o, _ := newOps(t, base)
	o.newAdapter = func(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
		return errorAdapter{id: id, err: errors.New("boom")}, nil
	}

	done := make(chan struct{})
	if _, err := o.DispatchSync(context.Background(), []string{"wb"}, func() { close(done) }); err != nil {
		t.Fatalf("DispatchSync: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("after callback never ran; release may be stuck")
	}

	// The coordinator slot must be free again even though the collector run
	// failed, proving release() ran via defer regardless of the error.
	second, err := o.DispatchSync(context.Background(), []string{"wb"}, nil)
	if err != nil {
		t.Fatalf("second DispatchSync: %v", err)
	}
	if len(second.Started) != 1 || second.Started[0] != "wb" {
		t.Fatalf("second dispatch = %+v, want Started:[wb] (slot must be released)", second)
	}
}

func TestMarketplaceOperationsDispatchSyncRejectsInvalidExplicitAndLaunchesNothing(t *testing.T) {
	cases := []struct {
		name string
		base config.Config
		id   string
	}{
		{"unknown", config.Config{}, "unknown-mp"},
		{"disabled", config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: false}}}, "wb"},
		{"invalid-credentials", config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: true, Token: "not-a-jwt"}}}, "wb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, _ := newOps(t, tc.base)
			calls := &countingFactory{}
			o.newAdapter = calls.build

			dispatch, err := o.DispatchSync(context.Background(), []string{tc.id}, nil)
			if err == nil {
				t.Fatalf("DispatchSync(%s) = nil error, want rejection", tc.id)
			}
			if len(dispatch.Started) != 0 || len(dispatch.Busy) != 0 {
				t.Fatalf("dispatch = %+v, want zero value on error", dispatch)
			}
			if got := calls.count(); got != 0 {
				t.Fatalf("adapter constructed %d times, want 0 (rejected request must launch no goroutine)", got)
			}
		})
	}
}

func TestMarketplaceOperationsDispatchSyncEmptyRequestSkipsInvalidEnabled(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{
		WB:   config.WBConfig{Enabled: true, Token: personalWBToken(t)},
		YM:   config.YMConfig{Enabled: true, APIKey: "k", BusinessID: "b"},
		Ozon: config.OzonConfig{Enabled: true}, // enabled but missing client id/api key: invalid
	}}
	o, _ := newOps(t, base)
	wbAdapter := newBlockingAdapter("wb")
	ymAdapter := newBlockingAdapter("ym")
	o.newAdapter = func(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
		switch id {
		case "wb":
			return wbAdapter, nil
		case "ym":
			return ymAdapter, nil
		default:
			return nil, fmt.Errorf("unexpected adapter build for %s", id)
		}
	}

	dispatch, err := o.DispatchSync(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("DispatchSync(nil): %v", err)
	}
	if len(dispatch.Started) != 2 {
		t.Fatalf("dispatch.Started = %v, want [wb ym] (ozon skipped, invalid enabled)", dispatch.Started)
	}
	seen := map[string]bool{}
	for _, id := range dispatch.Started {
		seen[id] = true
	}
	if !seen["wb"] || !seen["ym"] || seen["ozon"] {
		t.Fatalf("dispatch.Started = %v, want exactly {wb, ym}", dispatch.Started)
	}

	close(wbAdapter.unblock)
	close(ymAdapter.unblock)
}

func TestMarketplaceOperationsDispatchSyncAfterCallbackRunsOnceForWholeBatch(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{
		WB: config.WBConfig{Enabled: true, Token: personalWBToken(t)},
		YM: config.YMConfig{Enabled: true, APIKey: "k", BusinessID: "b"},
	}}
	o, _ := newOps(t, base)
	wbAdapter := newBlockingAdapter("wb")
	ymAdapter := newBlockingAdapter("ym")
	o.newAdapter = func(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
		if id == "wb" {
			return wbAdapter, nil
		}
		return ymAdapter, nil
	}

	var calls int32
	var mu sync.Mutex
	done := make(chan struct{})
	after := func() {
		mu.Lock()
		calls++
		mu.Unlock()
		close(done)
	}

	dispatch, err := o.DispatchSync(context.Background(), []string{"wb", "ym"}, after)
	if err != nil || len(dispatch.Started) != 2 {
		t.Fatalf("dispatch = %+v, err=%v", dispatch, err)
	}

	<-wbAdapter.entered
	<-ymAdapter.entered
	close(wbAdapter.unblock)
	close(ymAdapter.unblock)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("after callback never ran")
	}

	// Give any (incorrect) per-marketplace invocation a chance to land before
	// asserting the final count.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("after callback ran %d times, want exactly 1 for the whole dispatch", calls)
	}
}

// --- RunSync (synchronous CLI path) -------------------------------------

func TestMarketplaceOperationsRunSyncWaitsForCompletionAndCallsAfterOnce(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: true, Token: personalWBToken(t)}}}
	o, _ := newOps(t, base)
	o.newAdapter = func(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
		return fakeAdapter{id: id}, nil
	}

	var afterCalls int
	results, err := o.RunSync(context.Background(), []string{"wb"}, func() { afterCalls++ })
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	if len(results) != 1 || results[0].Marketplace != "wb" || results[0].Error != nil {
		t.Fatalf("results = %+v", results)
	}
	if afterCalls != 1 {
		t.Fatalf("after calls = %d, want 1", afterCalls)
	}
}

func TestMarketplaceOperationsRunSyncRejectsExplicitInvalidMarketplace(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: true, Token: "not-a-jwt"}}}
	o, _ := newOps(t, base)

	if _, err := o.RunSync(context.Background(), []string{"wb"}, nil); err == nil {
		t.Fatal("RunSync with invalid explicit WB = nil error, want rejection")
	}
}

func TestMarketplaceOperationsRunSyncSkipsInvalidEnabledWhenEmptyRequest(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{
		WB: config.WBConfig{Enabled: true, Token: "not-a-jwt"}, // enabled but invalid
		YM: config.YMConfig{Enabled: true, APIKey: "k", BusinessID: "b"},
	}}
	o, _ := newOps(t, base)
	o.newAdapter = func(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
		return fakeAdapter{id: id}, nil
	}

	results, err := o.RunSync(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("RunSync(nil): %v", err)
	}
	if len(results) != 1 || results[0].Marketplace != "ym" {
		t.Fatalf("results = %+v, want only ym (bad wb token must not block it)", results)
	}
}

// --- Publisher resolution and Ozon probe --------------------------------

func TestMarketplaceOperationsResolveReplyPublisherStableUnsupportedError(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{WB: config.WBConfig{Enabled: true, Token: personalWBToken(t)}}}
	o, _ := newOps(t, base)
	o.newAdapter = func(cfg config.Config, id string, _ *apihttp.Executor) (marketplace.Adapter, error) {
		return fakeAdapter{id: id}, nil // implements neither publisher interface
	}

	_, err := o.ResolveReplyPublisher(context.Background(), config.MarketplaceWB)
	if !errors.Is(err, errReplyPublishUnsupported) {
		t.Fatalf("ResolveReplyPublisher error = %v, want errReplyPublishUnsupported", err)
	}

	_, err = o.ResolveQuestionPublisher(context.Background(), config.MarketplaceWB)
	if !errors.Is(err, errQuestionPublishUnsupported) {
		t.Fatalf("ResolveQuestionPublisher error = %v, want errQuestionPublishUnsupported", err)
	}
}

func TestMarketplaceOperationsCheckOzonProductsPropagatesValidationError(t *testing.T) {
	base := config.Config{Marketplaces: config.MarketplaceConfig{Ozon: config.OzonConfig{Enabled: false}}}
	o, _ := newOps(t, base)

	if err := o.CheckOzonProducts(context.Background()); err == nil {
		t.Fatal("CheckOzonProducts with Ozon disabled = nil error, want rejection")
	}
}
