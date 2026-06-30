package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	MarketplaceWB   = "wb"
	MarketplaceYM   = "ym"
	MarketplaceOzon = "ozon"
)

type Config struct {
	DB           DBConfig
	Sync         SyncConfig
	Log          LogConfig
	Web          WebConfig
	Marketplaces MarketplaceConfig
	Media        MediaConfig
}

type MediaConfig struct {
	// Allowlist of CDN host suffixes the image proxy may fetch from.
	Allowlist []string
	// MaxBytes caps a single fetched image; the response body is truncated at
	// this many bytes before processing. Default 8 MiB.
	MaxBytes int64
	// CacheEntries bounds the in-memory blurred-image LRU. Default 512.
	CacheEntries int
}

type DBConfig struct {
	Driver string
	DSN    string
}

type SyncConfig struct {
	Interval       time.Duration
	BackfillMonths int
	Overlap        time.Duration
}

type LogConfig struct {
	Level  string
	Format string
}

type WebConfig struct {
	ProductURLTemplate string
	ProductLinksPath   string
	// ShopOrigins are the embedding site origins allowed to fetch reviews data
	// cross-origin (CORS). Parsed from REVIEWS_SHOP_ORIGIN (comma-separated).
	ShopOrigins []string
	// SitemapURL is the shop sitemap crawled to map product pages to seller
	// articles. From REVIEWS_SITE_SITEMAP_URL, defaulting to the first shop
	// origin + "/sitemap.xml".
	SitemapURL string
	// PrivacyContact is the contact shown for data-subject requests (152-ФЗ).
	PrivacyContact string
	// PrivacyURL and ReviewTermsURL are shown next to the public submission form
	// consent checkboxes.
	PrivacyURL     string
	ReviewTermsURL string
	// UploadDir stores visitor-submitted review media.
	UploadDir string
}

type MarketplaceConfig struct {
	WB   WBConfig
	YM   YMConfig
	Ozon OzonConfig
}

type WBConfig struct {
	Enabled bool
	Token   string
}

type YMConfig struct {
	Enabled    bool
	APIKey     string
	OAuthToken string
	BusinessID string
	CampaignID string
}

type OzonConfig struct {
	Enabled  bool
	ClientID string
	APIKey   string
}

func LoadFromEnv() (Config, error) {
	if err := LoadDotEnv(".env"); err != nil {
		return Config{}, err
	}

	cfg := Config{
		DB: DBConfig{
			Driver: envString("REVIEWS_DB_DRIVER", "sqlite"),
			DSN:    envString("REVIEWS_DB_DSN", "./reviews.db"),
		},
		Sync: SyncConfig{
			Interval:       time.Hour,
			BackfillMonths: 12,
			Overlap:        time.Hour,
		},
		Log: LogConfig{
			Level:  envString("REVIEWS_LOG_LEVEL", "info"),
			Format: envString("REVIEWS_LOG_FORMAT", "text"),
		},
		Web: WebConfig{
			ProductURLTemplate: envString("REVIEWS_SITE_PRODUCT_URL_TEMPLATE", ""),
			ProductLinksPath:   envString("REVIEWS_SITE_PRODUCT_LINKS", "data/product-links.json"),
			ShopOrigins:        envList("REVIEWS_SHOP_ORIGIN"),
			PrivacyContact:     envString("REVIEWS_PRIVACY_CONTACT", ""),
			PrivacyURL:         envString("REVIEWS_PRIVACY_URL", ""),
			ReviewTermsURL:     envString("REVIEWS_REVIEW_TERMS_URL", ""),
		},
		Marketplaces: MarketplaceConfig{
			WB: WBConfig{
				Enabled: envBool("REVIEWS_WB_ENABLED", true),
				Token:   envFirst("REVIEWS_WB_TOKEN", "WB_API_TOKEN"),
			},
			YM: YMConfig{
				Enabled:    envBool("REVIEWS_YM_ENABLED", true),
				APIKey:     envFirst("REVIEWS_YM_API_KEY", "YM_API_KEY"),
				OAuthToken: envFirst("REVIEWS_YM_OAUTH_TOKEN", "YM_OAUTH_TOKEN"),
				BusinessID: envFirst("REVIEWS_YM_BUSINESS_ID", "YM_BUSINESS_ID"),
				CampaignID: envFirst("REVIEWS_YM_CAMPAIGN_ID", "YM_CAMPAIGN_ID"),
			},
			Ozon: OzonConfig{
				Enabled:  envBool("REVIEWS_OZON_ENABLED", false),
				ClientID: envFirst("REVIEWS_OZON_CLIENT_ID", "OZON_CLIENT_ID"),
				APIKey:   envFirst("REVIEWS_OZON_API_KEY", "OZON_API_KEY"),
			},
		},
	}

	cfg.Media = MediaConfig{
		Allowlist:    envList("REVIEWS_MEDIA_ALLOWLIST"),
		MaxBytes:     8 << 20,
		CacheEntries: 512,
	}
	if len(cfg.Media.Allowlist) == 0 {
		cfg.Media.Allowlist = []string{"wbbasket.ru", "images.wildberries.ru", "avatars.mds.yandex.net"}
	}

	cfg.Web.SitemapURL = envString("REVIEWS_SITE_SITEMAP_URL", "")
	if cfg.Web.SitemapURL == "" && len(cfg.Web.ShopOrigins) > 0 {
		cfg.Web.SitemapURL = strings.TrimRight(cfg.Web.ShopOrigins[0], "/") + "/sitemap.xml"
	}
	cfg.Web.UploadDir = envString("REVIEWS_UPLOAD_DIR", defaultUploadDir(cfg.DB.DSN))

	if value := os.Getenv("REVIEWS_SYNC_INTERVAL"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("REVIEWS_SYNC_INTERVAL: %w", err)
		}
		cfg.Sync.Interval = parsed
	}

	if value := os.Getenv("REVIEWS_SYNC_OVERLAP"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("REVIEWS_SYNC_OVERLAP: %w", err)
		}
		cfg.Sync.Overlap = parsed
	}

	if value := os.Getenv("REVIEWS_SYNC_BACKFILL_MONTHS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("REVIEWS_SYNC_BACKFILL_MONTHS: %w", err)
		}
		cfg.Sync.BackfillMonths = parsed
	}

	if err := cfg.ValidateBase(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func defaultUploadDir(dbDSN string) string {
	if strings.HasPrefix(dbDSN, "/data/") || dbDSN == "/data/reviews.db" {
		return "/data/review-uploads"
	}
	return "data/review-uploads"
}

func (cfg Config) ValidateBase() error {
	switch cfg.DB.Driver {
	case "sqlite", "postgres":
	default:
		return fmt.Errorf("unsupported DB driver %q", cfg.DB.Driver)
	}

	if cfg.DB.DSN == "" {
		return fmt.Errorf("database DSN is required")
	}
	if cfg.Sync.Interval <= 0 {
		return fmt.Errorf("sync interval must be positive")
	}
	if cfg.Sync.Overlap < 0 {
		return fmt.Errorf("sync overlap must not be negative")
	}
	if cfg.Sync.BackfillMonths < 0 {
		return fmt.Errorf("sync backfill months must not be negative")
	}

	return nil
}

func (cfg Config) ValidateMarketplaceCredentials(only string) error {
	check := func(marketplace string) bool {
		return only == "" || only == marketplace
	}

	if only != "" && !cfg.IsMarketplaceEnabled(only) {
		return fmt.Errorf("marketplace %s is disabled", only)
	}

	if cfg.Marketplaces.WB.Enabled && check(MarketplaceWB) && cfg.Marketplaces.WB.Token == "" {
		return fmt.Errorf("WB token is required when WB is enabled")
	}
	if cfg.Marketplaces.YM.Enabled && check(MarketplaceYM) {
		hasAuth := cfg.Marketplaces.YM.APIKey != "" || cfg.Marketplaces.YM.OAuthToken != ""
		if !hasAuth || cfg.Marketplaces.YM.BusinessID == "" {
			return fmt.Errorf("YM api key or oauth token, and business id are required when YM is enabled")
		}
	}
	if cfg.Marketplaces.Ozon.Enabled && check(MarketplaceOzon) && (cfg.Marketplaces.Ozon.ClientID == "" || cfg.Marketplaces.Ozon.APIKey == "") {
		return fmt.Errorf("Ozon client id and api key are required when Ozon is enabled")
	}

	return nil
}

func (cfg Config) IsMarketplaceEnabled(value string) bool {
	switch value {
	case MarketplaceWB:
		return cfg.Marketplaces.WB.Enabled
	case MarketplaceYM:
		return cfg.Marketplaces.YM.Enabled
	case MarketplaceOzon:
		return cfg.Marketplaces.Ozon.Enabled
	default:
		return false
	}
}

func (cfg Config) EnabledMarketplaces() []string {
	var marketplaces []string
	if cfg.Marketplaces.WB.Enabled {
		marketplaces = append(marketplaces, MarketplaceWB)
	}
	if cfg.Marketplaces.YM.Enabled {
		marketplaces = append(marketplaces, MarketplaceYM)
	}
	if cfg.Marketplaces.Ozon.Enabled {
		marketplaces = append(marketplaces, MarketplaceOzon)
	}
	return marketplaces
}

func IsKnownMarketplace(value string) bool {
	switch value {
	case MarketplaceWB, MarketplaceYM, MarketplaceOzon:
		return true
	default:
		return false
	}
}

func LoadDotEnv(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNo)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return fmt.Errorf("%s:%d: empty key", path, lineNo)
		}
		if unquoted, ok := strings.CutPrefix(value, `"`); ok {
			var closed bool
			unquoted, closed = strings.CutSuffix(unquoted, `"`)
			if !closed {
				return fmt.Errorf("%s:%d: unterminated quoted value", path, lineNo)
			}
			value = strings.ReplaceAll(unquoted, `\"`, `"`)
		} else if unquoted, ok := strings.CutPrefix(value, `'`); ok {
			var closed bool
			unquoted, closed = strings.CutSuffix(unquoted, `'`)
			if !closed {
				return fmt.Errorf("%s:%d: unterminated quoted value", path, lineNo)
			}
			value = unquoted
		}

		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("%s:%d: set %s: %w", path, lineNo, key, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	return nil
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

// envList parses a comma-separated env var into a slice, trimming whitespace
// and dropping empty entries. Returns nil when unset or empty.
func envList(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
