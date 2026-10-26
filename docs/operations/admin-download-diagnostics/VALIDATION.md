# Issue168 implementation evidence

Base:8094ff36704f75f0b841533bd4bd57431b645aa6, merged homepage167, main
CI40329349106 completed/success verified before implementation. This is a
candidate for independent Architect review, not acceptance or production delivery.
Exact HEAD and three-job CI are attached to the implementation PR.

## Checks performed

- Windows Node all suites:161/161 PASS, no skips. CI explicit list has158 tests;
  the other three are existing file-surface.test.cjs, outside its explicit list.
- New frontend11 methods exercise exact ADMIN route, safe fields, all state-pair
  validation, owner/status/cursor filters, real PENDING, READY with REMOVED Copy,
  wrong IDs/states/budgets/times/error codes, 404/503/network, stale responses,
  logout/role revocation, principal/new-request/filter/navigation boundaries,
  latest same-ID refresh, duplicate rows and focus restoration.
- New domain tests check authorization before reads, optional-port failure,
  cursor binding to owner/status/actor/config, stable pagination, safe linked DTOs,
  corrupt records, no-Job state and unavailable/not-found classifications.
- HTTP tests cover anonymous/USER/ADMIN, GET-only/HEAD/POST, strict UUID/query,
  repeated/empty/unknown status, rejected offset, 404/503 and Accept handling.
- Isolated .50 Go1.27.1: full `go test ./... -count=1`; targeted producthttp and
  adminread race; `go vet ./...`; all three CLI builds PASS. DB-dependent suites
  without explicit test environment are not presented as database evidence.
- Dedicated PostgreSQL18 container, loopback-only ephemeral port, CPUs2/memory1GB/
  pids256, disposable panta_168_test database: `go test -race
  ./internal/store/postgres -run TestPostgresAdminDownload -count=1 -v` PASS.
  No real provider/database/service credentials used.
- PG cases cover every legal Manifest/Job pair/status, disabled/history-compatible
  Copy REMOVED, no Job/PENDING, owner/Binding visibility, no secret projection,
  wrong payload/type/state/failure budget, wrong Copy Binding, dangling owner,
  read-only enforcement, fail-closed error hook, owner+status keyset traversal,
  true empty and paired concurrent commit after the coherent single SELECT.
- Linux readonly service safety11/11 and local Git-checkout deployment safety3/3
  PASS. No assertion or permission guard was weakened.
- CI adds new tests to existing PG16/18 gates and PG18 admin concurrency-race step;
  the PR attaches actual exact-head results, not a skipped opt-in claim.

## Browser evidence - SYNTHETIC FIXTURES, not live production

Run `node test/admin-preview.cjs --share-review`; loopback ephemeral server.
Use Playwright CLI with an isolated named browser session. Each page visibly says
ISOLATED UI FIXTURE; all accounts/IDs/task/error examples are synthetic. No real
115, OpenList, IndexCore, production API or protected file is contacted.

- [Desktop list1440px](fixture-list-desktop.png)
- [Desktop detail1440px](fixture-detail-desktop.png)
- [Mobile inline detail390px](fixture-detail-mobile.png)

Real browser checks: row button opens exact detail, RECOVERY_REQUIRED renders
manual follow-up instead of retry, detail receives keyboard focus, Escape closes
and returns focus to the original Manifest button, status filtering narrows two
fixture rows to one and closes old detail. Page scrollWidth equals viewport width
on desktop1440px and mobile390px; the wide data table scrolls internally. No browser
console errors/warnings; existing verbose password-form suggestions are not an
accessibility certification. Images were inspected, not production screenshots.

## Review / rollout boundary

Source contains no migration changes, command endpoints, provider operations,
source filesystem mutation or production rollout. Consumer nav165 and homepage167
regressions remain covered. AI131 and human-paused private139 remain excluded.
Parent155 stays OPEN. Stop at one unmerged PR for Architect review; production153
database upgrade is a separate operation, not part of this candidate.
