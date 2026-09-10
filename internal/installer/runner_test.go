package installer

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

type fakeResolver map[string][]net.IP

func (r fakeResolver) LookupIP(_ context.Context, host string) ([]net.IP, error) {
	if ips, ok := r[host]; ok {
		return ips, nil
	}
	return nil, errors.New("not found")
}

type commandCall struct {
	Command string
	Sudo    bool
}

type fakeExecutor struct {
	calls  []commandCall
	writes map[string]string
	osBody string
	port   string
}

func (e *fakeExecutor) Run(_ context.Context, command string, sudo bool) (string, error) {
	e.calls = append(e.calls, commandCall{Command: command, Sudo: sudo})
	switch {
	case strings.Contains(command, "/etc/os-release"):
		return e.osBody, nil
	case strings.Contains(command, "ss -ltnp"):
		if e.port != "" {
			return e.port, nil
		}
		return "OK\n", nil
	default:
		return "", nil
	}
}

func (e *fakeExecutor) WriteFile(_ context.Context, path, content string, _ string, sudo bool) error {
	e.calls = append(e.calls, commandCall{Command: "write " + path, Sudo: sudo})
	if e.writes == nil {
		e.writes = map[string]string{}
	}
	e.writes[path] = content
	return nil
}

type fakeAdmin struct {
	calls []string
}

func (a *fakeAdmin) WaitHealth(context.Context, string) error {
	a.calls = append(a.calls, "health")
	return nil
}
func (a *fakeAdmin) SetupAdmin(context.Context, string, string) error {
	a.calls = append(a.calls, "setup")
	return nil
}
func (a *fakeAdmin) Login(context.Context, string, string) error {
	a.calls = append(a.calls, "login")
	return nil
}
func (a *fakeAdmin) SaveMarketplaceCredentials(_ context.Context, req CredentialRequest) error {
	a.calls = append(a.calls, "creds:"+req.ID)
	return nil
}
func (a *fakeAdmin) RefreshSiteLinks(context.Context) error {
	a.calls = append(a.calls, "refresh")
	return nil
}
func (a *fakeAdmin) PublishReviews(context.Context) error {
	a.calls = append(a.calls, "publish")
	return nil
}

func TestRunHappyPath(t *testing.T) {
	cfg := validConfig()
	exec := &fakeExecutor{osBody: "ID=ubuntu\n"}
	admin := &fakeAdmin{}
	var events []StepEvent
	err := Run(context.Background(), cfg, exec, fakeResolver{
		"reviews.example.com": {net.ParseIP("203.0.113.10")},
	}, admin, func(event StepEvent) { events = append(events, event) })
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected progress events")
	}
	if !containsCommand(exec.calls, "docker compose pull && docker compose up -d") {
		t.Fatalf("missing compose pull/up call: %+v", exec.calls)
	}
	if !containsCommand(exec.calls, "docker compose exec -T reviews sync --once") {
		t.Fatalf("missing deterministic sync call: %+v", exec.calls)
	}
	if got := exec.writes[cfg.Deploy.SourceDir+"/.env"]; !strings.Contains(got, "REVIEWS_WB_ENABLED=false") || strings.Contains(got, "wb-token") {
		t.Fatalf("unexpected env write:\n%s", got)
	}
	if got := strings.Join(admin.calls, ","); got != "health,setup,login,creds:wb,creds:ym,creds:ozon,refresh,publish" {
		t.Fatalf("admin calls = %s", got)
	}
}

func TestRunEnablesAutoUpdates(t *testing.T) {
	cfg := validConfig()
	cfg.Deploy.AutoUpdate = true
	exec := &fakeExecutor{osBody: "ID=ubuntu\n"}
	err := Run(context.Background(), cfg, exec, fakeResolver{
		"reviews.example.com": {net.ParseIP("203.0.113.10")},
	}, &fakeAdmin{}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	script := exec.writes["/usr/local/bin/reviews-auto-update"]
	if !strings.Contains(script, "docker compose pull") || !strings.Contains(script, "rolling back") {
		t.Fatalf("auto-update script missing pull/rollback logic:\n%s", script)
	}
	service := exec.writes["/etc/systemd/system/reviews-update.service"]
	if !strings.Contains(service, "REVIEWS_COMPOSE_DIR="+cfg.Deploy.SourceDir) {
		t.Fatalf("service unit must point at the compose dir:\n%s", service)
	}
	timer := exec.writes["/etc/systemd/system/reviews-update.timer"]
	if !strings.Contains(timer, "OnCalendar") || !strings.Contains(timer, "RandomizedDelaySec") {
		t.Fatalf("timer unit missing schedule/jitter:\n%s", timer)
	}
	if !containsCommand(exec.calls, "systemctl enable --now reviews-update.timer") {
		t.Fatalf("timer was not enabled: %+v", exec.calls)
	}
}

func TestRunSkipsAutoUpdatesWhenDisabled(t *testing.T) {
	cfg := validConfig()
	cfg.Deploy.AutoUpdate = false
	exec := &fakeExecutor{osBody: "ID=ubuntu\n"}
	err := Run(context.Background(), cfg, exec, fakeResolver{
		"reviews.example.com": {net.ParseIP("203.0.113.10")},
	}, &fakeAdmin{}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, ok := exec.writes["/etc/systemd/system/reviews-update.timer"]; ok {
		t.Fatalf("timer written despite AutoUpdate=false")
	}
	if containsCommand(exec.calls, "reviews-update.timer") {
		t.Fatalf("timer enabled despite AutoUpdate=false: %+v", exec.calls)
	}
}

func TestRunFailsUnsupportedOS(t *testing.T) {
	cfg := validConfig()
	exec := &fakeExecutor{osBody: "ID=fedora\n"}
	err := Run(context.Background(), cfg, exec, fakeResolver{
		"reviews.example.com": {net.ParseIP("203.0.113.10")},
	}, &fakeAdmin{}, nil)
	if err == nil || !strings.Contains(err.Error(), "Ubuntu/Debian") {
		t.Fatalf("unsupported OS error = %v", err)
	}
}

func TestRunFailsOccupiedPorts(t *testing.T) {
	cfg := validConfig()
	exec := &fakeExecutor{osBody: "ID=debian\n", port: "LISTEN 0 4096 0.0.0.0:80 users:(\"nginx\")"}
	err := Run(context.Background(), cfg, exec, fakeResolver{
		"reviews.example.com": {net.ParseIP("203.0.113.10")},
	}, &fakeAdmin{}, nil)
	if err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("occupied port error = %v", err)
	}
}

func TestRunFailsDNSMismatch(t *testing.T) {
	cfg := validConfig()
	err := Run(context.Background(), cfg, &fakeExecutor{osBody: "ID=ubuntu\n"}, fakeResolver{
		"reviews.example.com": {net.ParseIP("203.0.113.11")},
	}, &fakeAdmin{}, nil)
	if err == nil || !strings.Contains(err.Error(), "does not resolve") {
		t.Fatalf("dns mismatch error = %v", err)
	}
}

func containsCommand(calls []commandCall, needle string) bool {
	for _, call := range calls {
		if strings.Contains(call.Command, needle) {
			return true
		}
	}
	return false
}
