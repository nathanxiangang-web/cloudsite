# Product-owned Copy presentation - Issue 176, Stage 1

## Baseline and delivery gate

Starts from main f868da264255ddf93457c8f8f67117bb8bb33349 (PR 175 merged).
Merged-main CI 40532208171 was independently fetched as completed/success before
implementation. One backend draft PR for independent review; no self-merge.
Parent 155 remains open. Production 153 owns the separately authorized upgrade,
backup/restore, canary and rollback. AI 131 and private 139 remain outside scope.

This PR adds an exact protected backend API, not a resource editor UI. The only
frontend change is the existing read-only audit action/code label compatibility;
it receives no metadata write port or new operational button.

## Module boundaries and dependency direction

HTTP -> adminops service -> optional CopyPresentationReader/Applier ports -> PG.
`copypresentation` owns the bounded product label/category/overlay contracts only.
`adminops` freezes the intent, validates version/IDs, reserves through the existing
ledger and dispatches only a fresh request to the specific write port. PostgreSQL
owns DB authorization, scope, lock order, CAS and atomic audit commit. Runtime
already composes the real operations service/repository under its exact schema
startup guard; missing optional ports fail unavailable. No new provider, scanner,
IndexCore mutation, user quota, general SQL/command executor or dependency.

The overlay is keyed by exact immutable CopyID, with its checked Binding, and
works when variant_id is NULL. It never creates Asset/Release/Variant. Copies,
IndexCore root/resource IDs, availability and Canonical paths remain authoritative
and unchanged by this command. Do not use an AssetID, Q4 path or guessed newest
file to identify a target. Copy identity survives PRESENT -> REMOVED -> PRESENT.
The overlay is retained through that transition and through actor soft deletion.

## API and product-owned fields

GET /api/v1/admin/copies/{copy_id}/presentation?binding_id={configured_binding_id}
PUT /api/v1/admin/copies/{copy_id}/presentation

PUT requires application/json and EXACTLY:

    {"request_id":"UUID","expected_version":0,"binding_id":"UUID",
     "display_name":"Product label","category":"DOCUMENT","hidden":false}

All six fields are mandatory. Unknown/duplicate/null fields, additional query
parameters, unsupported methods, invalid UTF8 or escaped lone surrogates are
rejected. Origin/Host, fresh ADMIN and cookie/session protections remain in force.
Names are empty (deliberate clear) or trimmed labels up to 160 Unicode codepoints
and 640 UTF8 bytes, without HTML delimiters, controls, format-control characters
or replacement characters. No physical path/name is generated from an alias.
Categories: AUTO, SOFTWARE, VIDEO, DOCUMENT, IMAGE, AUDIO, ARCHIVE, OTHER. AUTO and
an empty label delegate fallback to a future current Canonical display read.
No tags or new interpretation of file type/codec are introduced.

Read response separates `evidence` (CopyID, Binding, IndexCore root/resource IDs,
PANTA projection cursor, Copy availability/update time) from `presentation`
(label, category, hidden, version, updated_at/by). A missing overlay is explicit
version 0 / empty label / AUTO / false / null update fields. First write is version
1. Versions are integers bounded by 9007199254740991 for safe future JSON clients.
`can_edit` requires present projected Copy and remaining version space. Removed
Copies can be read with retained metadata, but writes reject COPY_UNAVAILABLE.
`read_at` is DB query time, not provider observation time. Source is explicitly
PANTA_COPY_PROJECTION: the endpoint does not fetch or claim fresh Canonical or
provider state. A positive existing per-Binding cursor plus exact Copy/Binding/root
mapping is required; it trusts the existing projector/database writer boundary,
not an independently re-proven per-row Journal provenance.

`visibility_effect=PRESENTATION_ONLY_NO_ACCESS_CONTROL`. The hidden flag is
stored product metadata only in Stage 1. Consumer catalog, authenticated content,
shares, grants and direct 302 delivery DO NOT read it. Saving hidden=true does
not block downloads, revoke sessions/grants, delete files or change current share
permissions. A true publication/access toggle needs a separate authorized policy
PR with grant/session revocation and explicit security tests. No admin GET silently
conceals a removed/hidden resource. Exposure of the flag in a consumer DTO is also
a separate integration with an explicit policy contract.

## Transactions, receipts and failure semantics

Lock order: active actor -> active session -> existing action (write only) ->
Binding -> projection cursor -> Copy -> overlay. Binding is rechecked against the
configured allowlist and Copy/root mapping under locks. Cursor precedes Copy to
match the projector, which takes cursor then Copy. The command never advances a
cursor or changes Copy availability, identity or variant. No external call while
holding locks. GET takes shared locks for coherent evidence/overlay and rechecks
session DB time before returning; it performs no mutation.

Reserve/Apply/Status reuse the existing product_admin_actions/results ledger,
exact actor/request key and SHA256 digest. Adding an omitted optional intent field
preserves all previous command digest bytes. The new digest binds target, Binding,
label, category, hidden and frozen expected version. Same request/digest returns
its original result; different digest conflicts. UNKNOWN never authorizes Apply
or replay. A new request with an old version gets a durable VERSION_CONFLICT.

Write transaction locks current authorization/scope, checks present Copy and CAS,
upserts only the product overlay and inserts its terminal result in ONE commit.
Time/authorization are checked again after lock waits and before commit. DB/FK or
receipt/commit failure rolls back the write; a transport/Commit error is ambiguous
to the caller, never an instruction to issue PUT again. Use the existing protected
GET /api/v1/admin/actions/{original_request_id} only. The existing generic UNKNOWN
receipt carries the reservation timestamp, not proof of terminal completion. Reserve itself can also have
an ambiguous commit; the same status-only rule applies. No automatic retry path.

Successful receipt contains action, actor-scoped request, supplied Copy target,
status/code/record time and resulting presentation_version. It contains no label,
category, hidden value, provider data or account material. New-action status and
duplicate reads validate their exact stored header/result identity, code/status,
version and timestamp; corrupt or extra fields fail unavailable. General audit
extends its explicit action/code and SQL allowlists and redacts Copy target and
version. Existing Binding scope and malformed/secret-field negative matrices stay.

HTTP: 200 confirmed success/read, 202 UNKNOWN, 400 invalid, 401 unauthenticated,
403 non-ADMIN/Origin, 404 absent/unmapped/out-of-scope Copy, 405 unsupported method,
406 incompatible Accept, 409 version/presence/intent conflict, 503 unavailable or
unconfirmed. No DB/provider error text is returned. Unknown or corrupt data is not
an empty/default/healthy overlay.

## Threat model and residual assumptions

Treat every browser ID, label, category, version, token, prior read and persisted
receipt as untrusted. A non-ADMIN, stale/revoked session, foreign Binding, guessed
Copy, cross-origin request, conflicting version or poisoned receipt must never
produce a metadata write or disclose a label. Duplicate/ambiguous requests cannot
repeat the write. Concurrent projector and administrator transactions serialize
in the same cursor-before-Copy order. Existing direct SQL maintenance is outside
the product API; the DB/projector are trusted identity writers, with corrupted
mapping/overlays failing closed rather than proving external provider state.
The stored hidden flag is explicitly NOT a security control. Physical presence,
Canonical paths and existing content/share authorization remain independent.

## Schema and release boundary

Append-only 0020_product_copy_presentation.sql adds only the product overlay and
extends the existing admin action CHECK. SQL files 0001 through 0019 stay byte
identical. Schema head/runtime guard become 20. Legacy 19 and production 16 cannot
run this binary; old binaries must not run on schema20. Migration rollback tests
prove a failed/uncommitted 19->20 upgrade rolls back both DDL and action allowlist
while preserving accounts, previous actions and receipts. After a committed
upgrade, rollback requires the separately approved restore/compatible runtime
plan; no destructive down migration or live release is performed in this PR.

## Validation and evidence limits

Tests use real isolated PostgreSQL 16 and 18 with synthetic accounts and projected
Copies, not production data. They cover unclassified Copy, empty alias clear,
retained removed/recovered identity, two admins/duplicate requests, stale versions,
projector lock interleavings in both orders, actor revoke/delete, expiry rechecks,
missing/foreign/mismatched scope, corrupt overlay/receipt, receipt insertion and
before/after Commit faults, exact HTTP permissions and transactional schema20.
The complete existing consumer/resource/Q4/Q5/share/direct delivery regressions
remain required alongside full Go tests, race/vet/build, all Node tests and exact
HEAD three-job CI. No real provider call, physical mutation, live UI editor or
production acceptance is claimed. Draft PR records actual command/CI results and
migration SHA256.

Stage 2, after Stage 1 merge and green main, should inject this exact read/update
port into a separate admin editor, freeze request/version/Copy/Binding, confirm
one audited write, reconcile UNKNOWN by original GET only, refresh on conflicts
and fence principal/Binding/tab changes. Required copy: product display only;
no drive file rename, index mutation or change to current sharing permissions.
Future fields require coordinated validation, digest/versioning, DB constraints,
receipt/audit allowlists, threat model and independent tests. Do not expand this
module into Canonical, provider, Catalog identity, access policy or quota control.
