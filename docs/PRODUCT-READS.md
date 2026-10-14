# Product task and resource reads - D-042 Stage 2A

Status: implemented under review in Issue #79 after auth PR #78 acceptance.
This is a read-only slice, not submission, accounting, access URLs or sharing.
Auth PR #78 merged at `eeedd3af242353822c0541cd1e74e48475fe596d` after exact HEAD
`95ca01a6cf450b20f4f2e357587cbeb31b84b78a`, CI `38506025321` all three groups
successful. Read handlers exist but there is no listener, UI or live deployment.
Stage 2B separately implements exact-source admission and durable quota/usage.

## Modules and authority

`producthttp -> product.Reads -> narrow TaskReader/TopologyReader + IndexCore
ReadPort/ResolvePort + existing ResourceView`; PostgreSQL implements owner-only
task reads. Product module owns policy and allowlisted DTOs, not physical truth,
Job transitions, credential resolution or a second Copy writer. HTTP imports no
SQL, store, provider SDK or integration adapter. Architecture tests enforce it.

Every route requires the accepted auth middleware. `auth.Principal` identifies
an enabled product user; never accept user_id, role, provider or Hint identity
from request headers/query/body. USER and ADMIN both use owner-only task reads;
there is no implicit admin/global/historical-task endpoint in this slice.
Missing dependencies fail closed. No fake production fallback.

The operator supplies a nonempty immutable allowlist of Binding UUIDs. Users
can read only these designated shared resources. Requests specify a Binding,
never an arbitrary IndexCore Root or provider/OpenList path. Topology resolves
the Binding's exact IndexCore root. Disabled Binding may still expose historical
reads when allowlisted; it never grants provider mutation. Credentials, native
scope and OpenList mount configuration are absent from DTOs.

## Frozen endpoints and bounds

- GET `/api/v1/tasks?limit=20&cursor=<manifest_uuid>`: only current user's
  manifests in allowed Bindings, ordered by Manifest UUID ascending. Limit
  1-50, cursor optional valid UUID. Result `{items,next_cursor}`; use one extra
  row to determine continuation. Empty items is normal. No global total count.
- GET `/api/v1/tasks/{manifest_id}`: owner filter before returning any data.
  Missing, unowned or nonallowlisted task all return 404, including ADMIN.
- GET `/api/v1/resources?binding_id=...&parent_resource_id=...&cursor=...&limit=20`:
  one Q4 Browse page from that Binding's root, default visible-only semantics,
  limit 1-50; opaque parent/cursor bounded to 2048 UTF-8 bytes, no NUL. No removed,
  deprecated or deleted visibility override supplied by callers.
- GET `/api/v1/resources/resolve?binding_id=...&path=/...`: exact Q5 through the
  accepted PhysicalService; no heuristic matching. Path uses existing normalized
  scope validation (max 2048 characters); no backslash/NUL/dot components.
  Zero matches -> 404; ambiguity or more than one match -> 409. Never pick one.

Unknown/duplicate query parameters, invalid bounds/UUID/path -> 400; no session
401; nonallowlisted Binding 404; corrupt upstream/DB linkage or dependency
failure 503. Error bodies contain static codes only. All responses no-store,
nosniff; outer router retains configured Host and mutation Origin protections.
Each application read has a bounded 10-second context timeout, caller
cancellation preserved. No unbounded lists or indefinite request work.

## DTOs and consistency

The PostgreSQL adapter reads both aggregates in one read-only REPEATABLE READ
snapshot. Concurrent paired stage commits cannot combine an old Manifest with
a new Job. An isolated test commits such a writer between those two reads and
verifies the old consistent pair, then the new consistent pair.

Task DTO fields: manifest_id, nullable job_id/job_state, manifest_state, nullable
result_copy_id, claim_attempts, failure_count, created_at, updated_at. No raw
source, expected/result name, provider task reference, payload, last_error,
lease owner or secret. PENDING may have no Job. Every other state requires the
same linked ACQUISITION Job and frozen idempotency/payload identity; terminal
pair must agree (READY/SUCCEEDED, FAILED/FAILED, CANCELED/CANCELED), and recovery
must pair RECOVERY_REQUIRED. Corrupt pairs fail closed, never auto-repair.

Resource: binding_id, resource_id, path/name/directory/size_bytes where provided,
presence, optional projected copy_id and copy availability, optional logical
asset/release/variant IDs only when existing ResourceView validated lineage.
Do not expose raw integration objects or Release.SourceRef/attributes. Returned
resources must be PRESENT, match the selected root, and carry valid identity.
A canonical resource without a projected Copy is a legitimate physical-only
read, NOT an access entitlement or READY assertion. Copy attachment must match
both canonical identity and selected Binding; no read path creates a Copy.

## Required tests and extension boundaries

Auth required before any read; forged client identity denied; two-user and
ADMIN isolation; null legacy owners excluded; nonallowlisted Binding denied;
deterministic bounded pagination; corrupt Job linkage/terminal disagreement;
Q4 root/presence mismatch, Q5 zero/ambiguous/multiple matches, malformed DTO,
redacted failures, canceled/timed-out reads; no provider/Hint/write calls.
Real isolated PostgreSQL ownership/pagination/linkage tests and authenticated
HTTP tests plus existing whole CI. Existing migrations remain unchanged; this
read slice adds no migration or actual listener/deployment.

Stage 2B cannot derive user identity from HTTP fields. It must reserve one
zero-price operation and one PENDING Manifest atomically, then use existing
two-step SubmissionService for activation without changing its durable PENDING
failure semantics. A same operation key replays stable IDs and never bills
another claim/poll. That later contract must define quota/idempotency/rollback
before implementation; it is not implemented by this read contract.
