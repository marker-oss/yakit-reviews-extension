package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reviews/internal/store"
)

type ctxKey string

const userIDKey ctxKey = "adminUserID"

const sessionCookieName = "reviews_session"

// securityHeaders sets conservative defaults for all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// reloadShopOrigins rebuilds the admin-configured CORS origin set from the
// stored shop_origin setting. Called when the route tree is built and again
// whenever the setting is saved, so a seller can fix CORS from the admin
// panel without a restart.
func (s *Server) reloadShopOrigins(ctx context.Context) {
	set := make(map[string]bool)
	if s.store != nil {
		if value, err := s.store.GetAppSetting(ctx, store.SettingShopOrigin); err == nil {
			for _, origin := range originAndSibling(value) {
				set[origin] = true
			}
		}
	}
	s.originsMu.Lock()
	s.shopOrigins = set
	s.originsMu.Unlock()
}

// shopOriginAllowed reports whether origin matches the admin-configured shop
// origin (or its www/apex sibling).
func (s *Server) shopOriginAllowed(origin string) bool {
	s.originsMu.RLock()
	defer s.originsMu.RUnlock()
	return s.shopOrigins[origin]
}

// originAndSibling normalizes a stored shop origin (sellers often paste a URL
// with a path or trailing slash) to its scheme://host origin, paired with its
// www/apex sibling because shops commonly answer on both hosts. IPs and
// dot-less hosts (localhost) get no sibling.
func originAndSibling(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil
	}
	origin := u.Scheme + "://" + u.Host
	host := u.Hostname()
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return []string{origin}
	}
	sibling := "www." + u.Host
	if strings.HasPrefix(u.Host, "www.") {
		sibling = strings.TrimPrefix(u.Host, "www.")
	}
	return []string{origin, u.Scheme + "://" + sibling}
}

// cors adds Access-Control headers for configured shop origins on public
// routes so the embedded widget can fetch reviews data cross-origin. Admin
// routes are skipped (same-origin only). When no origins are configured the
// middleware is a no-op, preserving prior behavior.
func (s *Server) cors(next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(s.cfg.AllowedOrigins))
	for _, o := range s.cfg.AllowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/admin/") {
			if origin := r.Header.Get("Origin"); origin != "" && (allowed[origin] || s.shopOriginAllowed(origin)) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Accept, Content-Type")
					h.Set("Access-Control-Max-Age", "86400")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requireSession rejects requests without a valid session cookie.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, errors.New("authentication required"))
			return
		}
		sess, err := s.store.GetValidSession(r.Context(), cookie.Value, time.Now())
		if err != nil {
			writeError(w, http.StatusUnauthorized, errors.New("authentication required"))
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, sess.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromContext(ctx context.Context) (uint, bool) {
	id, ok := ctx.Value(userIDKey).(uint)
	return id, ok
}

// setSessionCookie writes the session cookie with hardened attributes.
func setSessionCookie(w http.ResponseWriter, token string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
