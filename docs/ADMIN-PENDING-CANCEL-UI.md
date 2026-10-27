# Pending cancellation UI (Issue172 candidate)

PR171/backend HEAD b799a8de9f69 merged as main4b1329a47ee2. Exact merged-main
CI40457606174 completed SUCCESS in all three jobs before this branch started.
Backend acceptance is not production activation. This is one frontend candidate
for independent Architect review, no self-merge. Parent155/170/153 remain open.

## Modules and interfaces

`AdminPendingCancel` owns confirmation and one audited request. Injected ports
are exact detail GET, authenticated command, receipt GET, local auth generation,
principal/active-tab/Binding/capability checks and server refresh callbacks. It has
no provider, Job, Copy, filesystem, database, retry or arbitrary command port.

`AdminDownloadDetail` owns strict DTO validation and read/request generations;
it emits valid fresh snapshots or null on invalidation and returns an exact GET
result. `AdminLists` uses its shared pair validator. `AdminOverview` separately
validates and displays canceled_without_job; omitted means zero, explicit invalid
values fail unavailable. Existing CANCELED+Job still requires a CANCELED Job.
ACTIVE/READY/FAILED/RECOVERY_REQUIRED without a Job remain invalid.

A read-only `admin_pending_cancellation_enabled` workspace flag requires schema19,
the actual optional cancellation port, and the exact diagnostic read port. Schema
or UI presence alone is insufficient. Consumer descriptors always return false.
Every command preflight rechecks the fresh descriptor and Binding. Existing
backend CAS/authorization remains authoritative; no SQL/migration change occurs.

## State and confirmation boundaries

Only a freshly validated PENDING/no-Job/no-Copy snapshot in current ADMIN scope
can offer the button. Clicking it repeats the exact detail GET. Confirmation
freezes the persisted owner, Binding and full microsecond updated_at string from
that read; it never substitutes Date.now(), a list row or a rounded JS Date.
Snapshot/confirmation expires after30 seconds; a change or later read closes it.

The controlled modal identifies the exact Manifest, explains the permanent
product-intent transition and that no115 content is deleted. Initial focus is on
return/no-cancel; Tab/Shift-Tab are trapped, Escape dismisses without closing the
detail, and mobile content scrolls inside a bounded dialog. Submitting moves focus
to the audit-result panel. It is an explicit one-POST action, never browser confirm.

One WebCrypto randomUUID is generated after confirmation. Request ID and frozen
intent live only in memory for the current local ADMIN/session generation. No
cookie, bearer, credential or intent is written to local/session storage. Tab or
target changes invalidate late writes but retain an unresolved same-session ID
for manual receipt lookup; logout/principal/role changes discard it. Reload does
not restore or replay a command. Keep the displayed request ID if a later manual
operator audit is needed; this is not a claim of durable browser persistence.

POST202, 503, network failure, deadline and abort are UNKNOWN, not cancellation
success or failure. Only an explicit GET /api/v1/admin/actions/{request_id} can
reconcile that same request. Duplicate click/confirmation cannot create another
POST. Even timeout before preflight completes fences late command dispatch.
Each request has its own abort controller; an obsolete deadline cannot abort a
new receipt lookup. Target/filter/tab/Binding/principal/session changes fence
responses. 401/403 clear the session; 409 refreshes current reads, 404 hides the
scoped target. Invalid/foreign receipts stay UNKNOWN with no payload rendering.

Terminal receipts are checked against exact action/request/target/status/code
and time; success requires CANCELED/no Job evidence. Receipts do not synthesize a
Manifest row or counts. Terminal/conflict handling re-GETs list and overview,
then exact detail if still visible and in the same context. Failed reads remain
unavailable/stale under the existing modules' strict rules.

## Extension and release constraints

All Node suites are included in CI. Tests use injected/fixture ports and no real
provider or product data. Browser evidence is explicitly synthetic (see
operations/admin-pending-cancel-ui/QA.md). Do not add native/provider cancellation,
Job retry, hidden/unowned-task access, private139 or AI131 under this module.
Future commands need their own typed preconditions and receipts, not a generic
state update or retry loop.

The production schema16 runtime, assets and databases are untouched. Stage2 needs
exact-head CI + Architect acceptance and subsequently green merged-main CI.
Only then consider the separate #153 backup/restore/schema16->19/runtime+asset
compatibility and rollback gate. This candidate is not whole UI-3B completion,
production release, or authorization for a live administrative cancellation.
