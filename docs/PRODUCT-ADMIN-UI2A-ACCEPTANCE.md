# Admin UI-2A — implementation and review handoff

**Historical handoff:** PR146 is now verified merged at
`a645e90229b71209f9a6036f25583113df9f1281`; merged-main CI39639475953 all3 SUCCESS.
Issue147 separately authorizes the [UI-2B candidate](PRODUCT-ADMIN-UI2B-ACCEPTANCE.md).
Candidate-era scope/evidence below is preserved, not current execution status.

2026-10-21 · [Issue145](https://github.com/nathanxiangang-web/cloudsite/issues/145)
· D-066 · **implementation candidate; NOT merged, deployed or Architect accepted**.

Authorized baseline: `09a53df34de6731b09eb78f3d22db72d2ff53f2c` (UI-1 PR144).
Merged-main CI39588808706 was independently checked: all3 jobs SUCCESS. Issue143
closed. Exact candidate SHA and its CI receipt belong in the PR handoff, not a
self-referential SHA inside this commit. No production access in this task.

## Implemented scope

- Real paged product-user list, including ADMIN/USER and enabled/disabled users.
- Real admin list of owned acquisition Manifest/Job summaries, optionally filtered
  by exact product-user UUID, only inside configured DisplayBinding allowlist.
- Separate Users and Global Downloads panels, source path, client fetch timestamp,
  refresh/next-page/filter controls and UUID-based user-to-task navigation.
- Owner-only My Tasks remains a different API and UI scope. Other five future
  modules and global counts/health/audit/actions remain unconnected, without fake
  totals, trends, provider progress, fabricated users or ownership reassignment.

## Modules, dependency direction and extension constraints

| Module | Responsibility / port boundary |
| --- | --- |
| `internal/adminread` | Minimal typed DTOs, ADMIN principal validation, ordering/cursor/page validation, allowlist and paired-state checks; only auth + standard-library imports |
| `adminread.Reader` | Bounded sorted projection pages, never raw User/Manifest/Job, secrets or provider records |
| `postgres.AdminReadRepository` | Parameterized single SELECT per page, read-only REPEATABLE READ transaction; one LEFT JOIN for the entire Manifest/Job page; no N+1 |
| `producthttp/admin_reads.go` | Protected GET-only routes, fresh server role, strict query/media negotiation, static redacted errors, missing port fails closed |
| `platform/productserver` | Composes repository/service from existing pool and server-configured Binding IDs; no new listener/provider dependency |
| `web/admin-lists.js` | Independent paged view controllers with injected read/session/error ports; allowlisted literal DOM fields, request/scope/session fences |
| `web/app.js` | Existing auth/logout/navigation integration; owner task/resource controllers stay separate |

No new dependency, framework, migration, database index or provider job engine.
To extend: authorize a new Issue and minimal DTO/port first, then its adapter and
HTTP role/ACL tests, then the corresponding admin-only panel. Never add generic
SQL/SDK calls, raw serialization, impersonation or writes to these read ports.
The import architecture guard explicitly enforces the new domain's boundaries.

## HTTP and paging contract

`GET /api/v1/admin/users?limit=20&cursor=<user_uuid>`

Response `{items,next_cursor}`. Each item contains **only** `user_id`, `login`,
`role`, `enabled`, `created_at`, `updated_at`. No login filter or user detail API
is introduced in this bounded stage.

`GET /api/v1/admin/downloads?limit=20&cursor=<manifest_uuid>&user_id=<optional_uuid>`

Each item contains **only** `manifest_id`, `owner_user_id`, `binding_id`, `job_id`,
`manifest_state`, `job_state`, `result_copy_id`, `claim_attempts`, `failure_count`,
`created_at`, `updated_at`. Job IDs/states and Copy IDs remain nullable, not fake
empty identifiers. Claim generation reads `jobs.claim_attempts`; failure budget
reads legacy column `jobs.attempt_count`, not the claim generation.

Both lists default20, accept1..50, query limit+1, order by immutable UUID ASC and
return the last visible UUID only when an extra row exists. Next pages replace
the current page. No OFFSET, mutable-time cursor, total count or offset guess.
Strict query parsing rejects unknown/duplicate/empty supplied cursor or owner,
malformed UUID/limit/encoding. Source URLs, expected/result names, target paths,
provider refs, native IDs, operation keys, payloads, credential refs, signed URLs,
hashes, error text, session/token/hash material never enter the admin read port.

The SQL evaluates frozen Job type/idempotency/payload linkage as a boolean without
returning those columns. Domain rejects broken/missing Jobs, mismatched state
pairs, invalid counts/identities/timestamps/order, wrong owner/filter or Binding.
Only PENDING has no Job. READY needs SUCCEEDED and stable result Copy ID; this is
**not** a claim that a file is currently PRESENT or playable. Corrupt lookahead
also fails the page closed rather than silently skipping it. Unowned historical
tasks and out-of-allowlist bindings are excluded before materializing rows.

One transaction/SELECT gives both sides of every pair a consistent MVCC snapshot.
Pages are individually consistent, not a pinned export across user clicks: new
rows inserted behind a cursor require refresh from the start to be discovered.
The domain imposes a10s child-context deadline and honors caller cancellation.

## Security and failure behavior

Every route goes through existing cookie authentication, then current RoleAdmin,
before read-port invocation. USER/forged role/owner headers cannot authorize it.
Database role downgrade or account disable takes effect on the next request.
401/403 clears both list caches, owner filter and cursors in the UI; old responses
cannot repopulate after navigation/logout/new login or overwrite a newer filter.
Existing UI-1 logout failure notice/manual retry/15s deadline is preserved.

- Unknown session401; ordinary USER403; invalid query400; unsupported JSON Accept406;
  unsupported methods405; unavailable/missing read port or corruption503
  `READ_UNAVAILABLE`. Errors do not echo database/request/provider secret strings.
- Existing Host, same-origin cookie/transport, no-store, CSP, nosniff and framing
  protections remain. No CORS widening or browser token storage.
- Loading, actual successful empty page, failure, and STALE same-context page are
  distinct. Changed owner/cursor clears the previous page; failed refresh does
  not claim empty/zero or a new observation. Timestamps explicitly say **client
  read time, not database observation time**.

## Verification evidence

Local `.50` fixtures only; Go1.27.1 build image and fresh network-isolated
PostgreSQL16/18 containers. No production database, real115, OpenList, credentials
or protected operator scripts were used. Temporary fixture containers are removed
after testing; source snapshot retained for reproducibility.

- Domain tests cover principal/role, constructor and scope copying, limit+1,
  immutable cursor/order, malformed projections, state matrix/null semantics,
  DTO field allowlist, static error redaction, cancellation and deadlines.
- HTTP tests cover anonymous/USER/admin, forged headers, bad cookies, fresh
  downgrade/disabled accounts, strict duplicate/unknown parameters, limits,
  media ranges including q=0, unsupported methods, Host, missing port and no-store.
- PG16 **and PG18**: 2,307 mixed-role/enabled users and 2,307 owned manifests each
  traversed in47 pages without gaps/duplicates; exclusion of unowned/outside
  records, owner filter, empty result, one query per page, read-only transaction
  enforcement, concurrent atomic writer, exact claim/failure counters, broken
  type/key/payload links, mismatched pairs, deliberate missing-FK-row fixture,
  real-cookie cross-user admin read versus unchanged owner-only API, database
  role disable/downgrade and blocked-query cancellation/recovery.
- Node suite includes existing77 tests plus5 UI-2A DOM tests, total82: literal
  safe rendering, both scopes, cursor/filter, nulls, empty/fail/STALE, late replies,
  role invalidation and new-session isolation; all consumer regressions retained.
- Full Go unit/architecture/race/vet/three-command build and original Linux
  deployment safety suites are required. Exact-head all3 GitHub CI will be linked
  in the PR; no candidate is called accepted based on local tests alone.
- CI PG16's existing full repository tests include `TestPostgresAdmin*`; PG18 has
  an explicit separate admin-test database and step alongside unchanged controlled
  IndexCore E2E, avoiding concurrent schema-reset interference.

## Browser screenshots / limitations

Browser verification uses **explicitly labelled synthetic loopback fixtures**,
not production data and not a fallback embedded in the product. Real database /
HTTP contracts are separately verified by the isolated integration tests above.
Desktop1440 and mobile390 responsive checks: navigation, both lists, exact-owner
filter and clear, source/time labels, no page-wide horizontal overflow; tables
scroll horizontally within panels. This is not production visual acceptance.

- [Users desktop](design/admin-ui2a/users-desktop.jpg)
- [Downloads desktop](design/admin-ui2a/downloads-desktop.jpg)
- [Users mobile](design/admin-ui2a/users-mobile.jpg)
- [Downloads mobile](design/admin-ui2a/downloads-mobile.jpg)

## Explicitly untouched / next bounded phase

No merge/deploy, runtime replacement, source scan, download submit, Worker start,
original provider reservation/known-ref mutation, native directory change,
secret access, auth TTL/password policy/quota change, schema/index/migration,
consumer controller/content/preview/play/share/302 change or private UserSpace139.
No task detail/operation drilldown, audit stream, global metrics, management writes
or UI-2B/3/4. Subsequent admin capability requires a separately authorized Issue;
this worker stops at one open PR for Architect review. Issue145 stays open until
review/merge/merged-main CI, not closed merely because a branch's tests pass.
