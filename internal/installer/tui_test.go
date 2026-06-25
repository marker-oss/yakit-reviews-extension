package installer

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// consentModel returns a model parked on the consent page with a no-op runner.
func consentModel() model {
	m := newModel(context.Background(), TUIOptions{
		Run: func(context.Context, Config, Progress) error { return nil },
	})
	m.page = pageConsent
	return m
}

func TestConsentGateBlocksWithoutAcceptance(t *testing.T) {
	// Any non-accept navigation key must NOT advance to installation.
	for _, key := range []string{"n", "enter", "x", " "} {
		m := consentModel()
		next, _ := m.Update(runeKey(key))
		got := next.(model)
		if got.page == pageInstalling {
			t.Fatalf("key %q advanced consent gate to pageInstalling; installation must require explicit accept", key)
		}
		if got.page != pageConsent {
			t.Fatalf("key %q moved off consent page to %d; expected to stay on pageConsent", key, got.page)
		}
	}
}

func TestConsentGateAcceptsY(t *testing.T) {
	for _, key := range []string{"y", "Y"} {
		m := consentModel()
		next, _ := m.Update(runeKey(key))
		got := next.(model)
		if got.page != pageInstalling {
			t.Fatalf("accept key %q did not advance to pageInstalling; got page %d", key, got.page)
		}
	}
}

func TestReviewEnterRoutesThroughConsent(t *testing.T) {
	// Pressing Enter on the review page must land on the consent gate,
	// never directly on installation.
	m := newModel(context.Background(), TUIOptions{
		Run: func(context.Context, Config, Progress) error { return nil },
	})
	m.cfg.Server.Host = "203.0.113.10"
	m.cfg.Server.Password = "s3cret-password"
	m.cfg.Admin.Password = "admin-password"
	m.cfg.Admin.PasswordConfirm = "admin-password"
	if err := m.cfg.Validate(); err != nil {
		t.Fatalf("test config should be valid, got: %v", err)
	}
	m.page = pageReview
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(model)
	if got.page == pageInstalling {
		t.Fatal("review Enter skipped the consent gate and reached pageInstalling")
	}
	if got.page != pageConsent {
		t.Fatalf("review Enter landed on page %d; expected pageConsent", got.page)
	}
}
