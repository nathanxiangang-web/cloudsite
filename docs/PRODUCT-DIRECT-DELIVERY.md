# Direct file delivery versus private document preview

Issue105 / D-052, direct human correction,2026-10-15.

## Verified difference

Local .50 runs d5d056b's byte proxy. Public initially runs68d9457's byte proxy.
The HTTP streaming branch is unchanged; the adapter delta only adds protected
token reloading. Neither implementation issued302. Local acceptance proved
functional playback/Range/download over LAN/SSH, not public direct delivery.
Therefore the absence of302 is a transport implementation gap, not an IndexCore
failure or evidence that a reverse proxy stripped a redirect.

A Windows client read an existing public-OpenList-resolved115 URL directly:
2MiB,206,0.640s,3.125MiB/s, matching SHA256. Earlier public proxy serial samples
took21.140/22.850s,0.095/0.088MiB/s. Samples are not sustained-capacity guarantees.

## Modules and dependency direction

1. `fileaccess.Deliver`: product authorization and exact IndexCore canonical
   identity/PRESENT/file/size/mount validation, then a narrow LinkSource port.
2. `integrations/openlist/content.ResolveLink`: authenticated known-file metadata
   only; bounded caller User-Agent, rotating protected app token; no file GET.
3. `producthttp`:302 Location with no response body, no-store/no-referrer. The
   browser follows to upstream Range206. CSP media-src permits only the exact
   configured HTTPS CDN hosts, never all HTTPS destinations.
4. `fileshare`: current grant/expiry/revoke and immutable target checks before
   delivery; invalid or revoked grants never resolve links. No new schema.
5. `PreviewService`/`filecache`: unchanged private one-hour document cache and
   server rendering. Preview never returns a source URL. Its byte-stream port is
   separate from redirect delivery, with canonical checks even on cache hits.

IndexCore owns resource truth, not product auth, upstream URLs or file streaming.
No direct OpenList catalog scan/visibility decision is added.

## Configuration and failure behavior

Protected file-access config may set `direct_link_hosts` to exact lowercase DNS
hosts (at most16). Empty/omitted preserves proxy delivery for rollback. Nonempty
enables direct ordinary AND guest delivery. Permit only HTTPS URLs without
userinfo/fragment, matching an approved host on443. Missing/wrong metadata,
unapproved/private/internal URLs, malformed UA, canonical mismatch or invalid
Range fail closed: no silent server-proxy fallback and no secret errors.

Forward only caller UA to metadata resolution, never product cookies/grants or
OpenList Authorization to the CDN. Direct mode issues a temporary upstream bearer
URL to an authorized client. Product revoke denies NEW redirects; it cannot
revoke already issued CDN URLs before upstream expiry, including resumed requests.
This limitation supersedes older 'source URL never visible' promises for file
delivery, not for previews. CDN controls final filename/disposition, content type,
link lifetime and browser-compatible codecs. Do not promise transcoding or speed.

## Validation and deployment boundary

Tests cover authorization before metadata, wrong identity/missing/removed/
ambiguous/directory, unsafe URLs/hosts, UA and token isolation/rotation, zero file
GET on direct requests,302 GET/HEAD/Range, guest grants/revoke/target mismatch,
restricted media CSP, private preview and proxy compatibility. Existing full
unit/architecture/race/vet/build/Node and PG16/18 CI remain required.

Deploy only product binary and protected file-access config. Preserve image,
binary/config hashes and old config before cutover. No DB/index changes, scans,
Hint/Worker/native115 StartDownload or historical acquisition mutation. Verify
real302->CDN206/byte hash, document preview, ordinary and redeemed guest downloads,
video playback and seek. Rollback is prior product image/config, no data restore.
