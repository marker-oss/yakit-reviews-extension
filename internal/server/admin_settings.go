package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"reviews/internal/store"
)

type settingsResponse struct {
	AgreementURL      string `json:"agreementUrl"`
	ShopOrigin        string `json:"shopOrigin"`
	SitemapURL        string `json:"sitemapUrl"`
	PublishRepliesWb  string `json:"publishRepliesWb"`
	PublishRepliesYm  string `json:"publishRepliesYm"`
	PublishRepliesOzon string `json:"publishRepliesOzon"`
}

type settingsRequest struct {
	AgreementURL      *string `json:"agreementUrl"`
	ShopOrigin        *string `json:"shopOrigin"`
	SitemapURL        *string `json:"sitemapUrl"`
	PublishRepliesWb  *string `json:"publish_replies_wb"`
	PublishRepliesYm  *string `json:"publish_replies_ym"`
	PublishRepliesOzon *string `json:"publish_replies_ozon"`
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

// effectiveSitemapURL resolves the shop sitemap the catalog refresh crawls.
// Priority: an explicit admin-stored sitemap URL, then a sitemap derived from
// the admin-stored shop origin, then the env-configured default (s.cfg.SitemapURL).
func (s *Server) effectiveSitemapURL(ctx context.Context) string {
	if stored, err := s.store.GetAppSetting(ctx, store.SettingSitemapURL); err == nil && stored != "" {
		return stored
	}
	if origin, err := s.store.GetAppSetting(ctx, store.SettingShopOrigin); err == nil && origin != "" {
		return strings.TrimRight(origin, "/") + "/sitemap.xml"
	}
	return s.cfg.SitemapURL
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	resp, err := s.loadSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	urlUpdates := []struct {
		key   string
		value *string
		label string
	}{
		{store.SettingAgreementURL, req.AgreementURL, "agreement URL"},
		{store.SettingShopOrigin, req.ShopOrigin, "shop origin"},
		{store.SettingSitemapURL, req.SitemapURL, "sitemap URL"},
	}
	for _, u := range urlUpdates {
		if u.value == nil {
			continue
		}
		value := strings.TrimSpace(*u.value)
		if value != "" && !validHTTPURL(value) {
			writeError(w, http.StatusBadRequest, errors.New(u.label+" must be an http(s) URL"))
			return
		}
		if err := s.store.SetAppSetting(r.Context(), u.key, value); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	toggleUpdates := []struct {
		key   string
		value *string
	}{
		{store.PublishRepliesKey("wb"), req.PublishRepliesWb},
		{store.PublishRepliesKey("ym"), req.PublishRepliesYm},
		{store.PublishRepliesKey("ozon"), req.PublishRepliesOzon},
	}
	for _, u := range toggleUpdates {
		if u.value == nil {
			continue
		}
		if err := s.store.SetAppSetting(r.Context(), u.key, *u.value); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	resp, err := s.loadSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) loadSettings(ctx context.Context) (settingsResponse, error) {
	agreement, err := s.store.GetAppSetting(ctx, store.SettingAgreementURL)
	if err != nil {
		return settingsResponse{}, err
	}
	origin, err := s.store.GetAppSetting(ctx, store.SettingShopOrigin)
	if err != nil {
		return settingsResponse{}, err
	}
	sitemap, err := s.store.GetAppSetting(ctx, store.SettingSitemapURL)
	if err != nil {
		return settingsResponse{}, err
	}
	publishWb, err := s.store.GetAppSetting(ctx, store.PublishRepliesKey("wb"))
	if err != nil {
		return settingsResponse{}, err
	}
	publishYm, err := s.store.GetAppSetting(ctx, store.PublishRepliesKey("ym"))
	if err != nil {
		return settingsResponse{}, err
	}
	publishOzon, err := s.store.GetAppSetting(ctx, store.PublishRepliesKey("ozon"))
	if err != nil {
		return settingsResponse{}, err
	}
	return settingsResponse{
		AgreementURL:       agreement,
		ShopOrigin:         origin,
		SitemapURL:         sitemap,
		PublishRepliesWb:   publishWb,
		PublishRepliesYm:   publishYm,
		PublishRepliesOzon: publishOzon,
	}, nil
}

func validHTTPURL(value string) bool {
	return strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")
}
