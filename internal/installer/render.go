package installer

import (
	"fmt"
	"sort"
	"strings"
)

func RenderEnv(cfg Config) string {
	sitemapURL := cfg.Domains.SitemapURL
	if sitemapURL == "" {
		sitemapURL = strings.TrimRight(cfg.Domains.ShopOrigin, "/") + "/sitemap.xml"
	}
	lines := []string{
		"REVIEWS_DB_DRIVER=sqlite",
		"REVIEWS_SYNC_INTERVAL=1h",
		"REVIEWS_SYNC_BACKFILL_MONTHS=12",
		"REVIEWS_SYNC_OVERLAP=1h",
		"REVIEWS_LOG_LEVEL=info",
		"REVIEWS_LOG_FORMAT=json",
		"REVIEWS_INSECURE_COOKIES=",
		"REVIEWS_SITE_PRODUCT_URL_TEMPLATE=" + shellEnvValue(strings.TrimRight(cfg.Domains.ShopOrigin, "/")+"/search?query={seller_article_url}"),
		"REVIEWS_SHOP_ORIGIN=" + shellEnvValue(strings.TrimRight(cfg.Domains.ShopOrigin, "/")),
		"REVIEWS_PUBLIC_DOMAIN=" + shellEnvValue(cfg.Domains.ReviewsDomain),
		"REVIEWS_SITE_SITEMAP_URL=" + shellEnvValue(sitemapURL),
		"REVIEWS_WB_ENABLED=false",
		"REVIEWS_WB_TOKEN=",
		"REVIEWS_YM_ENABLED=false",
		"REVIEWS_YM_API_KEY=",
		"REVIEWS_YM_BUSINESS_ID=",
		"REVIEWS_YM_CAMPAIGN_ID=",
		"REVIEWS_OZON_ENABLED=false",
		"REVIEWS_OZON_CLIENT_ID=",
		"REVIEWS_OZON_API_KEY=",
	}
	return strings.Join(lines, "\n") + "\n"
}

// RenderComposeOverride pins the service to the prebuilt registry image so the
// VPS pulls it instead of compiling from source, and binds the app port to
// localhost (Caddy terminates TLS in front of it).
func RenderComposeOverride(cfg Config) string {
	return fmt.Sprintf(strings.TrimSpace(`
services:
  reviews:
    image: %s
    pull_policy: always
    ports:
      - "127.0.0.1:8080:8080"
`), cfg.Deploy.Image) + "\n"
}

func RenderCaddyfile(cfg Config) string {
	return fmt.Sprintf(`%s {
	encode zstd gzip
	reverse_proxy 127.0.0.1:8080
}
`, cfg.Domains.ReviewsDomain)
}

func shellEnvValue(value string) string {
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`")
	return `"` + replacer.Replace(value) + `"`
}

func MarketplaceCredentialRequests(cfg MarketplaceConfig) []CredentialRequest {
	requests := []CredentialRequest{
		{ID: "wb", Enabled: cfg.WB.Enabled, Values: map[string]string{}},
		{ID: "ym", Enabled: cfg.YM.Enabled, Values: map[string]string{}},
		{ID: "ozon", Enabled: cfg.Ozon.Enabled, Values: map[string]string{}},
	}
	if cfg.WB.Token != "" {
		requests[0].Values["token"] = cfg.WB.Token
	}
	if cfg.YM.APIKey != "" {
		requests[1].Values["api_key"] = cfg.YM.APIKey
	}
	if cfg.YM.OAuthToken != "" {
		requests[1].Values["oauth_token"] = cfg.YM.OAuthToken
	}
	if cfg.YM.BusinessID != "" {
		requests[1].Values["business_id"] = cfg.YM.BusinessID
	}
	if cfg.YM.CampaignID != "" {
		requests[1].Values["campaign_id"] = cfg.YM.CampaignID
	}
	if cfg.Ozon.ClientID != "" {
		requests[2].Values["client_id"] = cfg.Ozon.ClientID
	}
	if cfg.Ozon.APIKey != "" {
		requests[2].Values["api_key"] = cfg.Ozon.APIKey
	}
	return requests
}

type CredentialRequest struct {
	ID      string
	Enabled bool
	Values  map[string]string
}

func SecretValues(cfg Config) []string {
	secrets := []string{
		cfg.Server.Password,
		cfg.Server.KeyPassphrase,
		cfg.Server.SudoPassword,
		cfg.Admin.Password,
		cfg.Admin.PasswordConfirm,
		cfg.Marketplaces.WB.Token,
		cfg.Marketplaces.YM.APIKey,
		cfg.Marketplaces.YM.OAuthToken,
		cfg.Marketplaces.Ozon.APIKey,
	}
	sort.Slice(secrets, func(i, j int) bool {
		return len(secrets[i]) > len(secrets[j])
	})
	return secrets
}

func MaskSecrets(text string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) < 4 {
			continue
		}
		text = strings.ReplaceAll(text, secret, "****")
	}
	return text
}

func MaskedSummary(cfg Config) string {
	marketplaces := []string{}
	if cfg.Marketplaces.WB.Enabled {
		marketplaces = append(marketplaces, "WB")
	}
	if cfg.Marketplaces.YM.Enabled {
		marketplaces = append(marketplaces, "Yandex Market")
	}
	if cfg.Marketplaces.Ozon.Enabled {
		marketplaces = append(marketplaces, "Ozon")
	}
	if len(marketplaces) == 0 {
		marketplaces = append(marketplaces, "none")
	}
	return fmt.Sprintf(
		"Server: %s@%s:%d\nReviews domain: %s\nShop origin: %s\nAdmin login: %s\nMarketplaces: %s\nRepo: %s@%s",
		cfg.Server.User,
		cfg.Server.Host,
		cfg.Server.Port,
		cfg.Domains.ReviewsDomain,
		cfg.Domains.ShopOrigin,
		cfg.Admin.Login,
		strings.Join(marketplaces, ", "),
		cfg.Deploy.RepoURL,
		cfg.Deploy.DeployRef,
	)
}
