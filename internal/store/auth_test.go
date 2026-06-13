package store

import (
	"context"
	"testing"
	"time"
)

func TestAdminUserLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if n, err := st.CountAdminUsers(ctx); err != nil || n != 0 {
		t.Fatalf("expected 0 users, got %d err=%v", n, err)
	}

	user, err := st.CreateAdminUser(ctx, "admin", "hash-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if user.ID == 0 || user.TenantID != DefaultTenantID {
		t.Fatalf("unexpected user %+v", user)
	}

	got, err := st.GetAdminUserByLogin(ctx, "admin")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PasswordHash != "hash-1" {
		t.Fatalf("unexpected hash %q", got.PasswordHash)
	}

	if err := st.UpdateAdminPassword(ctx, user.ID, "hash-2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err = st.GetAdminUserByLogin(ctx, "admin")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.PasswordHash != "hash-2" {
		t.Fatalf("password not updated: %q", got.PasswordHash)
	}

	if n, err := st.CountAdminUsers(ctx); err != nil || n != 1 {
		t.Fatalf("expected 1 user, got %d err=%v", n, err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	user, err := st.CreateAdminUser(ctx, "admin", "h")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	expires := time.Now().Add(time.Hour)
	if err := st.CreateSession(ctx, "tok-1", user.ID, expires); err != nil {
		t.Fatalf("create session: %v", err)
	}

	sess, err := st.GetValidSession(ctx, "tok-1", time.Now())
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.UserID != user.ID {
		t.Fatalf("unexpected session user %d", sess.UserID)
	}

	if _, err := st.GetValidSession(ctx, "tok-1", time.Now().Add(2*time.Hour)); err == nil {
		t.Fatal("expected expired session to be rejected")
	}

	if err := st.DeleteSession(ctx, "tok-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetValidSession(ctx, "tok-1", time.Now()); err == nil {
		t.Fatal("expected deleted session to be gone")
	}
}

func TestDeleteSessionsByUser(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	user, err := st.CreateAdminUser(ctx, "admin", "h")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	other, err := st.CreateAdminUser(ctx, "other", "h")
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	expires := time.Now().Add(time.Hour)
	if err := st.CreateSession(ctx, "tok-1", user.ID, expires); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := st.CreateSession(ctx, "tok-2", other.ID, expires); err != nil {
		t.Fatalf("create other session: %v", err)
	}
	if err := st.DeleteSessionsByUser(ctx, user.ID); err != nil {
		t.Fatalf("delete user sessions: %v", err)
	}
	if _, err := st.GetValidSession(ctx, "tok-1", time.Now()); err == nil {
		t.Fatal("expected user session deleted")
	}
	if _, err := st.GetValidSession(ctx, "tok-2", time.Now()); err != nil {
		t.Fatalf("expected other session to remain: %v", err)
	}
}
