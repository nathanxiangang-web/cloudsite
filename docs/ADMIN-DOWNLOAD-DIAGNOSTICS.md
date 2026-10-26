# Admin acquisition diagnosis (Issue168 candidate)

Bounded read capability, not cancellation/recovery commands or production delivery.
Base main8094ff36704f contains homepage167; merged-main CI40329349106 succeeded.
Parent155 remains OPEN. AI131/private139 and production153 stay separate.

## Modules and dependency boundaries

`producthttp -> adminread.DownloadDiagnosticReads -> Service ->
DownloadDiagnosticReader -> AdminReadRepository -> PostgreSQL`.

The additive optional port leaves legacy Reads/Reader and consumer owner-only
task contracts unchanged. Runtime already composes Service/AdminReadRepository;
no new worker/provider/IndexCore dependency or composition fallback is introduced.
Detail and existing list share the same frozen SQL Manifest-to-Job linkage
projection and domain state-pair validator. New browser detail module receives
verified ADMIN GET, identity/session, active-tab and configured-Binding ports;
AdminLists retains list/filter ownership. Neither receives a command port.

## Routes and data matrix

| Route | Port / result | Scope |
| --- | --- | --- |
| GET /api/v1/admin/downloads?limit=20&status=ALL&user_id=UUID&cursor=opaque | SearchDownloads / DownloadPage | Configured Bindings, product-owned Manifests; no total claim |
| GET /api/v1/admin/downloads/{manifestID} | DownloadDetail / DownloadDetail | Exact UUID, same Binding allowlist and persisted owner |
| Existing /api/v1/tasks/{id} | Unchanged | Consumer owner-only; never used for ADMIN detail |

Fresh session and ADMIN authorization on every read; 401 anonymous/revoked,
403 USER, 400 malformed/repeated/unknown query, 406 incompatible Accept, 405
unsupported method (including HEAD), 503 missing port/corrupt/unavailable data.
404 DOWNLOAD_NOT_FOUND conflates absent, unowned and outside-config detail, avoiding
scope disclosure. A corrupt visible link is 503, never a healthy invented record.
Responses are no-store. Existing Origin protections still apply to rejected writes.

Status filters are exact **Manifest** states: ALL, PENDING, ACTIVE,
AWAITING_VISIBILITY, AWAITING_CANONICAL, READY, FAILED, CANCELED,
RECOVERY_REQUIRED. Not Job states or age-derived categories. Empty status is
rejected when explicitly supplied; omitted defaults to ALL. Owner UUID/cursor/limit
remain server-validated. Unknown status/offset/SQL-like fields are rejected.

UUID ascending keyset paging, at most limit+1 rows, no OFFSET or sampled totals.
New opaque canonical base64url cursors bind after-ID, exact owner/status,
current administrator ID and sorted configured-Binding fingerprint. Reusing a
cursor with another filter/principal/config is rejected before the query. Cursors
are **not authentication, signatures or stable snapshot tokens**: every page still
reauthorizes and reapplies scope; concurrent transitions can change later pages.
Legacy domain Downloads retains its UUID cursor API; HTTP compositions lacking
the optional port retain legacy unfiltered reads but fail unavailable for status.

## Detail evidence and redaction

One bounded SELECT in a read-only REPEATABLE READ transaction joins exact Manifest,
Job and result Copy, with database read_at from the same statement/snapshot.
Exact payload schema/idempotency/reverse pairing, budgets, known states and times
are checked before returning. Visible owner must exist. READY requires its exact
Copy in the same Binding; Copy PRESENT or REMOVED is valid historical evidence.
Historical READY/SUCCEEDED is not rewritten when a file later disappears.

Allowlisted fields: existing Download DTO plus source
CLOUDSITE_POSTGRES_MANIFEST_JOB_COPY, read_at, max_attempts, job_created_at,
job_updated_at, started_at, finished_at, next_attempt_at, error_recorded,
error_code and optional Copy {copy_id,binding_id,availability,updated_at}.
PENDING has no Job or Copy and zero execution counters; all optional Job/Copy
fields are null. Unknown/missing/broken metadata fails unavailable.

Raw last_error, source/name/path, payload/idempotency, provider references,
lease owner, native directory, credentials and upstream response never leave SQL.
Only exact persisted POLL_TRANSPORT_TIMEOUT/OTHER, POLL_HTTP_STATUS, POLL_DECODE,
POLL_PROVIDER_TEMPORARY or POLL_PERMANENT_OR_UNKNOWN strings are exposed. Any other
nonblank error is RECORDED_REDACTED. NONE means no recorded error, not health.
These are historical categories, not fresh provider diagnosis. Existing columns
do not establish last-failure/latest-attempt timestamps: UI explicitly labels
Job updated_at and first started_at rather than inventing those times.

## UI and extension contract

Server-side owner/status form resets pagination and outstanding detail reads.
Manifest links and exact-ID lookup open an inline keyboard-focusable detail panel;
manual refresh clears old facts while loading, close/Escape restores trigger focus.
State explanations distinguish waiting for canonical evidence, failed, canceled
and manual/unknown RECOVERY_REQUIRED. No cancel/retry/recovery/probe controls.

Abort plus latest-request, active-tab, principal and session generation fence
detail results across navigation, filters, logout, auth revocation and new reads.
List validates complete records before replacing rows, preserving explicitly STALE
same-page history on read failure; new-context failure cannot look empty/successful.
Details clear on failure and distinguish LOADING, NOT_FOUND and UNAVAILABLE.
Both use configured Binding checks; unknown fields never render. Preserve homepage
separate denominators, consumer nav165, sharing and user management.

Future states/diagnostic codes require explicit SQL/domain/HTTP/UI tests, not a
generic success bucket. Mutating commands need separate audited/idempotent ports
and approval; never reinterpret this read surface as recovery authorization.
No migration changes, production writes, source reads or native provider calls.
