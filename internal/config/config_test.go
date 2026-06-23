package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvDoesNotOverrideExistingEnv(t *testing.T) {
	t.Setenv("REVIEWS_WB_TOKEN", "from-env")
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("REVIEWS_WB_TOKEN=from-file\nYM_BUSINESS_ID=business-1\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("load dotenv: %v", err)
	}

	if got := os.Getenv("REVIEWS_WB_TOKEN"); got != "from-env" {
		t.Fatalf("REVIEWS_WB_TOKEN = %q", got)
	}
	if got := os.Getenv("YM_BUSINESS_ID"); got != "business-1" {
		t.Fatalf("YM_BUSINESS_ID = %q", got)
	}
}

func TestLoadFromEnvDefaultsAreVendorNeutral(t *testing.T) {
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Web.ProductURLTemplate != "" {
		t.Fatalf("ProductURLTemplate default = %q, want empty (vendor-neutral)", cfg.Web.ProductURLTemplate)
	}
	if cfg.Web.ProductLinksPath != "data/product-links.json" {
		t.Fatalf("ProductLinksPath default = %q, want data/product-links.json", cfg.Web.ProductLinksPath)
	}
}

func TestSitemapURLDefaultsToShopOrigin(t *testing.T) {
	t.Setenv("REVIEWS_SHOP_ORIGIN", "https://myshop.example")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Web.SitemapURL != "https://myshop.example/sitemap.xml" {
		t.Fatalf("SitemapURL = %q, want derived from shop origin", cfg.Web.SitemapURL)
	}
}

func TestSitemapURLExplicitOverridesDefault(t *testing.T) {
	t.Setenv("REVIEWS_SHOP_ORIGIN", "https://myshop.example")
	t.Setenv("REVIEWS_SITE_SITEMAP_URL", "https://myshop.example/custom-sitemap.xml")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Web.SitemapURL != "https://myshop.example/custom-sitemap.xml" {
		t.Fatalf("SitemapURL = %q, want explicit override", cfg.Web.SitemapURL)
	}
}

func TestLoadFromEnvParsesShopOrigins(t *testing.T) {
	t.Setenv("REVIEWS_SHOP_ORIGIN", "https://shop.ru, https://www.shop.ru ,")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	got := cfg.Web.ShopOrigins
	want := []string{"https://shop.ru", "https://www.shop.ru"}
	if len(got) != len(want) {
		t.Fatalf("ShopOrigins = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ShopOrigins[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadFromEnvAcceptsMarketplaceAliases(t *testing.T) {
	t.Setenv("WB_API_TOKEN", "wb-token")
	t.Setenv("YM_OAUTH_TOKEN", "ym-token")
	t.Setenv("YM_BUSINESS_ID", "business-1")
	t.Setenv("OZON_CLIENT_ID", "ozon-client")
	t.Setenv("OZON_API_KEY", "ozon-key")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Marketplaces.WB.Token != "wb-token" {
		t.Fatalf("WB token alias was not loaded")
	}
	if cfg.Marketplaces.YM.OAuthToken != "ym-token" {
		t.Fatalf("YM oauth alias was not loaded")
	}
	if cfg.Marketplaces.YM.BusinessID != "business-1" {
		t.Fatalf("YM business id alias was not loaded")
	}
	if cfg.Marketplaces.Ozon.ClientID != "ozon-client" || cfg.Marketplaces.Ozon.APIKey != "ozon-key" {
		t.Fatalf("Ozon aliases were not loaded")
	}
}
