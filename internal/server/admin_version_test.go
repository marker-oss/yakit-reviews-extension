package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type versionPayload struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseUrl"`
}

func getVersion(t *testing.T, s *Server, cookie *http.Cookie) versionPayload {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/version", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("version endpoint = %d, body=%s", rec.Code, rec.Body.String())
	}
	var payload versionPayload
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode version: %v", err)
	}
	return payload
}

func TestVersionEndpointReportsUpdateAndCachesCheck(t *testing.T) {
	hits := 0
	releases := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","html_url":"https://github.com/example/releases/tag/v9.9.9"}`))
	}))
	defer releases.Close()

	s := newAuthTestServer(t)
	s.cfg.Version = "v0.3.0"
	s.cfg.LatestReleaseURL = releases.URL
	cookie := loginTestAdmin(t, s)

	payload := getVersion(t, s, cookie)
	if payload.Current != "v0.3.0" || payload.Latest != "v9.9.9" {
		t.Fatalf("payload = %+v", payload)
	}
	if !payload.UpdateAvailable {
		t.Fatalf("updateAvailable = false, want true: %+v", payload)
	}
	if payload.ReleaseURL != "https://github.com/example/releases/tag/v9.9.9" {
		t.Fatalf("releaseUrl = %q", payload.ReleaseURL)
	}

	// The latest-release lookup is cached: a second request must not re-hit
	// the releases API.
	_ = getVersion(t, s, cookie)
	if hits != 1 {
		t.Fatalf("releases API hits = %d, want 1 (cached)", hits)
	}
}

func TestVersionEndpointNoUpdateWhenUpToDate(t *testing.T) {
	releases := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.3.0","html_url":"https://example.test"}`))
	}))
	defer releases.Close()

	s := newAuthTestServer(t)
	s.cfg.Version = "v0.3.0"
	s.cfg.LatestReleaseURL = releases.URL
	cookie := loginTestAdmin(t, s)

	payload := getVersion(t, s, cookie)
	if payload.UpdateAvailable {
		t.Fatalf("updateAvailable = true for equal versions: %+v", payload)
	}
}

func TestVersionEndpointDisabledCheck(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.Version = "v0.3.0"
	// LatestReleaseURL left empty: checking is disabled.
	cookie := loginTestAdmin(t, s)

	payload := getVersion(t, s, cookie)
	if payload.Current != "v0.3.0" || payload.Latest != "" || payload.UpdateAvailable {
		t.Fatalf("payload = %+v, want current only", payload)
	}
}

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.3.0", "v0.4.0", true},
		{"v0.9.9", "v0.10.0", true}, // numeric, not lexicographic
		{"v1.0.0", "v1.0.0", false},
		{"v2.0.0", "v1.9.9", false},
		{"dev", "v1.0.0", false}, // non-semver current never triggers a banner
		{"v1.0.0", "main", false},
	}
	for _, c := range cases {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Fatalf("versionLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
