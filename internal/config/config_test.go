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
