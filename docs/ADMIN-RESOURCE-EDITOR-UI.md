# Operational resource editor - Issue 176 accelerated delivery

Base main 1611d37bd443e01b82c1eb66381bb91ecc3ee0d2 includes the backend from
PR177. Merged-main CI40549771841 completed SUCCESS 3/3 before implementation.
This is one complete resource editor PR for one independent review; no extra
planning issue or backend-only follow-up. Parent155 stays open. Production153
preparation is concurrent, not a permission to migrate or deploy this candidate.

## User-visible workflow and module ownership

A projected, configured PRESENT Copy gets an actual resource-row edit button.
Existing file/card/Q5 details retain their Canonical name/path/presence and Copy
facts. The edit action scrolls/focuses the product presentation section. It reads
GET /api/v1/admin/copies/{copy_id}/presentation?binding_id=... before showing
stored label/category/hidden/version/read time. Unprojected physical files show
an honest read-only reason, never a synthetic Copy. Removed Copies stay read-only.

`AdminResourceEditor` owns exact evidence validation, bounded form fields,
confirmation, one frozen request and recovery. app.js injects fresh ADMIN GET/PUT,
principal/generation/Binding/navigation ports; it keeps existing canonical resource
rendering and the UI shell. The service advertises admin_copy_presentation_enabled
only when its actual reader and applier ports exist; workspace also requires
schema20. Consumer descriptor always says false. Missing capability never creates
an enabled edit control from schema alone. Other admin operations stay separate.

The form edits product name (empty clears), the backend's eight category enums,
and a presentation-only hidden marker. Required visible copy: product display only;
no drive filename, index or current sharing-permission change. It does not claim
that hidden blocks downloads. No consumer/share/302 contract or migration changes.

## Specific action and truthful recovery

Input bounds mirror Go: <=160 Unicode codepoints /640 UTF8 bytes, trimmed label,
no control/format-control/HTML delimiters, invalid surrogates or replacement char.
Fresh exact GET before confirmation rechecks identity, actual availability,
permission and version; a changed version refreshes instead of silently replacing
another administrator's changes. Confirmation has a30s lifetime, Escape restoration
and two-button Tab containment. One secure random UUID is generated only on the
human confirmation. One PUT freezes CopyID, Binding, expected_version and fields.

Before PUT, an actor-scoped recovery reference is saved: request_id, target_id,
binding_id and expected_version only. No label/category/hidden, credentials,
executable body or token is stored in browser storage. Storage/UUID failure sends
nothing. Transport ambiguity,503, UNKNOWN, abort or navigation keeps the original
reference. No request is automatically retried. The explicit recovery button uses
GET /api/v1/admin/actions/{original_id}; only matching allowlisted status/code,
request, Copy and expected next version is terminal evidence. Malformed/foreign
or missing receipts keep UNKNOWN. Lookup404 is not evidence that a late write
cannot commit. Loss of the browser session storage remains a recovery limitation;
normal audit history is the independent server record.

A confirmed result is shown with its original request and version, followed by a
fresh exact GET. Persisted fields come only from that read, never a guessed local
merge or a receipt copy of the submitted label. Terminal409/404 refresh the target;
401/403 clear private UI and require authentication while retaining the scoped
opaque recovery reference. Deadline failures close editing and re-enable reads.
A confirmed receipt with a failed fresh GET remains a confirmed historical result
with current metadata unavailable. New edits stay disabled until a valid read.

Generations/actor/Copy/Binding/capability/tab checks fence late responses and
pre-PUT authorization changes. Changing selection or tab clears form values,
confirmation and stale private fields without discarding an unresolved intent.
Logout/account change clears visible private data; only the original actor may
recover its reference after login. A pending request blocks new writes across
resource selections. Existing share, cancellation, user and provider modules are
not a generic execution fallback. No provider/115/IndexCore/filesystem mutation.

## Verification and delivery

Shared current HTTP DTOs and actual PG16/PG18 protected handlers retain real
GET->PUT->status GET->GET coverage. DOM tests cover single PUT, frozen fields,
UNKNOWN/relogin recovery, conflicts, old schema/optional port, corrupt data,
revocation, Binding/capability/tab fences, storage/UUID failures, deadlines and
keyboard controls. Full Node, Go, race/vet/build, PG16 full regression and PG18
copy/audit race tests run alongside three-job exact-head CI. Actual browser
fixtures are explicitly isolated, not production acceptance. See
operations/resource-editor-ui/QA.md for evidence. No new database migration.

Future extension must use the same exact port and explicit enum/field/schema,
version/digest, safe receipt, boundary and independent test contracts. It must not
turn a product label into Canonical identity, provider rename or an access ACL.
