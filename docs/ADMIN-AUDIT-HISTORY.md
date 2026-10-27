# Current-administrator product audit history (Issue174 candidate)

Base mainbb603f7e1dbe includes backend171 and UI173. Exact merged-main
CI40465107546 completed SUCCESS in all three jobs before development. This is
one draft code PR for independent review, not a production rollout or self-merge.
Parent155 and production153 stay open; AI131/private139 are outside scope.

## Scope, privacy and retention

Only the currently authenticated active ADMIN's own persisted product management
reservations/results are visible. An actor_id filter may equal that administrator
only; foreign list scope is403 and foreign exact identity is404 without a query.
This conservative MVP is not a global cross-administrator report. Deleted or
revoked administrators cannot authenticate to inspect their former history.

PROBE_115 requires its target Binding to be configured. REVOKE_SHARE and
CANCEL_PENDING_MANIFEST require the current persisted target to map into the
configured Binding allowlist. Missing/deleted/unmapped/outside targets are not
shown, including an old denied operation whose Binding cannot be established.
User lifecycle history is retained with target identifiers removed; a deleted or
missing user target is explicitly classified. No hidden-target enumeration.

This is the retained product action ledger only, not a complete system security,
115 native, download Job or IndexCore event log. No retention duration, export,
archive/cleanup, immutable-storage guarantee or all-time total is introduced.
Existing append-only application writers are unchanged. Restore/retention policy
belongs to a separately authorized operator process, not these GET routes.

## Modules and ports

`producthttp -> adminaudit.Service -> Reader -> postgres.AdminAuditRepository`.

The new domain owns typed filters, safe item contract, per-action result-code
validation, actor/config/filter-bound canonical cursors and safe errors. The SQL
adapter owns authentication rechecks, visibility joins and a bounded allowlisted
projection from the existing ledger. The HTTP adapter owns fresh protected ADMIN,
Host/Origin conventions, strict query/method/Accept checks and no-store responses.
Runtime composes a separate read-only port; missing/typed-nil ports fail503.
The descriptor advertises actual read composition at schema16+, never completeness
or liveness; the consumer descriptor advertises false.

`AdminAudit` has GET, principal/session/tab/capability and rendering ports only.
It cannot receive an execution/provider/Job/SQL/command/replay port. Existing user,
share, task/overview and safe cancellation modules keep their separate ownership.
Future actions require explicitly extending the SQL/domain/UI allowlists and
matrix. Unknown action/status/code, corrupt rows or unexpected receipt fields
fail unavailable, not a generic success bucket or silent fabricated healthy data.

## API and projection

- GET /api/v1/admin/audit/actions?limit=20&action=ALL&status=ALL&actor_id=UUID&from=RFC3339&before=RFC3339&cursor=opaque
- GET /api/v1/admin/audit/actions/{actor_id}/{request_id}

Limit1..50. Action accepts the existing nine commands or ALL; status is only ALL,
UNKNOWN, SUCCEEDED, REJECTED. Time range applies to reservation created_at with
inclusive from/exclusive before and microsecond precision. Empty/duplicate/unknown
parameters, offsets, raw JSON/SQL selectors and noncanonical cursors are rejected.
403/401 authorization,404 hidden exact record,400 invalid query,405 unsupported
method,406 incompatible Accept and503 missing/corrupt/unavailable reads are explicit.

DTO: actor_id (self), request_id, bounded action/status/code, created_at, nullable
recorded_at, target_status and optional enum manifest_state. Page/detail include
CLOUDSITE_POSTGRES_ADMIN_ACTION_LEDGER, CURRENT_ADMIN_ACTIONS_IN_CONFIGURED_BINDINGS
and DB read_at. No login, target UUID, digest, raw/nested outcome JSON, account
capacity, path/source, provider/native reference, credential or arbitrary error
text crosses SQL into the public domain model. Known private-shaped user/capacity
fields remain in SQL and are omitted; unexpected keys/types or broken pairing are
unavailable. Successful probe recording does not assert provider health.

UNKNOWN is a durable reservation WITHOUT a terminal row. A stored UNKNOWN or
malformed terminal result is not reinterpreted as a missing receipt. Recorded
results must match their reservation/request/action/target and timestamp; code
must match action and terminal status. Reading never reserves/completes an action.

## Snapshot, pagination and authorization

One READ ONLY REPEATABLE READ transaction reads the reservation/result pair,
visibility and classifications coherently. Ties use created_at DESC,request_id DESC
under the fixed self actor. Limit+1 produces a cursor, never an inferred total.
The cursor binds exact action/status/time filters, administrator and sorted Binding
fingerprint. It is a paging context, not authorization or a signed permission;
every page reauthorizes and reapplies SQL scope. Pages are independent snapshots:
a concurrent terminal commit may change later status-filter membership.

Initial identity/session checks use the snapshot; after closing it, a current-pool
SELECT rechecks enabled ADMIN, matching principal/login, nondeleted user, unrevoked
session and DB-time expiry. Returning the transaction connection first prevents a
single-connection pool from waiting on itself. No row/advisory locks or writes,
provider/IndexCore reads, direct SQL recovery or native task calls are performed.

## UI behavior and evidence

The actual audit destination supports action/status/UTC reservation-time filters,
keyset next page and exact evidence detail. It labels all three result states,
masked/deleted targets, source, read time and no-confirmed-result history. UTC
filter controls and browser-local display times are explicit. Microsecond ordering
uses full precision instead of millisecond rounding. No operational action button.

LOADING, scoped empty results, same-query STALE, unavailable and not-found are
separate. New filters/cursors/targets, auth/session/principal/tab/Binding changes
fence pending results; logout clears private rows/facts. Deadline errors clear
partial detail and re-enable refresh. Escape restores the opening record's focus.
Fixture screenshots/keyboard/mobile evidence are in operations/admin-audit-ui/QA.md.

## Compatibility and release

All migrations1..19 and cancellation/acquisition persistence contracts are unchanged.
The audit SQL adapter has isolated schema16 tests for legacy reservations/results;
missing tables fail unavailable. This is not permission to run a schema19 binary
against schema16 or a claim that other new modules work on an old schema. Existing
startup compatibility guards remain. Production153 owns backups/restore,
schema16->19, runtime/assets and rollback, with no deployment/migration here.

Validation includes real PG16/18 snapshots, code/action matrix, UNKNOWN, secret and
malformed JSON rejection, ties/cursors/filter isolation, cross-actor/Binding denial,
revocation during reads, deleted targets, schema16, missing tables, one-connection
pools and real HTTP sessions with unchanged action/provider counts. All Node suites,
full Go/race/vet/build and exact-head three-job CI are recorded in the draft PR.
