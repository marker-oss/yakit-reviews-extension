# Marketplace API execution and sync coordination

**Date:** 2026-08-13  
**Status:** Approved design

## Problem

A client installation consistently receives HTTP `429` from the Wildberries feedbacks API. Production evidence established the following:

- the stored token has an effective limit of one request per roughly 12 minutes (`X-Ratelimit-Limit: 1`, `X-Ratelimit-Retry: 696`);
- one review synchronization makes multiple sequential API requests without pacing;
- manual synchronization can be started repeatedly and concurrently;
- the scheduler captures enabled marketplaces and adapters at process startup, so a marketplace enabled later in the admin panel is absent from scheduled runs;
- reply and question-answer publishers also capture credentials at startup;
- WB, Yandex Market, and Ozon clients each call `http.Client.Do` directly and implement inconsistent error handling.

Replacing the WB token with a personal token restores a practical request budget, but does not remove the concurrency, stale-credential, or rate-limit handling defects.

## Decisions

1. WB accepts only a valid, unexpired personal API token (`acc=3`).
2. Existing invalid or non-personal WB tokens disable only WB operations; they do not prevent the service from starting or affect other marketplaces.
3. Every marketplace API request uses shared execution infrastructure.
4. Retry and pacing remain explicit marketplace policies rather than generic guesses.
5. Read operations may be retried once after a documented rate-limit delay. Write operations are never retried automatically.
6. Synchronization is coordinated independently per marketplace. Different marketplaces may run concurrently; the same marketplace may not.
7. Enabled flags and credentials are loaded immediately before each scheduled, manual, or publishing operation.

## Architecture

### Shared API executor

Add a small package under `internal/marketplace/apihttp` containing an executor and marketplace policies.

The composition root creates one process-wide executor and injects that same instance into every fresh marketplace client. Per-marketplace serialization and pacing would not hold if each adapter created its own executor.

The executor accepts:

- marketplace ID;
- operation class: `Read` or `Write`;
- a request factory that creates a fresh `*http.Request` for every attempt;
- the existing `*http.Client`;
- the selected marketplace policy.

A request factory is required because a request body cannot safely be reused after an attempt. It also keeps authentication and payload construction inside each marketplace client.

The executor owns:

- serialization of active API requests per marketplace;
- context-aware waiting before attempts and retries;
- parsing rate-limit status and headers;
- closing a response body before a retry;
- at most one retry for `Read` operations;
- no automatic retry for `Write` operations;
- returning a structured error containing marketplace, endpoint path without query parameters, HTTP status, and retry delay.

The executor must never log or include authorization headers, request bodies, credentials, full URLs with query strings, or response bodies that may contain personal data.

### Marketplace policies

Each policy answers three questions:

1. Which response statuses indicate throttling?
2. Which response header supplies the retry delay?
3. What minimum interval applies between requests from this process?

Policies:

| Marketplace | Throttle statuses | Retry delay | Minimum interval | Read retries | Write retries |
|---|---|---|---:|---:|---:|
| WB | `429` | `X-Ratelimit-Retry` seconds | 350 ms | 1 | 0 |
| Yandex Market | `420`, `429` | standard `Retry-After` when present | none | 1 | 0 |
| Ozon | `429` | standard `Retry-After` when present | none | 1 | 0 |

`Retry-After` supports both integer seconds and an HTTP date. WB's header is parsed as integer seconds. A missing, malformed, non-positive, or greater-than-15-minute delay does not trigger a guessed retry; the executor returns the structured throttle error. Waiting is always interruptible through the request context.

The executor does not retry generic `5xx` responses or transport errors in this change. Those failures do not have an established idempotency contract and are outside the diagnosed defect.

### Client migration

Replace every direct `httpClient.Do` call in:

- `internal/marketplace/wb/client.go`;
- `internal/marketplace/ym/client.go`;
- `internal/marketplace/ozon/client.go`.

All fetches, probes, product mappings, reply publication, and question-answer publication use the executor.

Operation class is semantic, not derived from HTTP method. Ozon read-only endpoints that use `POST` are `Read`; publication endpoints are `Write`.

Marketplace clients remain responsible for:

- URL and payload construction;
- authentication headers;
- decoding successful and marketplace-specific error payloads;
- mapping API models into domain models.

### Operation coordinator

Add a process-local coordinator for marketplace operations. It owns a non-blocking synchronization lease per marketplace.

- A scheduled or manual sync must acquire the marketplace lease before constructing an adapter.
- A second sync for the same marketplace is rejected immediately as already running.
- Different marketplaces use independent leases.
- Leases are released with `defer`, including error and cancellation paths.

The coordinator acquisition API returns `started` and `busy` marketplace IDs before work is dispatched. The admin endpoint returns HTTP `202` when at least one requested marketplace starts and includes both lists; it returns HTTP `409` when every valid requested marketplace is busy. Unknown, disabled, or invalidly configured explicitly requested marketplaces return HTTP `400` and start nothing.

Publishing is not rejected by the sync lease. It uses the shared API executor's per-marketplace serialization, so it waits for the active external request and may run only at a request boundary between sync pages. This preserves queued reply behavior without losing a seller action while still enforcing marketplace pacing.

No persistent job table or distributed lock is introduced. The current deployment runs one service process; a process-local coordinator is sufficient. A distributed lock becomes necessary only if multiple application replicas share one database and credentials.

### Dynamic configuration and credentials

Create one operation factory in the composition root (`cmd/reviews`) that:

1. loads stored marketplace credentials;
2. overlays them on environment configuration;
3. validates the selected marketplace;
4. constructs a fresh adapter using the shared executor;
5. runs the requested operation.

Both scheduler and manual sync call this factory. The scheduler no longer stores a startup-time marketplace list or startup-time adapters. On every tick it loads the current enabled marketplace list.

Reply and question-answer publication use resolver functions supplied by the composition root. Each resolver reloads current credentials, validates the marketplace, and constructs the publisher immediately before the external call. Startup-captured publisher maps are removed. Therefore token changes, enabling, and disabling apply without restarting the container.

If credentials become invalid between queueing and publication, the queued item remains failed with an actionable error and can be retried after credentials are corrected. The resolver never returns an adapter built from stale credentials.

## WB personal-token policy

### Validation

Validate a WB token locally before saving or using it:

- exactly three JWT segments;
- base64url payload decodes as JSON;
- numeric `acc` claim equals `3` (personal token);
- numeric `exp` claim exists and is later than the current time.

JWT decoding identifies token metadata; it does not verify the signature because the service does not possess WB's signing keys. API authorization remains the authoritative authenticity check. The validator must describe this distinction in its name and tests rather than implying cryptographic verification.

The token's reviews/questions category cannot be established from the validated claims above. The UI instructs the seller to select that category; WB remains authoritative and returns `401` or `403` when the token lacks access.

Malformed, expired, base (`acc=1`), test (`acc=2`), service (`acc=4`), or unknown token types are rejected by the credentials endpoint with HTTP `400` and an actionable Russian error: a personal WB API token with the reviews/questions category is required.

Service tokens are not accepted because this self-hosted product is not connected through the official WB solutions catalog. If that distribution model changes, service-token support requires a separate policy decision.

### Existing installations

An existing stored non-personal or invalid WB token must not stop application startup.

For WB it produces:

- `configured=false`;
- a marketplace warning instructing the seller to replace the token;
- no scheduled or manual synchronization;
- no reply or question-answer publication.

Yandex Market, Ozon, the admin panel, public widget, and locally stored reviews remain available.

### Admin UI

On the marketplace settings page:

- rename the field to `Персональный WB API-токен`;
- add concise help directing the seller to create a personal token with the reviews/questions category;
- display the backend warning for an existing unsupported token;
- disable WB synchronization while `configured=false`;
- preserve the existing masked-secret behavior.

A failed save leaves the previously stored credential unchanged.

## Error behavior

Rate-limit errors stored in sync runs and presented by the admin use a stable actionable message, for example:

`Wildberries временно ограничил запросы. Повторите через 12 мин.`

Logs and stored errors contain marketplace, endpoint path, status, and retry delay. They do not contain secrets or response data.

A context cancellation returns the context error rather than a rate-limit error. This ensures shutdown is not delayed by a retry timer.

Write operations return the throttle error immediately even when a retry delay is present. The existing pending-publication state records the failure and permits an explicit later retry without risking duplicate publication.

## Verification

### API executor

- serializes simultaneous requests for one marketplace;
- does not serialize different marketplaces against each other;
- applies the WB 350 ms minimum interval;
- retries a `Read` once after a valid marketplace delay;
- closes the first response before retrying;
- rebuilds request bodies for the retry;
- does not retry `Write` operations;
- does not retry when the delay header is invalid or absent;
- cancels waits when the context is canceled;
- sanitizes errors so credentials and query strings are absent.

### Token validation

- accepts an unexpired personal token;
- rejects base, test, service, unknown, expired, and malformed tokens;
- credentials API returns `400` without changing the stored credential;
- an existing unsupported token marks only WB unconfigured and warned.

### Coordination and dynamic configuration

- a second sync for the same marketplace is reported busy and does not run;
- different marketplaces may synchronize concurrently;
- a marketplace enabled after process startup runs on the next scheduler tick;
- token replacement applies to the next sync without restart;
- token replacement applies to reply and question-answer publication without restart;
- disabling WB blocks new WB operations without affecting other marketplaces.

### End-to-end smoke check
1. saving a base token produces the personal-token error;
2. saving a valid personal token succeeds and marks WB configured;
3. starting WB sync reports `started`;
4. starting it again while active reports `busy` rather than spawning a duplicate;
5. an injected WB throttle response appears in sync activity with the retry duration;
6. YM and Ozon controls remain usable while WB is blocked.

## Non-goals

- generic retries for transport failures or `5xx` responses;
- automatic retries of publication requests;
- distributed coordination across replicas;
- persistent job history beyond existing sync runs;
- accepting WB base, test, or service tokens;
- changing marketplace page sizes or collection semantics;
- adding dependencies when standard-library synchronization, JWT payload decoding, and HTTP date parsing are sufficient.

## Security and operations

- Tokens remain write-only in the admin API and masked in status responses.
- Token values and JWT claims other than the type/expiry decision are not logged.
- Existing credentials are not migrated or rewritten automatically.
- The client installation continues running after upgrade even if its current WB token is rejected for future operations.
- Operators should replace the exposed server root password independently of this application change.
