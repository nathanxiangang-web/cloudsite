# Product file downloads and expiring shares - 2026-10-15

Human priority: continue real downloads and sharing, referencing CloudSite.
This bounded slice is Panta-hosted single-file sharing, NOT native 115 shares,
provider execution or full Gate4/MVP acceptance. Native ShareProvider remains
a separate future adapter. No source/Job/Manifest/ProviderTask or StartDownload.

## Module boundaries

- `fileshare`: create/owner-list/revoke/redeem/grant domain and Store port;
  fileaccess validates the exact canonical identity before creation and download.
- `store/postgres`: append-only v14 shares/grants, hashes only, DB-time expiry,
  current enabled owner, owner-scoped revoke/list, bounded grants/active shares.
- `producthttp`: authenticated owner routes, same-origin guest redemption and
  grant-cookie download routes. Reuse fileaccess byte and HTTP Range semantics.
- Embedded consumer UI: per-file download/share, expiration/code/link-copy and
  owner share list/revoke. Guest `/s/` needs no product login but only the single
  granted file; never directory browse or arbitrary path/URL input.

Create validates exact binding/resource/path via Q5 PRESENT (including any Copy
availability), then stores immutable target/name/size. Share token 256-bit random,
SHA256 stored, returned once in URL fragment (not query/server request logs).
Optional code is bcrypt hashed. PR #95 / D-048 supersedes the old6-32 ASCII
creation rule: new codes are exactly4 A-Z/a-z/0-9, browser-random on form open;
clearing means no code and is not refilled on submit. Existing longer hashed
codes still redeem. Expirations 5m/1h/6h/24h/7d, no permanent
default. Link must be presented with code to get a separate 256-bit hashed grant;
knowing the link token alone cannot forge a code-verified grant cookie.

Grant cookie HttpOnly/SameSiteStrict/scoped to share content path and <= share
expiry. DB checks expiry/revoked/enabled owner on every new download, then Q5
exact identity is rechecked through existing fileaccess. Revoke blocks future
requests/grants, not bytes already in flight. No source file deletion.
Cap 100 live shares/owner, 100 live grants/share; bounded anonymous redemption
rate/concurrency. Owner list has no tokens, hashes, source paths or credentials.
No outgoing CDN credentials/raw URL disclosure; reuse existing bounded transport.

Download remains streamed, no whole-file browser Blob/memory buffering. Browser
download control explains that save/progress is managed by browser; errors during
access probe are visible instead of leaving users on an unexplained JSON page.

## Persistence and rollout

v14 only adds product sharing tables to the OWNED product v13 database after
review. Original staging v12/index DBs unchanged. Preserve Issue83 WIP untouched;
its unmerged draft v14 must be rebased/renumbered later, not silently deployed.
Database transition is backed up; stop only owned product/candidate processes,
apply reviewed append-only migration, start reviewed binary. Rollback service
to reviewed v14-compatible metadata/content binary or restore owned DB backup
only if no newer product data would be lost; do not run old strict-v13 binary
against v14 and claim it works. No listener/public-network expansion. Current
localhost links work for clients with this service reachable; not internet shares.

Tests: unit Store/identity/code boundaries; real PG expiry/revoke/current owner,
hash-only grants, concurrent caps; HTTP auth/Origin/path/Range/HEAD/code isolation;
DOM late-result/logout/error/revoke; all exact-head existing three CI groups.
Staging acceptance uses a real small file, one temporary product share, separate
guest redemption/download and revoke -> denied. No 115 native share is created.
CloudSite service.py is the inspected primary reference for expiry/code/revoke
UX only, not copied storage/CAPTCHA/permanent-link/unsafe preview behavior.

## Current code/scope extension

PR #95, exact HEAD `e5961f8fe060`, CI `38733576527` and merge CI `38734637547`
all three SUCCESS, same-agent review `5533047604`, merge `9c902a3650df`.
Four-character/empty code and a real legacy old-Binding share passed against
candidate and formal service; copied result keeps its original code even if
the input later changes. All test links revoked. Product schema14 unchanged.
Complete CloudSite283 files now selected in consumer; old three backend read
Bindings remain for old shares. [Current contract](PRODUCT-CLOUDSITE-LIBRARY.md).

## Original sharing acceptance/deployment evidence

PR #93 reviewed `d98f79a2d39f6add0e6d7d4bb98126516623bfa7`, exact CI
`38702187329` all three SUCCESS, same-agent review `5529930361`, merge
`49b98b21bff0c6be3e11f05a6292ba6c383da3af`, merge CI `38702621096` SUCCESS.
Actual candidate browser: normal/guest DOCX downloads15423 bytes, same SHA256
`2bc8090eca36718b3c682ba66f9b3f1a97b88be0d850a1f7c55f1ffedf298925`;
wrong code denied, UI revoke then download denied. Formal8180 separate guest
client has no product session; download/HEAD/206 Range and revoke/denied redeem
pass. Formal UI download also checked. Test shares revoked, originals unchanged.

Go/race/architecture/vet/build, 24 Node, ten Python and isolated real PG tests
pass. Product-only explicit13->14 migration with protected backup; reviewed14
sharing-disabled content-only fallback started/verified before feature enable.
Binary SHA256 `38d8f0c5c75053b263c98c8cd0e0997f9c7be074b2eaa8584583d6e5f2eb951c`.
Query PID/generations0/1/1, original staging table hashes and unrelated services
unchanged. No native115 share/download/login, scans/Hint, role changes or file
deletion. Existing PDF/Office/video/content regressions pass. No full MVP claim.
