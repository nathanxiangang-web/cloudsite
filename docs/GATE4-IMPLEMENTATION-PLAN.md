# Minimum usable MVP execution plan

Status: **ARCHITECTURE DECISION - implementation not complete**
Authority: D-042 and the user's 2026-10-14 instruction to perform Architect and
Developer roles and proceed without repeated stage authorization.

## Definition of usable

An operator can start the isolated product application, sign in with a product
account, browse Catalog/known-path resources, submit a supported exact source,
observe the same durable acquisition reach READY/Job SUCCEEDED, and access a
PRESENT Copy. Authenticated share create/inspect/revoke and a minimal zero-price
usage/quota ledger must work. Account identity is independent of 115 credentials.

These are **acceptance targets**, not current functionality. Existing accepted
code provides Gate 3 acquisition/canonical behavior; product auth, HTTP/UI,
access integration, concrete share and accounting are still missing.

## Execution model

One agent performs two explicit phases per bounded task: Architect writes or
amends the plan/contracts; Developer implements them. Then a separate review
pass checks actual diff, tests and exact-head CI before merge. This is a
same-agent review, not falsely labelled an independent reviewer. Ordinary
configuration/compile/test defects are resolved without another user approval.

Keep one concern per Issue/PR. Do not parallelize a fictional Worker team. Preserve
current work and historical records; no reset of uncertain provider reservations.

## Stages and evidence

| Stage | Deliverable | Completion evidence |
| --- | --- | --- |
| 0: audit and domain | Correct misleading docs, freeze minimum scope/roles and source-backed domain ownership | Diff/link/source checks, actual CI and review record |
| 1: auth/session | Product accounts, password verification, revocable bounded sessions and a protected identity endpoint | Isolated PG + HTTP tests for success/denial/expiry/revocation/errors; no provider calls |
| 2: product reads/submission/usage | Authorized bounded resource/task reads, exact-source submission via existing services, operation-level ledger and quota | Cross-user denial, idempotency/concurrency/rollback, controlled same-Job READY E2E |
| 3: access/share | Accepted access adapter for authorized PRESENT Copy; tracked share create/inspect/revoke through ShareProvider | Real adapter protocol tests, DB-time/uncertainty protections, zero duplicate side effects |
| 4: minimal interface and controlled operation | Small usable web interface, explicit startup/shutdown/configuration, integration with staging dependencies | Browser flow, login/resource/task/access/share evidence, safe shutdown and rollback |
| 5: closeout | Provider-replacement regression, optional Agent boundaries, honest limitations and runbook | Exact-head CI, real staged evidence where required, accepted feature matrix |

Stages 2-5 require concrete contracts from actual source before their coding
starts; this high-level table is not a substitute for inspecting their ports.
Stage 1 contract is frozen below. Later decisions are recorded by the delegated
Architect, not silently invented during implementation.

## Stage 1 frozen product-auth contract

- New `auth` domain/service with narrow user/session store and clock/random
  injection; PostgreSQL adapter outside domain; HTTP transport outside domain.
- Account: immutable server-generated UUID, unique normalized login name,
  password hash, `USER` or `ADMIN`, enabled flag and timestamps. No premium tier,
  signup from the public network, Provider identity alias or client-chosen role.
- Password: bcrypt using the already pinned x/crypto dependency, bounded UTF-8
  bytes (12-72). Never log/store plaintext. Operator bootstrap is explicit and
  reads a protected external source, not a default hardcoded credential.
- Session: cryptographically random opaque bearer, store only SHA-256 digest,
  expiry and revocation state, linked to the product user. No JWT role claims,
  provider-cookie login or trusted-Hint-token reuse. Verify account state and
  role from the store; DB time authorizes expiry/revocation in durable adapters.
- `POST /api/v1/auth/login`: bounded JSON login/password; 200 allowlisted user
  DTO plus HttpOnly host-only session cookie; bad credential 401; invalid request
  400; unavailable store 503. No token in JSON, URL or diagnostics.
- `GET /api/v1/auth/me`: 200 `{user_id, login, role}` only for valid enabled user
  session; missing/invalid/expired/revoked credential 401; backend failure 503.
- `POST /api/v1/auth/logout`: revoke the current session idempotently, clear
  cookie, 204; credential data never appears in the body.
- SameSite=Strict; Secure required on non-loopback HTTPS deployment. Non-public
  loopback HTTP development is the only permitted Secure-cookie exception.
  Mutating browser requests validate explicit configured Origin and host;
  no wildcard CORS. Later CSRF policy must preserve this boundary.
- No implicit auth fake fallback. Fakes are test injection only. Plain startup
  does not expose the service or start acquisition; product startup is explicit.
- Append product migrations and validate compatibility centrally. Existing v12
  staging DB is not automatically upgraded. Never edit old migration files or
  rewrite historical Manifest.UserID attribution.

Required tests: password bounds/hashing, missing/bad/disabled users, constant
public error semantics, session expiry/revocation, store failure/cancellation,
illegal Principal, DTO/cookie redaction, Origin rejection and zero provider calls.
Add real isolated PostgreSQL round-trips and HTTP contract tests before claiming
the account/session capability implemented. No UI or new live download in Stage 1.

## Product policy and isolation

For minimum MVP, all product resource/control operations require product login.
Authenticated users may view the operator-designated shared Catalog; task/source
details are owner-only. Admin access is explicit and redacted; no implicit
ownership of legacy unattributed tasks. No external/public signup or payment.

Product user data goes to Panta PostgreSQL only. Use an isolated MVP database
and owned process; keep `panta_gate318`, IndexCore DB, old services and the eight
unknown provider reservations unchanged. Source strings remain byte-exact.
Access may use an OpenList access adapter as allowed by the original MVP;
acquisition visibility still uses only IndexCore Hint/Q5/Q8/Projector.

Real final validation is bounded to dedicated staging resources. Each new
acquisition permits at most one StartDownload after a durable fence; uncertainty
stops that operation, never a blind second call. Sharing tests may only use the
operator-designated non-sensitive test object, preserve the object and revoke
the test share. Never expose secrets or alter unrelated service credentials.
Production/public-network exposure, payment, destructive data operations or
new third-party account provisioning remain outside this delegated MVP scope.

## Deferred / honest completion language

Recommendations, premium/payment, favorites/social features, polished UI and
autonomous Agents are not minimum-MVP prerequisites. Agent integration remains
optional/disabled until its own contract and evidence exist; do not call an
unavailable Agent implemented. Original full Gate 5/6 goals remain recorded in
MVP-BLUEPRINT; minimum usable **core MVP** is distinct from full planned product.

Do not report MVP usable until actual startup, authenticated user flow, resource
access, acquisition/canonical confirmation, share revocation and usage evidence
exist. A passing docs PR or an isolated auth test is only its own stage complete.
