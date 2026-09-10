package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"reviews/internal/marketplace/wbtoken"
	"reviews/internal/store"
)

var errSyncDisabled = errors.New("sync is disabled")
var errSyncBusy = errors.New("Синхронизация уже выполняется")

type MarketplaceStatus struct {
	ID         string          `json:"id"`
	Enabled    bool            `json:"enabled"`
	Configured bool            `json:"configured"`
	Fields     map[string]bool `json:"fields,omitempty"`
	// Warning surfaces marketplace-specific misconfiguration the seller must
	// fix themselves (e.g. an Ozon Api-Key without the products role).
	Warning string `json:"warning,omitempty"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.DashboardStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	runs, err := s.store.RecentSyncRuns(r.Context(), 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_reviews":   stats.TotalReviews,
		"visible_reviews": stats.VisibleReviews,
		"pending_reviews": stats.PendingReviews,
		"deleted_reviews": stats.DeletedReviews,
		"average_rating":  stats.AverageRating,
		"by_marketplace":  stats.ByMarketplace,
		"recent_syncs":    runs,
	})
}

func (s *Server) handleMarketplaces(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"marketplaces": s.marketplaceStatuses(r)})
}

func (s *Server) handleTriggerSync(w http.ResponseWriter, r *http.Request) {
	if s.cfg.TriggerSync == nil {
		writeError(w, http.StatusServiceUnavailable, errSyncDisabled)
		return
	}
	mp := r.URL.Query().Get("marketplace")
	var marketplaces []string
	if mp != "" {
		marketplaces = []string{mp}
	}
	result, err := s.cfg.TriggerSync(marketplaces)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(result.Started) == 0 && len(result.Busy) > 0 {
		writeError(w, http.StatusConflict, errSyncBusy)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

type marketplaceCredentialsRequest struct {
	Enabled *bool             `json:"enabled"`
	Values  map[string]string `json:"values"`
}

func (s *Server) handleSaveMarketplaceCredentials(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isKnownMarketplace(id) {
		writeError(w, http.StatusBadRequest, errors.New("unknown marketplace"))
		return
	}
	var req marketplaceCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	values := allowedCredentialValues(id, req.Values)
	if id == "wb" {
		if err := validateWBCredentialUpdate(r.Context(), s.store, values, req.Enabled); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	cred, err := s.store.SaveMarketplaceCredential(r.Context(), store.MarketplaceCredentialPatch{
		Marketplace: id,
		Enabled:     req.Enabled,
		Values:      values,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if id == "ozon" {
		// New credentials must re-probe immediately, not after the cache TTL.
		s.invalidateOzonProbe()
	}
	writeJSON(w, http.StatusOK, map[string]any{"marketplace": statusFromCredential(id, cred)})
}

// validateWBCredentialUpdate rejects a WB credential save whose effective
// token is not a valid personal API token, before the store is touched so a
// failed save leaves the previously stored credential unchanged. A newly
// submitted token is validated directly; an empty token field that enables
// WB falls back to validating the retained stored token.
func validateWBCredentialUpdate(ctx context.Context, st *store.Store, values map[string]string, enabled *bool) error {
	if token := values["token"]; token != "" {
		return wbtoken.ValidatePersonalTokenMetadata(token, time.Now())
	}
	if enabled == nil || !*enabled {
		return nil
	}
	existing, err := st.GetMarketplaceCredential(ctx, "wb")
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	token := existing.PayloadMap()["token"]
	if token == "" {
		return nil
	}
	return wbtoken.ValidatePersonalTokenMetadata(token, time.Now())
}

func (s *Server) marketplaceStatuses(r *http.Request) []MarketplaceStatus {
	byID := make(map[string]MarketplaceStatus, len(s.cfg.Marketplaces))
	order := make([]string, 0, len(s.cfg.Marketplaces))
	for _, item := range s.cfg.Marketplaces {
		byID[item.ID] = item
		order = append(order, item.ID)
	}

	creds, err := s.store.ListMarketplaceCredentials(r.Context())
	if err != nil {
		return s.cfg.Marketplaces
	}
	for _, cred := range creds {
		if _, ok := byID[cred.Marketplace]; !ok {
			order = append(order, cred.Marketplace)
		}
		byID[cred.Marketplace] = statusFromCredential(cred.Marketplace, cred)
	}

	items := make([]MarketplaceStatus, 0, len(order))
	for _, id := range order {
		item := byID[id]
		if item.ID == "ozon" && item.Enabled && item.Configured {
			item.Warning = s.ozonProductsWarning(r.Context())
		}
		items = append(items, item)
	}
	return items
}

func statusFromCredential(id string, cred store.MarketplaceCredential) MarketplaceStatus {
	payload := cred.PayloadMap()
	fields := credentialFields(id, payload)
	configured, warning := credentialConfigured(id, fields, payload)
	return MarketplaceStatus{
		ID:         id,
		Enabled:    cred.Enabled,
		Configured: configured,
		Fields:     fields,
		Warning:    warning,
	}
}

func allowedCredentialValues(id string, values map[string]string) map[string]string {
	allowed := credentialFieldNames(id)
	filtered := make(map[string]string)
	for key, value := range values {
		if allowed[key] {
			filtered[key] = value
		}
	}
	return filtered
}

func credentialFieldNames(id string) map[string]bool {
	fields := map[string]bool{}
	switch id {
	case "wb":
		fields["token"] = true
	case "ym":
		fields["api_key"] = true
		fields["oauth_token"] = true
		fields["business_id"] = true
		fields["campaign_id"] = true
	case "ozon":
		fields["client_id"] = true
		fields["api_key"] = true
	}
	return fields
}

func credentialFields(id string, payload map[string]string) map[string]bool {
	fields := map[string]bool{}
	switch id {
	case "wb":
		fields["token"] = payload["token"] != ""
	case "ym":
		fields["api_key"] = payload["api_key"] != ""
		fields["oauth_token"] = payload["oauth_token"] != ""
		fields["business_id"] = payload["business_id"] != ""
		fields["campaign_id"] = payload["campaign_id"] != ""
	case "ozon":
		fields["client_id"] = payload["client_id"] != ""
		fields["api_key"] = payload["api_key"] != ""
	}
	return fields
}

// credentialConfigured reports whether id's stored fields amount to a usable
// credential. For WB, presence alone is not enough: the token must also pass
// ValidatePersonalTokenMetadata, and an unusable token yields an explanatory
// warning instead of a silent "not configured".
func credentialConfigured(id string, fields map[string]bool, payload map[string]string) (bool, string) {
	switch id {
	case "wb":
		if !fields["token"] {
			return false, ""
		}
		if err := wbtoken.ValidatePersonalTokenMetadata(payload["token"], time.Now()); err != nil {
			return false, err.Error()
		}
		return true, ""
	case "ym":
		return fields["business_id"] && (fields["api_key"] || fields["oauth_token"]), ""
	case "ozon":
		return fields["client_id"] && fields["api_key"], ""
	default:
		return false, ""
	}
}

func isKnownMarketplace(id string) bool {
	return id == "wb" || id == "ym" || id == "ozon"
}
