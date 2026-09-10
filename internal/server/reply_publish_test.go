package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"reviews/internal/marketplace"
	"reviews/internal/store"
)

var errNoPublisher = errors.New("no publisher")

func testTime() time.Time { return time.Unix(1700000000, 0).UTC() }

type fakePublisher struct {
	calls int
	err   error
	last  string
}

func (f *fakePublisher) PublishReply(_ context.Context, _, text string) error {
	f.calls++
	f.last = text
	return f.err
}

func TestPublishReplySuccessAndUnsupported(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	pub := &fakePublisher{}
	s.cfg.ResolveReplyPublisher = func(_ context.Context, _ string) (marketplace.ReplyPublisher, error) { return pub, nil }

	rating := 5
	res, _ := s.store.UpsertReview(ctx, marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-1", ExternalProductID: "p1",
		Rating: &rating, Text: "t", CreatedAtMP: testTime(),
	})
	reply := "Спасибо!"
	_ = s.store.SetReviewReply(ctx, res.Review.ID, &reply)
	rv, _ := s.store.ReviewByID(ctx, res.Review.ID)
	s.publishReply(ctx, rv)

	if pub.calls != 1 || pub.last != "Спасибо!" {
		t.Fatalf("publisher not called correctly: %+v", pub)
	}
	got, _ := s.store.ReviewByID(ctx, res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "published" {
		t.Fatalf("state = %v", got.ReplyPublishState)
	}

	// A site review is never published.
	res2, _ := s.store.CreateSiteReview(ctx, store.SiteReviewInput{
		ExternalReviewID: "site-x", SellerArticle: "a", Rating: 5,
		AuthorName: "A", AuthorEmail: "a@b.co", Text: "hi",
	})
	rv2, _ := s.store.ReviewByID(ctx, res2.ID)
	s.publishReply(ctx, rv2)
	got2, _ := s.store.ReviewByID(ctx, res2.ID)
	if got2.ReplyPublishState == nil || *got2.ReplyPublishState != "unsupported" {
		t.Fatalf("site state = %v", got2.ReplyPublishState)
	}
}

func TestPublishReplyFailureRecorded(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	s.cfg.ResolveReplyPublisher = func(_ context.Context, _ string) (marketplace.ReplyPublisher, error) {
		return &fakePublisher{err: errors.New("boom")}, nil
	}
	rating := 5
	res, _ := s.store.UpsertReview(ctx, marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-2", ExternalProductID: "p1",
		Rating: &rating, Text: "t", CreatedAtMP: testTime(),
	})
	reply := "x"
	_ = s.store.SetReviewReply(ctx, res.Review.ID, &reply)
	rv, _ := s.store.ReviewByID(ctx, res.Review.ID)
	s.publishReply(ctx, rv)
	got, _ := s.store.ReviewByID(ctx, res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "failed" || got.ReplyPublishError == nil {
		t.Fatalf("expected failed+error, got %+v", got)
	}
}

func TestReplyHandlerPublishesAndRetry(t *testing.T) {
	s := newAuthTestServer(t)
	pub := &fakePublisher{err: errors.New("boom")}
	s.cfg.ResolveReplyPublisher = func(_ context.Context, _ string) (marketplace.ReplyPublisher, error) { return pub, nil }
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	rating := 5
	res, _ := s.store.UpsertReview(context.Background(), marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-9", ExternalProductID: "p1",
		Rating: &rating, Text: "t", CreatedAtMP: testTime(),
	})

	put := func(path string, method string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(`{"text":"Спасибо!"}`))
		req.AddCookie(cookie)
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
		req.Header.Set(csrfHeaderName, csrf)
		rec := httptest.NewRecorder()
		s.adminMux().ServeHTTP(rec, req)
		return rec.Code
	}

	if code := put("/admin/api/reviews/"+strconv.FormatUint(uint64(res.Review.ID), 10)+"/reply", http.MethodPut); code != http.StatusOK {
		t.Fatalf("reply status %d", code)
	}
	got, _ := s.store.ReviewByID(context.Background(), res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "failed" {
		t.Fatalf("expected failed after publish attempt, got %v", got.ReplyPublishState)
	}

	pub.err = nil // marketplace recovers
	if code := put("/admin/api/reviews/"+strconv.FormatUint(uint64(res.Review.ID), 10)+"/reply/retry", http.MethodPost); code != http.StatusOK {
		t.Fatalf("retry status %d", code)
	}
	got, _ = s.store.ReviewByID(context.Background(), res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "published" {
		t.Fatalf("expected published after retry, got %v", got.ReplyPublishState)
	}
}

// TestRetryRejectsAlreadyPublished verifies that the retry endpoint returns
// HTTP 409 and does not call the publisher again when the reply is already
// in the "published" state (decision D3: publish-once).
func TestRetryRejectsAlreadyPublished(t *testing.T) {
	s := newAuthTestServer(t)
	pub := &fakePublisher{}
	s.cfg.ResolveReplyPublisher = func(_ context.Context, _ string) (marketplace.ReplyPublisher, error) { return pub, nil }
	cookie := loginTestAdmin(t, s)
	csrf := getCSRFToken(t, s, cookie)

	rating := 5
	res, _ := s.store.UpsertReview(context.Background(), marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-once", ExternalProductID: "p1",
		Rating: &rating, Text: "t", CreatedAtMP: testTime(),
	})
	reply := "Спасибо!"
	_ = s.store.SetReviewReply(context.Background(), res.Review.ID, &reply)
	rv, _ := s.store.ReviewByID(context.Background(), res.Review.ID)

	// First publish succeeds — state becomes "published", publisher called once.
	s.publishReply(context.Background(), rv)
	if pub.calls != 1 {
		t.Fatalf("expected 1 publisher call after first publish, got %d", pub.calls)
	}

	// Now retry via the HTTP endpoint — must get 409.
	req := httptest.NewRequest(http.MethodPost,
		"/admin/api/reviews/"+strconv.FormatUint(uint64(res.Review.ID), 10)+"/reply/retry",
		strings.NewReader(""))
	req.AddCookie(cookie)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rec := httptest.NewRecorder()
	s.adminMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", rec.Code)
	}
	if pub.calls != 1 {
		t.Fatalf("publisher call count should still be 1, got %d", pub.calls)
	}
}

// TestPublishReplyUnsupportedNoPublisher verifies that a review whose
// marketplace has no registered publisher is marked "unsupported".
func TestPublishReplyUnsupportedNoPublisher(t *testing.T) {
	s := newAuthTestServer(t)
	// Empty publisher map — ozon has no publisher.
	s.cfg.ResolveReplyPublisher = func(_ context.Context, _ string) (marketplace.ReplyPublisher, error) { return nil, errNoPublisher }
	ctx := context.Background()

	rating := 5
	res, _ := s.store.UpsertReview(ctx, marketplace.Review{
		Marketplace: "ozon", ExternalReviewID: "ozon-1", ExternalProductID: "p1",
		Rating: &rating, Text: "t", CreatedAtMP: testTime(),
	})
	reply := "Спасибо!"
	_ = s.store.SetReviewReply(ctx, res.Review.ID, &reply)
	rv, _ := s.store.ReviewByID(ctx, res.Review.ID)

	s.publishReply(ctx, rv)

	got, _ := s.store.ReviewByID(ctx, res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "unsupported" {
		t.Fatalf("expected unsupported (no publisher), got %v", got.ReplyPublishState)
	}
}

// TestPublishReplyUnsupportedToggleDisabled verifies that a review is marked
// "unsupported" when the marketplace has a publisher but the toggle is disabled
// via the app_setting (publish_replies_wb = "").
func TestPublishReplyUnsupportedToggleDisabled(t *testing.T) {
	s := newAuthTestServer(t)
	pub := &fakePublisher{}
	s.cfg.ResolveReplyPublisher = func(_ context.Context, _ string) (marketplace.ReplyPublisher, error) { return pub, nil }
	ctx := context.Background()

	// Disable the toggle for WB by setting it to anything other than "true".
	if err := s.store.SetAppSetting(ctx, store.PublishRepliesKey("wb"), "false"); err != nil {
		t.Fatalf("SetAppSetting: %v", err)
	}

	rating := 5
	res, _ := s.store.UpsertReview(ctx, marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-disabled", ExternalProductID: "p1",
		Rating: &rating, Text: "t", CreatedAtMP: testTime(),
	})
	reply := "Спасибо!"
	_ = s.store.SetReviewReply(ctx, res.Review.ID, &reply)
	rv, _ := s.store.ReviewByID(ctx, res.Review.ID)

	s.publishReply(ctx, rv)

	got, _ := s.store.ReviewByID(ctx, res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "unsupported" {
		t.Fatalf("expected unsupported (toggle disabled), got %v", got.ReplyPublishState)
	}
	if pub.calls != 0 {
		t.Fatalf("publisher should not be called when toggle disabled, got %d calls", pub.calls)
	}
}

// TestPublishReplyResolverFreshness proves two publications resolve the
// publisher afresh each time: resolver returns A first, B second.
func TestPublishReplyResolverFreshness(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	pubA, pubB := &fakePublisher{}, &fakePublisher{}
	calls := 0
	s.cfg.ResolveReplyPublisher = func(_ context.Context, _ string) (marketplace.ReplyPublisher, error) {
		calls++
		if calls == 1 {
			return pubA, nil
		}
		return pubB, nil
	}

	for _, ext := range []string{"wb-a", "wb-b"} {
		rating := 5
		res, _ := s.store.UpsertReview(ctx, marketplace.Review{
			Marketplace: "wb", ExternalReviewID: ext, ExternalProductID: "p1",
			Rating: &rating, Text: "t", CreatedAtMP: testTime(),
		})
		reply := "ok"
		_ = s.store.SetReviewReply(ctx, res.Review.ID, &reply)
		rv, _ := s.store.ReviewByID(ctx, res.Review.ID)
		s.publishReply(ctx, rv)
	}

	if pubA.calls != 1 || pubB.calls != 1 {
		t.Fatalf("expected A then B, got A=%d B=%d", pubA.calls, pubB.calls)
	}
}

// TestPublishReplyResolverErrorRecordsFailed verifies a resolver error (e.g.
// invalid credentials) persists state "failed" with the actionable error,
// not "unsupported", and never leaks the secret sentinel.
func TestPublishReplyResolverErrorRecordsFailed(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	s.cfg.ResolveReplyPublisher = func(_ context.Context, _ string) (marketplace.ReplyPublisher, error) {
		return nil, errors.New("нужен персональный WB API-токен")
	}
	rating := 5
	res, _ := s.store.UpsertReview(ctx, marketplace.Review{
		Marketplace: "wb", ExternalReviewID: "wb-err", ExternalProductID: "p1",
		Rating: &rating, Text: "t", CreatedAtMP: testTime(),
	})
	reply := "x"
	_ = s.store.SetReviewReply(ctx, res.Review.ID, &reply)
	rv, _ := s.store.ReviewByID(ctx, res.Review.ID)
	s.publishReply(ctx, rv)

	got, _ := s.store.ReviewByID(ctx, res.Review.ID)
	if got.ReplyPublishState == nil || *got.ReplyPublishState != "failed" {
		t.Fatalf("state = %v, want failed", got.ReplyPublishState)
	}
	if got.ReplyPublishError == nil || !strings.Contains(*got.ReplyPublishError, "персональный") {
		t.Fatalf("error = %v, want actionable message", got.ReplyPublishError)
	}
}
