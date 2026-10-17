# Real cloud acquisition: deployment evidence and boundaries

## Reviewed artifact

- Issue120 / PR121 / D-058; reviewed c1ed10be896ff6f1e6011c8c86ef979c382f6be4.
- Exact-head CI38992241383 and merge CI38992448685: all three groups SUCCESS.
- Merge/runtime f8be03ccf1d798898b52618f3fd773a5a9094699, image cloudsite:acquisition-f8be03c.
- Separate same-agent review; this is not an independent architect review.
- Binary SHA256:47d249a69a36deceebb895548d076fc8426b291b331dca7170391df582ace047.
- Schema remains15. Dedicated PostgreSQL operator tests run in their own database.

## Module boundaries and path mapping

Native115 saves to a privately verified ID for **/CloudSite/云下载**. The application
OpenList account remains based at /115open/CloudSite. Manifest target, Hint scope
and IndexCore Q5 use **/云下载**. These are different coordinate systems, not
interchangeable strings. Existing CloudSite Root, Binding and mount identities
remain unchanged. A guarded, atomic operator CAS attaches a fresh native115
Connection; the old observation Connection and credential absence are preserved.

IndexCore remains observation/canonical truth. Standard OpenList supplies files;
the native115 session creates/polls acquisitions. No new scanner, queue or Job
engine. One serial managed Worker composes existing RunOnce/stage services. Native
success alone never makes READY: the existing Q5/Q8/Projector/Copy finalization
must finish under its database-time lease and transaction rules.

The old physical root /云下载 and files were not moved or deleted. Its Alias and
old read Binding remain configured for historical access, but that alias-only
Binding is no longer part of the active CloudSite consumer feed.

## Observed public operation

On2026-10-17, preflight passed without claiming or submitting; bounded parent
and child PARTIAL Hints returned202. Q5 resolves the new directory to exactly one
PRESENT match. The persistent IndexCore single writer and loopback-only Hint
boundaries stay intact. Public workspace enables real submission, exposes quota,
and rejects extra provider/target fields with400. The actual browser shows the
form and saved-location label; no browser download POST was made during review.

One sealed ordinary ISO smoke POST returned201 at01:54UTC. The same pair has a
durable REFERENCE_KNOWN ProviderTask; one bounded native readonly observation
confirmed destination ID match, status RUNNING, progress0 at01:58UTC. Manifest is
ACTIVE, Job RETRY_WAIT, recorded failures0. This is **not download completion or
READY acceptance**. The one-start smoke budget is consumed. No duplicate POST,
new source, or historical reservation retry is permitted to accelerate this test.
Operational receipts/IDs/source/reference stay on the protected server, not Git.

Original283 files remain visible. Real ordinary DOCX preview and15423-byte hash,
authorized302->CDN200 download, video302->CDN206/2MiB hash, HEAD, anonymous denial,
invalid Range rejection, code/no-code guest delivery and revoke-denial all passed.
Temporary shares were revoked. Product and new catalog are healthy. Original
OpenList, edge, old catalog and both database service IDs/images/PIDs are unchanged.
The reused cloned CloudSite Root generation legitimately advances after the new
Hints; do not confuse it with the untouched original catalog database.

## Failure and rollback

The Worker uses a stable owner, serial ticks,60s tick timeout,2m lease,90s process
stop grace and bounded on-failure:3 container restart policy. Existing durable
START_RESERVED/REFERENCE_KNOWN semantics apply. Never clear a reservation or cancel
only one half of a Manifest/Job pair. Eight original unknown reservations stay
frozen outside the owned public pair.

Before cutover, exact product config/compose and the schema15-compatible readonly
image cloudsite:session30-caabc0a were retained privately. If rollback is needed:
stop/join the managed product first; restore that config/compose using the protected
formal environment; start only the product service. Preserve all task/reference
rows, native files and metadata topology. Do not use an old strict14 binary, reset
the database, or reverse a live Connection attachment. The protected operational
rollback script implements this sequence; it is not an authorization to repeat
external side effects.

## Remaining acceptance

Observe only the existing pair. Full real-chain acceptance requires provider
completion, new Q8 resource-added, exact Q5 PRESENT, projected PRESENT Copy,
Manifest READY/result_copy_id and the same Job SUCCEEDED. Until those are actually
observed, keep Issue113 open and do not call the full cloud-download MVP accepted.
