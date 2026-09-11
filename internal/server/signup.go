package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"reviews/internal/auth"
	"reviews/internal/store"
)

// signupRequest registers a new tenant (SaaS self-serve). Login+password
// become the tenant's first admin; shopOrigin is the seller's shop the
// widget will be embedded on (per-tenant CORS allowlist).
type signupRequest struct {
	Login      string `json:"login"`
	Password   string `json:"password"`
	ShopOrigin string `json:"shopOrigin"`
}

// handleSignup creates a tenant with a 14-day trial and its first admin,
// then logs the new admin in. Only mounted in SaaS mode (strict tenant
// mode); single-tenant installs keep setup as their only onboarding path.
func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	if !store.StrictTenantMode() {
		writeError(w, http.StatusNotFound, errors.New("signup is not enabled"))
		return
	}

	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}
	req.Login = strings.TrimSpace(req.Login)
	req.ShopOrigin = strings.TrimRight(strings.TrimSpace(req.ShopOrigin), "/")
	if req.Login == "" {
		writeError(w, http.StatusBadRequest, errors.New("login is required"))
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, errors.New("password must be at least 8 characters"))
		return
	}
	if req.ShopOrigin == "" || !strings.HasPrefix(req.ShopOrigin, "http") {
		writeError(w, http.StatusBadRequest, errors.New("адрес магазина обязателен, например https://myshop.ru"))
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Login is globally unique (admin_users.login uniqueIndex); a collision
	// is a plain 409 without leaking whether the login exists.
	if _, err := s.store.GetAdminUserByLogin(context.WithoutCancel(r.Context()), req.Login); err == nil {
		writeError(w, http.StatusConflict, errors.New("этот логин уже занят"))
		return
	}

	tenant, err := s.store.CreateTenantWithAdmin(r.Context(), req.Login, hash, req.ShopOrigin)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate key") {
			writeError(w, http.StatusConflict, errors.New("этот логин уже занят"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Log the admin in right away: the SPA lands on its own tenant.
	token, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.store.CreateSession(store.WithTenant(r.Context(), tenant.Tenant.ID), token, tenant.AdminID, tenant.Tenant.ID, expires); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	setSessionCookie(w, token, expires, s.cfg.SecureCookies)
	writeJSON(w, http.StatusCreated, map[string]any{
		"status":    "ok",
		"publicKey": tenant.Tenant.PublicKey,
		"trialEnds": tenant.Tenant.TrialEndsAt,
	})
}
