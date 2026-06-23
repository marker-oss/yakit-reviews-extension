package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshSiteLinksCrawlsAndRegeneratesExport(t *testing.T) {
	// Fake Kit shop: a sitemap pointing at one product page carrying "sku":"107".
	var shopURL string
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + shopURL + `/products/cotton-dress-107</loc></url>
</urlset>`))
		case "/products/cotton-dress-107":
			_, _ = w.Write([]byte(`<html><head><script type="application/ld+json">{"@type":"Product","sku":"107"}</script></head><body>ok</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer shop.Close()
	shopURL = shop.URL

	s := newAuthTestServer(t)
	s.cfg.StaticDir = t.TempDir()
	s.cfg.ProductLinksPath = filepath.Join(t.TempDir(), "product-links.json")
	s.cfg.SitemapURL = shop.URL + "/sitemap.xml"

	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	req := httptest.NewRequest(http.MethodPost, "/admin/api/site-links/refresh", nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Products int `json:"products"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Products != 1 {
		t.Fatalf("products = %d, want 1", resp.Products)
	}

	// In-memory map was swapped.
	if s.productLinks()["107"] == "" {
		t.Fatalf("product links map missing article 107: %v", s.productLinks())
	}

	// The crawled list was persisted.
	persisted, err := os.ReadFile(s.cfg.ProductLinksPath)
	if err != nil {
		t.Fatalf("read product-links file: %v", err)
	}
	if !strings.Contains(string(persisted), `"107"`) {
		t.Fatalf("product-links file missing article 107: %s", persisted)
	}

	// The widget link index was regenerated.
	if _, err := os.Stat(filepath.Join(s.cfg.StaticDir, "reviews-data", "links.json")); err != nil {
		t.Fatalf("links.json not written: %v", err)
	}
}

func TestRefreshSiteLinksRequiresSitemapURL(t *testing.T) {
	s := newAuthTestServer(t)
	// SitemapURL left empty.
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	req := httptest.NewRequest(http.MethodPost, "/admin/api/site-links/refresh", nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when sitemap URL unset", rec.Code)
	}
}
