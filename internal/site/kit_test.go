package site

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestFetchProductURLsFiltersToProducts(t *testing.T) {
	var shopURL string
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%[1]s/</loc></url>
  <url><loc>%[1]s/catalog</loc></url>
  <url><loc>%[1]s/products/a-107</loc></url>
  <url><loc>%[1]s/products/b-208</loc></url>
</urlset>`, shopURL)
	}))
	defer shop.Close()
	shopURL = shop.URL

	urls, err := FetchProductURLs(context.Background(), nil, shop.URL+"/sitemap.xml")
	if err != nil {
		t.Fatalf("FetchProductURLs: %v", err)
	}
	want := []string{shopURL + "/products/a-107", shopURL + "/products/b-208"}
	if !reflect.DeepEqual(urls, want) {
		t.Fatalf("urls = %v, want %v", urls, want)
	}
}

func TestCrawlProductLinksReportsProgress(t *testing.T) {
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<html><head><script>{"sku":"sku%s"}</script></head></html>`, r.URL.Path[len(r.URL.Path)-3:])
	}))
	defer shop.Close()

	urls := []string{
		shop.URL + "/products/a-107",
		shop.URL + "/products/b-208",
		shop.URL + "/products/c-309",
	}
	var progress []int
	links, err := CrawlProductLinks(context.Background(), nil, urls, func(done int) {
		progress = append(progress, done)
	})
	if err != nil {
		t.Fatalf("CrawlProductLinks: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("links = %v, want 3 entries", links)
	}
	if !reflect.DeepEqual(progress, []int{1, 2, 3}) {
		t.Fatalf("progress = %v, want [1 2 3]", progress)
	}
}

func TestCrawlProductLinksCancelledContextReturnsError(t *testing.T) {
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sku":"107"}`))
	}))
	defer shop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	links, err := CrawlProductLinks(ctx, nil, []string{shop.URL + "/products/a-107"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(links) != 0 {
		t.Fatalf("links = %v, want none for a cancelled context", links)
	}
}

func TestNewProductURLsSkipsAlreadyCrawled(t *testing.T) {
	sitemap := []string{
		"https://shop.test/products/a-107",
		"https://shop.test/products/b-208",
		"https://shop.test/products/c-309",
	}
	known := []ProductLink{
		{SellerArticle: "107", URL: "https://shop.test/products/a-107"},
		{SellerArticle: "999", URL: "https://shop.test/products/removed"},
	}
	got := NewProductURLs(sitemap, known)
	want := []string{
		"https://shop.test/products/b-208",
		"https://shop.test/products/c-309",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NewProductURLs = %v, want %v", got, want)
	}
}

func TestMergeProductLinksPrunesRemovedAndOverlaysCrawled(t *testing.T) {
	sitemap := []string{
		"https://shop.test/products/a-107",
		"https://shop.test/products/b-208",
	}
	known := []ProductLink{
		{SellerArticle: "107", URL: "https://shop.test/products/a-107", Title: "Old title"},
		{SellerArticle: "999", URL: "https://shop.test/products/removed"},
	}
	crawled := []ProductLink{
		{SellerArticle: "208", URL: "https://shop.test/products/b-208", Title: "New product"},
	}
	got := MergeProductLinks(known, sitemap, crawled)
	want := []ProductLink{
		{SellerArticle: "107", URL: "https://shop.test/products/a-107", Title: "Old title"},
		{SellerArticle: "208", URL: "https://shop.test/products/b-208", Title: "New product"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MergeProductLinks = %v, want %v", got, want)
	}
}

func TestMergeProductLinksCrawledWinsOverKnown(t *testing.T) {
	sitemap := []string{"https://shop.test/products/a-107"}
	known := []ProductLink{
		{SellerArticle: "107", URL: "https://shop.test/products/a-107", Title: "Stale"},
	}
	crawled := []ProductLink{
		{SellerArticle: "107-NEW", URL: "https://shop.test/products/a-107", Title: "Fresh"},
	}
	got := MergeProductLinks(known, sitemap, crawled)
	want := []ProductLink{
		{SellerArticle: "107-NEW", URL: "https://shop.test/products/a-107", Title: "Fresh"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MergeProductLinks = %v, want %v", got, want)
	}
}

func TestProductLinkMapKeepsOnePerArticleAndSkipsEmpty(t *testing.T) {
	links := []ProductLink{
		{SellerArticle: "107", URL: "https://shop.test/products/a-107"},
		{SellerArticle: "", URL: "https://shop.test/products/no-article"},
		{SellerArticle: "208", URL: ""},
		{SellerArticle: "107", URL: "https://shop.test/products/a-107-dup"},
	}
	m := ProductLinkMap(links)
	if len(m) != 1 {
		t.Fatalf("map = %v, want a single entry for article 107", m)
	}
	if m["107"] == "" {
		t.Fatalf("article 107 missing from map: %v", m)
	}
}
