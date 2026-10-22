# Admin LIVE UI-3A / Issue149: implementation handoff

Status: IMPLEMENTATION CANDIDATE, not production deployment or Architect acceptance.
Baseline: `97cb94fee932e231147c25c74bcbf773f27538fe` (UI-2B PR148 merged).
Merged-main CI39701070318: independently checked, all three jobs SUCCESS.
One fresh branch/PR. Private UserSpace139 remains PAUSED. No personal GB quota.

## Module boundaries and actual runtime data

| Module | Sole responsibility / dependencies |
| --- | --- |
| `adminops` | Typed commands/results, deadlines, admission/replay orchestration; auth and minimal adminread ID validation only; no SQL/HTTP/SDK |
| PostgreSQL admin operations adapter | Fresh transactional actor/session checks, durable intent/result, USER CAS and session revocation, connection cooldown |
| `providers/115/account.go` | Pinned 115driver v1.3.5 GetInfo, strict response validation and safe shared-account byte projection; no downloader calls |
| `platform/adminprovider` | Approved protected Connection+CredentialRef session binding and lazy secret resolution |
| product composition / HTTP | Real PG and protected provider port injection; fresh ADMIN, Host/Origin, strict request schema |
| `admin-operations.js` | Explicit commands, confirmation, busy/unknown handling, result lookup and session fences; no optimistic account-state changes |

Runtime has **no fixture/demo fallback**. Existing configured Binding and Connection
must both be ACTIVE, provider_type must be115, CredentialRef must exactly match an
approved session. Failure is NOT_CONNECTED/UNKNOWN, not synthetic online/zero bytes.
ProviderScope, OpenList mount, Root and provider account ID are not probe input/output.
The configuration list and Q9 remain distinct from provider health. IndexCore owns
file visibility; this capability neither queries OpenList nor scans/indexes files.

## Protected composition (no startup network probe)

Existing product `cloud_download.bootstrap_file` is reused by default. An operator
may explicitly set `admin_probe_bootstrap_file` in the protected product config to
use an independently approved existing session binding. This is a **file path in
protected server configuration**, not an HTTP parameter. It reuses the established
bootstrap format: protected secret_dir and provider_id/connection_id/credential_ref/
secret_file entries. Hint fields may remain in that file but this reader does not
load/use a Hint token or construct a Worker. Duplicate session bindings, unsafe
paths/permissions, unknown fields and unsupported providers fail closed. Credentials
are resolved on the explicit probe; rotation is read through protected file rules.
Absent probe wiring is allowed: USER management still works and a manual probe
records NOT_CONNECTED. Invalid explicitly supplied bootstrap fails startup closed.

Compatibility name `serve-product-readonly` / descriptor READ_ONLY still means
**no acquisition Worker**, not that authenticated ADMIN commands are unavailable.
No new default background operation, login/refresh, native directory or file action.

## Exact endpoints

All require fresh enabled ADMIN and a valid unexpired, unrevoked product session.
Commands additionally require exact Origin/Host and reject cross-site requests.
Requests are JSON objects <=4096 bytes; unknown/duplicate/missing/null fields,
query-string controls, unsupported media/Accept and malformed UUIDs are rejected.

| Method / route | Exact body / result |
| --- | --- |
| POST `/api/v1/admin/users` | request_id UUID, login, password; existing auth.Service.Register enforces USER and normal password policy |
| PATCH `/api/v1/admin/users/{user_id}` | request_id UUID, enabled boolean, expected_version integer; USER-only version CAS |
| POST `/api/v1/admin/storage/{binding_id}/probe` | request_id UUID only; server resolves approved mapping and credentials |
| GET `/api/v1/admin/actions/{request_id}` | Current administrator's one durable intent/result only; no global audit or bearer lookup |

Results contain request_id, action, opaque target_id, status, static code and DB-time
recorded_at. User success additionally gives user_id/login/enabled/version.
Probe evidence gives status, source115_GET_INFO, probed_at and **decimal-string**
total_bytes/used_bytes/remaining_bytes only on AUTHENTICATED. Decimal strings avoid
JavaScript rounding above2^53. Sizes must be explicit nonnegative int64 integer
bytes, total>0 and used+remaining=total. Missing/null/fractional/inconsistent data is
UNKNOWN. Device/IP/identity/URL/cookie/response bodies never leave the SDK adapter.
AUTHENTICATED is evidence at probed_at, not a promise of continued session validity.
Values are **shared native account capacity**, never user usage balances/GB limits.

No general execute endpoint, generic SQL, role promotion, password reset, bulk action,
ADMIN/self/last-ADMIN disable, download retry/cancel or physical deletion exists.

## Audit, idempotency and threat model

1. HTTP verifies current session. Each DB transaction checks and locks enabled
   ADMIN identity, then matching session (DB-time expiry/revocation). Actor is not
   caller-selected. Arbitrary Role/User headers cannot impersonate an actor.
2. Durable intent keyed by `(actor_id, request_id)` is committed **before** any
   command/probe. The intent digest binds action, target, login and expected state/
   version; it contains neither password nor a fast password-derived hash.
   Reusing a create key always refers to the original creation, even if a different
   password is supplied: replay never resets a password or creates another user.
   Changing nonsecret intent under the same key is409 ACTION_CONFLICT.
3. Existing auth registration/hashing is used through a transaction-bound auth
   repository. Registration uniqueness rejection uses a savepoint. USER changes
   and append-only confirmed outcome commit together; audit failure rolls back
   account creation/state/session revocation. Target locks exclude ADMIN/self
   **before locking**, avoiding cross-admin lock upgrade/deadlock scenarios.
4. USER state changes use admin_version CAS. Both disable and enable revoke old
   sessions in the same transaction; enable cannot resurrect a legacy disabled
   account's tokens. Login's existing user-row share lock coordinates creation
   of concurrent sessions. Previously authorized in-flight reads are not recalled;
   subsequent requests and newly attempted sessions respect committed state.
5. Probe admission CAS reserves one60-second cooldown per **Connection**, using
   PostgreSQL time across processes. <=8s request / <=10s operation, no redirect,
   SDK retry, fan-out or background polling. Exact mapping is checked before the
   call against the cooldown-reserved mapping and after it before publishing.
   One reserved operation calls GetInfo at most once; no other native endpoint.
6. Any crash/timeout/failed commit may leave intent without outcome. GET returns
   UNKNOWN / RESERVED_NO_CONFIRMED_RESULT (202). **Replay never resumes it**.
   The UI blocks new commands until it obtains a confirmed terminal result; it
   stores only the last opaque request ID under the actor UUID in sessionStorage
   and can look it up explicitly after reload. No automatic POST retry or polling.
   An abandoned UNKNOWN requires later operator investigation, not clearing or
   repeating a provider action. Provider calls are read-only but still not replayed.
7. Semantic rejections (duplicate account, version conflict, forbidden target,
   unconfigured scope, rate limit) have durable REJECTED outcomes. Authentication,
   Origin and malformed-body failures occur before intent admission and are HTTP
   refusals, **not** claimed to be a complete security-event audit feed. Raw DB/
   SDK errors and passwords are never included in API errors or application logs.

## Schema16 and deployment / rollback gate

Only append `0016_product_admin_actions.sql`; migrations1..15 are unchanged.
Adds product_users.admin_version(default0), immutable action intents, separately
appended terminal JSON outcomes, and DB-time per-connection probe cooldowns.
No download/provider history, file, Copy, Root, quota or existing session TTL migration.
All ordinary startup still refuses any schema other than the exact supported head.

**This implementation does not deploy or migrate production.** After Architect
review/merge and explicit rollout connection, the operator must:

1. Record current image/commit/config and schema15 status; freeze writers and take
   a protected, restorable full DB/config backup (including historical reservations).
2. Validate migration16 in an isolated restored snapshot. Test deployment image
   and protected session bindings without dumping secrets. No real canary in CI.
3. Explicitly apply the existing migrator under its lock, verify checksums/head16,
   then start the reviewed image. No schema auto-upgrade in HTTP startup.
4. Canary one designated approved configured connection manually through the
   authenticated route; record only classified state/probe timestamp and pass/fail
   checks of actual capacity, never live secret/account details in GitHub.
5. Test ordinary USER creation/state change only with an approved disposable test
   user, verify its sessions denied after disable and unchanged ADMIN/other users.
6. Rollback is **restore the matched schema15 backup + old image/config under
   writer freeze**, not launch the old exact-schema15 image against16 or drop
   audit tables/columns live. Preserve post-migration audit evidence separately;
   a restore loses later writes, so abort/coordinate if unrelated writes occurred.
   Transactional DDL rollback is tested in isolation, not a production down-migrator.

## Verification and honest provenance

Executed on development192.168.178.50 using pinned Go1.27.1 with independent
PostgreSQL16/18 containers, bounded CPU/memory. No production connection or secret
access. Test credentials are synthetic and only in test code/harnesses.

- Domain/HTTP: fresh ADMIN/session, denied role, Origin/Host, duplicate/unknown/
  null/oversize JSON, scope, safe errors, idempotent/no-resume and byte validation.
- Actual pinned SDK over recording transport: exact GET endpoint/UA, one call,
  integer/string sizes>2^53, missing/null/fractional values, rejected login990001,
  HTTP401/5xx/redirect, malformed/oversize body and cancellation. No live provider.
- Real PG16/18: USER create/duplicate/replay, version conflict between admins,
  disable/login/old-token revocation, actor disable/downgrade, shared cooldown,
  mapping change, pending reservation, audit constraint failure, real backend
  termination rollback, schema15->16 with preserved legacy data and DDL rollback.
- Node DOM: commands are manual, double-submit guard, disable confirmation/CAS,
  unknown->GET reconciliation, literal rendering, exact byte strings, old-session
  responses fenced; existing consumer/admin read suites remain mandatory.
- Playwright desktop1440 and mobile390: forms reachable, synthetic user create/
  disable and probe/receipt interaction, no page-wide horizontal overflow.
  Screenshots use explicitly **SYNTHETIC UI TEST - NOT LIVE115** fixtures only.

Browser layout/interaction evidence (synthetic; not provider/account acceptance):
[users desktop](design/admin-ui3a/ui3a-users-desktop.jpg),
[users mobile](design/admin-ui3a/ui3a-users-mobile.jpg),
[storage desktop](design/admin-ui3a/ui3a-storage-desktop.jpg),
[storage mobile](design/admin-ui3a/ui3a-storage-mobile.jpg).

Live protected-provider canary: **NOT_RUN**. Live USER mutation: **NOT_RUN**.
Production deployment/migration: **NOT_RUN**. No claim of live-account acceptance.
Exact-head CI receipt belongs in the PR/Issue handoff, not a self-referential SHA
inside its own commit. Architect review/merge remains a separate gate.

## Extension rules

New admin actions require an action-specific Issue/permission/typed port, durable
intent/result, replay semantics and independent tests. Do not extend this into
generic SDK execution, generic storage edits or acquisition repair. Provider
capabilities stay sealed in adapters; user state remains auth-owned and audit
atomic. Q9/consumer delivery/owner ACL/Worker state machines remain independent.
