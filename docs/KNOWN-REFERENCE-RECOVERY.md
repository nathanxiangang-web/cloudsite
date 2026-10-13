# Known-reference recovery (Issue #62, Gate 3.18)

This is not a generic retry of uncertain downloads. The eight historical
`START_RESERVED` acquisitions remain frozen. No migration, new Job Engine,
download source, Manifest, Job, login, OpenList API or provider start is added.
The live pair must remain fenced until independent review and merge.

## Modules and dependencies

- `acquisition.KnownPollScope` binds Manifest/Job, Binding/Connection, provider,
  IndexCore Root and SHA-256 of the exact original source/reference UTF-8 bytes.
  It depends on domain contracts only, not PostgreSQL or the 115 SDK.
- PostgreSQL `KnownRecoveryRepository.ResumeKnownAcquisition` owns the paired
  transaction; the operator cannot mutate one row separately.
- The 115 adapter overrides only offline listing with GET over the pinned SDK
  request/response types. One HTTP request per bounded page, no hidden retry.
  SDK creation/crypto/cancellation code is unchanged.
- Execution wraps errors **only after a durable REFERENCE_KNOWN row**, dispatcher
  selects the poll transition, and the existing outcome repository commits it.
- `start-known-acquisition` composes the accepted runtime, filters claims and
  expired-lease recovery by one Job, checks the namespace before each stage,
  and adds a no-start execution fence. Normal start/preflight remain separate.

Extensions must retain provider-neutral domain contracts, exact immutable scope,
separate transaction ownership and independently testable adapters. Do not infer
that any unknown provider error is temporary or retryable.

## State and locking

```
RECOVERY_REQUIRED / RECOVERY_REQUIRED + REFERENCE_KNOWN
  -- exact identity + CAS + no lease + preserved budget --> ACTIVE / RETRY_WAIT
  -- ordinary scoped claim --> ACTIVE / RUNNING (generation + 1)
  -- GET DownloadStatus only --> normal provider handoff
  -- explicitly temporary poll error --> ACTIVE / RETRY_WAIT (failure + 1)
  -- permanent/unknown or exhausted budget --> RECOVERY_REQUIRED / RECOVERY_REQUIRED
  -- provider success --> AWAITING_VISIBILITY / RETRY_WAIT
  -- existing Hint, Q5/Q8, Projector, canonical finalization --> READY / SUCCEEDED
```

Recovery locks Manifest then Job then ProviderTask, validates bidirectional
payload/idempotency linkage, exact task/ref digest, namespace, no result Copy,
generation and both `updated_at` CAS tokens. It holds mapping share locks to
prevent namespace replacement during commit. Both rows use one PostgreSQL
`clock_timestamp()`; the retry deadline is database time plus a bounded delay.
An ACTIVE/RETRY_WAIT exact-identity replay is read-only; a new claim, other state,
changed identity or exhausted budget conflicts. Reference, source, result name,
business associations, failure count/max, generation and previous error are
never reset by recovery. Only ClaimNext increments the generation.

Poll handoffs lock the same pair and the known task. They validate owner,
generation and database-time lease validity, including **after lock wait and at
the final Job mutation**. A failed second mutation rolls back the Manifest too.
Poll-error replay requires the same generation and recorded closed category,
and does not consume budget again. Prior error text is retained but not logged.
Normal progress never refunds failure count or erases prior error history.

## Poll policy and diagnostics

| Evidence | Closed category | Automatic retry |
| --- | --- | --- |
| Typed network timeout | POLL_TRANSPORT_TIMEOUT | Yes, within original budget |
| Other network error | POLL_TRANSPORT_OTHER | No |
| Observed HTTP 429 / 5xx | POLL_HTTP_STATUS | Yes, within original budget |
| Other observed HTTP failure | POLL_HTTP_STATUS | No |
| Typed JSON decode error | POLL_DECODE | No |
| Explicit provider temporary contract | POLL_PROVIDER_TEMPORARY | Yes; this adapter currently asserts none |
| Unclassified, unknown status, login rejection, missing task | POLL_PERMANENT_OR_UNKNOWN | No |

No guessed status code is emitted for transport failure (status is 0). HTTP
success with API failure is not provider success. `errors.Is/As` retain the cause;
public `Error()` and operator diagnostics contain only closed category/type,
observed status and elapsed time. Never log raw cause strings, body, headers,
URL, ref, cookie or provider file metadata.

Each error increments the existing `attempt_count` up to `max_attempts`. If the
next failure reaches the original maximum, both rows enter recovery rather than
creating a terminal mismatch. No new retry budget is introduced (a Job with
max_attempts=1 stops on its first new failure). Retry backoff is database time
plus 5, 10, 20, 40, 80 or 160 seconds, capped at 300 seconds. No HTTP retry is
hidden inside one claimed stage.

`event=STAGE ... poll_handoff=RETRY_WAIT` means the temporary poll failure was
**already durably committed**; do not kill it as FIRST_ERROR_STOP. An uncommitted
`TRANSIENT_ERROR`, failed fence/transaction, or paired recovery stops the scoped
entry. A real READY/SUCCEEDED handoff stops normally. If a transaction cannot
commit, retain the lease and use existing D-035 only after actual DB expiry; no
success, manual SQL requeue, budget refund or direct binary bypass is allowed.

## Protected operator invocation after architect review

Do not create an executable live GO before accepted merge. Inspect the single
approved pair, remote task and unchanged old-eight hashes first; retain the
original C3-only credential, four coordinates and exact reference. This document
is not a live GO and does not approve any other pair.

Use a new owner-protected directory (0700) and GO JSON (0600), reviewed against
the selected binary SHA externally. The GO must contain:

```json
{
  "execute": false,
  "binary_sha256": "64_LOWERCASE_HEX_OF_REVIEWED_EXECUTABLE",
  "expires_at": "REPLACE_WITH_APPROVED_WINDOW_END",
  "request": {
    "scope": {
      "manifest_id": "EXACT_APPROVED_UUID",
      "job_id": "EXACT_APPROVED_UUID",
      "binding_id": "EXACT_APPROVED_UUID",
      "connection_id": "EXACT_APPROVED_UUID",
      "provider_id": "115",
      "root_id": "EXACT_APPROVED_ROOT",
      "reference_sha256": "64_LOWERCASE_HEX_OF_EXACT_REF",
      "source_sha256": "64_LOWERCASE_HEX_OF_EXACT_SOURCE"
    },
    "expected_claim": 22,
    "manifest_updated_at": "EXACT_SNAPSHOT_TIMESTAMP",
    "job_updated_at": "EXACT_SNAPSHOT_TIMESTAMP",
    "retry_delay_ns": 5000000000
  }
}
```

The template is intentionally non-executable. The reviewer/operator supplies the
specific approved values; do not publish IDs, raw refs or secrets. Admission
uses PostgreSQL time and limits the whole GO window to 20 minutes. The explicit
commands are:

```
PANTA_KNOWN_RECOVERY_GO_FILE=<protected-go> panta resume-known-acquisition
PANTA_KNOWN_RECOVERY_GO_FILE=<same-protected-go> panta start-known-acquisition
```

Existing protected database/runtime/bootstrap configuration is also required;
no secret is a CLI argument. Before either real action the command creates an
exclusive fsynced `.reserved` marker in the protected directory. It is never
removed/reset on crash or error. Start requires the resume reservation's exact
GO byte SHA-256, preventing adoption of a modified tuple. Each command hashes its
own executable against `binary_sha256` before reservation. A previously failed
resume also cannot start unless its durable pair proves ACTIVE/RETRY_WAIT at the
expected generation. Both markers deny duplicate invocation. Keep the reviewed
GO bytes immutable and retain failed reservations as evidence.

Start claims/reclaims at most that one Job, never unrelated queued or expired
work. Loss/change of the known task or namespace fails closed before a provider
operation; there is no fallback to StartDownload. An external launcher must not
override this scope or reinterpret a committed retry as an uncommitted error.

At window end join the worker, read back pair/ref/generation/budget and Q8/cursor/
Copy/result_copy_id, restore only C3 Binding/collector permissions using existing
accepted controls, and check old-eight hashes. Running 0% is not download
success; a safe RETRY_WAIT/recovery result is not READY. Do not cancel/delete the
provider task, manufacture success, clear reservations, or add a second start.

## Verification

Unit tests cover typed/redacted poll errors, GET and exact request count,
no-start negative cases, default-denied protected GO, durable concurrent
reservation, tuple changes, and committed-retry versus fatal-stop handling.
PostgreSQL tests cover paired commit/replay, preserved identity/budget/history,
unknown/missing/mismatched rows, concurrent recovery/claim, backend termination,
lease expiry between mutations, bounded errors, the real 115 adapter + execution
+ dispatcher chain, exact-job DB-time claims, and eight frozen reservations.
CI runs these on PostgreSQL16 and a separate PostgreSQL18 test database, alongside
existing unit/race/vet/build, architecture and controlled IndexCore observation.
