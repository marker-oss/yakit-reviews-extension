package installer

import (
	"strings"
	"testing"
)

func validConfig() Config {
	cfg := DefaultConfig()
	cfg.Server.Host = "203.0.113.10"
	cfg.Server.Password = "ssh-password"
	cfg.Domains.ReviewsDomain = "reviews.example.com"
	cfg.Domains.ShopOrigin = "https://shop.example.com"
	cfg.Admin.Password = "admin-password"
	cfg.Admin.PasswordConfirm = "admin-password"
	cfg.Marketplaces.WB.Enabled = true
	cfg.Marketplaces.WB.Token = "wb-token"
	cfg.Marketplaces.YM.Enabled = true
	cfg.Marketplaces.YM.APIKey = "ym-key"
	cfg.Marketplaces.YM.BusinessID = "business-1"
	return cfg
}

func TestConfigValidate(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}

	cfg.Admin.PasswordConfirm = "different"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("password mismatch error = %v", err)
	}

	cfg = validConfig()
	cfg.Domains.ReviewsDomain = "https://reviews.example.com"
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "without scheme") {
		t.Fatalf("domain error = %v", err)
	}

	cfg = validConfig()
	cfg.Marketplaces.YM.APIKey = ""
	cfg.Marketplaces.YM.OAuthToken = ""
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "YM API key or OAuth token") {
		t.Fatalf("ym credential error = %v", err)
	}
}

func TestConfigValidateSSHKey(t *testing.T) {
	cfg := validConfig()
	cfg.Server.AuthMethod = SSHAuthKey
	cfg.Server.Password = ""
	cfg.Server.KeyPath = "/home/me/.ssh/id_ed25519"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("key auth config: %v", err)
	}
	cfg.Server.KeyPath = ""
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "private key path") {
		t.Fatalf("key path error = %v", err)
	}
}
