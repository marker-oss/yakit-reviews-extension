package store

import (
	"context"
	"testing"
	"time"
)

func TestQuestionAnswerFlow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	q, err := s.UpsertQuestion(ctx, QuestionInput{
		Marketplace: "wb", ExternalQuestionID: "q1", SellerArticle: "a1",
		AuthorName: "Иван Иванов", Text: "Есть в наличии?", CreatedAtMP: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if q.AuthorName == "Иван Иванов" {
		t.Fatalf("author name not anonymized: %q", q.AuthorName)
	}
	if q.Visibility != "hidden" {
		t.Fatalf("new question should be hidden, got %q", q.Visibility)
	}

	if err := s.SetQuestionAnswer(ctx, q.ID, "Да, есть"); err != nil {
		t.Fatalf("answer: %v", err)
	}
	got, _ := s.QuestionByID(ctx, q.ID)
	if got.AnswerText == nil || *got.AnswerText != "Да, есть" || got.Visibility != "visible" || got.Status != "answered" {
		t.Fatalf("unexpected after answer: %+v", got)
	}
	if got.AnswerPublishState == nil || *got.AnswerPublishState != "pending" {
		t.Fatalf("expected pending publish, got %v", got.AnswerPublishState)
	}
	queued, _ := s.QuestionsNeedingAnswerPublish(ctx)
	if len(queued) != 1 {
		t.Fatalf("expected 1 queued, got %d", len(queued))
	}
}

func TestSiteQuestionHiddenUntilAnswered(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	q, err := s.CreateSiteQuestion(ctx, SiteQuestionInput{
		SellerArticle: "a1", AuthorName: "A", AuthorEmail: "a@b.co", Text: "Когда отгрузка?",
	})
	if err != nil {
		t.Fatalf("create site q: %v", err)
	}
	if q.Visibility != "hidden" || q.Status != "pending" {
		t.Fatalf("site question should start hidden/pending: %+v", q)
	}
	// Not visible in the public list until answered.
	vis, _ := s.ListQuestions(ctx, QuestionFilter{Visibility: "visible", SellerArticle: "a1"})
	if len(vis) != 0 {
		t.Fatalf("unanswered site question must not be visible")
	}
	_ = s.SetQuestionAnswer(ctx, q.ID, "На следующей неделе")
	vis, _ = s.ListQuestions(ctx, QuestionFilter{Visibility: "visible", SellerArticle: "a1"})
	if len(vis) != 1 {
		t.Fatalf("answered site question should be visible, got %d", len(vis))
	}
}
