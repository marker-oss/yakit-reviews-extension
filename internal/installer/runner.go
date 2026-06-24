package installer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

type Executor interface {
	Run(ctx context.Context, command string, sudo bool) (string, error)
	WriteFile(ctx context.Context, path, content string, mode string, sudo bool) error
}

type Resolver interface {
	LookupIP(ctx context.Context, host string) ([]net.IP, error)
}

type NetResolver struct{}

func (NetResolver) LookupIP(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

type AdminClient interface {
	WaitHealth(ctx context.Context, baseURL string) error
	SetupAdmin(ctx context.Context, login, password string) error
	Login(ctx context.Context, login, password string) error
	SaveMarketplaceCredentials(ctx context.Context, request CredentialRequest) error
	RefreshSiteLinks(ctx context.Context) error
	PublishReviews(ctx context.Context) error
}

type StepStatus string

const (
	StepStarted StepStatus = "started"
	StepOK      StepStatus = "ok"
	StepFailed  StepStatus = "failed"
)

type StepEvent struct {
	Name    string
	Status  StepStatus
	Message string
}

type Progress func(StepEvent)

func Run(ctx context.Context, cfg Config, exec Executor, resolver Resolver, admin AdminClient, progress Progress) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if resolver == nil {
		resolver = NetResolver{}
	}
	if progress == nil {
		progress = func(StepEvent) {}
	}
	secrets := SecretValues(cfg)
	step := func(name string, fn func() error) error {
		progress(StepEvent{Name: name, Status: StepStarted})
		if err := fn(); err != nil {
			progress(StepEvent{Name: name, Status: StepFailed, Message: MaskSecrets(err.Error(), secrets)})
			return fmt.Errorf("%s: %w", name, err)
		}
		progress(StepEvent{Name: name, Status: StepOK})
		return nil
	}

	if err := step("Validate DNS", func() error {
		return ValidateDNS(ctx, resolver, cfg.Domains.ReviewsDomain, cfg.Server.Host)
	}); err != nil {
		return err
	}
	if err := step("Verify SSH and OS", func() error {
		out, err := exec.Run(ctx, "cat /etc/os-release", false)
		if err != nil {
			return err
		}
		return ValidateOSRelease(out)
	}); err != nil {
		return err
	}
	if err := step("Check server ports", func() error {
		out, err := exec.Run(ctx, portCheckCommand(), true)
		if err != nil {
			return err
		}
		return ValidatePortCheckOutput(out)
	}); err != nil {
		return err
	}
	if err := step("Install Docker and Caddy", func() error {
		_, err := exec.Run(ctx, installPackagesCommand(), true)
		return err
	}); err != nil {
		return err
	}
	if err := step("Prepare source checkout", func() error {
		_, err := exec.Run(ctx, sourceCheckoutCommand(cfg), true)
		return err
	}); err != nil {
		return err
	}
	if err := step("Write server configuration", func() error {
		if err := exec.WriteFile(ctx, cfg.Deploy.SourceDir+"/.env", RenderEnv(cfg), "0600", true); err != nil {
			return err
		}
		if err := exec.WriteFile(ctx, cfg.Deploy.SourceDir+"/docker-compose.override.yml", RenderComposeOverride(cfg), "0644", true); err != nil {
			return err
		}
		if err := exec.WriteFile(ctx, "/etc/caddy/Caddyfile", RenderCaddyfile(cfg), "0644", true); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if err := step("Start Reviews", func() error {
		_, err := exec.Run(ctx, "cd "+shellQuote(cfg.Deploy.SourceDir)+" && docker compose pull && docker compose up -d", true)
		return err
	}); err != nil {
		return err
	}
	if err := step("Reload Caddy", func() error {
		_, err := exec.Run(ctx, "systemctl reload caddy || systemctl restart caddy", true)
		return err
	}); err != nil {
		return err
	}
	if err := step("Wait for healthcheck", func() error {
		waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		return admin.WaitHealth(waitCtx, cfg.BaseURL())
	}); err != nil {
		return err
	}
	if err := step("Create admin", func() error {
		return admin.SetupAdmin(ctx, cfg.Admin.Login, cfg.Admin.Password)
	}); err != nil {
		return err
	}
	if err := step("Save marketplace credentials", func() error {
		if err := admin.Login(ctx, cfg.Admin.Login, cfg.Admin.Password); err != nil {
			return err
		}
		for _, request := range MarketplaceCredentialRequests(cfg.Marketplaces) {
			if err := admin.SaveMarketplaceCredentials(ctx, request); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := step("Run first sync", func() error {
		_, err := exec.Run(ctx, "cd "+shellQuote(cfg.Deploy.SourceDir)+" && docker compose exec -T reviews sync --once", true)
		return err
	}); err != nil {
		return err
	}
	if err := step("Refresh catalog", func() error {
		return admin.RefreshSiteLinks(ctx)
	}); err != nil {
		return err
	}
	if err := step("Publish reviews data", func() error {
		return admin.PublishReviews(ctx)
	}); err != nil {
		return err
	}
	return nil
}

func ValidateDNS(ctx context.Context, resolver Resolver, domain, expectedHost string) error {
	ips, err := resolver.LookupIP(ctx, domain)
	if err != nil {
		return err
	}
	expected := net.ParseIP(expectedHost)
	if expected == nil {
		hostIPs, err := resolver.LookupIP(ctx, expectedHost)
		if err != nil {
			return fmt.Errorf("resolve server host %q: %w", expectedHost, err)
		}
		for _, ip := range hostIPs {
			if containsIP(ips, ip) {
				return nil
			}
		}
		return fmt.Errorf("%s does not resolve to server host %s", domain, expectedHost)
	}
	if containsIP(ips, expected) {
		return nil
	}
	return fmt.Errorf("%s does not resolve to %s (got %s)", domain, expected, ipList(ips))
}

func containsIP(ips []net.IP, expected net.IP) bool {
	for _, ip := range ips {
		if ip.Equal(expected) {
			return true
		}
	}
	return false
}

func ipList(ips []net.IP) string {
	values := make([]string, 0, len(ips))
	for _, ip := range ips {
		values = append(values, ip.String())
	}
	return strings.Join(values, ", ")
}

func ValidateOSRelease(body string) error {
	lower := strings.ToLower(body)
	if strings.Contains(lower, "id=ubuntu") || strings.Contains(lower, "id=debian") ||
		strings.Contains(lower, "id_like=debian") || strings.Contains(lower, "id_like=\"debian\"") {
		return nil
	}
	return errors.New("only Ubuntu/Debian servers are supported in installer v1")
}

func ValidatePortCheckOutput(out string) error {
	out = strings.TrimSpace(out)
	if out == "" || out == "OK" {
		return nil
	}
	return fmt.Errorf("ports 80/443/8080 appear occupied:\n%s", out)
}

func portCheckCommand() string {
	return strings.TrimSpace(`
if command -v ss >/dev/null 2>&1; then
  busy="$(ss -ltnp 2>/dev/null | awk '$4 ~ /:(80|443|8080)$/ && $0 !~ /caddy|docker-proxy/ {print}')"
  if [ -n "$busy" ]; then printf '%s\n' "$busy"; exit 0; fi
fi
printf 'OK\n'
`)
}

func installPackagesCommand() string {
	return strings.TrimSpace(`
set -eu
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl git gnupg lsb-release
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh
fi
if ! docker compose version >/dev/null 2>&1; then
  apt-get install -y docker-compose-plugin
fi
if ! command -v caddy >/dev/null 2>&1; then
  apt-get install -y debian-keyring debian-archive-keyring apt-transport-https
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' > /etc/apt/sources.list.d/caddy-stable.list
  apt-get update
  apt-get install -y caddy
fi
systemctl enable --now docker
systemctl enable --now caddy
`)
}

func sourceCheckoutCommand(cfg Config) string {
	src := shellQuote(cfg.Deploy.SourceDir)
	repo := shellQuote(cfg.Deploy.RepoURL)
	ref := shellQuote(cfg.Deploy.DeployRef)
	return fmt.Sprintf(strings.TrimSpace(`
set -eu
if [ -d %[1]s/.git ]; then
  git -C %[1]s fetch --prune origin
else
  rm -rf %[1]s
  git clone %[2]s %[1]s
fi
git -C %[1]s checkout -B %[3]s origin/%[3]s
git -C %[1]s reset --hard origin/%[3]s
`), src, repo, ref)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
