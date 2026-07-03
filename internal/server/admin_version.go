package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// How long a latest-release answer is trusted before the next lookup; failed
// lookups retry sooner so a transient GitHub hiccup does not hide an update
// for a whole day.
const (
	versionCheckTTL      = 24 * time.Hour
	versionCheckErrorTTL = time.Hour
)

type latestRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// handleVersion reports the running version and, when a release feed is
// configured, the latest published release so the admin SPA can show an
// update banner. The lookup runs server-side and is cached; an empty
// LatestReleaseURL disables checking entirely (REVIEWS_UPDATE_CHECK=false).
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	current := s.cfg.Version
	if current == "" {
		current = "dev"
	}
	latest := s.latestRelease(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"current":         current,
		"latest":          latest.TagName,
		"updateAvailable": versionLess(current, latest.TagName),
		"releaseUrl":      latest.HTMLURL,
	})
}

func (s *Server) latestRelease(ctx context.Context) latestRelease {
	if s.cfg.LatestReleaseURL == "" {
		return latestRelease{}
	}

	s.versionMu.Lock()
	defer s.versionMu.Unlock()
	ttl := versionCheckTTL
	if s.versionCache.TagName == "" {
		ttl = versionCheckErrorTTL
	}
	if !s.versionCheckedAt.IsZero() && time.Since(s.versionCheckedAt) < ttl {
		return s.versionCache
	}

	s.versionCheckedAt = time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.LatestReleaseURL, nil)
	if err != nil {
		return s.versionCache
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		s.logger.Warn("latest release check failed", "error", err)
		return s.versionCache
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		s.logger.Warn("latest release check failed", "status", res.StatusCode)
		return s.versionCache
	}
	var release latestRelease
	if err := json.NewDecoder(res.Body).Decode(&release); err != nil {
		s.logger.Warn("latest release check failed", "error", err)
		return s.versionCache
	}
	s.versionCache = release
	return release
}

// versionLess reports whether semver tag a is strictly older than b. Anything
// that does not parse as vX.Y.Z (e.g. "dev") never compares as older, so dev
// builds and unexpected tags never trigger the update banner.
func versionLess(a, b string) bool {
	av, aok := parseSemver(a)
	bv, bok := parseSemver(b)
	if !aok || !bok {
		return false
	}
	for i := range av {
		if av[i] != bv[i] {
			return av[i] < bv[i]
		}
	}
	return false
}

func parseSemver(tag string) ([3]int, bool) {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "v")
	parts := strings.SplitN(tag, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, part := range parts {
		// Tolerate suffixes like "1.2.3-rc1" on the last component.
		if i == 2 {
			if dash := strings.IndexAny(part, "-+"); dash >= 0 {
				part = part[:dash]
			}
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
