package store

import (
	"context"
	"testing"
)

func TestAppSettingGetSet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Missing key returns empty string without error.
	got, err := s.GetAppSetting(ctx, SettingAgreementURL)
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if got != "" {
		t.Fatalf("missing key = %q, want empty", got)
	}

	// First write stores the value (trimmed).
	if err := s.SetAppSetting(ctx, SettingAgreementURL, "  https://shop.example/agreement  "); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err = s.GetAppSetting(ctx, SettingAgreementURL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "https://shop.example/agreement" {
		t.Fatalf("value = %q, want trimmed URL", got)
	}

	// Second write upserts the same row rather than failing on the unique index.
	if err := s.SetAppSetting(ctx, SettingAgreementURL, "https://shop.example/v2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err = s.GetAppSetting(ctx, SettingAgreementURL)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got != "https://shop.example/v2" {
		t.Fatalf("updated value = %q", got)
	}
}
