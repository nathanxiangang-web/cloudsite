# Authenticated file content: preview, download and playback

## Download/media reliability correction - 2026-10-15

Consumer and redeemed guest downloads use the actual clicked same-origin link,
native attachment navigation, target=_blank/rel=noopener. Server success-only
Content-Disposition determines filename. No async HEAD-to-synthetic-click chain,
whole-file Blob, direct CDN URL or fake completion claim. A server failure stays
visible in the target instead of replacing the library or silently saving an
error body under a fabricated download filename.

An explicit play/pause button supplements native media controls. User gesture
invokes play; loading/ready/playing/buffering/error messages reflect media events.
Detail changes/logout invalidate old callbacks and release the previous stream.
No transcoding or guarantee for unsupported browser codecs is introduced.

Real incident: user Chrome retained stopped candidate8182, not formal8180. On
formal8180 the reported MP4 and another MKV loaded metadata with no decoder error;
HEAD/206 Range were healthy. Do not misdiagnose that as SDK/cookie failure. Formal
entry is8180 via existing SSH forward. No backend/schema/source/provider mutation.

Issue #96 / PR #97 accepted/deployed at reviewed HEAD `d5d056b0e1e2`, CI
`38736852134` all3 SUCCESS; same-agent review `5533296802`, merge `57a9d626cd62`,
merge CI `38737121369` all3 SUCCESS.28 Node tests and focused Go tests pass.
Actual Chrome native download initiation/save dialog human-confirmed (automated
download event was NOT observed). Full formal DOCX15423 bytes hash-match prior
acceptance; HEAD200/Range206 for document/two videos verified. New explicit
play button produced advancing playback beyond37s, readyState4/error=null; pause
works. Reported MP4 and MKV metadata decode normally, not all codecs guaranteed.
Only owned product restarted, schema14 and protected histories/catalog unchanged.
Binary SHA256 `84b5c2cda9b5778433c87ec4a9b84e9d026d7b03828d1e5799a9185b3e19c194`;
old14 receipt/binary retained for rollback. No new native provider calls.

## Frozen contract - 2026-10-15

Human requests real document previews, downloads and video playback and asks
sharing to reference CloudSite. Prioritize this bounded access slice, separate
from unfinished Issue #83. Existing two UIs, real IndexCore source scopes,
auth roles, v13 schema and eight historical unknown provider tasks stay unchanged.
No per-file manual approval, file deletion, new acquisition/115 offline download,
IndexCore writer, new scan, public exposure or third-party preview service.

## Modules and source of truth

- `fileaccess`: product-authenticated canonical identity authorization, streaming
  port and bounded Office text extraction. Does not create Copy or Job.
- `integrations/openlist`: server-only known-file content adapter. IndexCore Q5,
  through existing product Reads, must first confirm exact binding/resource/path,
  PRESENT, directory=false and size. OpenList is not consulted for acquisition
  visibility or canonical truth; its byte response cannot make a Manifest READY.
- `producthttp`: session-authenticated GET/HEAD content and preview routes,
  Range/content disposition, bounded concurrency and redacted errors.
- `productserver`: optional protected access config and token composition. Only
  existing configured read bindings qualify, including observation-only disabled
  Bindings; this grants no provider mutation or worker activation. Content access
  does not require inventing Copy for preexisting physical-only resources.
- Consumer UI: same-origin download, HTML5 video with Range, server-rendered PDF pages,
  literal text/Markdown and clearly labelled Office text preview. No remote poster,
  fake content, external Office/Google viewer or client-visible source Token/URL.
- `filecache`: private 0700 cache; atomic 0600 originals, one-hour TTL without
  sliding extension, single concurrent miss download, size/quota limits and
  minute cleanup of expired owned files/orphan temporaries.
- `filepreview`: existing Poppler `pdfinfo/pdftoppm` using fixed absolute paths,
  no shell; prlimit CPU/memory/output limits, two concurrent renders, bounded PNG
  page returned to browser. No PDF scripts or macros are executed.

This explicitly advances content access beyond D-045's metadata-only slice.
Normal login and server resource validation remain, without per-file approvals.
Exact canonical path must lie inside persisted observation mount scope; never
accept a client URL, infer provider_scope or use OpenList to establish presence.
An existing REMOVED Copy or ambiguous/stale resource must refuse access.

## Transport and preview rules

OpenList fs/get uses protected server Token and a fixed request UA; content
requests never forward the product cookie, OpenList Authorization or browser
headers to CDN hosts. Bound/validate response and discard readme/related entries.
Same configured OpenList origin is trusted; other HTTP(S) content/redirect targets
must resolve to public IPs at dial time. Reject credentials/fragments, private/
loopback/link-local/multicast/reserved destinations, IP-switching and redirect
loops. No outgoing default proxy. Never return a raw URL or raw backend error.

Support single bytes Range, 200/206/416 and HEAD correctly; validate upstream
status/size/range instead of labelling a full body as partial. Per-request 30min
stream lifetime with cancellation and bounded concurrency; shutdown cancels and
joins before pool close. Non-stream read deadlines stay short. Attachments use
safe filename disposition; HTML/SVG/script/executable never inline as active
documents. nosniff, private/no-store and same-origin framing/media CSP.

Latest human correction supersedes native-browser PDF rendering: download
preview originals into a private server cache for one hour and render there.
Every cache hit still revalidates auth and exact canonical identity. Cache key:
binding/resource/path/name/size; same-size edits may remain cached until TTL,
not claimed instantly fresh. PDF <=128MiB, total cache <=1GiB. TTL enforced on
reads, disk purge every minute. Only owned cache files are removed, never originals.
Video remains Range-streamed and attachments direct, not full-video cached.

Text/Markdown preview <=2MiB, UTF-8 literal display. Office zipped input <=16MiB,
bounded archive entries/uncompressed XML and output; reject bombs/malformed XML.
DOCX/PPTX textual content, XLSX stored cell values (no formula execution), clearly
labelled text-only rather than original-layout rendering. PDF uses server PNG pages;
unsupported formats remain downloadable, not presented as successful preview.
No macros, executable content, transcoding or server document execution.

## Sharing reference (not implemented by this access slice)

Verified current CloudSite primary source via its GitHub contents API:
`apps/api/cloudsite/shares/service.py`, `routers/previews.py`, 2026-10-15.
Reuse product interaction ideas: single-file target, expiring link (5m/1h/6h/24h/
7d), optional share code, inspect and revoke, revocation preserving the file.
Do NOT copy its database, source mapping, permanent-link defaults, placeholder
CAPTCHA check or unsigned public Office-cache behavior. Panta sharing needs its
own persisted owner/expiry/revocation contract and original ShareProvider goal
remains distinct; no share is created while implementing content reads.

## Tests and deployment

Fake HTTP protocol and identity tests cover auth denial, cross-scope/path/ID,
unknown/directory/removed/ambiguous, URL/redirect SSRF, token/header isolation,
Range/HEAD/errors/cancellation, zip/output limits and literal preview. Existing
Go/race/architecture/vet/build, Node and PG16/18 exact-head CI remain required.
Real candidate validation reads existing small documents and video ranges only;
verify actual body signatures/bytes plus browser preview/playback/download.
No real-file upload/delete or native provider StartDownload.

Deploy reviewed binary/config only to owned product UI service, preserve previous
hash receipt/config/binary for rollback. Existing OpenList account is used via a
new protected server Token file, no password/role change or native session refresh.
Retain localhost listeners. Update startup guard for explicit optional content
config, never weaken original DSN/port/binary identity checks.

## Candidate evidence - 2026-10-15

On the isolated .50 loopback 8181 candidate, existing real files were read, not
uploaded or fabricated: PDF 165987 bytes, DOCX 15423, XLSX 8516, PPTX 35627,
Markdown 8004. Full downloads returned correct lengths/signatures. Office and
Markdown server previews returned literal content. PDF server rendering produced
three pages; browser images were 990x1400 and page 1 -> 2 worked. Cache-hit
authorization and one-download/multiple-hit behavior are regression-tested;
TTL/cleanup/concurrent misses are tested without waiting an actual hour.

Real Bilby.mp4 playback advanced with readyState 4, no media error, 1280x720 and
481.44-second duration; keyboard seeking advanced playback. Backend Range
0-65535 returned 206 and 65536 bytes, HEAD 200 with no body and total 129705894.
This proves this MP4/browser combination, not all codecs or transcoding.

All Go unit/architecture/race, vet/build checks, 20 Node tests and nine owned
service guard tests pass locally. PostgreSQL and exact-head CI must still pass
before acceptance. No native 115 login, acquisition StartDownload, metadata scan,
Hint, schema migration, role escalation or share creation was performed.

Accepted/deployed: PR #91 reviewed HEAD `34863813a53f8b1be187bfb9ceaecafbab4cb886`,
CI `38699810879` all three SUCCESS, same-agent review `5529707246`, merge
`e5783b5fcef27f3a6f520b08d5773819d3f6aea5`, merge CI `38700142744` SUCCESS.
Formal8180 repeated all file checks, including PDF page 1 -> 2 -> 1 and unchanged
cache mtimes. Only owned product service restarted, clean binary SHA256
`3625b49c028962faacd492642385b6e760897946dced92d3ab3ad700c3116c2c`.
Catalog PID/generations 0/1/1, original staging table hashes and unrelated services
unchanged; old receipt/config/launcher/binary retained. No full MVP or share claim.
