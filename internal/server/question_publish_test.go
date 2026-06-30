package server

import (
	"context"
	"errors"
	"testing"

	"reviews/internal/marketplace"
	"reviews/internal/store"
)

type fakeQuestionPublisher struct {
	calls    int
	err      error
	lastID   string
	lastSKU  string
	lastText string
}

func (f *fakeQuestionPublisher) PublishQuestionAnswer(_ context.Context, externalQuestionID, sku, text string) error {
	f.calls++
	f.lastID = externalQuestionID
	f.lastSKU = sku
	f.lastText = text
	return f.err
}

func TestPublishQuestionAnswerSuccess(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	pub := &fakeQuestionPublisher{}
	s.questionAnswerPublishers = map[string]marketplace.QuestionAnswerPublisher{"wb": pub}

	q, err := s.store.UpsertQuestion(ctx, store.QuestionInput{
		Marketplace: "wb", ExternalQuestionID: "q1", ExternalSKU: "sku42",
		AuthorName: "Иван", Text: "Есть в наличии?", CreatedAtMP: testTime(),
	})
	if err != nil {
		t.Fatalf("upsert question: %v", err)
	}
	if err := s.store.SetQuestionAnswer(ctx, q.ID, "Да, есть"); err != nil {
		t.Fatalf("set answer: %v", err)
	}
	q, _ = s.store.QuestionByID(ctx, q.ID)
	s.publishQuestionAnswer(ctx, q)

	if pub.calls != 1 || pub.lastText != "Да, есть" || pub.lastID != "q1" || pub.lastSKU != "sku42" {
		t.Fatalf("publisher not called correctly: %+v", pub)
	}
	got, _ := s.store.QuestionByID(ctx, q.ID)
	if got.AnswerPublishState == nil || *got.AnswerPublishState != "published" {
		t.Fatalf("state = %v", got.AnswerPublishState)
	}
	if got.AnswerPublishedAt == nil {
		t.Fatalf("expected AnswerPublishedAt set")
	}
}

func TestPublishQuestionAnswerFailure(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	s.questionAnswerPublishers = map[string]marketplace.QuestionAnswerPublisher{
		"wb": &fakeQuestionPublisher{err: errors.New("network error")},
	}

	q, err := s.store.UpsertQuestion(ctx, store.QuestionInput{
		Marketplace: "wb", ExternalQuestionID: "q2",
		AuthorName: "Петр", Text: "Когда доставка?", CreatedAtMP: testTime(),
	})
	if err != nil {
		t.Fatalf("upsert question: %v", err)
	}
	if err := s.store.SetQuestionAnswer(ctx, q.ID, "Завтра"); err != nil {
		t.Fatalf("set answer: %v", err)
	}
	q, _ = s.store.QuestionByID(ctx, q.ID)
	s.publishQuestionAnswer(ctx, q)

	got, _ := s.store.QuestionByID(ctx, q.ID)
	if got.AnswerPublishState == nil || *got.AnswerPublishState != "failed" {
		t.Fatalf("expected failed, got %v", got.AnswerPublishState)
	}
	if got.AnswerPublishError == nil || *got.AnswerPublishError != "network error" {
		t.Fatalf("expected error text, got %v", got.AnswerPublishError)
	}
}

func TestPublishQuestionAnswerSiteUnsupported(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	s.questionAnswerPublishers = map[string]marketplace.QuestionAnswerPublisher{}

	q, err := s.store.CreateSiteQuestion(ctx, store.SiteQuestionInput{
		SellerArticle: "a1", AuthorName: "Аня", AuthorEmail: "a@b.co", Text: "Как заказать?",
	})
	if err != nil {
		t.Fatalf("create site question: %v", err)
	}
	// Manually set answer text so publishQuestionAnswer has something to publish.
	if err := s.store.SetQuestionAnswer(ctx, q.ID, "Через сайт"); err != nil {
		t.Fatalf("set answer: %v", err)
	}
	q, _ = s.store.QuestionByID(ctx, q.ID)
	s.publishQuestionAnswer(ctx, q)

	got, _ := s.store.QuestionByID(ctx, q.ID)
	if got.AnswerPublishState == nil || *got.AnswerPublishState != "unsupported" {
		t.Fatalf("expected unsupported for site question, got %v", got.AnswerPublishState)
	}
}

func TestPublishQuestionAnswerNoPublisher(t *testing.T) {
	s := newAuthTestServer(t)
	ctx := context.Background()
	// No publisher registered for ozon.
	s.questionAnswerPublishers = map[string]marketplace.QuestionAnswerPublisher{}

	q, err := s.store.UpsertQuestion(ctx, store.QuestionInput{
		Marketplace: "ozon", ExternalQuestionID: "ozon-q1",
		AuthorName: "Мария", Text: "Есть гарантия?", CreatedAtMP: testTime(),
	})
	if err != nil {
		t.Fatalf("upsert question: %v", err)
	}
	if err := s.store.SetQuestionAnswer(ctx, q.ID, "Да, 1 год"); err != nil {
		t.Fatalf("set answer: %v", err)
	}
	q, _ = s.store.QuestionByID(ctx, q.ID)
	s.publishQuestionAnswer(ctx, q)

	got, _ := s.store.QuestionByID(ctx, q.ID)
	if got.AnswerPublishState == nil || *got.AnswerPublishState != "unsupported" {
		t.Fatalf("expected unsupported (no publisher), got %v", got.AnswerPublishState)
	}
}
