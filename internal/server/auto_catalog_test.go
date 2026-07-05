package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAutoRefreshCatalogOnce(t *testing.T) {
	s := newAuthTestServer(t)
	s.cfg.StaticDir = t.TempDir()
	s.cfg.ProductLinksPath = s.cfg.StaticDir + "/product-links.json"

	// No sitemap configured → the tick must do nothing.
	if s.autoRefreshCatalogOnce(context.Background()) {
		t.Fatal("tick without a configured sitemap must be a no-op")
	}

	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + "http://" + r.Host + `/products/item-1</loc></url>
</urlset>`))
		default:
			_, _ = w.Write([]byte(`<div data-testid="ProductDetails">"sku":"107"</div>`))
		}
	}))
	defer shop.Close()
	s.cfg.SitemapURL = shop.URL + "/sitemap.xml"

	if !s.autoRefreshCatalogOnce(context.Background()) {
		t.Fatal("tick with a configured sitemap must start the crawl")
	}

	// While the job runs, the next tick must not start a second one — and
	// after completion the job must finish successfully.
	deadline := time.Now().Add(10 * time.Second)
	for {
		snap := s.siteLinksSnapshot()
		if snap.State == "running" {
			if s.autoRefreshCatalogOnce(context.Background()) {
				t.Fatal("tick must not start a second crawl while one is running")
			}
		} else {
			if snap.State != "done" {
				t.Fatalf("job state = %q, want done (error=%q)", snap.State, snap.Error)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("crawl did not finish, state=%+v", s.siteLinksSnapshot())
		}
		time.Sleep(50 * time.Millisecond)
	}
}
