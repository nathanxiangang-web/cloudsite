# Admin UI-2B — implementation and review handoff

2026-10-22 (Asia/Shanghai), Issue147 / D-066. **Candidate for review, NOT merged,
deployed or Architect accepted.** Baseline `a645e90229b71209f9a6036f25583113df9f1281`
is the verified UI-2A PR146 squash merge; main CI39639475953 all3 SUCCESS checked.
Exact candidate head/CI receipt is recorded on its PR, not self-referenced here.

## Implemented / module responsibilities and dependency direction

| Module | Responsibility |
| --- | --- |
| `internal/adminobserve` | Provider-neutral minimal DTOs and MappingReader/RootReader ports; ADMIN, allowlist, bounds, identity/state/status validation, deadline/concurrency/rate and remap fences; only standard library + auth |
| `postgres.AdminObserveRepository` | One bounded parameterized READ ONLY REPEATABLE READ projection query per page; configured UUIDs anchor the join to Binding + Connection, no unconfigured rows or N+1 |
| `platform/adminindex` | Existing IndexCore RootStatusReadPort -> narrow RootReader adapter; Q9 only, typed redacted errors, no generic proxy/CLI |
| `platform/productserver` | Compose from existing pool, configured DisplayBindings and the shared existing IndexCore client |
| `producthttp/admin_observe.go` | Two new independent fresh ADMIN-protected GET-only routes, strict media/query/method handling and static errors |
| `web/admin-observe.js` | Separate configuration and manual-root controllers; injected auth/read ports, literal allowlisted DOM, abort/scope/session fences |

No new framework/dependency, engine, migration, index, provider or IndexCore source
change. Extend only by a separately authorized minimal port/DTO + adapter/security
tests + corresponding panel. Never import SDK/SQL/runtime into the read domain,
serialize storage domain structs, relax owner-only APIs or add mutable operations.
Architecture import guard explicitly covers the new domain.

## Data contracts and provenance

`GET /api/v1/admin/storage?limit=20&cursor=<binding_uuid>`: `{items,next_cursor}`,
default20/limit1..50, immutable UUID ascending keyset, limit+1 up to51, server
allowlist at most64 configured bindings. Not a whole-database connection browser.
Items contain only `binding_id`, approved configured `display_label`,
`connection_id`, `provider_type`, `binding_config_status`,
`connection_config_status`, `binding_updated_at`, `connection_updated_at`,
`indexcore_root_configured`. Display labels come from server config, not DB/provider
paths. Provider category is SQL-normalized to115 (115/115drive), openlist, rclone,
local, mock or OTHER; arbitrary raw provider text is not leaked as a category.

Join anchored on the explicit configured UUID array preserves missing-binding /
missing-parent corruption as an error instead of silent empty success. Only the
minimal fields are scanned. CredentialRef, ProviderScope/native115 directory,
OpenList path, server URL, DSN and other fields are never selected/materialized.
Storage pages do not select RootID, just whether a Root mapping is configured.

`GET /api/v1/admin/indexcore?binding_id=<exact_configured_uuid>`: one manual Root
read. Server-only exact mapping query resolves RootID. The adapter calls existing
Q9 with explicit deprecated/deleted lifecycle visibility, never Q4/Q5/Q8/refresh/
Hint/provision. Returned RootID must equal server-resolved RootID; lifecycle is
NEW/ACTIVE/DEPRECATED/DELETED, generation and any admission seq must be nonnegative.
After the response, one second mapping read must match the original metadata;
remapping or update rejects the result without probing a second Root.

Successful JSON contains only `binding_id`, `root_lifecycle_state`,
`current_generation`, nullable `last_applied_admission_seq`, `observed_at`,
`evidence_source=INDEXCORE_Q9_ROOT_STATUS`. RootID is not in the public DTO, selector
or query API. Zero and null remain different. Root configuration enabled/disabled
does not change into a provider reachability statement.

`observed_at` records **CloudSite successful read completion**, not upstream
observation time or data freshness. The UI separately labels client fetch/display
time. Admission seq is **not** asserted to equal a Q8 consumer/projector cursor.
Successful Q9 proves only that Root's Q9 responded; no full-system green badge.

## Limits / security / error semantics

- Fresh existing cookie auth + current ADMIN role on every request and domain
  authorization. USER403, disabled/revoked/unauthenticated401. Headers/query cannot
  forge identity. Existing Host/no-store/CSP/nosniff and same-origin transport remain.
- Strict query allowlists: storage limit/cursor only; Q9 exactly binding_id. Empty,
  duplicate, malformed, unknown parameters, raw Root/URL/provider input rejected400.
  Unsupported methods405, JSON media negotiation406; no new mutations.
- Each storage/root operation has a5s child context. Q9 max2 in flight and8 upstream
  read operations/minute **per process** (shared across administrators); rejection
  is immediate, no waiting/retry queue. Missing mapping/client never makes a call.
- Q9 mapping uses at most two bounded SQL reads and one Q9 operation. Storage list
  is one SELECT/page. No permanent transaction held across network I/O. Each read
  is coherent; observed metadata is not a lease/freshness guarantee after response.
- Root404 ->404 ROOT_NOT_FOUND (unknown, not a claim of deleted files); not-ready
  ->503 ROOT_NOT_READY; missing trusted client/mapping ->503 NOT_CONNECTED; busy
  ->429 OBSERVATION_BUSY/Retry-After60. Unknown/timeout/corruption ->503
  OBSERVATION_UNKNOWN; bad/unallowlisted Binding ->400/404. Missing composition
  read port ->503 READ_UNAVAILABLE. No raw IndexCore error message/URL in output/logs.

## UI behavior

Storage and IndexCore replace only those two missing panels. Configured statuses
use neutral CONFIGURED presentation, not green online.115 native session and
shared-account capacity stay NOT_CHECKED; OpenList and Q8 freshness NOT_TRACKED.
Manual selector has only approved labels/Binding IDs. Navigation/focus/visibility
does not probe Q9; one click triggers at most one request and disables repeat clicks.
No poll, fan-out, automatic retry or capacity call. Null/zero and unknown states
are explicit, failed same-context reads retain old data only as STALE, changed
Binding/cursor clears it. Canceled pending probes show no successful new evidence.
Session/role changes and logout clear data and abort in-flight requests. Old data/
401 cannot repaint or invalidate a new session. UI-1 logout failure retry retained.

Users, Global Downloads, My Tasks, resources, consumer pages and file actions are
preserved. Shares/Audit/Settings remain explained placeholders. No fabricated
online/usage/uptime/Q8/global-health KPI. Large numeric values outside JS safe
integer range fail frontend display validation rather than show rounded evidence.

## Tests / evidence and truthful limitations

- Full Go unit/architecture/race/vet/three CLI builds required; new domain covers
  pagination/scope/status/DTOs, lifecycle4, mismatch/negative/null evidence,
  remapping, no-client/no-root, cancellation,5s deadlines,2-slot concurrency and
  eight-probe budget with minute rollover. No secrets/runtime imports in domain.
- Existing real IndexCore client via controlled HTTP: Q9 method/path/visibility,
  all4 lifecycle values, nullable seq,404/not-ready/malformed/transport/timeout,
  exactly one call/no retry and redacted errors. Existing SDK/client unchanged.
- Isolated PG16 and PG18 tests:61 allowlisted bindings across4 pages, extra
  unconfigured bindings/connections excluded, ACTIVE/DISABLED config, poison
  credential/scope/mount/category, one query/page, coherent paired snapshot,
  actual read-only write refusal, missing parent/Binding, invalid/exact scope,
  cancellation, and real-cookie persisted role downgrade/disable.
- Previous82 Node cases retained plus6 new UI-2B cases, total88. Configuration
  literal projection, manual Q9/no auto probes, null/zero/malformed, failure/STALE,
  abort/tab/Binding/session fences and role loss. Full consumer regressions kept.
- Controlled pinned IndexCore `6f0eec85c59bd8cbe55011b0d9e512e0cafd6615`: new
  `TestControlledObservation/AdminQ9ScopedReadAgainstPinnedIndexCore` executes
  real Q9 through new domain+PG mapping+adapter, checks ACTIVE/generation/evidence.
  This is in the existing explicitly enabled CI PG18 E2E, not a skipped unit-only
  stand-in. Local `.50` does not run that external binary test; exact-head CI must
  confirm execution before handoff. Other lifecycle contracts use HTTP/unit fixtures,
  not newly initiated real Root lifecycle mutations.
- CI PG18's existing `TestPostgresAdmin` step includes both new observation tests;
  PG16 full repository regex includes them. Existing real-cookie tests and safety
  suites remain; no assertions weakened or optional skips relabelled successful.
- No live production/user-data/115/OpenList verification, login or production
  visual acceptance. Local tests use owned disposable network-isolated databases.

## Screenshots (synthetic isolated fixtures, not production)

Desktop1440 and mobile390 browser QA: real DOM navigation/manual query, factual
status classes and time labels, panel-internal table scroll, no page-wide overflow.
The fixture has an explicit test-data banner and is never embedded as fallback.

- [Storage desktop](design/admin-ui2b/storage-desktop.jpg)
- [IndexCore desktop](design/admin-ui2b/indexcore-desktop.jpg)
- [Storage mobile](design/admin-ui2b/storage-mobile.jpg)
- [IndexCore mobile](design/admin-ui2b/indexcore-mobile.jpg)

## Untouched / next stage

No merge/deploy, production commands/restart, protected secret read, native115
login/GetInfo/capacity/dir/file call, new acquisition/Job/Worker/retry/cancel/delete,
IndexCore source/scan/Hint/Q8 repair, DB schema/migration/index, owner ACL/auth TTL/
quota change, consumer / or /downloads/ content/preview/play/share/302 change.
Private folders139 remain PAUSED; no per-user GB quota. UI-3 audit/UI-4 writes
require separate authorization. Stop at one open PR for Architect review; local
and exact-head CI success is worker evidence, not Architect acceptance or release.
