package site

import "testing"

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
