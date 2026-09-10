package installer

import (
	"strings"
	"testing"
)

func TestRenderEnvExcludesMarketplaceSecrets(t *testing.T) {
	cfg := validConfig()
	env := RenderEnv(cfg)
	for _, want := range []string{
		"REVIEWS_DB_DRIVER=sqlite",
		`REVIEWS_SHOP_ORIGIN="https://shop.example.com"`,
		`REVIEWS_PUBLIC_DOMAIN="reviews.example.com"`,
		`REVIEWS_SITE_SITEMAP_URL="https://shop.example.com/sitemap.xml"`,
		"REVIEWS_WB_ENABLED=false",
		"REVIEWS_YM_ENABLED=false",
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("env missing %q:\n%s", want, env)
		}
	}
	for _, secret := range []string{"wb-token", "ym-key", "admin-password", "ssh-password"} {
		if strings.Contains(env, secret) {
			t.Fatalf("env leaked secret %q:\n%s", secret, env)
		}
	}
}

func TestRenderComposeAndCaddy(t *testing.T) {
	cfg := validConfig()
	compose := RenderComposeOverride(cfg)
	if !strings.Contains(compose, `"127.0.0.1:8080:8080"`) {
		t.Fatalf("compose should bind app port to localhost:\n%s", compose)
	}
	// Without !override compose MERGES the base file's 0.0.0.0:8080 with the
	// override's 127.0.0.1:8080 and the container fails to start on vanilla
	// docker.io ("address already in use") — seen on a customer server.
	if !strings.Contains(compose, "ports: !override") {
		t.Fatalf("compose must replace (not merge) the base ports list:\n%s", compose)
	}
	if !strings.Contains(compose, "image: "+cfg.Deploy.Image) {
		t.Fatalf("compose should pin the prebuilt image:\n%s", compose)
	}
	caddy := RenderCaddyfile(cfg)
	if !strings.Contains(caddy, "reviews.example.com {") || !strings.Contains(caddy, "reverse_proxy 127.0.0.1:8080") {
		t.Fatalf("unexpected caddyfile:\n%s", caddy)
	}
}

func TestMarketplaceCredentialRequests(t *testing.T) {
	cfg := validConfig()
	requests := MarketplaceCredentialRequests(cfg.Marketplaces)
	if len(requests) != 3 {
		t.Fatalf("requests len = %d", len(requests))
	}
	if requests[0].ID != "wb" || !requests[0].Enabled || requests[0].Values["token"] != "wb-token" {
		t.Fatalf("wb request = %+v", requests[0])
	}
	if requests[1].ID != "ym" || requests[1].Values["api_key"] != "ym-key" || requests[1].Values["business_id"] != "business-1" {
		t.Fatalf("ym request = %+v", requests[1])
	}
	if requests[2].ID != "ozon" || requests[2].Enabled {
		t.Fatalf("ozon request = %+v", requests[2])
	}
}

func TestMaskSecrets(t *testing.T) {
	cfg := validConfig()
	text := "failed with wb-token and admin-password"
	got := MaskSecrets(text, SecretValues(cfg))
	if strings.Contains(got, "wb-token") || strings.Contains(got, "admin-password") {
		t.Fatalf("secrets were not masked: %s", got)
	}
	if strings.Count(got, "****") != 2 {
		t.Fatalf("masked output = %s", got)
	}
}
