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
	s.replyPublishers = map[string]marketplace.ReplyPublisher{"wb": pub}

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
	s.replyPublishers = map[string]marketplace.ReplyPublisher{"wb": &fakePublisher{err: errors.New("boom")}}
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
	s.replyPublishers = map[string]marketplace.ReplyPublisher{"wb": pub}
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
