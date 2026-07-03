package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reviews/internal/auth"
	"reviews/internal/config"
	"reviews/internal/store"
)

func TestDiscoverSiteURLsWritesPartialCatalogOnTimeout(t *testing.T) {
	// Fake shop: product A responds instantly, product B hangs past the scan
	// timeout, so the crawl ends with a partial result that must still be
	// persisted (a re-run continues from it).
	release := make(chan struct{})
	var shopURL string
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + shopURL + `/products/a-107</loc></url>
  <url><loc>` + shopURL + `/products/b-208</loc></url>
</urlset>`))
		case "/products/a-107":
			_, _ = w.Write([]byte(`{"sku":"107"}`))
		case "/products/b-208":
			<-release
		default:
			http.NotFound(w, r)
		}
	}))
	defer shop.Close()
	// Unblock the hung handler before shop.Close (defers run LIFO).
	defer close(release)
	shopURL = shop.URL

	out := filepath.Join(t.TempDir(), "product-links.json")
	code := run([]string{"discover-site-urls", "--sitemap", shop.URL + "/sitemap.xml", "--out", out, "--timeout", "700ms"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (partial result is still written)", code, exitOK)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("partial catalog not written: %v", err)
	}
	if !strings.Contains(string(data), `"107"`) {
		t.Fatalf("partial catalog missing crawled article: %s", data)
	}
}

func TestAdminResetPasswordCommand(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reviews.db")
	t.Setenv("REVIEWS_DB_DRIVER", "sqlite")
	t.Setenv("REVIEWS_DB_DSN", dbPath)
	t.Setenv("REVIEWS_WB_ENABLED", "false")
	t.Setenv("REVIEWS_YM_ENABLED", "false")

	st, err := store.Open(config.DBConfig{Driver: "sqlite", DSN: dbPath})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	oldHash, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatalf("hash old: %v", err)
	}
	user, err := st.CreateAdminUser(context.Background(), "admin", oldHash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.CreateSession(context.Background(), "tok-1", user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	code := run([]string{"admin", "reset-password", "--login", "admin", "--password", "new-password"})
	if code != exitOK {
		t.Fatalf("exit code = %d", code)
	}

	got, err := st.GetAdminUserByLogin(context.Background(), "admin")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	ok, err := auth.VerifyPassword(got.PasswordHash, "new-password")
	if err != nil || !ok {
		t.Fatalf("new password not accepted ok=%v err=%v", ok, err)
	}
	if _, err := st.GetValidSession(context.Background(), "tok-1", time.Now()); err == nil {
		t.Fatal("expected reset to delete active sessions")
	}
}
