package store

import (
	"context"
	"os"
	"strconv"
)

// tenantCtxKey is the context key carrying the tenant ID. Unexported struct
// type so no other package can collide with it.
type tenantCtxKey struct{}

// DefaultTenantID is the implicit tenant used in single-tenant (open-source)
// mode. All pre-multitenancy rows and every compat fallback resolve to it.
const DefaultTenantID uint = 1

// strictTenantMode treats a missing tenant as a programming error instead of
// falling back to DefaultTenantID. Enabled by REVIEWS_COMPAT_SINGLE_TENANT=false
// on SaaS instances; open-source defaults to compat.
var strictTenantMode = func() bool {
	value := os.Getenv("REVIEWS_COMPAT_SINGLE_TENANT")
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && !parsed
}()

// WithTenant returns a copy of ctx carrying the given tenant ID. Every store
// call resolves its tenant through TenantIDFromCtx, so all entry points
// (admin session middleware, public routes, jobs, CLI) must pass a
// tenant-carrying context.
func WithTenant(ctx context.Context, tenantID uint) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenantID)
}

// TenantIDFromCtx returns the tenant ID from ctx. In strict mode a missing
// tenant panics (programming error on a SaaS instance); in compat mode it
// falls back to DefaultTenantID so existing open-source deployments keep
// working unchanged.
func TenantIDFromCtx(ctx context.Context) uint {
	id, ok := ctx.Value(tenantCtxKey{}).(uint)
	if ok && id != 0 {
		return id
	}
	if strictTenantMode {
		panic("store: tenant id missing from context")
	}
	return DefaultTenantID
}

// TenantIDFromCtxSafe reports the tenant without the strict-mode panic:
// middleware that may run on tenantless requests (health, statics) uses it
// instead of TenantIDFromCtx.
func TenantIDFromCtxSafe(ctx context.Context) (uint, bool) {
	id, ok := ctx.Value(tenantCtxKey{}).(uint)
	if !ok || id == 0 {
		return 0, false
	}
	return id, true
}

// StrictTenantMode reports whether the instance runs without the single-tenant
// fallback (SaaS). Public middleware uses it to require public_key.
func StrictTenantMode() bool {
	return strictTenantMode
}

// SetStrictTenantModeForTest overrides strict mode for a test and returns a
// restore function. Production code must not call it.
func SetStrictTenantModeForTest(strict bool) func() {
	prev := strictTenantMode
	strictTenantMode = strict
	return func() { strictTenantMode = prev }
}
