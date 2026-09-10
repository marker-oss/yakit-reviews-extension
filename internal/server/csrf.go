package server

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"reviews/internal/auth"
)

const (
	csrfCookieName = "reviews_csrf"
	csrfHeaderName = "X-CSRF-Token"
)

// requireCSRF enforces the double-submit cookie pattern on state-changing
// requests: the X-CSRF-Token header must equal the reviews_csrf cookie.
func requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(csrfCookieName)
		header := r.Header.Get(csrfHeaderName)
		if err != nil || header == "" ||
			subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
			writeError(w, http.StatusForbidden, errors.New("invalid CSRF token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleCSRFToken issues a CSRF token cookie and returns the value so the SPA
// can echo it back in the X-CSRF-Token header.
func (s *Server) handleCSRFToken(w http.ResponseWriter, _ *http.Request) {
	token, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": token})
}
