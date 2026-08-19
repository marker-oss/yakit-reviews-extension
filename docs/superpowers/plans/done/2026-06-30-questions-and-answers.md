# Questions & Answers (Q&A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collect product questions from marketplaces (WB, Ozon) and from a site form, answer them in the admin, publish answers back to the marketplace, and surface a "Вопросы" tab in the widget. Site questions stay hidden until the seller answers them.

**Architecture:** A new `Question` entity mirrors `Review`'s conventions (tenancy, PD-anonymization at ingestion, curation/visibility, media-free). It reuses Feature 2's publish-state pattern via a parallel `marketplace.QuestionAnswerPublisher` capability. Two phases: **Phase A** = backend (model, store, MP fetch, admin answer, publish) — independently shippable; **Phase B** = site intake form + public API + widget "Вопросы" tab (the widget scaffold already exists, disabled).

**Dependency:** **Feature 2 (MP reply publishing) must land first** — this plan reuses its publish-state column pattern, the `app_settings` toggle helper, the server publisher-injection wiring, and the `RetryPendingReplies`-style retry. References below assume `internal/server/reply_publish.go`, `cmd/reviews/main.go` publisher wiring, and `store.PublishRepliesKey` exist.

**Tech Stack:** Go (pure-Go), GORM/SQLite, `net/http`, `httptest`; React/TS admin SPA; vanilla-JS widget.

## Global Constraints

- Reuse each MP client's existing auth/base (WB `Authorization`+`feedbacks-api.wildberries.ru`; Ozon `Client-Id`+`Api-Key`+`api-seller.ozon.ru`). **YM has no product-questions API — YM is out of scope for Q&A.**
- PD posture identical to reviews: anonymize author name at ingestion, do not store a `Raw` blob.
- Question publish-state values: `pending`, `published`, `failed`, `unsupported` (same vocabulary as Feature 2).
- Site question visibility: a site-submitted question is **hidden until it has a published answer** — there is no separate "approve" step; `Visibility` flips to `visible` only when an answer is saved.
- Per-MP "publish answers" reuses the **same toggle** as replies (`store.PublishRepliesKey(marketplace)`) — one switch governs both replies and question-answers for a marketplace.
- Admin/widget copy is Russian. Rebuild + embed the admin bundle before committing admin UI; the widget files (`web/reviews-widget/*`) are deployed as-is from source.
- ⚠️ **Endpoint verification:** WB and Ozon *question* endpoints below are from current docs but their exact request/response field names MUST be confirmed against the live docs before finalizing the fetch/answer tasks (WB: https://dev.wildberries.ru/openapi/user-communication ; Ozon: https://docs.ozon.ru/api/seller/ — `/v1/question/*` beta). Keep tests as the contract.

## Marketplace question endpoints (verify field names before coding)

| MP | List | Answer | Auth |
|---|---|---|---|
| WB | `GET /api/v1/questions?isAnswered=false&take=&skip=` | `PATCH /api/v1/questions` body `{"id":"<qid>","answer":{"text":"<t>"},"state":"wbRu"}` | `Authorization: <token>` |
| Ozon | `POST /v1/question/list` body `{filter:{...}}` | `POST /v1/question/answer/create` body `{"question_id":"<qid>","sku":<sku>,"text":"<t>"}` | `Client-Id`+`Api-Key` |

`Question.ExternalQuestionID` is stored as a string (WB ids are strings; Ozon question_id is a string/uuid; Ozon `sku` is numeric and must be stored on the question for answering).

---

## Phase A — backend foundation

### Task A1: `Question` model + migration

**Files:**
- Modify: `internal/store/models.go`
- Modify: `internal/store/store.go` (AutoMigrate)

**Interfaces:**
- Produces: `store.Question` with the fields below; `store.MarketplaceSite` reused for site questions.

- [ ] **Step 1: Add the model**

Append to `internal/store/models.go`:

```go
// Question is a product question from a marketplace or the shop site. Author
// names are anonymized at ingestion (same posture as Review); no Raw blob.
type Question struct {
	ID                 uint   `gorm:"primaryKey"`
	TenantID           uint   `gorm:"not null;default:1;index;uniqueIndex:idx_marketplace_question"`
	Marketplace        string `gorm:"size:16;not null;uniqueIndex:idx_marketplace_question"`
	ExternalQuestionID string `gorm:"size:128;not null;uniqueIndex:idx_marketplace_question"`
	ExternalProductID  string `gorm:"size:128;index"`
	SellerArticle      string `gorm:"size:128;index"`
	ExternalSKU        string `gorm:"size:64"` // Ozon needs the numeric sku to answer
	AuthorName         string
	Text               string
	AnswerText         *string
	AnswerAt           *time.Time
	Status             string `gorm:"size:32;not null;default:imported"` // imported | pending | answered
	Visibility         string `gorm:"size:16;not null;default:hidden;index"`
	CreatedAtMP        time.Time `gorm:"not null;index"`
	AnswerPublishState *string   `gorm:"size:16;index"`
	AnswerPublishError *string
	AnswerPublishedAt  *time.Time
	AuthorEmailHash    string `gorm:"size:64;index"` // site questions only
	SubmissionIPHash   string `gorm:"size:64"`
	ConsentPrivacyAt   *time.Time
	FetchedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time
}
```

- [ ] **Step 2: Register in AutoMigrate**

In `internal/store/store.go`, add `&Question{},` to the `AutoMigrate(...)` list.

- [ ] **Step 3: Verify build**

Run: `go build ./internal/store/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/store/models.go internal/store/store.go
git commit -m "feat(store): Question model + migration"
```

---

### Task A2: Question store methods

**Files:**
- Create: `internal/store/questions.go`
- Test: `internal/store/questions_test.go`

**Interfaces:**
- Produces:
  - `type QuestionInput struct { Marketplace, ExternalQuestionID, ExternalProductID, SellerArticle, ExternalSKU, AuthorName, Text string; CreatedAtMP time.Time }`
  - `func (s *Store) UpsertQuestion(ctx, in QuestionInput) (Question, error)` — anonymizes `AuthorName` via the same helper reviews use (find it: the ingestion anonymizer in the store/marketplace path — reuse it; do not reimplement).
  - `func (s *Store) ListQuestions(ctx, filter QuestionFilter) ([]Question, error)` with `QuestionFilter{ Marketplace, Status, Visibility, SellerArticle string; Limit, Offset int }`.
  - `func (s *Store) SetQuestionAnswer(ctx, id uint, text string) error` — sets `AnswerText`, `AnswerAt=now`, `Status="answered"`, `Visibility="visible"`, and `AnswerPublishState="pending"` for non-site marketplaces (site stays unpublished → leave state nil).
  - `func (s *Store) SetQuestionAnswerPublishState(ctx, id uint, state string, errText *string, publishedAt *time.Time) error`
  - `func (s *Store) QuestionsNeedingAnswerPublish(ctx) ([]Question, error)`
  - `func (s *Store) QuestionByID(ctx, id uint) (Question, error)`
  - `func (s *Store) CreateSiteQuestion(ctx, in SiteQuestionInput) (Question, error)` — `SiteQuestionInput{ SellerArticle, AuthorName, AuthorEmail, Text, IPHash string }`; stores `Marketplace="site"`, `Status="pending"`, `Visibility="hidden"`, `ExternalQuestionID="site-"+token` (token from `auth.NewSessionToken`).

- [ ] **Step 1: Write the failing test**

Create `internal/store/questions_test.go`:

```go
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
```

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/store/ -run 'TestQuestion|TestSiteQuestion' -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/store/questions.go`. Mirror `submissions.go`/`curation.go` patterns. For author anonymization, locate the function reviews use at ingestion (e.g. an `anonymizeName`/`SellerArticleForReview`-adjacent helper in the store or marketplace mapping) and call the same one — do not write a second anonymizer. Implement each method per the Interfaces block: `SetQuestionAnswer` updates `answer_text`, `answer_at`, `status='answered'`, `visibility='visible'`, and (when `marketplace <> 'site'`) `answer_publish_state='pending'`. `QuestionsNeedingAnswerPublish` selects `marketplace <> 'site' AND answer_text IS NOT NULL AND answer_text <> '' AND (answer_publish_state IS NULL OR answer_publish_state IN ('pending','failed'))`. `ListQuestions` filters by the non-empty fields of `QuestionFilter` and orders `created_at_mp desc`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/ -run 'TestQuestion|TestSiteQuestion' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/questions.go internal/store/questions_test.go
git commit -m "feat(store): question upsert/list/answer/publish-queue methods"
```

---

### Task A3: `QuestionAnswerPublisher` capability + WB/Ozon fetch & answer

**Files:**
- Modify: `internal/marketplace/model.go` — `QuestionAnswerPublisher` + `QuestionFetcher` interfaces.
- Modify: `internal/marketplace/wb/client.go` (+ test) — `FetchQuestions`, `PublishQuestionAnswer`.
- Modify: `internal/marketplace/ozon/client.go` (+ test) — `FetchQuestions`, `PublishQuestionAnswer`.

**Interfaces:**
- Produces:
  ```go
  type Question struct { ExternalQuestionID, ExternalProductID, SellerArticle, ExternalSKU, AuthorName, Text string; CreatedAtMP time.Time }
  type QuestionFetcher interface {
      FetchQuestions(ctx context.Context, since time.Time, cursor string) ([]Question, string, error)
  }
  type QuestionAnswerPublisher interface {
      PublishQuestionAnswer(ctx context.Context, externalQuestionID, sku, text string) error
  }
  ```

- [ ] **Step 1: Add the interfaces + marketplace.Question type**

Append to `internal/marketplace/model.go` the `Question`, `QuestionFetcher`, `QuestionAnswerPublisher` definitions above.

- [ ] **Step 2: WB — write failing tests then implement**

Add to `internal/marketplace/wb/client_test.go` an `httptest`-based test for `FetchQuestions` (GET `/api/v1/questions`, `Authorization` header, maps the response array to `[]marketplace.Question`) and for `PublishQuestionAnswer` (PATCH `/api/v1/questions`, body contains `"id"` and `"answer":{"text":...}`, success on 2xx). Run to confirm fail.

Implement on `*wb.Client` mirroring the existing `fetchFeedbacks`/new `PublishReply` plumbing:

```go
func (c *Client) PublishQuestionAnswer(ctx context.Context, externalQuestionID, _ /*sku*/, text string) error {
	payload := map[string]any{"id": externalQuestionID, "answer": map[string]string{"text": text}, "state": "wbRu"}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/api/v1/questions", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("WB publish question answer: status %d", resp.StatusCode)
	}
	return nil
}
```

Implement `FetchQuestions` mirroring `fetchFeedbacks` (GET `/api/v1/questions`, `isAnswered=false`, `take`/`skip` paging, decode, map to `marketplace.Question`). Verify the WB question JSON field names against the live docs and adjust the decode struct + test fixture to match.

- [ ] **Step 3: Ozon — write failing tests then implement**

Add to `internal/marketplace/ozon/client_test.go` `httptest` tests for `FetchQuestions` (POST `/v1/question/list`) and `PublishQuestionAnswer` (POST `/v1/question/answer/create`, body has `question_id`, `sku`, `text`; `Client-Id`+`Api-Key` headers). Run to confirm fail.

Implement on `*ozon.Client`:

```go
func (c *Client) PublishQuestionAnswer(ctx context.Context, externalQuestionID, sku, text string) error {
	payload := map[string]any{"question_id": externalQuestionID, "sku": sku, "text": text}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/question/answer/create", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", c.clientID)
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Ozon publish question answer: status %d", resp.StatusCode)
	}
	return nil
}
```

Implement `FetchQuestions` (POST `/v1/question/list`), mapping `question_id`→ExternalQuestionID, `sku`→ExternalSKU. Verify field names against docs; adjust struct + fixtures.

- [ ] **Step 4: Run marketplace tests**

Run: `go test ./internal/marketplace/... -count=1`
Expected: PASS (the pre-existing `ym` `TestFetchReviewsMapsYMResponse` time-fixture failure is unrelated — confirm it is the only failure).

- [ ] **Step 5: Commit**

```bash
git add internal/marketplace
git commit -m "feat(marketplace): WB+Ozon question fetch and answer publishing"
```

---

### Task A4: Collector fetches questions; server publishes & retries answers

**Files:**
- Modify: `internal/collector/collector.go` — after fetching reviews for a marketplace, also fetch questions if the adapter implements `QuestionFetcher`, upserting via `store.UpsertQuestion`.
- Create: `internal/server/question_publish.go` (+ test) — `publishQuestionAnswer`, `RetryPendingQuestionAnswers`, reusing `replyPublishEnabled`.
- Modify: `cmd/reviews/main.go` — build a `map[string]marketplace.QuestionAnswerPublisher`, inject into the server; call `RetryPendingQuestionAnswers` in the same post-sync hook as `RetryPendingReplies`.
- Modify: `internal/server/server.go` — `Config.QuestionAnswerPublishers` + `Server` field.

**Interfaces:**
- Consumes: `marketplace.QuestionFetcher`, `marketplace.QuestionAnswerPublisher`, Task A2 store methods, `s.replyPublishEnabled` (Feature 2).
- Produces: `Server.questionAnswerPublishers`; `func (s *Server) publishQuestionAnswer(ctx, q store.Question)`; `func (s *Server) RetryPendingQuestionAnswers(ctx)`.

- [ ] **Step 1: Collector question fetch**

In `runMarketplace` (collector.go), after the review fetch loop completes, add: if `qf, ok := adapter.(marketplace.QuestionFetcher); ok { … }` page through `qf.FetchQuestions` and `r.store.UpsertQuestion(...)` each, accumulating a seen/upserted count into `Result` (extend `Result` if you want question counters, or reuse Seen/Upserted). Keep it non-fatal: a question-fetch error logs and does not fail the review sync (set a separate `result.QuestionError` or log+continue).

- [ ] **Step 2: Server publish orchestration (mirror reply_publish.go)**

Create `internal/server/question_publish.go` mirroring `internal/server/reply_publish.go` exactly, but for questions: site/no-publisher/disabled → `unsupported`; otherwise call `pub.PublishQuestionAnswer(ctx, q.ExternalQuestionID, q.ExternalSKU, *q.AnswerText)` and persist via `SetQuestionAnswerPublishState`. `RetryPendingQuestionAnswers` iterates `QuestionsNeedingAnswerPublish`. Reuse `s.replyPublishEnabled(ctx, marketplace)` (same toggle governs both).

- [ ] **Step 3: Wire publishers + retry in main**

In `cmd/reviews/main.go`, build `qaPublishers` by type-asserting each adapter to `marketplace.QuestionAnswerPublisher` (same loop that builds reply publishers), pass into `server.Config{QuestionAnswerPublishers: qaPublishers}`, and in the post-sync hook call both `srv.RetryPendingReplies(ctx)` and `srv.RetryPendingQuestionAnswers(ctx)`.

- [ ] **Step 4: Test publish orchestration**

Add `internal/server/question_publish_test.go` mirroring `reply_publish_test.go`'s fake-publisher tests: success → `published`; error → `failed`+error; site question → `unsupported`. Run:

Run: `go test ./internal/server/ -run TestQuestion -count=1` and `go build ./...`
Expected: PASS / build clean.

- [ ] **Step 5: Commit**

```bash
git add internal/collector internal/server cmd/reviews/main.go
git commit -m "feat: fetch MP questions on sync; publish & retry question answers"
```

---

### Task A5: Admin questions list + answer endpoints

**Files:**
- Create: `internal/server/admin_questions.go` (+ test)
- Modify: `internal/server/server.go` (routes)

**Interfaces:**
- Produces routes:
  - `GET /admin/api/questions?status=&marketplace=` → list (with answer + publish state).
  - `PUT /admin/api/questions/{id}/answer` body `{text}` → saves answer, triggers `publishQuestionAnswer`.
  - `POST /admin/api/questions/{id}/answer/retry` → re-attempt publish.

- [ ] **Step 1: Write failing handler test**

Create `internal/server/admin_questions_test.go`: seed a WB question via `s.store.UpsertQuestion`, log in, `PUT .../answer` with a fake `QuestionAnswerPublisher` set on the server, assert the question becomes `answered`/`visible` and publish-state `published`; assert `GET /admin/api/questions` returns it; assert auth required (401 without cookie). Mirror `admin_settings_test.go` CSRF usage.

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/server/ -run TestAdminQuestions -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement handlers**

Create `internal/server/admin_questions.go` mirroring `admin_reviews.go`'s list + reply handlers: a list handler reading a `QuestionFilter` from query params and mapping `store.Question` to a JSON struct (include `answer`, `answerPublish` state like the review reply did); an answer handler that calls `s.store.SetQuestionAnswer` then `s.publishQuestionAnswer(ctx, q)`; a retry handler calling `s.publishQuestionAnswer`. Register the three routes in `server.go` (`GET` plain, `PUT`/`POST` wrapped in `requireCSRF`).

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/server/ -run TestAdminQuestions -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/admin_questions.go internal/server/admin_questions_test.go internal/server/server.go
git commit -m "feat(server): admin questions list/answer/retry endpoints"
```

---

### Task A6: Admin "Вопросы" page

**Files:**
- Create: `web/admin/src/pages/Questions.tsx`
- Modify: `web/admin/src/App.tsx` (add `Вопросы` under the Отзывы/content area of the consolidated nav — Feature 1's route model), `web/admin/src/types.ts`
- Regenerate: `internal/server/admin_dist/*`

- [ ] **Step 1: Build the page**

Create `Questions.tsx` mirroring `Reviews.tsx`: load `GET /admin/api/questions`, list each question (article, text, author, marketplace badge, date), an answer textarea + «Ответить» (`PUT /admin/api/questions/{id}/answer`), and the publish-state badge + «Повторить» (`POST .../answer/retry`) reusing the same status vocabulary as Reviews' reply badge.

- [ ] **Step 2: Add to nav**

Add a `'questions'` route to Feature 1's `Route` model (`web/admin/src/App.tsx`): a top-level "Вопросы" entry (or under the content group), `routeTitle`/`routeSection` cases, nav link, and render `{route === 'questions' && <Questions />}`.

- [ ] **Step 3: Rebuild bundle + verify**

```bash
cd /home/mama/DEV/Reviews/web/admin && npm run build
cd /home/mama/DEV/Reviews && rm -rf internal/server/admin_dist && cp -r web/admin/dist internal/server/admin_dist
go build ./... && grep -c "Вопросы" internal/server/admin_dist/assets/index.js
```
Expected: build PASS; grep ≥1.

- [ ] **Step 4: Commit**

```bash
git add web/admin/src internal/server/admin_dist
git commit -m "feat(admin): Вопросы page — list, answer, publish status"
```

---

## Phase B — site intake + widget tab

### Task B1: Public site-question submission endpoint

**Files:**
- Create: `internal/server/question_submissions.go` (+ test)
- Modify: `internal/server/server.go` (public routes)

**Interfaces:**
- Produces: `POST /api/questions` (rate-limited, consent, honeypot+timing) → creates a hidden site question via `store.CreateSiteQuestion`; `GET /api/question-submission-config` (mirrors `handleReviewSubmissionConfig`, returns `agreementUrl`).

- [ ] **Step 1: Write failing test**

Create `internal/server/question_submissions_test.go`: POST a valid question (with `openedAt`, consent, no honeypot) → 201 + the question is stored hidden/pending; POST without consent → 400; verify it does NOT appear in the public `GET /api/questions?article=` list until answered. Mirror `review_submissions_test.go`.

- [ ] **Step 2: Run to confirm failure**

Run: `go test ./internal/server/ -run TestQuestionSubmission -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement**

Create `internal/server/question_submissions.go` mirroring `review_submissions.go`: reuse `validateSubmissionTrap`, `clientIP`, `formBool`, the `submissionLimiter` (add a parallel limiter or reuse with a question key), `store.HashPII`. No media for questions (text only). Require `privacyConsent`. Call `s.store.CreateSiteQuestion`. Register `POST /api/questions` and `GET /api/question-submission-config` on the public mux.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/ -run TestQuestionSubmission -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/question_submissions.go internal/server/question_submissions_test.go internal/server/server.go
git commit -m "feat(server): public site-question submission endpoint"
```

---

### Task B2: Public questions API + static export

**Files:**
- Modify: `internal/server/server.go` — `GET /api/questions?article=` public handler (visible+answered only).
- Modify: `internal/export/export.go` — include a per-article questions bundle in the static export (so the static/widget path has questions too). (If out of scope for v1, serve questions only via the live API and note the static path is deferred — `log` that limitation.)

- [ ] **Step 1: Implement the public list handler**

Add `GET /api/questions` returning answered+visible questions for an article (filter `Visibility="visible"`, `SellerArticle=`), mapped to a small public JSON shape `{question, answer, date}`. Write a handler test asserting an unanswered question is absent and an answered one present.

- [ ] **Step 2: Decide static export inclusion**

If including questions in the static bundle: extend `staticexport` to write `questions/<article>.json` alongside reviews-data and regenerate it in `regenerateSiteData`. If deferring: add a one-line `log` in the refresh path noting questions are live-API-only for now, and document it here.

- [ ] **Step 3: Test + commit**

Run: `go test ./internal/server/ ./internal/export/ -count=1`
Expected: PASS.

```bash
git add internal/server internal/export
git commit -m "feat: public questions API (answered+visible)"
```

---

### Task B3: Widget "Вопросы" tab

**Files:**
- Modify: `web/reviews-widget/reviews-widget.js` — enable the existing disabled "Вопросы" tab (line ~256), fetch + render questions, add a "задать вопрос" form (mirror the review submission form), gate on a new `config.visibility.questions` flag.
- Modify: `web/reviews-widget/reviews-widget.css` — minimal styles for the Q&A list.

- [ ] **Step 1: Wire the tab**

In `reviews-widget.js`: remove `disabled` from the "Вопросы" tab button; add tab state (`state.activeTab = 'reviews' | 'questions'`); on switching to questions, fetch from the questions endpoint (live `/api/questions?article=` or static `questions/<article>.json`, matching how reviews are loaded — reuse the existing data-base/proxy resolution); render a Q&A list (question text + seller answer) and the "задать вопрос" form posting to `/api/questions`. Gate the whole tab on `config.visibility.questions` (default true), mirroring the existing `visibility` flags.

- [ ] **Step 2: Syntax check + manual verify**

Run: `node --check web/reviews-widget/reviews-widget.js`
Expected: OK.

Manual: open the widget test harness (`web/reviews-widget/test/fixture-render.html`) and confirm the Вопросы tab switches, lists answered questions, and the form submits. (No JS test runner — manual gate, per the widget's existing convention.)

- [ ] **Step 3: Commit**

```bash
git add web/reviews-widget
git commit -m "feat(widget): Вопросы tab — list answered Q&A and submit a question"
```

---

## Self-Review

- **Spec coverage:** Q1 (MP WB+Ozon fetch — A3/A4; site form — B1) ✓; Q2 (publish answers reusing F2 pattern + shared toggle — A3/A4) ✓; Q3 (separate widget tab, existing scaffold enabled — B3) ✓; Q4 (site question hidden until answered — A2 `SetQuestionAnswer` flips visibility; test `TestSiteQuestionHiddenUntilAnswered`) ✓. YM correctly excluded (no questions API).
- **Placeholder scan:** novel parts (model, store methods, publish, endpoints) carry full code or precise mirror-this-file instructions with the exact pattern named; the MP client field-name verification is an explicit ⚠️ step, not hand-waving.
- **Type consistency:** publish-state vocabulary (`pending/published/failed/unsupported`) shared with Feature 2; `QuestionAnswerPublisher.PublishQuestionAnswer(ctx, externalQuestionID, sku, text)` identical across A3 (interface), wb/ozon impls, and A4 (caller); `config.visibility.questions` flag named consistently (B3).
- **Dependency note:** assumes Feature 2 landed (publish-state pattern, `replyPublishEnabled`, publisher wiring, post-sync retry hook). If Feature 2 is not yet merged, do it first.
- **Phasing:** Phase A is independently shippable (MP Q&A end-to-end in admin) without Phase B (site intake + widget). Ship A, then B.
