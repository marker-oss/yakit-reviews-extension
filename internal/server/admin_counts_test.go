package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCounts(t *testing.T) {
	s := newAuthTestServer(t)
	cookie := loginTestAdmin(t, s)
	id := seedAdminReview(t, s, "w1", 5)
	if err := s.store.SetReviewStatus(context.Background(), id, "pending"); err != nil {
		t.Fatalf("set pending: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/api/counts", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		PendingReviews   int64 `json:"pendingReviews"`
		PendingQuestions int   `json:"pendingQuestions"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.PendingReviews != 1 {
		t.Fatalf("pendingReviews = %d, want 1", got.PendingReviews)
	}
	if got.PendingQuestions != 0 { // none seeded
		t.Fatalf("pendingQuestions = %d, want 0", got.PendingQuestions)
	}
}
