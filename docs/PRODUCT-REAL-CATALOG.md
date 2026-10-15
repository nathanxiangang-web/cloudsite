# Real catalog source integration - bounded operator contract

## Status and authority

Frozen before execution, 2026-10-14. The human prioritizes real software, video
and document display and reiterates Panta's IndexCore boundary. D-042 delegated
delivery and D-043 readonly presentation remain governing. Issue #83 admission
WIP is preserved separately; no draft v14 migration or provider task is deployed.
This is source configuration/controlled indexing plus owned readonly process
lifecycle, not new acquisition/content functionality. The subsequent human
instruction excludes UI and API file deletion: users remove files in their drive
themselves. IndexCore's existing conservative observation/removal rules stay
unchanged; share revocation must not delete source files either.

## Module responsibility and dependency direction

Operator configures existing IndexCore CLI/Collector/Kernel. OpenList is the
Collector source; Panta's runtime only uses IndexCore HTTP Query and its product
storage metadata. Browser uses authenticated Panta read routes. No Panta direct
OpenList query, direct IndexCore DB read, alternate resource truth or Copy writer.
Source inventory is private operator discovery, never a UI response or Copy.

## Isolation and scan contract

Pinned IndexCore source 7ef47217f23ee1ba921f2dcc8378c0a2a9fb84e1. Create exactly
one new `indexcore_panta_mvp_catalog_20261014` database in the existing staging
PostgreSQL 18 instance; refuse if already present. Explicit IndexCore migrations
apply only to this new database. New Roots have generated immutable UUIDs and
existing root/config/adapter administration. No old Root policy or generation
changes and no original v12 Panta migration.

Three individually identified scopes: a software subtree, a single MP4 source
directory and a tutorials/documents subtree. Paths and complete source evidence
are stored in owner-private files, not public logs. Do not scan the entire drive,
other movie scopes or unrelated storage mounts. Serial one-shot CLI scans, at
most 120 seconds each and one initial attempt per Root. No daemon, rescan loop,
trusted Hint or downloader action. Existing ordinary collector reads use
refresh=false and accepted pagination checks; never loosen deletion safety or
truncation checks. Canonical reconcile and Q8 evidence are mandatory before UI
switch. A failed scan remains isolated for diagnosis; original UI stays working.

Credential values stay in protected files/process environment. Adapter JSON
stores only username/password environment names, not secrets. No new account,
permission grant or native 115 session refresh. Runtime raw output/errors stay
in private files; public evidence exposes safe counts/status/hash only.

## Query and product composition

Run an independent loopback QueryReader/Readiness/httpapi entrypoint over the new
catalog, with PostgreSQL default_transaction_read_only=on enforced at startup.
No Store/writer/collector/worker/Hint in that process. Keep the original readonly
staging QueryReader intact for rollback. IndexCore binary/module source stays
pinned; private operator composition is not claimed an official new release.

Product observation-only Connections and Bindings are NEW, DISABLED, no
credential_ref and no provider_scope. Create through existing StorageRepository
ports in one product transaction; v13 schema unchanged. Native mutation scope
cannot be derived from source path. The product allowlist explicitly selects
these three new Bindings; old Binding/config remain intact. All UI resource
DTOs must originate from authenticated Q4/Q5 over these real canonical Roots.
Labels and category counts describe actual scope/page, not entire-drive totals.
No historical user/task ownership adoption or acquisition activation.

## Verification, rollback and remaining MVP

Snapshot all public tables of original indexcore_gate318 and panta_gate318
read-only before/after. Verify old service/container identity/state unchanged.
Confirm original unknown ProviderTask reservations unaffected. In the new
catalog verify schema compatibility, scan outcome, Q4 PRESENT resources, exact
Q5 and Q8 resource-added events for nonempty scopes. A source-confirmed empty
directory correctly has zero Q4 rows, no resource-added events and generation 0;
do not invent files or rescan until nonempty merely to satisfy a positive count.
Browser checks actual software/video/documents,
typed cards/details, scope and pagination; use no synthetic UI fixtures.

Rollback restores old owned UI config/process without deleting source files,
new catalog, old Roots, provider reservations or audit evidence. No full access
or READY claim from metadata: product Copy projection, authorized file-content
read/media gateway/download/share and Issue #83 still require accepted modules.
Use a narrow evidence/runbook PR with exact-head CI and honest same-agent review.

## Owned readonly lifecycle after VM reboot

The VM rebooted again before cutover. Without service management, manually
started readers disappear although both databases retain data. Use the two
user-systemd units in `ops/systemd/` and `ops/readonly_catalog_service.py`.
The existing user already has Linger=yes; do not change global login policy.
Units run only the exact-hash accepted product binary and pinned private
IndexCore QueryReader. Protected receipt/config/DSN validation rejects unowned
or readable secrets, binary drift, public listeners and old database targets.
The launcher does not propagate Provider/OpenList/Hint credentials. Product
worker is explicitly false; there is no migrate/scan/worker/delete action.

At startup only the exact-pinned existing staging PostgreSQL and owned preview
PostgreSQL containers may be started if stopped. Do not change Docker restart
policies, start other containers or run an IndexCore `serve` writer. The new
catalog remains one-shot indexed, with no scheduled rescans. Restart retries
are bounded (three per minute); SIGTERM/shutdown uses the accepted Go servers.
Readiness timeout refuses startup; it never grants acquisition authority.

Install the reviewed launcher in the owner-private deployment directory, with
the private receipt and `product-real-catalog.json`; install the two unit files
under the user's `.config/systemd/user`, then daemon-reload/enable/start them.
Verify loopback readiness, authenticated Panta scope and graceful stop/start.
Do not reboot the entire VM just to test this; other running services must not
be interrupted. Rollback stops/disables these two owned units and restarts the
prior retained readonly UI/query composition. Do not delete databases or files.

## Actual source evidence before the reboot

The three one-shot scans completed on 2026-10-14 in the isolated database.
The first root-policy command was rejected because this pinned Kernel supports
MinIndependentConfirmations=1 only. No scan had run at that point; the supported
policy was applied with 24h grace/horizon and two consecutive COMPLETE missing
observations. Existing additive-safe collector/removal rules were not changed.

| Bounded scope | Actual Q4 rows/files | Generation | Evidence |
| --- | --- | --- | --- |
| Software source | 0 / 0 | 0 | Source independently confirmed empty; zero Q8 events, not a failed scan |
| One video directory | 1 / 1 | 1 | PRESENT MP4, exact Q5, Q8 resource-added |
| Tutorials/documents | 8 / 5 | 1 | Three directories and five existing files: PDF, DOCX, PPTX, XLSX, Markdown; Q5/Q8 verified |

The accepted readonly product binary was tested on a temporary loopback port
through verified USER auth, new Binding allowlist, real Q4 and exact Q5. The
browser confirmed document cards/detail, video card/detail and empty software
state; scope changes cleared previous details. These are real existing stored
files (including user test documents), not frontend fixtures. No content body,
software installer execution, playback or new provider download was tested.

Protected evidence records contain full scope/UUID/snapshot/Journal details and
binary hashes. All public tables of original indexcore_gate318 and panta_gate318
matched their pre-configuration hashes. Other container/service identities and
states were unchanged. No upload/delete/refresh=true/StartDownload occurred.
The VM rebooted on 2026-10-15 before formal cutover; data persisted.

## Accepted deployment and formal-entry verification - 2026-10-15

Issue #86 / PR #87 accepted at exact HEAD
`d693878c7791b592fa21668a920abd322df9aa37`, CI `38669580171` all three jobs
SUCCESS; same-agent review `5527219011`, squash merge
`ddecd51c19b7431d9ca3611add3304eea725240a`. Review is a separate same-agent pass,
not independent approval. Python service safety tests (8), Go/architecture/race/
vet/build, PostgreSQL 16/18 CI and existing presentation tests passed.

Both named readonly user-systemd units are enabled and active. A controlled
graceful restart of only those two services passed: new process IDs, loopback
readiness, original table hashes and unchanged catalog generations 0/1/1.
Actual full VM reboot recovery has NOT been tested. Exact-hash startup initially
refused group-writable binaries; restricting only the two owned binaries to 0755
fixed the deployment without changing bytes or weakening the guard.

The formal server listener is 127.0.0.1:8180. The local browser's identical
127.0.0.1:8180 URL is an SSH loopback forward to .50, not a second local backend
or LAN/public exposure. The browser verified authenticated current Q4 counts,
five document cards, actual MP4 detail/PRESENT and unavailable playback controls.
No content body was read. Empty software scope remains honest and selectable.

Deployed product binary is retained PR #85 `af23cf2746a0`, SHA-256
`6e3bee39d3ea91a5d148387164605e59b2c40e652ac4da0e9af166b545994968`.
Private IndexCore query-only composition is pinned 7ef47217, SHA-256
`98400609bf15950e263d0bc55867d065ca4836154ff9206c4ab6d0d07229c08b`.
Product schema remains v13 and original staging v12 is untouched. Final private
deployment evidence is `execution-managed-real-catalog-issue86.json`; receipt,
configs, raw scopes/IDs and rollback artifacts remain owner-private. No new
StartDownload, provider history mutation, Hint, upload or delete occurred.
