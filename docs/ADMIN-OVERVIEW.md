# Admin overview read module (Issue 163 / PR-A)

Implementation contract, not production deployment or whole UI-3B acceptance.
Backend PR164 is merged (025813ca). Homepage167 is merged in8094ff36704f,
main CI40329349106 SUCCESS. Current Issue168 adds a separate read-only task-detail
and status-filter candidate; see [module contract](ADMIN-DOWNLOAD-DIAGNOSTICS.md).
No whole UI-3B acceptance or production rollout is implied by either slice.

## Homepage module (Issue166 candidate)

Dependency graph: `app.js -> AdminOverview -> injected verified ADMIN GET port ->
existing overview / DirectoryReads endpoints`. AdminOverview owns only homepage
read state, strict DTO allowlisting and rendering. Authentication, transport,
navigation and server aggregation retain their existing owners. The module has no
command, provider, IndexCore or persistence dependency and is independently tested
through the real HTML/JS DOM boundary. Never reuse write-command ports for reads.

Initial verified ADMIN entry and explicit refresh fetch unfiltered overview and
the existing one-item DirectoryReads page with its authoritative total, separately.
Both datasets display their own database read_at; neither is presented as a shared
snapshot. Validate exact source/scope, timestamp, every safe nonnegative integer,
bucket sums, incomplete definition, Job denominator and paired terminal counts.
Render only allowlisted fields; malformed/missing/503 responses are unavailable.
Real zero is visible as 0. Refresh clears old counts and timestamps while loading;
failure leaves dashes and UNAVAILABLE, never stale numbers labelled current.

Each read is fenced by current ADMIN identity, session generation, latest request
and active tab. Supersession/navigation/logout aborts the local read controller,
invalidates callbacks and clears counts. Existing server authorization rechecks
remain mandatory. No background task/status POST or automatic provider probe.
Homepage links navigate only to existing users, shares, downloads, resources,
manual storage checks and Q9 observation panels. Audit/settings remain honest
placeholders; consumer navigation remains independent (PR165).

Extensions must update this contract and boundary tests when adding states or
sources; do not infer metrics from list samples or combine Manifest/Job totals.
Synthetic browser fixture data is isolated in test/admin-preview.cjs and explicitly
labelled on screenshots, never shipped as runtime fallback. No migration or
production change is authorized by this UI slice.

## Modules and dependencies

`producthttp -> adminread.OverviewReads -> adminread.OverviewReader -> postgres`.
The additive ports do not widen legacy `Reads` / `Reader` requirements. Runtime
composition already injects `adminread.Service` and `AdminReadRepository`;
implementations without the optional port return unavailable, never zero.
No provider, IndexCore, scanner, Worker or mutation dependency is introduced.

## HTTP contract

`GET /api/v1/admin/overview[?user_id=<canonical UUID>]`. The sole optional filter
uses the existing admin downloads owner semantics. Reject unknown, repeated,
malformed or empty query fields. Fresh session/ADMIN authorization on every read;
401 anonymous/revoked, 403 USER, 400 invalid query, 406 incompatible Accept,
405 unsupported methods including HEAD, 503 read/port/data unavailable.
Absent Accept retains the existing JSON default. Responses are `no-store`.

Allowlisted response: `source`, `scope`, optional `owner_user_id`, PostgreSQL
`read_at`, `manifests` and `jobs`. No raw domain entities, source/name/path,
payload, reference, credential, failure detail or user authentication material.

## Scope and definitions

Source: `CLOUDSITE_POSTGRES_MANIFESTS_AND_JOBS`.
Scope: `PRODUCT_ACQUISITIONS_IN_CONFIGURED_BINDINGS`.
Include persisted manifests with non-null user ownership, target Binding in the
server-configured admin read set, and optional exact owner filter. Include disabled
Bindings and historical/disabled/deleted owners; these are persisted history, not
online capability. Exclude unowned/operator manifests and other Bindings. Do not
claim all database Jobs, all accounts, remote Provider tasks or root health.

`manifests.total` counts manifests once. State buckets are `pending`, `active`,
`awaiting_visibility`, `awaiting_canonical`, `ready`, `failed`, `canceled`,
`recovery_required`. `incomplete` is the first four, excluding recovery debt and
all terminal states. `failed` is exactly FAILED, not retries or recovery.

`jobs.total` counts only the unique Jobs linked to those manifests; PENDING has
no Job. Its state buckets are `queued`, `running`, `retry_wait`, `succeeded`,
`failed`, `canceled`, `recovery_required`. Job and Manifest totals are separate
denominators. Recovery is RECOVERY_REQUIRED on both sides, never inferred from
age, failure count or a missing provider response. READY is historical success;
later Copy removal does not rewrite that acquisition outcome.

## Integrity, consistency and extensibility

One fixed-size SQL aggregate in a read-only REPEATABLE READ transaction. The
statement's database timestamp accompanies the same MVCC snapshot. No sampled
page, application row loop, external calls or writes. Concurrent transactions may
make the result stale immediately; the next explicit read obtains a fresh view.

Check exact Job type/idempotency/payload schema and linkage, permitted pair states,
retry-budget shape, timestamps and READY result link, following admin downloads.
Unknown/missing/corrupt records remain in the denominator and invalidate the
entire response (503), not silently disappear into zero or a healthy category.
The service independently validates bucket sums, paired terminal totals and owner,
scope/source. Nonnegative counts and all sums must fit JSON's exact safe integer
range 0..9007199254740991; overflow/inconsistency fails unavailable.

Future states require an explicit contract, SQL and test update, not a generic
"other" success bucket. User counts still belong to DirectoryReads. Sharing,
115 capacity and IndexCore Q9 remain separately sourced modules. No migrations
or task transitions are added; migrations 1..18 remain byte-identical.

## Validation gates

Pure service/HTTP tests cover true zero vs unavailable, missing optional ports,
authorization renewal, strict negotiation/query, redaction, unknown/inconsistent
counts and overflow. Disposable PostgreSQL 16/18 tests cover all pair states,
more-than-page totals, filtered/unfiltered scope, removed historical Copy,
concurrent paired commits with a barrier (no timing sleeps), corrupt links and
unknown states, read-only enforcement, query and commit failures. Race, unit,
architecture, vet/build and exact-head three-job CI are required before acceptance.
No production database or Provider calls are used for validation.
