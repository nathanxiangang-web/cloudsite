# Issue187: versioned library pages and durable presentation snapshots

State: implementation candidate for review, not a production rollout. The Owner
authorized publishing [the plan](PLAN.md) and implementing it in a separate PR.
Base main:9eb4111a705bc60d264c19a2a2cecfe6e0a9b255. Existing website1.0.2 and
IndexCore runtime/configuration/data are not modified by this development task.

## Problem and resulting behavior

Real authenticated Chrome observed18 serial list requests returning858 records
(343 files/515 discarded directories),16.115s from first list start to all list
responses finishing. The browser repeated this enumeration on its10s timer.
Visible category counts were partial loaded-array counts, not independent totals.
[Sanitized requests](chrome-before.json), [screenshot](chrome-before.png).

The opt-in path returns complete published counts and a50-file server page in
one data request. Search/category filters cover the published visible dataset,
not only the currently downloaded page. Counts and rows come from one immutable
publication. Background validation uses conditional GET;304 retains the actual
card nodes. A count/version change with identical current rows also retains
those nodes. Stale cursors reacquire the same page number in the latest dataset;
if the dataset shrank, the page is clamped to the last available page. Errors
retain the last success; authentication/scope revocation clears metadata.

An absent/corrupt cache produces a bounded preview with counts=null/“—”, never
zero-as-unknown or a partial count presented as complete. Complete publications
are persisted and restored without source IO. Unknown directory/file types are
not counted as files and are disclosed separately. No browser-persistent cache
or public/private cross-account response cache is introduced.

## Modules, dependencies and extension rules

| Module | Responsibility and dependencies |
| --- | --- |
| `libraryview` | Read-model sync, validated publications, classification and cursor/query rules. Depends only on IndexCore read ports, Storage metadata port and a snapshot Store; no HTTP/SQL/provider/Kernel writer. |
| `product.LibraryService` | Fresh Principal/Binding checks, configured library/download partition, exact optional Copy attachment for the requested page. Uses the existing product contracts, not invented ownership or Copy totals. |
| PostgreSQL adapters | Durable publication/CAS and bounded identity batch. `FOR SHARE` binds publication to current ACTIVE Binding/Root; locks cover database work only, never source HTTP. |
| `/api/v1/library` | Strict protected transport,50-row page, counts/revision/read time, optional numeric page recovery and cookie-varying private ETag. Authentication precedes every304. |
| `library-page.js` | Bounded16-entry conditional-response cache per authenticated controller; no disk persistence, native fetch receiver preserved. |
| `library.js` | Existing file/detail/player/download/share actions with snapshot paging. Only active layout is constructed; query/session fences discard late results. |
| Product composition | Explicit opt-in, startup privilege/DB restore, one owned/joined sync lane. Only consumer-library and cloud-download Bindings are synchronized, not unrelated admin Roots. |

The existing Copy lookup gains a narrow optional batch port. Production uses one
exact Root/resource-ID SQL batch per page Binding (maximum50 IDs); single-item
fallback remains for existing narrow adapters. No name/path fallback is added.
Physical-only files remain visible even when Copy projection is absent.

New presentation fields must be versioned and incorporated into the appropriate
layer. The current consumer contract still uses the existing physical name/path/
extension categories; this change does not activate the separate admin alias/
hidden/category overlay in consumer browsing. Copy attachment fields are read
fresh in the bounded batch and participate in the response ETag, so delayed Copy
updates are observable even if physical publication revision is unchanged.
Future consumer overlay support must include its own invalidation/version and
statistics rules; a physical-generation-only cache is not sufficient.

## Sync and correctness

- Q8 is polled in one15s background lane per product process, not per visitor.
  Unchanged Roots issue status/checkpoint/Journal reads, **zero Q6 enumeration**.
- Existing public Q7 exact resource reads update at most100 changed file IDs per
  batch. Canonical REMOVED is explicit removal evidence; HTTP404 or partial reads
  never delete a cached file. Current Root/resource identity is revalidated.
- Directory/unknown-type events, missing exact reads, large batches or gaps use
  complete Q6 reconstruction, preserving descendant/path semantics. No opaque
  event payload is interpreted as an authoritative path or identity.
- Complete Q6 publication is bracketed by Q8 checks and Root lifecycle checks.
  A changing traversal candidate is discarded; the previous complete snapshot
  remains visible. This is a presentation snapshot, not a new Canonical writer.
- A persisted checkpoint includes the exact Journal event digest. Prefix rewind/
  rewrite or a gap triggers a new complete baseline; a verified recovery can
  replace a rewound cache only with its previous publication CAS.
- Per-request multi-Binding data is a vector of immutable Root publications,
  not a claim of one cross-Root transaction instant. Cursors bind publication,
  filter and user; counts/selected page are computed from the same vector.
- Confirmation-generation/time-only changes do not invalidate file content
  revisions. Source errors set stale=true; counts remain last-success values.
- Exact Q5 revalidation closes a selected removed/replaced file. Absence from
  one50-item page is not deletion evidence; failed Q5 retains the player.
- Limits:50 HTTP page rows,16 browser cache entries,64 configured scopes,
 100 changed IDs,100K total retained canonical records,64MiB per persisted JSON,
 100K catch-up events and2min per Root sync. Exceeding a bound fails closed and
  retains the last publication; these are safety bounds, not capacity acceptance.

Continuous source churn can prevent a stable baseline; expose stale/partial
status rather than lowering correctness. Large directory updates still rebuild
their Root. Whole-dataset filtering currently uses bounded in-memory data;
20K/long-lived memory/CPU and background workload need separate measurement.

## Schema21, privileges and rollout

Migration0021 adds only `product_library_snapshots`. Historical0001..0020 bytes
are unchanged. No user/task/Copy/action/Canonical/Journal data is rewritten. The
20->21 migration and publication transaction/rebind/real non-owner LOGIN tests
run on PG16 and PG18 CI. Historical19->20 tests stay pinned to that boundary.

Protected product configuration opts in with:

```json
{"whole_root_library":true,"library_snapshots":true}
```

`library_snapshots` defaultsfalse; enabling it requires whole-root mode. Flag-off
keeps the existing whole-root scope and legacy UI behavior, starts no sync lane
and does not require snapshot write grants. A feature-only rollback uses the
same schema21 binary with library_snapshots=false. This does not undo schema21.

Apply [explicit runtime grants](runtime-grants.sql) as the relevant owner after
the separately authorized migration, then run the existing product preflight
with the actual runtime identity/config. Both preflight and serve check cache
SELECT/INSERT/UPDATE, Binding SELECT and a column UPDATE privilege needed for
the publication SHARE lock, before source activity/listeners. No automatic
GRANT, PUBLIC wildcard or SECURITY DEFINER is introduced. The recipe grants
only updated_at column UPDATE for this locking requirement, not Root/status
updates. See [PostgreSQL locking privilege requirements](https://www.postgresql.org/docs/18/sql-select.html).

Before a future real rollout: confirm product PostgreSQL identity/version (the
product DB is distinct from IndexCore), take a complete consistent backup,
restore it in isolation, validate upgrade and runtime grants, stop/switch only
the product service, preserve tasks/queues and verify public assets/API/session
and the scoped flag. Old schema20 binaries reject21; binary-only rollback is
unsupported. Whole-DB restore after new writes is a separate recovery decision
with potential new-data loss. No production migration/deployment is performed
by this PR or its CI.

## Reproduction and evidence

```sh
node --test test/*.test.cjs
go test -json -p 1 ./... -count=1
go test -race -p 1 ./...
go vet ./...
go build ./cmd/panta ./cmd/panta-db ./cmd/panta-staging
```

Destructive PostgreSQL tests require a disposable purpose-specific database;
packages sharing its schema must run serially or use separate databases. The
first local parallel attempt's failure log is retained, not called a PASS.

`test/e2e/librarypreview` is an explicit loopback-only synthetic preview command,
excluded from all product commands/images. It refuses any database except its
exact isolated PG18 endpoint/name, creates858 synthetic records with the same
category counts, records actual Fake HTTP and SQL query classes, and exposes
test-only add/remove controls. It never connects to real OpenList/115. Browser
and SQL timings from it are functional evidence, not production latency claims.
Final-head tests, browser receipts and hashes are listed in the evidence manifest.


## Functional receipt (2026-11-16)

[Machine receipt](evidence/receipt.json), [source SHA256](evidence/source-sha256.json)
and compressed raw test NDJSON are committed together. Local and dsh copies of
393 code/schema/asset/workflow files matched exactly; migrations0001..0020 match
main byte-for-byte. Final Go test logs contain no failures. Full PG18 has1832
passing tests/subtests and4 explicit skips,43 passing packages and5 packages with
no tests; full Race without DB has1392 passing tests/subtests and325 explicit
skips,43 passing packages and5 packages with no tests.
The separate real-PG18 library Race run passes2 tests,0 skips. Node211/211,
Vet, three product command builds,11 readonly-safety Python tests and3 deployment
safety tests pass. Initial failed attempts are retained with descriptive names;
the remote Python Git-archive test needed a real worktree and passed on Windows.
GitHub exact-head CI is a separate receipt in the PR, not inferred from local tests.

Real Chrome used the isolated PG18 preview through a loopback SSH tunnel. Its
[request timeline and DOM observations](evidence/chrome-after.json) are sanitized:
no cookies, credentials or request headers. Refresh uses one initial library data
request plus subsequent conditional polls,0 legacy list requests;343 complete
file counts and50 current cards are returned together. Search finds fixture-0300
outside the initial page; clearing search restores50 rows. Software filter shows42.
Add/remove produces343->344->343 while page2 and the same first card node persist;
idle304 also retains card nodes. [Screenshot](evidence/chrome-after.png).

Actual Fake HTTP/SQL counters surrounding add/remove show2 Q7 reads,0 additional
Q6 reads,0 single Copy lookups and7 page-sized Copy batches for8 library requests
(one request was an expected expired-cursor409). These are receiver/tracer counts,
not derived queue estimates. The old18-request/16.115s sample is production;
the new functional sample is synthetic. **No production millisecond speedup,
resource-use,20K capacity or long-term stability result is claimed.** The test
preview and tunnel are stopped after evidence capture. No production changes.
