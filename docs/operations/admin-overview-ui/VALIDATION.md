# Issue166 operator homepage candidate

Base: a8acd3479198764524fc484a748a0047441db7c0, including merged backend164 and
consumer navigation165. This is implementation evidence, not Architect acceptance,
production deployment or full UI-3B completion. Exact candidate HEAD/CI is attached
to the implementation PR, avoiding a self-referential commit hash in this file.

## Scope and module review

- New AdminOverview module uses only the injected verified ADMIN GET port.
- Users use the existing DirectoryReads total; Manifest and Job use overview164.
- Source/scope, real database read_at, safe counts, bucket sums, incomplete and
  linked terminal denominators are checked before rendering.
- Loading/failure clears previous values and timestamps. Zero remains real zero.
- Local abort plus identity/session/request/tab fences reject late replies.
- Existing admin destinations only; no fake management commands or auto probes.
- No backend count duplication, migration changes, provider calls, new download,
  production connection or IndexCore action. Parent155 stays open; AI131 and
  private139 stay paused. Production153 remains a separate deployment gate.

## Reproducible checks

Windows Node: `node --test test/*.test.cjs` passed 150/150, no skips. The CI's
explicit existing suite plus new admin-overview.test.cjs passed 147/147. The extra
three tests are existing file-surface.test.cjs, outside that explicit CI list.

The new suite has 14 test methods, with nested field/value matrices including
all required Manifest/Job fields. It replaces eight obsolete homepage tests in
admin-ui3a.test.cjs, retaining their zero/unavailable/logout/latest-request coverage
and adding the new overview contract. Other USER lifecycle tests are retained.

Covered: automatic initial read; exact unfiltered GET; zero/loading/unavailable;
negative/unsafe/fractional/string/null/missing counts; source/scope/date/owner;
bucket and paired-state contradictions; recovery distinct from incomplete/failed;
503/network/bad JSON; DirectoryReads malformed response; superseded refresh;
tab leave/return; logout; different principal; revoked authorization; real links;
unique DOM IDs and responsive/status markup. All homepage navigation is GET-only.

On isolated source directory /home/nathan/cloudsite-166.uqjFt5 at development
host192.168.178.50 (not production), Go1.27.1:

- `go test ./... -count=1`
- `go test -race ./internal/producthttp ./internal/adminread -count=1`
- `go vet ./...`
- `go build ./cmd/panta ./cmd/panta-db ./cmd/panta-staging`
- Python readonly service safety suite: 11/11 passed on Linux.
- Release deployment safety: 3/3 passed in the real local Windows Git worktree.

Database-dependent tests without their opt-in environment are not live database
evidence. PostgreSQL16 and PostgreSQL18 are validated by the PR's exact-head three
CI jobs, including existing overview concurrency/rollback and observation gates.
Windows readonly service tests hit the pre-existing POSIX os.getuid dependency;
the suite is run on Linux rather than weakening its permission guard.
The copied Linux source archive has no Git HEAD, so its archive-preservation
deployment test cannot run there; the Windows Git-worktree pass and exact-head
CI checkout exercise that guard. No safety assertion was removed or weakened.

## Browser evidence: synthetic fixture, not live production

Run `node test/admin-preview.cjs --share-review` and use its loopback ephemeral
URL. Every page explicitly labels ISOLATED UI FIXTURE. API records are synthetic;
no real accounts, databases, credentials or drive calls are used.

- [Desktop fixture, 1440px](fixture-desktop.png)
- [Mobile fixture, 390px](fixture-mobile.png)

Playwright CLI inspected the real browser snapshot and rendered screenshots.
Desktop scrollWidth1440 == innerWidth1440; mobile scrollWidth390 == innerWidth390.
Mobile navigation opens as a modal; Tab reaches nav-overview, Escape closes and
returns focus to menu-open. Console reported zero errors/warnings; two existing
verbose password-form accessibility suggestions were observed. This is not a
comprehensive accessibility audit or a production visual acceptance.

Screenshot fixtures contain 2 users, 15 Manifests and 14 linked Jobs, including
2 recovery-required pairs. These are labelled test numbers, not current inventory.

Stop at one unmerged PR for independent Architect review. Do not deploy this branch
or migrate the production schema16 service through an ordinary code merge.
