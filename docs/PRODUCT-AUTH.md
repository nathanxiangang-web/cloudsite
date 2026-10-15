# Product authentication module - Issue #77

Status: accepted bounded module, PR #78 merged at `eeedd3af242353822c0541cd1e74e48475fe596d`.
Reviewed HEAD `95ca01a6cf450b20f4f2e357587cbeb31b84b78a`, CI `38506025321` all three
groups successful. This is not a running product deployment or full MVP.
Authority: D-042 / Stage 1 of [the MVP plan](GATE4-IMPLEMENTATION-PLAN.md).

## Responsibility and dependency direction

`producthttp -> auth.Access -> auth.Service -> auth.Store <- postgres.AuthRepository`.
The auth domain imports no HTTP, SQL, acquisition, provider or integration
packages. The transport imports no store/SDK. Architecture tests enforce these
boundaries. Later composition injects the real service explicitly; test fakes
are not a production login fallback.

- [Domain/service](../internal/auth/auth.go): product UUID, normalized ASCII
  login, USER/ADMIN, enabled account; operator-only Provision application method.
  Passwords require at least6 Unicode characters, at most72 UTF-8 bytes, without
  NUL; no case/digit/symbol complexity requirement. bcrypt DefaultCost is unchanged.
  This supersedes the original12-byte minimum at the human's explicit request.
  Duplicate provision fails rather than resetting an account.
- [Store](../internal/store/postgres/auth_repository.go): DB-time expiry,
  revocation, current account role/enabled checks. Session creation locks the
  enabled account and rechecks the password hash before insert.
- [Transport](../internal/producthttp/auth.go): login/me/logout handlers only.
  A Principal comes from a verified product session, never provider credentials,
  caller headers, client-chosen roles or Hint credentials.
- [Migration 0013](../migrations/0013_product_auth.sql): product users/sessions
  only; old migrations and historical Manifest attribution are unchanged.
  CurrentSchemaVersion is centralized in the migrator. The new build rejects
  unmigrated v12 rather than upgrading it automatically.

## HTTP contract

POST `/api/v1/auth/login` accepts exactly login/password JSON fields (8 KiB
maximum, duplicate/unknown keys rejected). Returns only user_id/login/role and
an opaque 32-byte random session transported as a cookie; token never in JSON.
Database stores only its SHA-256 digest. Session lifetime is one hour.

GET `/api/v1/auth/me` requires exactly one valid session cookie and returns the
same three-field Principal. POST `/api/v1/auth/logout` revokes idempotently and
clears the cookie (204). Invalid request 400; bad/missing/expired/revoked session
401; unsafe Origin/Host 403; missing dependency/backend failure 503. Error bodies
are static codes, never underlying database errors or credentials.

Cookie: host-only, Path=/, HttpOnly, SameSite Strict, Secure on HTTPS. Only
literal loopback/localhost HTTP configuration may omit Secure. All requests
require configured Host; mutations also require one exact configured Origin
and reject cross-site Fetch Metadata. No wildcard CORS or provider-token login.

## Evidence and extension constraints

Domain/HTTP tests exercise hashing/bounds/identity/errors/cancellation, strict
DTOs and cookies, origin/host denial and unwired fail-closed behavior. Real owned
PostgreSQL 16 tests cover DB-time expiry, revocation, role/disabled-account
changes, stale password hash, concurrent unique sessions, cancellation and
append-only upgrade preserving both null and legacy Manifest user IDs. The
entire PostgreSQL suite, race suite, vet, build and diff whitespace checks are
run before submission; exact-head CI/review evidence belongs in the PR.

This stage does not implement an operator provisioning CLI, rate limiting,
an HTTP listener, UI, product operation policy, usage, shares or live provider
calls. Before opening a product listener, the composition stage must add bounded
request/server shutdown timeouts and bounded login admission; production/public
exposure is not authorized. Later routes use this auth port, then enforce their
own target/owner policy, and preserve Host/Origin validation at the outer router.
Raw passwords/session tokens must never appear in URLs, logs, tables or DTOs.
Role changes invalidate cached assumptions: always use current store identity.

Existing Gate 3.18 database/binary stays v12. No live migration, account creation,
worker start or download was performed by these tests. Passing this stage is
not an assertion that the whole MVP is usable.
