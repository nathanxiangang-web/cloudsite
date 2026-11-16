# Cloud download session health — Issue189

This maintenance slice makes a rejected native115 session visible before a new
download is admitted. Website login and the server's native storage login are
different sessions. A healthy login is not evidence that a task was submitted,
downloaded, indexed or made READY.

## Module boundaries

| Module | Responsibility and dependencies |
| --- | --- |
| `internal/sessionhealth` | Provider-neutral status/checker/read port; one bounded cache and single flight per immutable runtime session. Standard library only; no secrets, database, actions, background goroutines or global visitor map. |
| `internal/providers/115/session_health.go` | One fresh read-only user-info GET using the exact immutable SDK cookie objects imported for this downloader. Fixed endpoint/UA, no redirects, retries, SSO LoginCheck, re-login, offline commands or source-link reads. |
| `internal/runtime` | Constructs one health reader per registered connection from its existing downloader; never re-reads a credential for the health result. Startup/preflight performs no provider request. |
| `internal/product` | Takes a health observation before persistence. Requires a checked store when health is configured. |
| `internal/store/postgres` | Fresh principal, existing owner serialization and exact intent replay/conflict first; gate only NEW reservations. Recheck observation expiry after owner and topology locks. Rollback all new quota/operation/Manifest state on rejection. No network inside the transaction. |
| `internal/acquisition` | Winning durable start reservation performs a fresh read-only check before StartDownload. Only this explicit no-start branch produces a safe closed failure; arbitrary StartDownload errors remain uncertain. |
| `internal/producthttp` / web | Protected status GET, explicit closed503 codes and Chinese status panel; preserve inputs/files, disable new submission, keep original-operation reconciliation available. |

Extensions must implement `sessionhealth.Checker` for the same runtime credential
snapshot. Do not add a second secret resolver, public credential input, unbounded
per-user health cache or provider-specific branch in the product domain. A future
provider without a checker is UNKNOWN in the managed product health gate.

## Read and admission contract

`GET /api/v1/cloud-downloads/session` requires a current website session. No query
parameters or caller-selected connection are accepted. It returns only `status`,
`checked_at`, `valid_until`. No credential, account ID, native target, source,
provider response body or arbitrary diagnostic text is returned.

Statuses: `AUTHENTICATED`, `LOGIN_REQUIRED`, `UNKNOWN`. Pending UI requests show
checking. User ID must match the imported credential's account. Missing/invalid
data, wrong identity,5xx,429, redirects, timeouts and transport errors are UNKNOWN.
The fixed authenticated user-info endpoint's explicit unauthenticated response
and auth codes99/990001 are LOGIN_REQUIRED. Failure JSON is parsed independently
of the SDK user-info data type. Response consumption is capped64KiB and wiped.

Each health reader coalesces concurrent visitors; success AND failure cache15s,
probe context5s. No visitors means no probes. New start winners additionally
perform one fresh GET before any offline submission. The cache is a short-lived
observation, not a promise that credentials cannot expire between GET and POST.
No health response may justify replaying a native command.

New intent rejection is HTTP503 `PROVIDER_LOGIN_REQUIRED` or
`PROVIDER_SESSION_UNKNOWN`, before any persistent new operation, Manifest, Job,
provider task or usage increment. Exact accepted operations retain replay and
conflict behavior during an outage. The existing operation GET does not probe or
submit. A canceled HTTP request never proves that an earlier submission failed.

After a201 response, credentials can still expire before Worker execution. A
winning reservation rejected by its explicit precheck commits FAILED with only
`PROVIDER_LOGIN_REQUIRED_BEFORE_START` or
`PROVIDER_SESSION_UNKNOWN_BEFORE_START`; no StartDownload is invoked. The durable
reservation is retained. Crash/cancellation/lease-loss before the paired failure
commit still follows the existing recovery fence. Existing START_RESERVED rows
without a reference remain uncertain and cannot be silently reset or reused.
An error AFTER StartDownload is invoked (including authentication errors or a
timeout) is never relabeled as this safe precheck failure.

The owner-facing Task DTO exposes only the two exact persisted no-start markers
on a valid FAILED pair. No substring/error-body matching or historical inference.
`claim_attempts` is a monotonically increasing scheduling/lease generation,
including polling and later stages; it is not an HTTP or download count.

## Frontend behavior

Managed workspace advertises `cloud_download_session_check=true`. The downloads
page checks health independently of file rendering, then refreshes at most once
per visible-page10s tick or manual click. Server coalescing/cache caps native
health requests independently of visitors. Expired/invalid views cannot enable
submission. Admission rejection keeps the same operation key, input and preview;
restoring login enables an explicit retry with that original key. There is no
automatic POST. Existing pending-operation GET remains available when health is
bad. Website401 retains its established sign-out/private-DOM behavior; native
login rejection uses503 so it does not log the website user out.

Historical pre-health servers/preview-only configurations retain their existing
capability behavior; the managed production composition always installs the gate.

## Incident evidence and limits

On2026-11-16 expired credentials were independently rejected by read-only native
user-info and scoped file APIs. An authorized same-account115driver QR refresh
restored authentication. A later authorized new task then timed out at the native
offline POST. A read-only list matched source hash, configured target and creation
time and reported100% completion. CloudSite still lacks a durable task reference;
this is NOT an end-to-end READY/Canonical/Copy acceptance.

This change preserves those historical records. Unknown-result task association
needs a separate audited identity/reconciliation design. Do not issue another
native start, guess a reference, set READY in SQL or reset its reservation.
The UUID task-list ordering and per-Manifest Job query fanout are separate
follow-ups. Their actual SQL latency is NOT_TRACKED in this incident.

## Verification and rollout

Tests cover strict read-only HTTP contract/same imported credentials, identity
mismatch, auth rejection, malformed/large/error/redirect/canceled responses;
single flight and failure caching; new admission rollback, expired observations,
concurrent rejection/recovery and exact replay/conflicts; safe precheck versus
actual POST uncertainty; frontend input/file preservation and no automatic POST.
Use dedicated disposable PostgreSQL18 and synthetic transports only.

Chrome screenshots use isolated synthetic data, not a production login failure
induced for testing. UI fixtures are not real115 throughput evidence. Test counts,
exact source HEAD, logs and CI are recorded separately after execution.

No migration, schema21 change, second writer, IndexCore/Canonical/Journal change,
cookie/QR administration capability or production configuration change. Retain
the serial Worker, existing Reader/budgets and full reservation fences. Deployment
is a separate versioned artifact/cutover, with exact CI, backup/restore, preserved
business state and rollback receipt. Restoring a native credential file alone
does not replace this runtime's immutable imported credential; reload the product
service through the existing controlled procedure. Historical quarantined tasks
do not automatically become resumable after re-login.
