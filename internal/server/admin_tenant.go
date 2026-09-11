package server

import (
	"errors"
	"net/http"

	"reviews/internal/store"
)

// handleTenant returns the tenant identity (public key, slug, plan) for the
// authenticated admin's tenant. The Embed page appends publicKey to widget
// API calls; public_key is a lookup credential by design, not a secret.
func (s *Server) handleTenant(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.store.TenantByID(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, errors.New("tenant not found"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          tenant.ID,
		"slug":        tenant.Slug,
		"publicKey":   tenant.PublicKey,
		"plan":        tenant.Plan,
		"status":      tenant.Status,
		"paidUntil":   tenant.PaidUntil,
		"trialEndsAt": tenant.TrialEndsAt,
	})
}
