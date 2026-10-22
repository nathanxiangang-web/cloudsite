# Admin UI-3B PR-A: account lifecycle boundary

Issue #155 authorizes this development slice from `c26678a87ca4`.
This is a candidate contract, not deployment or architecture acceptance.

## Modules and dependencies

- `adminops`: typed USER commands, validation and secret-free idempotency receipt.
- `adminread`: bounded user search/detail DTOs; no auth records or provider data.
- PostgreSQL adapters: actor/session authorization, account row locks, version CAS,
  soft-delete and revocations in the same transaction as the audit outcome.
- `auth`: existing bcrypt and database-time session checks, including deleted users.
- `producthttp`: fresh ADMIN authorization, strict JSON/Origin and allowlisted DTOs.
- UI and homepage aggregation belong to PR-B, not this backend PR. No fake buttons.

Domain packages depend only on narrow ports. No provider, IndexCore, shell or
filesystem dependency is introduced. Replacement stores must preserve the same
transaction, idempotency, authorization and ownership guarantees.

## Commands and invariants

Existing create and enable/disable remain. Added commands rename login, reset
password, soft-delete and restore. Every mutation requires a new request UUID and
the last observed `admin_version`. Completed request replays return the original
receipt; an intent with no result remains UNKNOWN and is never automatically run.
Passwords and password-derived digests never enter the intent or outcome ledger.
Reusing an ID is a receipt lookup, not a second password change.

All targets must be ordinary USERs, never the actor or any ADMIN. Thus self-disable,
last-admin removal and role escalation are categorically unavailable in this API.
Changing an ADMIN account or granting ADMIN requires a separately designed action.

Lock order is actor account -> actor session -> audit intent -> target USER.
Only USER targets are permitted, avoiding administrator/target lock cycles.
Product admission already locks the owning user before creating an operation;
deletion takes the conflicting account lock and refuses any nonterminal Manifest,
including PENDING or RECOVERY_REQUIRED. It never rewrites Job/Manifest/ProviderTask.

Rename preserves immutable user_id, task ownership and future private-path keys.
The old login is not a persistent identity. Rename/reset/enable/disable revoke all
current website sessions. Login creation uses the account lock and expected hash,
so a concurrent stale-password login cannot escape a reset or deletion.

Soft-delete disables the account, sets deleted_at/deleted_by, revokes all sessions
and owner shares atomically. Existing share grants are retained but unusable because
the share stays revoked; restore never un-revokes shares or sessions. Already issued
CDN URLs/in-flight byte transfers cannot be recalled; new product authorization is
denied. No physical drive/file/Copy deletion occurs. Login uniqueness is retained
for tombstones to avoid assigning historical ownership to a new person.

Restore requires a replacement password different from the old password and retains
the original user_id. The account remains disabled until a separate enable command.
All successful commands increment the CAS version, including idempotent state sets.
Uniqueness/conflict/active-download rejection is audited; commit uncertainty returns
ACTION_UNCONFIRMED and must be queried with the same request ID, not blindly retried.

## Read API

User search supports literal login substring or exact UUID, status
ALL/ENABLED/DISABLED/DELETED and UUID keyset pagination (max 50). Independent COUNT
and data query use the same read-only repeatable-read snapshot. Detail includes
creation/update/deletion metadata and total/nonterminal download counts; never hash,
session, credential, source URL, native reference or provider identity.

## HTTP contract for PR-B

All endpoints are beneath `/api/v1/admin`, authenticated ADMIN only, no-store.
Mutations require same-origin JSON, `request_id` and, except create/probe, the
observed `expected_version`. Unexpected/duplicate/null JSON keys are rejected.

| Method and relative path | Additional required fields | Result code |
| --- | --- | --- |
| POST `users` | login, password | USER_CREATED |
| PATCH `users/{id}` | enabled | USER_STATE_CHANGED |
| POST `users/{id}/rename` | login | USER_RENAMED |
| POST `users/{id}/password` | password | USER_PASSWORD_RESET |
| DELETE `users/{id}` | none | USER_DELETED |
| POST `users/{id}/restore` | password | USER_RESTORED (still disabled) |
| GET `users` | query: limit, cursor, q, status | items, total, next_cursor, read_at, scope |
| GET `users/{id}` | none | user fields plus download counters |
| GET `actions/{request_id}` | none | original actor's receipt |

`status` is ALL/ENABLED/DISABLED/DELETED (default ALL). HTTP login search is
case-insensitive, accepts literal login characters only, max64 characters; UUID
search is exact. Count reflects the filter but not the current cursor. Each page
is its own coherent snapshot, not a cross-request frozen export.

HTTP200 does not imply success: inspect `status` SUCCEEDED/REJECTED and `code`.
Expected denials include TARGET_FORBIDDEN, VERSION_CONFLICT, LOGIN_CONFLICT,
USER_STATE_CONFLICT, ACTIVE_DOWNLOADS and PASSWORD_UNCHANGED. UNKNOWN uses202.
Transport/commit uncertainty uses503 ACTION_UNCONFIRMED, never an automatic retry.
Unknown user detail uses404; invalid input400; unauthorized401/forbidden403.

## Schema and release

Append migration 17 only; migrations 1..16 are immutable. Deleted users cannot be
enabled. Existing rows default to nondeleted with unchanged IDs/hashes/sessions.
No production migration or deployment is authorized by this PR. Future rollout needs
consistent backup plus isolated restore/16->17 validation. An old schema16 binary
must not run on17. Rollback restores the backed-up database with a compatible binary;
never DROP new columns from the running database.

## Verification

Isolated PG16 and PG18: migration preservation, CRUD, original foreign-key ownership,
session/share/grant denial after delete/restore, duplicate login, USER/anonymous and
ADMIN-target denial, stale actor, CAS races, active-operation lock ordering,
rollback/backend disconnect, UNKNOWN receipts, search/count pagination and redaction.
Run unit/architecture, race, vet/build and existing UI regression tests. Neither
successful tests nor this document certifies production readiness or IndexCore D2.

### Isolated execution evidence (2026-10-22)

Compilation/testing host: operator-designated .50, Go1.27.1, GOMAXPROCS2,
GOMEMLIMIT512MiB. Two newly created `cloud155-disposable` PostgreSQL16/18 Alpine
containers, each CPU1/memory512MiB/PIDs128, loopback ports and `cloud155_test` DB.
No production DSN, provider credentials, shared volumes or account were used.

- Full PG18-enabled Go run:1509 passing test/subtest events,4 skips.
- PG16 integration/migration run:315 passing events,0 skips.
- Full Go race without database opt-in:1180 passing events,214 explicit integration
  skips. This is not substituted for real database tests.
- Final changed admin SQL/domain paths: all `TestPostgresAdmin` with race on both
  disposable PG16 and PG18; includes lifecycle, genuine admission lock contention,
  stale login/password fence and inherited admin operations/read/observation tests.
- Node existing frontend regressions:95/95. Read-only service safety:11/11.
- go vet/build and architecture import guards pass in the isolated run.

The4 PG18 full-run skips require separate opt-in instances/configuration:
`TestRefreshStepWithComposedPortAgainstRealPostgres`, the operator mapping DB test,
`TestControlledIndexCoreHintObservation`, and `TestControlledObservation`. GitHub
CI provisions those separately; no local live-provider/IndexCore D2 claim follows.
New lifecycle methods/subcases do not skip when the purpose DB is configured.

Initial local harness observations are retained, not hidden: the old DTO field-count
test needed the two explicitly allowlisted deletion fields; a deployment packaging
test requires a real Git checkout and cannot run inside a source-only tar staging
tree. Its migration head is updated16->17 without weakening byte/LF checks. It is
run against the committed Git tree, followed by exact-head GitHub CI. CI receipts
belong to the PR; this development document is not an acceptance or deployment GO.
