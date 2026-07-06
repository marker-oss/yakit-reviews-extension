package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reviews/internal/store"
)

// levelOf returns the Level for a check ID, or "" if absent.
func levelOf(items []DiagItem, id string) string {
	for _, c := range items {
		if c.ID == id {
			return c.Level
		}
	}
	return ""
}

func getDiagnostics(t *testing.T, s *Server, cookie *http.Cookie) []DiagItem {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/diagnostics", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Checks []DiagItem `json:"checks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got.Checks
}

func TestDiagnosticsFlagsMissingShopOrigin(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	if got := levelOf(getDiagnostics(t, s, cookie), "cors"); got != "fail" {
		t.Fatalf("cors level = %q, want fail when origin unset", got)
	}
}

func TestDiagnosticsCorsOkWhenOriginSet(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	if err := s.store.SetAppSetting(context.Background(), store.SettingShopOrigin, "https://shop.example"); err != nil {
		t.Fatal(err)
	}
	if got := levelOf(getDiagnostics(t, s, cookie), "cors"); got != "ok" {
		t.Fatalf("cors level = %q, want ok", got)
	}
}
