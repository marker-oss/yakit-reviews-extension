package installer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPAdminClientFlow(t *testing.T) {
	var calls []string
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "health")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/admin/api/setup", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "setup")
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("setup decode: %v", err)
		}
		if payload["login"] != "admin" || payload["password"] != "admin-password" {
			t.Fatalf("setup payload = %+v", payload)
		}
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/admin/api/login", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "login")
		http.SetCookie(w, &http.Cookie{Name: "reviews_session", Value: "session", Path: "/"})
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/admin/api/csrf", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "csrf")
		if _, err := r.Cookie("reviews_session"); err != nil {
			t.Fatalf("missing session cookie: %v", err)
		}
		http.SetCookie(w, &http.Cookie{Name: "reviews_csrf", Value: "csrf", Path: "/"})
		_, _ = w.Write([]byte(`{"csrf_token":"csrf"}`))
	})
	mux.HandleFunc("/admin/api/marketplaces/ym/credentials", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "creds")
		if got := r.Header.Get("X-CSRF-Token"); got != "csrf" {
			t.Fatalf("csrf header = %q", got)
		}
		var payload struct {
			Enabled bool              `json:"enabled"`
			Values  map[string]string `json:"values"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("credentials decode: %v", err)
		}
		if !payload.Enabled || payload.Values["api_key"] != "ym-key" || payload.Values["business_id"] != "business-1" {
			t.Fatalf("credentials payload = %+v", payload)
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/admin/api/site-links/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"state":"done","total":1,"crawled":1}`))
			return
		}
		calls = append(calls, "refresh")
		if got := r.Header.Get("X-CSRF-Token"); got != "csrf" {
			t.Fatalf("refresh csrf header = %q", got)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"state":"running"}`))
	})
	mux.HandleFunc("/admin/api/reviews/publish", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "publish")
		if got := r.Header.Get("X-CSRF-Token"); got != "csrf" {
			t.Fatalf("publish csrf header = %q", got)
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewHTTPAdminClient(server.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	client.pollInterval = 5 * time.Millisecond
	if err := client.WaitHealth(t.Context(), server.URL); err != nil {
		t.Fatalf("health: %v", err)
	}
	if err := client.SetupAdmin(t.Context(), "admin", "admin-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := client.Login(t.Context(), "admin", "admin-password"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := client.SaveMarketplaceCredentials(t.Context(), CredentialRequest{
		ID:      "ym",
		Enabled: true,
		Values:  map[string]string{"api_key": "ym-key", "business_id": "business-1"},
	}); err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if err := client.RefreshSiteLinks(t.Context()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if err := client.PublishReviews(t.Context()); err != nil {
		t.Fatalf("publish: %v", err)
	}
	want := "health,setup,login,csrf,creds,refresh,publish"
	if got := join(calls); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
}

func TestRefreshSiteLinksWaitsForBackgroundJob(t *testing.T) {
	polls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/api/site-links/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"state":"running","total":0,"crawled":0}`))
	})
	mux.HandleFunc("GET /admin/api/site-links/refresh", func(w http.ResponseWriter, r *http.Request) {
		polls++
		if polls < 3 {
			_, _ = w.Write([]byte(`{"state":"running","total":5,"crawled":2}`))
			return
		}
		_, _ = w.Write([]byte(`{"state":"done","total":5,"crawled":5,"products":5,"articles":1}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewHTTPAdminClient(server.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	client.csrf = "csrf"
	client.pollInterval = 5 * time.Millisecond

	if err := client.RefreshSiteLinks(t.Context()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if polls < 3 {
		t.Fatalf("polls = %d, want at least 3 (must wait for state=done)", polls)
	}
}

func TestRefreshSiteLinksReportsJobError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/api/site-links/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"state":"running"}`))
	})
	mux.HandleFunc("GET /admin/api/site-links/refresh", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"state":"error","error":"обход каталога не удался"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := NewHTTPAdminClient(server.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	client.csrf = "csrf"
	client.pollInterval = 5 * time.Millisecond

	err = client.RefreshSiteLinks(t.Context())
	if err == nil || !strings.Contains(err.Error(), "обход каталога не удался") {
		t.Fatalf("err = %v, want job error message", err)
	}
}

func TestHTTPAdminClientSetupAdminAllowsExistingAdmin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/api/setup" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		http.Error(w, `{"error":"admin already configured"}`, http.StatusConflict)
	}))
	defer server.Close()

	client, err := NewHTTPAdminClient(server.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	if err := client.SetupAdmin(t.Context(), "admin", "admin-password"); err != nil {
		t.Fatalf("setup existing admin should be allowed: %v", err)
	}
}

func join(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ","
		}
		out += value
	}
	return out
}
