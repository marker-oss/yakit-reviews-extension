package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reviews/internal/auth"
	"reviews/internal/config"
	"reviews/internal/store"
)

func TestAdminResetPasswordCommand(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reviews.db")
	t.Setenv("REVIEWS_DB_DRIVER", "sqlite")
	t.Setenv("REVIEWS_DB_DSN", dbPath)
	t.Setenv("REVIEWS_WB_ENABLED", "false")
	t.Setenv("REVIEWS_YM_ENABLED", "false")

	st, err := store.Open(config.DBConfig{Driver: "sqlite", DSN: dbPath})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	oldHash, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatalf("hash old: %v", err)
	}
	user, err := st.CreateAdminUser(context.Background(), "admin", oldHash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.CreateSession(context.Background(), "tok-1", user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	code := run([]string{"admin", "reset-password", "--login", "admin", "--password", "new-password"})
	if code != exitOK {
		t.Fatalf("exit code = %d", code)
	}

	got, err := st.GetAdminUserByLogin(context.Background(), "admin")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	ok, err := auth.VerifyPassword(got.PasswordHash, "new-password")
	if err != nil || !ok {
		t.Fatalf("new password not accepted ok=%v err=%v", ok, err)
	}
	if _, err := st.GetValidSession(context.Background(), "tok-1", time.Now()); err == nil {
		t.Fatal("expected reset to delete active sessions")
	}
}
