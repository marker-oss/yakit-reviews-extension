package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"reviews/internal/store"
)

type settingsResponse struct {
	AgreementURL string `json:"agreementUrl"`
}

type settingsRequest struct {
	AgreementURL *string `json:"agreementUrl"`
}

// agreementURL resolves the configured user-agreement / consent page URL,
// preferring the admin-editable stored value and falling back to the
// REVIEWS_PRIVACY_URL env default.
func (s *Server) agreementURL(r *http.Request) string {
	if stored, err := s.store.GetAppSetting(r.Context(), store.SettingAgreementURL); err == nil && stored != "" {
		return stored
	}
	return s.cfg.PrivacyURL
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	stored, err := s.store.GetAppSetting(r.Context(), store.SettingAgreementURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse{AgreementURL: stored})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	if req.AgreementURL != nil {
		value := strings.TrimSpace(*req.AgreementURL)
		if value != "" && !validAgreementURL(value) {
			writeError(w, http.StatusBadRequest, errors.New("agreement URL must be an http(s) URL"))
			return
		}
		if err := s.store.SetAppSetting(r.Context(), store.SettingAgreementURL, value); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	stored, err := s.store.GetAppSetting(r.Context(), store.SettingAgreementURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse{AgreementURL: stored})
}

func validAgreementURL(value string) bool {
	return strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")
}
