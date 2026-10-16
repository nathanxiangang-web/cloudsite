# Product cloud-download runtime and consumer entry

Issue111 / D-055, following accepted PR110 / D-054. Schema remains15.
This is a composition and consumer-entry slice, not public deployment acceptance.

## Readonly unavailability extension - D-056 / Issue114

An explicit protected `cloud_download_preview` object may contain
`label: "云下载"` and `unavailable_reason: "LOGIN_REQUIRED"` (or NOT_CONFIGURED).
It is mutually exclusive with `cloud_download`. The readonly process then
advertises `cloud_download_enabled=false`, `cloud_download_preview_enabled=true`
and the closed reason. Only the authenticated same-caller pure prepare route is
added. No admission, operation lookup, usage, session/bootstrap, provider, Hint,
Worker or reservation is constructed. No automatic health probe is claimed.

The page displays the actual declared unavailable state, allows fresh-source
compatibility checking and existing own-task reads, and keeps submit disabled.
It retains pending keys and never automatically probes absent mutation/lookup/
usage endpoints. Unconfigured readonly mode still exposes no preparation route;
managed mode remains unchanged. This narrowly supersedes the older blanket
absence-of-preview prose below. Actual native GET990001 is not fixed by a UI
change; real acceptance remains blocked until a valid native session exists.

## Modules and dependency direction

| Module | Responsibility | Boundary |
| --- | --- | --- |
| `product.OperationReader` | Source-free owner/key reconciliation port | No HTTP, provider, SQL or activation |
| postgres operation reader | Current enabled identity, operation ownership and Manifest linkage | GET only; no reservation/repair/activation |
| `producthttp` cloud adapter | Strict same-origin authenticated consumer input; fixed operator target | No provider/integration/store dependencies |
| download model/page | Same-operation submission, preview, own tasks, usage | No credential, directory or provider controls |
| `platform/productserver` | Explicit protected composition and HTTP/Worker lifecycle | Existing runtime owns concrete provider wiring |
| existing runtime | Serial RunOnce -> stage dispatcher -> provider/Hint/Q5/Q8/projector | No replacement engine, scanner, queue or Copy writer |

The composition root receives exactly three new architecture-guard exceptions:
acquisition constructors, runtime and app lifecycle events. Concrete provider and
job dependencies remain forbidden there. Product and HTTP dependency guards do
not change. New providers belong behind existing ports, never in the page.

## Explicit actions and configuration

`serve-product-readonly` remains read-only and rejects `cloud_download` config.
`serve-product-acquisition` requires explicit enabled acquisition policy and a
protected product config containing:

```json
{
  "cloud_download": {
    "target": {
      "binding_id": "98000000-0000-4000-8000-000000000001",
      "target_path": "/downloads",
      "label": "Cloud downloads"
    },
    "bootstrap_file": "/run/cloudsite/acquisition/bootstrap.json"
  }
}
```

This fragment supplements the existing required product config; it is not a
standalone deployable config. Binding must also be in configured display
Bindings, ACTIVE, have an explicit valid provider scope and canonical root, and
match a registered bootstrap session. Bootstrap uses existing protected secret
mounts and trusted loopback-only Hint; never put credential values in this JSON,
CLI arguments, docs or environment. Keep the four coordinates separate.

`preflight-product-acquisition` validates protected config, exact schema and the
complete runtime graph, then closes it without listening, claiming, calling115,
sending Hint or starting cache maintenance. Like existing operator preflight,
it validates secret availability/local parsing, NOT live session validity or
IndexCore network health. Those require separate deployment evidence.

Acquire the HTTP listener before Worker.Run. Either HTTP failure or unexpected
Worker exit cancels and joins the other side. HTTP requests and Worker finish
before their pools close. Parent cancellation stops both; no detached worker.
Existing lease, timeout, owner uniqueness, recovery-debt stop and retry rules
remain authoritative. Provisioning does not construct an acquisition runtime.

## Consumer contract

`/downloads/` is a file-first consumer page, separate from `/admin/`. The library
shows its link only when `/api/v1/workspace` advertises cloud-download capability.
An unconfigured read-only server exposes no cloud mutation endpoints. Static
page availability does not imply runtime capability.

| Endpoint | Input/output | Side effects |
| --- | --- | --- |
| POST `/api/v1/cloud-downloads/prepare` | Fresh source only; prepared execution/policy/change summary | Pure preparation, no operation/quota/Manifest/provider |
| POST `/api/v1/cloud-downloads` | operation_key, source, optional expected_name | Existing D-054 admission and stable-ID activation |
| GET `/api/v1/cloud-downloads/operations/{key}` | Caller-owned source-free IDs/status | No activation, retry, write or provider call |
| GET `/api/v1/tasks` | Existing owner-only redacted task DTOs | Read only |
| GET `/api/v1/usage` | Existing owner-only zero-price operation usage | Read only |

Binding, target path, provider scope, role, owner and quotas cannot be supplied
by the consumer. Duplicate/unknown fields, nulls, extra JSON and query parameters
are rejected. Body512KiB/source64KiB; preparation may reject execution >4096bytes.
HTTP/HTTPS/ed2k remain exact, including signed queries. D-053 controls magnets.
No fallback guesses based on provider filenames or newest/only OpenList files.

Preview is an explicit narrow exception to no-source DTOs: it returns only the
same caller's just-submitted input's prepared form. It never loads stored or
another user's source. Task/lookup/usage/error/log DTOs remain source-free.

Before its first explicit submit, the browser persists only a user-bound
operation key in sessionStorage. Raw source/expected name stay in memory/DOM;
no source URL in browser persistence, address, logs or HTML injection. Keep the
same frozen intent/key after failure; simultaneous submits are blocked. Reload
does only owner-key GET reconciliation, never automatic POST. Not-found is NOT
proof of rollback, so keep that key. PENDING_ACTIVATION is not successful READY
and keeps the pending key. Explicit retry uses the same key and original input;
the server replays its stored accepted policy, never re-prepares old operations.
Authentication expiry clears private DOM but retains the owner-bound key for
reauthentication; explicit logout clears private state/key. Page hide clears DOM
and memory while retaining only the key; BFCache restoration reauthenticates.

Task refresh uses GET only. Pagination pauses first-page automatic refresh so
loaded older rows are not silently discarded. READY display requires the same
Job SUCCEEDED and a nonempty result_copy_id. Provider success alone is not READY.

## Validation and deployment boundary

Tests cover strict HTTP, target injection, owner lookup, storage/model behavior,
lifecycle cancellation/failure and actual PostgreSQL operation reconciliation.
Controlled HTTP admission E2E discards its submit acknowledgement, recovers IDs
through GET, then exercises existing provider/Hint/Q5/Q8 to READY+SUCCEEDED with
one operation, one Job, one provider start and one usage charge. No real account
calls occur. CLI tests prove preflight/port/config/secret failures do not claim,
and an empty-queue managed process exits on SIGTERM without provider requests.

Current public product is still the accepted0b29 build/schema14, NOT this new
managed runtime. Native login restoration on the public server was separately
verified by read-only directory access, with zero StartDownload; its protected
credential must be deliberately mounted with the runtime identity. OpenList
application OAuth tokens are not native115 cookies.

Before public activation: backup the owned product14 database, build and test a
15-compatible admission-disabled fallback, explicitly migrate to15, validate
fixed coordinates and native login, and deploy a real single-writer incremental
IndexCore runtime with strict loopback Hint. A query-only catalog snapshot is
not that runtime. Do not run the old strict14 binary against15. Preserve ordinary
file preview/302 delivery/shares,8 old unknown reservations and original KNOWN
READY evidence. No actual download or production upgrade is implied by this PR.
