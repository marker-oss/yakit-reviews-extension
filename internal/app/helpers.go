package app

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"reviews/internal/config"
	"reviews/internal/marketplace/wbtoken"
	"reviews/internal/server"
	"reviews/internal/site"
	"reviews/internal/store"
)

// latestReleaseURL is the feed the admin update-banner checks once a day.
const latestReleaseURL = "https://api.github.com/repos/marker-oss/yakit-reviews-extension/releases/latest"

func MarketplaceStatuses(cfg config.Config) []server.MarketplaceStatus {
	wbConfigured := cfg.Marketplaces.WB.Token != ""
	var wbWarning string
	if wbConfigured {
		if err := wbtoken.ValidatePersonalTokenMetadata(cfg.Marketplaces.WB.Token, time.Now()); err != nil {
			wbConfigured = false
			wbWarning = err.Error()
		}
	}
	return []server.MarketplaceStatus{
		{
			ID:         config.MarketplaceWB,
			Enabled:    cfg.Marketplaces.WB.Enabled,
			Configured: wbConfigured,
			Fields: map[string]bool{
				"token": cfg.Marketplaces.WB.Token != "",
			},
			Warning: wbWarning,
		},
		{
			ID:      config.MarketplaceYM,
			Enabled: cfg.Marketplaces.YM.Enabled,
			Configured: cfg.Marketplaces.YM.BusinessID != "" &&
				(cfg.Marketplaces.YM.APIKey != "" || cfg.Marketplaces.YM.OAuthToken != ""),
			Fields: map[string]bool{
				"api_key":     cfg.Marketplaces.YM.APIKey != "",
				"oauth_token": cfg.Marketplaces.YM.OAuthToken != "",
				"business_id": cfg.Marketplaces.YM.BusinessID != "",
				"campaign_id": cfg.Marketplaces.YM.CampaignID != "",
			},
		},
		{
			ID:         config.MarketplaceOzon,
			Enabled:    cfg.Marketplaces.Ozon.Enabled,
			Configured: cfg.Marketplaces.Ozon.ClientID != "" && cfg.Marketplaces.Ozon.APIKey != "",
			Fields: map[string]bool{
				"client_id": cfg.Marketplaces.Ozon.ClientID != "",
				"api_key":   cfg.Marketplaces.Ozon.APIKey != "",
			},
		},
	}
}

func applyStoredMarketplaceCredentials(ctx context.Context, db *store.Store, cfg config.Config, logger *slog.Logger) config.Config {
	creds, err := db.ListMarketplaceCredentials(ctx)
	if err != nil {
		logger.Warn("load marketplace credentials", "error", err)
		return cfg
	}
	for _, cred := range creds {
		values := cred.PayloadMap()
		switch cred.Marketplace {
		case config.MarketplaceWB:
			cfg.Marketplaces.WB.Enabled = cred.Enabled
			if values["token"] != "" {
				cfg.Marketplaces.WB.Token = values["token"]
			}
		case config.MarketplaceYM:
			cfg.Marketplaces.YM.Enabled = cred.Enabled
			if values["api_key"] != "" {
				cfg.Marketplaces.YM.APIKey = values["api_key"]
			}
			if values["oauth_token"] != "" {
				cfg.Marketplaces.YM.OAuthToken = values["oauth_token"]
			}
			if values["business_id"] != "" {
				cfg.Marketplaces.YM.BusinessID = values["business_id"]
			}
			if values["campaign_id"] != "" {
				cfg.Marketplaces.YM.CampaignID = values["campaign_id"]
			}
		case config.MarketplaceOzon:
			cfg.Marketplaces.Ozon.Enabled = cred.Enabled
			if values["client_id"] != "" {
				cfg.Marketplaces.Ozon.ClientID = values["client_id"]
			}
			if values["api_key"] != "" {
				cfg.Marketplaces.Ozon.APIKey = values["api_key"]
			}
		}
	}
	return cfg
}

func LoadProductLinks(path string, logger *slog.Logger) map[string]string {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		logger.Warn("open product links", "path", path, "error", err)
		return nil
	}
	defer file.Close()
	links, err := site.LoadProductLinkMap(file)
	if err != nil {
		logger.Warn("load product links", "path", path, "error", err)
		return nil
	}
	logger.Info("product links loaded", "path", path, "count", len(links))
	return links
}

func EnvDuration(key string, fallback time.Duration, logger *slog.Logger) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if value == "0" || strings.EqualFold(value, "off") || strings.EqualFold(value, "false") {
		return 0
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		logger.Warn("invalid duration, using default", "env", key, "value", value, "default", fallback.String())
		return fallback
	}
	return parsed
}

func UpdateCheckURL() string {
	switch value := os.Getenv("REVIEWS_UPDATE_CHECK"); value {
	case "":
		return latestReleaseURL
	case "false", "0", "off":
		return ""
	default:
		return value
	}
}

func NewLogger(cfg config.LogConfig) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}
	if strings.EqualFold(cfg.Format, "json") {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
