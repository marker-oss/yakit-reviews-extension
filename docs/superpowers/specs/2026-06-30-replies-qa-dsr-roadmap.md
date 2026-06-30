# Roadmap Spec: MP Reply Publishing · Q&A · DSR (152-ФЗ)

**Status:** design spec — **all policy decisions resolved 2026-06-30** (see each feature's DECISIONS block). Remaining before each becomes a bite-sized `docs/superpowers/plans/` plan: verify the marketplace write-API request/response shapes against current docs (context7). Order of execution: nav consolidation (plan ready) → Feature 2 → Feature 3 → Feature 4.

These are independent subsystems; do not bundle into one plan.

---

## Current state (verified 2026-06-30)

- **Replies, local:** `PUT /admin/api/reviews/{id}/reply` → `store.SetReviewReply` writes `Review.AdminReplyText` + `AdminReplyAt` (local only — [curation.go:68](../../../internal/store/curation.go)). Admin UI editor exists in [Reviews.tsx](../../../web/admin/src/pages/Reviews.tsx). Widget renders it as `Answer{Kind:"seller"}` ([reviewjson.go](../../../internal/reviewjson/reviewjson.go)). **Nothing is pushed back to the marketplace.**
- **Marketplace answers, read:** `Review.MPAnswerText` / `MPAnswerState` carry the MP's own answer fetched during sync.
- **Collectors are read-only:** `internal/marketplace/{wb,ym,ozon}/client.go` only GET feedbacks. All three are wired as adapters in [main.go](../../../cmd/reviews/main.go) (Ozon enabled-gated).
- **DSR primitives exist:** `store.HardDeleteReview` (curation.go:359) + `handleAdminReviewPurge` (admin_reviews.go:207) hard-delete a single review and its media files; `store.ScrubPersonalData` anonymizes legacy rows at startup; `cfg.PrivacyContact` (REVIEWS_PRIVACY_CONTACT) is the published contact. **No subject-centric lookup/export, no delete-by-email/identity.**
- **No Q&A concept anywhere** (grep for "question" → empty).
- Reviewer identity is keyed by `ReviewerIdentity{EmailNormalized, EmailHash}`; site reviews link to it; submission stores `AuthorEmailHash`, `SubmissionIPHash`, `SubmissionUAHash`, consent timestamps.

---

## Feature 2 — Publish replies back to the marketplace

**Goal:** When an admin saves a reply to a *marketplace* review, also publish it to that marketplace via its API, tracking per-review publish state.

**Architecture:**
- Extend each marketplace client with a write method, gated behind a capability interface so only supporting/configured MPs attempt it:
  ```go
  // internal/marketplace/types.go (new or existing types file)
  type ReplyPublisher interface {
      PublishReply(ctx context.Context, externalReviewID, text string) error
  }
  ```
  `wb.Client`, `ym.Client`, `ozon.Client` implement it where the API allows.
- New `Review` columns: `ReplyPublishState *string` (`"pending"|"published"|"failed"|"unsupported"`), `ReplyPublishError *string`, `ReplyPublishedAt *time.Time`. Add to AutoMigrate.
- On `PUT .../reply` for a non-`site` review: store the text (as today), set state `pending`, then enqueue a publish attempt (synchronous attempt with stored failure, or a small worker — see decision D2). Site reviews skip publishing (no MP target) and report `unsupported`/`n/a`.
- A retry path: a `POST /admin/api/reviews/{id}/reply/retry` endpoint and/or the periodic sync re-attempting `failed`/`pending` rows.
- Admin UI ([Reviews.tsx]): show publish state badge next to the reply editor (Опубликовано / Ошибка: … / Не поддерживается) and a "Повторить" button on failure.

**Marketplace write APIs (VERIFY before coding — use context7 / official docs):**
- **WB:** Feedbacks API — answer endpoint (`PATCH`/`POST` to feedbacks "answer"). Requires the feedbacks/questions token scope. Confirm idempotency (editing an existing answer vs creating).
- **YM:** Business API — comments/answer on review. Confirm endpoint + that the configured auth (api-key vs OAuth) is sufficient for writes.
- **Ozon:** Reviews are Premium-Plus-gated; the comment-create endpoint requires it. Treat Ozon as `unsupported` when the account lacks access — surface that state rather than erroring.

**DECISIONS (resolved 2026-06-30):**
- **D1 — Scope:** all three MPs (WB/YM/Ozon) in one feature, via the shared `ReplyPublisher` capability. Each client gates itself: unsupported/unconfigured → state `unsupported`, no call.
- **D2 — Timing:** synchronous attempt on reply save; on failure persist state `failed` + error; the existing periodic sync loop re-attempts `failed`/`pending`. A manual `POST .../reply/retry` also exists.
- **D3 — Edit/withdraw:** phase 1 = publish-once + retry-on-failure only. No editing an already-published answer in v1.
- **D4 — Per-MP toggle:** yes — a "публиковать ответы" switch per marketplace in Настройки · Маркетплейсы, **off by default for Ozon** (Premium-Plus-gated), on where supported.

**Testable deliverables (per plan):** client write method against an `httptest` server (success + error mapping); store state transitions; handler sets `pending`→`published`/`failed`; site review → `unsupported` and no client call.

---

## Feature 3 — Questions & Answers (Q&A)

**Goal:** Collect product questions (from marketplaces and from the shop site), answer them in the admin, optionally publish answers back to the MP, and render a "Вопросы и ответы" block in the widget.

**Architecture (largest feature — likely 2 plans: backend+admin, then widget+site-intake):**
- New model `Question{ID, TenantID, Marketplace, ExternalQuestionID, SellerArticle, Text, AuthorName(anonymized), AnswerText, AnswerState, Status(pending/published/hidden), CreatedAtMP, ...}` mirroring `Review`'s tenancy + curation + PD-anonymization conventions (anonymize author at ingestion, no Raw blob — same 152-ФЗ posture as reviews).
- Collector side: extend marketplace clients with a `QuestionFetcher` capability (WB and Ozon expose product-questions APIs; YM may not — gate per MP). Wire into the sync loop as a parallel stream to reviews.
- Site intake: a public form analogous to the review-submission flow (`review_submissions.go` is the template) — `POST /api/questions`, rate-limited, consent, stored `pending`.
- Admin: a "Вопросы" sub-tab (under Отзывы or its own top-level after nav consolidation) — list, answer editor, moderate, publish (reuses the Feature-2 `ReplyPublisher`-style capability for answers).
- Public API + widget: `GET /api/questions?article=…`, a `renderQA()` block in `reviews-widget.js` with its own visibility config flag (mirror `config.visibility.photos`).

**DECISIONS (resolved 2026-06-30):**
- **Q1 — Intake:** both — fetch questions from MPs (WB/Ozon where the API exists; gate per MP) **and** a site "задать вопрос" form (modeled on `review_submissions.go`).
- **Q2 — Publish to MP:** yes — answers publish back via the **Feature 2** `ReplyPublisher`/state-column mechanism (reuse, don't duplicate). Feature 2 must land first.
- **Q3 — Widget placement:** a **separate Отзывы/Вопросы tab** inside the widget. NOTE for the plan author: the widget UI reportedly already has a toggle scaffold for this — **verify in `reviews-widget.js` before building** and reuse it rather than adding a new control.
- **Q4 — Site-question visibility:** a site-submitted question stays **hidden until the seller answers it** (visibility is tied to having an answer, not a separate approve step). Answer present + approved → visible; no answer → not shown publicly.

**Dependency:** Feature 2's `ReplyPublisher` + publish-state columns must land first; Q&A answer-publishing reuses them.

---

## Feature 4 — Data-subject requests (export / delete), 152-ФЗ

**Goal:** Given a subject identifier (email), let an admin find everything stored about that person, export it, and hard-delete it — closing the "право на доступ/удаление" obligations beyond the existing single-review purge.

**Data we actually hold (verified against models.go — drives the whole design):**
- **Site reviews:** linked to `ReviewerIdentity{EmailNormalized, EmailHash}` via `ReviewerIdentityID`; carry `AuthorEmailHash`, `SubmissionIPHash/UAHash/Origin/Referrer`, consent timestamps, and **uploaded media files on disk** (`ReviewMedia.StoragePath`). This is the real PD store.
- **Marketplace reviews:** `AuthorName` is **anonymized at ingestion** ("Анна К."); **no email, no contact, no MP account ID**; `ReviewerIdentityID` is nil. Media are **CDN URLs hosted by the marketplace** (`StoragePath` empty — we don't hold the image). The only stable handle is `ExternalReviewID` (+ `Marketplace`). The marketplace is the **primary operator**; we hold a secondary de-identified copy.

**Consequence — two lookup modes:**
- **By email** (`ReviewerIdentity.EmailNormalized`, exact) → site reviews. This is the main DSR path.
- **By marketplace review reference** → MP reviews: subject supplies the MP review URL/ID (→ `external_review_id` + marketplace) or article+date; **no in-system identity proof is possible** (no shared secret), so the operator confirms control out-of-band. Export/delete acts on **our copy only**; the original on WB/YM must go through the marketplace's own process.

**Architecture:**
- New store methods:
  - `FindSubjectByEmail(ctx, email) (SubjectExport, error)` — `ReviewerIdentity` + all linked `Review`s (+ media metadata, consent timestamps, submission hashes). JSON-serializable.
  - `FindReviewByExternalRef(ctx, marketplace, externalReviewID) (SubjectExport, error)` — single MP review lookup for the MP path.
  - `PurgeSubjectByEmail(ctx, email) (deleted int, err error)` — hard-delete the subject's reviews (reuse `HardDeleteReview` per row to remove media files too), then delete the `ReviewerIdentity`. Transactional + idempotent.
  - `PurgeReviewByExternalRef(ctx, marketplace, externalReviewID) error` — delete our stored copy of one MP review.
- New admin endpoints (auth + CSRF):
  - `GET /admin/api/dsr/lookup?email=…` and `?marketplace=…&reviewId=…` → preview (counts + redacted preview of what's stored).
  - `GET /admin/api/dsr/export?…` → JSON download (`Content-Disposition: attachment`).
  - `POST /admin/api/dsr/delete` `{email}` or `{marketplace, reviewId}` → purge, returns count; writes audit log.
- Audit: new `DSRLog{ID, EmailHash, Marketplace, ExternalReviewID, Action(lookup/export/delete), AdminUserID, At}` table (DSR3).
- Admin UI: a "Запросы субъектов (152-ФЗ)" panel under Настройки with two tabs — **«По email»** (site data) and **«По отзыву маркетплейса»** (URL/ID). Each: «Найти» → preview, «Скачать выгрузку» (JSON), «Удалить» (double-confirm). Panel text states plainly that for МП мы удаляем только свою копию; оригинал — через сам маркетплейс.

**DECISIONS (resolved 2026-06-30):**
- **DSR1 — Match:** email-exact for site data; `external_review_id`+marketplace (or article+date) for MP data. No name-based matching (anonymized/non-unique).
- **DSR2 — Process/format:** **Variant A** — manual intake (subject writes to `REVIEWS_PRIVACY_CONTACT`), operator verifies identity out-of-band, uses the admin panel to look up / **export as JSON** / delete, and forwards the file themselves. Self-service site form + email-token verification deferred to v2.
- **DSR3 — Audit log:** yes, minimal `DSRLog` (who/what/when, email hashed).
- **DSR4 — MP-side deletion:** we delete only our copy; original stays on the marketplace. Stated explicitly in the UI.

**Testable deliverables (per plan):** `FindSubjectByEmail` returns linked reviews for a seeded identity, empty for unknown email; `FindReviewByExternalRef` returns one MP review; `PurgeSubjectByEmail` removes reviews + identity + on-disk media and is idempotent; export handler sets attachment headers and includes consent timestamps; delete handler requires CSRF, returns the deleted count, and writes a `DSRLog` row.

---

## Cross-cutting notes

- All new tables go into the `AutoMigrate` list in [store.go](../../../internal/store/store.go).
- New admin routes register in [server.go](../../../internal/server/server.go) under the `protected` mux; writes wrapped in `requireCSRF`.
- New admin UI lands under the consolidated nav (Feature 1): Q&A as a content tab, DSR + reply-publish toggles under Настройки.
- Keep the PD-anonymization-at-ingestion posture for any new author-bearing data (Q&A), consistent with the legal-mitigation work.
- Deploy is manual `workflow_dispatch` on `feat/legal-mitigation`; push via `GIT_SSH_COMMAND='ssh -i ~/.ssh/reviews_deploy_ed25519 -o HostName=ssh.github.com -o Port=443'`.
