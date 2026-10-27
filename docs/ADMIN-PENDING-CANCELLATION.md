# Audited pending-intent cancellation (Issue #170, backend candidate)

Base: main `9903279e4ee73a068f0c73b73fc965205b99444b`, PR #169 merged,
exact main CI 40342381266 successful in all three jobs. Issue #168 is closed.
This candidate is backend only, requires independent Architect review, and must
not be self-merged or deployed. Parent #155 and production release #153 stay open.

## Modules and dependencies

`producthttp -> adminops.Service -> PendingCanceler ->
postgres.AdminOperationsRepository -> PostgreSQL`.

The optional command port is independent of user/share/provider ports. A missing
port fails unavailable before reserving an action. Composition already provides
the repository; cancellation never receives a provider, IndexCore, file or network
client. Acquisition Activate/Submit implementations remain unchanged.

`adminread` and owner-only `product` reads recognize a CANCELED Manifest without a
Job. Overview distinguishes `manifests.canceled_without_job` (omitted when zero)
from canceled Jobs: `jobs.total = manifests.total - manifests.pending -
manifests.canceled_without_job`. All other state-pair validation remains strict.
No fake Job, counts, source references or execution results are synthesized.

## HTTP and frozen precondition

`POST /api/v1/admin/downloads/{manifest_id}/cancel-pending`

JSON body contains exactly `request_id`, `expected_owner_user_id`,
`expected_binding_id`, `expected_updated_at`. All IDs are UUIDs; the timestamp is
the exact microsecond-precision database `updated_at` from a fresh exact ADMIN
detail GET. Equivalent timestamp offsets canonicalize to UTC in the intent digest.
No caller-chosen state, SQL, provider reference, source or password is accepted.

Fresh active ADMIN/session authorization, Origin/Host protections, JSON Accept
and no-store apply. 400 invalid input, 401 invalid session, 403 non-ADMIN/Origin,
404 hidden/absent target, 405 unsupported method, 406 incompatible Accept,
409 changed precondition/non-isolated target or request-key conflict,
503 unavailable/unconfirmed persistence. A reserved result is 202 UNKNOWN.

Receipts contain the existing action/request/target/status/code/time fields and,
only for a visible coherent target, current Manifest state, Job ID (or null), and
updated_at. A broken foreign Job link is MANIFEST_UNAVAILABLE without projecting
that Job. Hidden, unowned, missing-owner and outside-allowlist targets share
MANIFEST_NOT_FOUND and never receive a Manifest projection. Disabled/deleted owners
are rejected. Payload, source/path, provider IDs, credentials and raw errors are
never returned.

## Transactions and concurrency proof

1. Existing Reserve commits one actor-scoped action plus intent digest. Same key
   and same intent returns its original result; a different intent conflicts.
2. Only a fresh reservation enters Apply. UNKNOWN is status-only, including a
   duplicate received while Apply is still running. Never automatically reapply.
3. Apply locks actor -> session -> action -> owner -> provider fence -> Manifest.
   The initial scoped owner probe is non-authoritative; scope/owner/tuple are
   rechecked under the Manifest row lock. Session expiry is rechecked after waits
   and immediately before the terminal result is appended.
4. Require PENDING, no Job, no result Copy/name, no linked/orphan acquisition Job,
   no provider task and no product admission ledger entry. Ordinary managed web
   admissions are deliberately rejected even if activation is briefly pending;
   this slice does not undo admission accounting or broaden their replay rules.
5. The CAS changes only Manifest state/updated_at, and appends the successful
   receipt in the SAME commit. Receipt/commit failures roll back both. An ambiguous
   commit is reconciled with GET /api/v1/admin/actions/{request_id}, never a new
   apply. Committed historical receipts are not rewritten after later state changes.

Activate already locks the same Manifest before inserting its Job. If Activate
wins, cancellation observes ACTIVE/a Job and returns a durable conflict. If
cancellation wins, Activate rejects CANCELED before any Job insertion. Submit in
its create/activate gap fails closed and a replay cannot resurrect the Manifest.
Provider claim uses the same fence before inserting a provider row; cancellation
does not lock a Job or touch provider state. Lifecycle and admission take the user
lock before their target work, matching this lock order. No new reverse lock edge
is introduced. Standalone generic Job insertion/raw SQL is not an acquisition
activation API; pre-existing orphan evidence is rejected rather than repaired.

## Migration and rollback boundary

Schema19 only extends the existing audited action CHECK allowlist; migrations1..18
remain byte-identical to the base. There is no acquisition/provider schema change.
Before schema19, the new action cannot be reserved. Development integration tests
use isolated PG16/18 databases; no production database is migrated here.

Production is separately tracked in #153. A later operator release must handle
backup/restore and schema16->19 compatibility explicitly. Do not roll an old
runtime back onto schema19 as though it were schema18, drop new audit history,
or resurrect canceled intents. Use the separately reviewed restore gate.

## Stage 2 and extension constraints

No frontend file, button, automatic command replay or capability claim is added.
The separate UI PR after backend merge + green main CI must consume the new
no-Job CANCELED read shape and overview arithmetic, implement a capability-aware
confirmation, preserve principal/request fences, and use GET-only UNKNOWN
reconciliation. Existing strict UI validators may display unavailable for the new
shape until that follow-up; frontend compatibility and live rollout are NOT
claimed by this backend-only PR. Complete that gate before exposing the command
to real operators. Do not fabricate canceled Jobs to satisfy older validators.

ACTIVE, queued/running Jobs, READY and RECOVERY_REQUIRED actions, retries, native
115 cancellation/deletion, private139 and AI131 remain outside this port. Future
commands require separate ports, preconditions, audit contracts and concurrency
proofs; do not widen this action into a generic state-changing endpoint.

## Validation

The isolated matrix covers both real blocked-database race winners, Submit gap
and replay, concurrent same/different-intent duplicate keys, two administrators,
revoked/expired/mismatched/deleted/demoted actors, disabled/deleted/absent owners,
scope and CAS failures, orphan/mismatched Jobs, provider/admission/result evidence,
receipt/commit rollback and ambiguous commits, redaction, exact reads and real
HTTP authentication/receipt status. Focused tests run on both PG16 and PG18.
Final validation results and exact PR CI are recorded in the PR description.
