package installer

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const (
	DefaultSSHPort       = 22
	DefaultRepoURL       = "https://github.com/marker-oss/yakit-reviews-extension.git"
	DefaultDeployRef     = "main"
	DefaultSourceDir     = "/srv/reviews-src"
	DefaultReviewsDomain = "reviews.myshop.example"
	// DefaultImage is the prebuilt multi-arch image pulled on the VPS, so the
	// server is not compiled from source during installation.
	DefaultImage = "ghcr.io/marker-oss/yakit-reviews-extension:latest"
)

type SSHAuthMethod string

const (
	SSHAuthPassword SSHAuthMethod = "password"
	SSHAuthKey      SSHAuthMethod = "key"
)

type Config struct {
	Server       ServerConfig
	Domains      DomainConfig
	Admin        AdminConfig
	Marketplaces MarketplaceConfig
	Deploy       DeployConfig
}

type ServerConfig struct {
	Host          string
	Port          int
	User          string
	AuthMethod    SSHAuthMethod
	Password      string
	KeyPath       string
	KeyPassphrase string
	SudoPassword  string
}

type DomainConfig struct {
	ReviewsDomain string
	ShopOrigin    string
	SitemapURL    string
}

type AdminConfig struct {
	Login           string
	Password        string
	PasswordConfirm string
}

type MarketplaceConfig struct {
	WB   WBMarketplace
	YM   YMMarketplace
	Ozon OzonMarketplace
}

type WBMarketplace struct {
	Enabled bool
	Token   string
}

type YMMarketplace struct {
	Enabled    bool
	APIKey     string
	OAuthToken string
	BusinessID string
	CampaignID string
}

type OzonMarketplace struct {
	Enabled  bool
	ClientID string
	APIKey   string
}

type DeployConfig struct {
	RepoURL   string
	DeployRef string
	SourceDir string
	Image     string
}

func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Port:       DefaultSSHPort,
			User:       "root",
			AuthMethod: SSHAuthPassword,
		},
		Domains: DomainConfig{
			ReviewsDomain: DefaultReviewsDomain,
			ShopOrigin:    "https://myshop.example",
		},
		Admin: AdminConfig{
			Login: "admin",
		},
		Deploy: DeployConfig{
			RepoURL:   DefaultRepoURL,
			DeployRef: DefaultDeployRef,
			SourceDir: DefaultSourceDir,
			Image:     DefaultImage,
		},
	}
}

func (cfg Config) Validate() error {
	var problems []string
	if strings.TrimSpace(cfg.Server.Host) == "" {
		problems = append(problems, "server host/IP is required")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		problems = append(problems, "SSH port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.Server.User) == "" {
		problems = append(problems, "SSH user is required")
	}
	switch cfg.Server.AuthMethod {
	case SSHAuthPassword:
		if cfg.Server.Password == "" {
			problems = append(problems, "SSH password is required")
		}
	case SSHAuthKey:
		if strings.TrimSpace(cfg.Server.KeyPath) == "" {
			problems = append(problems, "SSH private key path is required")
		}
	default:
		problems = append(problems, "SSH auth method must be password or key")
	}
	if err := validateDomain(cfg.Domains.ReviewsDomain); err != nil {
		problems = append(problems, "reviews domain: "+err.Error())
	}
	if err := validateOrigin(cfg.Domains.ShopOrigin); err != nil {
		problems = append(problems, "shop origin: "+err.Error())
	}
	if cfg.Domains.SitemapURL != "" {
		if err := validateHTTPSURL(cfg.Domains.SitemapURL); err != nil {
			problems = append(problems, "sitemap URL: "+err.Error())
		}
	}
	if strings.TrimSpace(cfg.Admin.Login) == "" {
		problems = append(problems, "admin login is required")
	}
	if len(cfg.Admin.Password) < 8 {
		problems = append(problems, "admin password must be at least 8 characters")
	}
	if cfg.Admin.Password != cfg.Admin.PasswordConfirm {
		problems = append(problems, "admin password confirmation does not match")
	}
	if strings.TrimSpace(cfg.Deploy.RepoURL) == "" {
		problems = append(problems, "repo URL is required")
	}
	if strings.TrimSpace(cfg.Deploy.DeployRef) == "" {
		problems = append(problems, "deploy ref is required")
	}
	if strings.TrimSpace(cfg.Deploy.SourceDir) == "" || !strings.HasPrefix(cfg.Deploy.SourceDir, "/") {
		problems = append(problems, "source dir must be an absolute path")
	}
	if strings.TrimSpace(cfg.Deploy.Image) == "" {
		problems = append(problems, "deploy image is required")
	}
	problems = append(problems, validateMarketplaceConfig(cfg.Marketplaces)...)
	if len(problems) > 0 {
		errs := make([]error, 0, len(problems))
		for _, problem := range problems {
			errs = append(errs, errors.New(problem))
		}
		return errors.Join(errs...)
	}
	return nil
}

func validateMarketplaceConfig(cfg MarketplaceConfig) []string {
	var problems []string
	if cfg.WB.Enabled && cfg.WB.Token == "" {
		problems = append(problems, "WB token is required when WB is enabled")
	}
	if cfg.YM.Enabled {
		if cfg.YM.BusinessID == "" {
			problems = append(problems, "YM business ID is required when YM is enabled")
		}
		if cfg.YM.APIKey == "" && cfg.YM.OAuthToken == "" {
			problems = append(problems, "YM API key or OAuth token is required when YM is enabled")
		}
	}
	if cfg.Ozon.Enabled {
		if cfg.Ozon.ClientID == "" {
			problems = append(problems, "Ozon client ID is required when Ozon is enabled")
		}
		if cfg.Ozon.APIKey == "" {
			problems = append(problems, "Ozon API key is required when Ozon is enabled")
		}
	}
	return problems
}

func validateDomain(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("is required")
	}
	if strings.Contains(value, "://") || strings.Contains(value, "/") {
		return errors.New("must be a hostname without scheme or path")
	}
	if ip := net.ParseIP(value); ip != nil {
		return errors.New("must be a domain name, not an IP address")
	}
	if len(value) > 253 {
		return errors.New("is too long")
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" {
			return errors.New("contains an empty DNS label")
		}
		if len(label) > 63 {
			return errors.New("contains a DNS label longer than 63 characters")
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return errors.New("DNS labels must not start or end with '-'")
		}
	}
	return nil
}

func validateOrigin(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("is required")
	}
	u, err := url.Parse(value)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("must start with http:// or https://")
	}
	if u.Host == "" {
		return errors.New("must include a host")
	}
	if u.Path != "" && u.Path != "/" {
		return errors.New("must not include a path")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("must not include query or fragment")
	}
	return nil
}

func validateHTTPSURL(value string) error {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("must start with http:// or https://")
	}
	if u.Host == "" {
		return errors.New("must include a host")
	}
	return nil
}

func (cfg Config) BaseURL() string {
	return "https://" + strings.TrimSpace(cfg.Domains.ReviewsDomain)
}

func (cfg Config) AdminURL() string {
	return cfg.BaseURL() + "/admin"
}

func (cfg Config) HealthURL() string {
	return cfg.BaseURL() + "/healthz"
}

func (cfg Config) WidgetBaseURL() string {
	return cfg.BaseURL()
}

func (cfg Config) SSHAddress() string {
	return net.JoinHostPort(strings.TrimSpace(cfg.Server.Host), strconv.Itoa(cfg.Server.Port))
}

func (cfg Config) EffectiveSudoPassword() string {
	if cfg.Server.SudoPassword != "" {
		return cfg.Server.SudoPassword
	}
	return cfg.Server.Password
}

func (cfg Config) IsRootUser() bool {
	return strings.TrimSpace(cfg.Server.User) == "root"
}
