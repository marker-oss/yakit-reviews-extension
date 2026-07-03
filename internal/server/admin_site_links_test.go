package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type refreshStatusPayload struct {
	State    string `json:"state"`
	Total    int    `json:"total"`
	Crawled  int    `json:"crawled"`
	Products int    `json:"products"`
	Articles int    `json:"articles"`
	Error    string `json:"error"`
}

func postRefresh(t *testing.T, s *Server, cookie *http.Cookie, csrf, query string) (int, refreshStatusPayload) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/admin/api/site-links/refresh"+query, nil)
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	var status refreshStatusPayload
	_ = json.NewDecoder(rec.Body).Decode(&status)
	return rec.Code, status
}

func getRefreshStatus(t *testing.T, s *Server, cookie *http.Cookie) refreshStatusPayload {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/site-links/refresh", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status endpoint = %d, body=%s", rec.Code, rec.Body.String())
	}
	var status refreshStatusPayload
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return status
}

func waitForRefreshDone(t *testing.T, s *Server, cookie *http.Cookie) refreshStatusPayload {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status := getRefreshStatus(t, s, cookie)
		switch status.State {
		case "done":
			return status
		case "error":
			t.Fatalf("refresh ended in error: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for refresh to finish")
	return refreshStatusPayload{}
}

func TestRefreshSiteLinksStatusIdleInitially(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)

	status := getRefreshStatus(t, s, cookie)
	if status.State != "idle" {
		t.Fatalf("state = %q, want idle", status.State)
	}
}

func TestRefreshSiteLinksRunsInBackgroundAndRegeneratesExport(t *testing.T) {
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

	code, status := postRefresh(t, s, cookie, csrf, "")
	if code != http.StatusAccepted {
		t.Fatalf("refresh status = %d, want 202", code)
	}
	if status.State != "running" {
		t.Fatalf("state after start = %q, want running", status.State)
	}

	final := waitForRefreshDone(t, s, cookie)
	if final.Products != 1 {
		t.Fatalf("products = %d, want 1", final.Products)
	}
	if final.Total != 1 || final.Crawled != 1 {
		t.Fatalf("progress = %d/%d, want 1/1", final.Crawled, final.Total)
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

func newIncrementalTestShop(t *testing.T) (*httptest.Server, func(path string) int) {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int{}
	var shopURL string
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
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
			_, _ = w.Write([]byte(`{"sku":"208"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	shopURL = shop.URL
	return shop, func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[path]
	}
}

func seedKnownLinks(t *testing.T, path, shopURL string) {
	t.Helper()
	known := `[
  {"sellerArticle":"107","url":"` + shopURL + `/products/a-107","title":"A"},
  {"sellerArticle":"999","url":"` + shopURL + `/products/removed","title":"Gone"}
]`
	if err := os.WriteFile(path, []byte(known), 0o644); err != nil {
		t.Fatalf("seed product-links file: %v", err)
	}
}

func TestRefreshSiteLinksIncrementalSkipsKnownURLs(t *testing.T) {
	shop, hits := newIncrementalTestShop(t)
	defer shop.Close()

	s := newAuthTestServer(t)
	s.cfg.StaticDir = t.TempDir()
	s.cfg.ProductLinksPath = filepath.Join(t.TempDir(), "product-links.json")
	s.cfg.SitemapURL = shop.URL + "/sitemap.xml"
	seedKnownLinks(t, s.cfg.ProductLinksPath, shop.URL)

	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	code, _ := postRefresh(t, s, cookie, csrf, "")
	if code != http.StatusAccepted {
		t.Fatalf("refresh status = %d, want 202", code)
	}
	final := waitForRefreshDone(t, s, cookie)

	if hits("/products/a-107") != 0 {
		t.Fatalf("known product was re-crawled %d times, want 0", hits("/products/a-107"))
	}
	if hits("/products/b-208") == 0 {
		t.Fatalf("new product was not crawled")
	}
	if final.Total != 1 {
		t.Fatalf("total = %d, want 1 (only the new URL)", final.Total)
	}
	if final.Products != 2 {
		t.Fatalf("products = %d, want 2 (known kept + new added)", final.Products)
	}

	persisted, err := os.ReadFile(s.cfg.ProductLinksPath)
	if err != nil {
		t.Fatalf("read product-links file: %v", err)
	}
	body := string(persisted)
	if !strings.Contains(body, `"107"`) || !strings.Contains(body, `"208"`) {
		t.Fatalf("merged file missing links: %s", body)
	}
	if strings.Contains(body, "removed") {
		t.Fatalf("URL gone from sitemap was not pruned: %s", body)
	}
}

func TestRefreshSiteLinksFullModeRecrawlsKnownURLs(t *testing.T) {
	shop, hits := newIncrementalTestShop(t)
	defer shop.Close()

	s := newAuthTestServer(t)
	s.cfg.StaticDir = t.TempDir()
	s.cfg.ProductLinksPath = filepath.Join(t.TempDir(), "product-links.json")
	s.cfg.SitemapURL = shop.URL + "/sitemap.xml"
	seedKnownLinks(t, s.cfg.ProductLinksPath, shop.URL)

	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	code, _ := postRefresh(t, s, cookie, csrf, "?full=1")
	if code != http.StatusAccepted {
		t.Fatalf("refresh status = %d, want 202", code)
	}
	final := waitForRefreshDone(t, s, cookie)

	if hits("/products/a-107") == 0 {
		t.Fatalf("full refresh did not re-crawl known product")
	}
	if final.Total != 2 {
		t.Fatalf("total = %d, want 2 (full recrawl)", final.Total)
	}
}

func TestRefreshSiteLinksConflictWhenAlreadyRunning(t *testing.T) {
	release := make(chan struct{})
	var shopURL string
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + shopURL + `/products/a-107</loc></url>
</urlset>`))
		case "/products/a-107":
			<-release
			_, _ = w.Write([]byte(`{"sku":"107"}`))
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

	code, _ := postRefresh(t, s, cookie, csrf, "")
	if code != http.StatusAccepted {
		t.Fatalf("first refresh status = %d, want 202", code)
	}
	code, status := postRefresh(t, s, cookie, csrf, "")
	if code != http.StatusConflict {
		t.Fatalf("second refresh status = %d, want 409", code)
	}
	if status.State != "running" {
		t.Fatalf("second refresh state = %q, want running", status.State)
	}

	close(release)
	waitForRefreshDone(t, s, cookie)
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
