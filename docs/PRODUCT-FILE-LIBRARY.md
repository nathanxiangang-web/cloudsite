# File library and administrator workspace

## Frozen delegated contract - 2026-10-15

The human corrects the presentation model: maintain TWO UIs. The consumer
frontend cares about files, not storage directories; retain a separate backend
UI rather than deleting existing operational capability. D-042/043/044 apply.
No file-delete UI/API, provider download, schema migration or new drive scan.

## Audit of the existing screen

Current-run screenshot: `panta-file-ui-before.png`, inspected before editing.
1. Entry/browse: functional real data and explicit unavailable features, but
   counters for schema/PRESENT/execution and large operational notices dominate
   the screen before any files; reads as an admin dashboard.
2. Find a file: requires selecting a storage Binding, navigating folders or
   entering a canonical path. Category counts apply only to one Q4 page. This
   exposes implementation topology instead of a consumer file library.
3. Detail: file metadata is truthful but Copy IDs and canonical diagnostics
   compete with the user's file task. Keep these in the admin UI.

Keyboard labels/visible focus are present. Small muted labels and dense technical
status text are readability risks; screenshots do not prove accessibility
compliance. Verify keyboard focus, narrow layout and actual controls after coding.

## Routes, identity and ownership

- `/`: consumer login and file library. All metadata still needs product auth.
- `/admin/`: separate admin login/workspace shell, retains existing directory,
  exact Q5 resolution, diagnostics and owner-only task view. Its workspace
  configuration endpoint requires ADMIN server-side; a USER is rejected, not
  merely hidden with CSS. Login does not grant a role or change credentials.
- Existing auth, IndexCore HTTP Q4/Q5 and product read allowlist remain the sole
  data sources. Shared sessions; no new credential store or permission grant.
- Existing original UI becomes the admin asset set; consumer HTML/CSS/controller
  and bounded file loading module are separate. Shared pure suffix classifier.

## Consumer read composition

Reuse authenticated Q4 with each configured allowlisted Binding. Root-level Q4
is NOT recursive: directory rows enqueue child Q4 reads internally, never display
as files. Only explicit directory=false resources may become file cards. Retain
identity as binding_id + resource_id; paths/names are not identifiers. Missing
directory flags stay unknown and excluded, not guessed from suffixes.

Loading is bounded metadata reads, not a storage scan: at most four Q4 pages per
round, limit50 each, at most32 pages and256 queued scopes per session. The user
explicitly loads more rounds. Cycle/duplicate cursor/identity checks fail safely;
no silent infinite crawl. Session/page failure or cap is shown as incomplete,
never empty-library success. Counts/search refer to loaded files, not whole drive.
No automatic periodic query, Hint, OpenList access or write.

Consumer entry shows categories (all/software/videos/documents/other), name
search, cards/list, size and actual file extension. No fake covers, thumbnails,
vendor/version, duration or preview. Detail stays file-oriented; unavailable
content access is clear. Directory, source path, Root/Binding/Copy/schema values
are not rendered in this UI. Backend keeps operational facts.

Use textContent for untrusted names; no embedded remote assets or HTML injection.
Logout cancels requests and clears private file DTOs, details/counts/search.
Request generations prevent late results crossing logout/relogin/refresh. Focus
returns to the opener after closing detail; status and errors are announced.

## Verification, deployment and rollback

Pure loader tests: nested dirs, pagination, empty/unknown, duplicate names across
Bindings, bounds, errors, malformed/cycling pages, cancellation. DOM tests:
classification/search/detail, literal XSS names, stale session responses and logout.
HTTP tests: frontend/admin assets, ADMIN success, USER403, unauth401, strict query,
security headers, no new mutation/delete routes. Existing tests remain.
Go/unit/race/architecture/vet/build, Node tests and exact-head three-job CI before
honest same-agent review and merge. Browser verifies both surfaces, real file
counts, category/search/detail, denied USER admin access and narrow layout.

Deploy only the reviewed product binary to the existing owned .50 UI service;
archive exact prior binary/receipt/config, update protected hash receipt and
gracefully restart that service only. No IndexCore restart/migration/rescan or
credential/role changes. Rollback restores prior receipt and restarts owned UI.
Issue #83 admission WIP and historical provider records remain unchanged. This
is a presentation slice, not playback/content access or full MVP acceptance.

## Implementation evidence before PR review

Issue #88. Separate existing admin assets and new library assets implemented.
Node: 17 tests pass (12 new library/loader tests plus five existing presentation
tests); targeted and whole Go tests/race/vet/build pass. Local ordinary Go tests
do not claim live PostgreSQL coverage; exact-head CI must verify PostgreSQL.
Candidate on independent loopback8181 uses the unchanged real catalog configuration
and product schema13, no fixtures. Browser shows six actual files, no directory
cards; explicit further round exhausts three empty subdirectories without more
files. Video category/detail, PDF filename search and list toggle work.
Ordinary USER sees administrator-required denial on the admin workspace; server
tests prove ADMIN success. This is not a live administrator-login test and does
not provision/escalate any user. Private candidate/config/log are rollback evidence.

Audit follow-through: 1 entry/file library - file-first, no KPI dashboard;
2 find/detail - category/search/direct file details, operational topology absent;
3 backend entry - original operational UI preserved, USER denied. Current-run
before/after/search/detail/denied captures accompany the local delivery. Keyboard
focus restoration is tested; no full accessibility certification is claimed.
