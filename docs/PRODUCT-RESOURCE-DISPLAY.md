# Resource display module - user-prioritized readonly follow-up

## Contract (2026-10-14)

The user requests actual cloud-drive data for software, videos and documents.
This slice extends the accepted readonly UI under D-042/D-043, not the unreviewed
admission/usage branch. Before deployment: tests, exact-head CI and same-agent
review. No schema migration or new credentials, scope, index writer or Worker.

## Verified data boundary

Existing IndexCore QueryReader exposes three staging Roots. The selected clean
C3 Root currently contains one PRESENT file, ACQUISITION-STAGE-DISPATCH.md,
1955 bytes. This is real controlled 115 data, not a demonstration fixture or
the entire user's drive. Independent product DB has not projected its Copy.
UI must say so. No synthetic software/video entries will fill empty categories.

## Modules and dependencies

`resource-display.js` is a pure metadata presentation module: classify an
explicit file from its extension, derive labels, filter this page and format
size. It does not fetch, trust source content as instructions or alter DTOs.
`app.js` owns authenticated request/session and DOM interactions; it consumes
Q4/Q5 through the accepted product read routes. HTML/CSS provide responsive
list/cards/detail. No package/CDN or image generation is required.

Software: recognized installer suffixes (exe/msi/msix/apk/dmg/pkg/deb/rpm etc).
Video: common media suffixes (mp4/mkv/webm/mov etc). Document: common document,
Markdown/plain-text/spreadsheet/presentation suffixes. Image/archive/audio/other
remain distinct presentations but may be grouped under Other in the category
bar. A zip file is an archive, not proven software. Folders override suffixes;
missing directory flags stay unknown. No MIME or playback support is inferred.

## Interaction contract

Category controls: All, Software, Videos, Documents, Other. Each shows only
counts within the current Q4 page. Search is explicitly current-page filename
and path matching, not global IndexCore search. Next page still uses the accepted
cursor; filters never trigger scanning or combine unbounded pages behind the
user's back. List/card toggle does not perform another network read.

Click an actual file to view name/path/type/size, PRESENT and separate Copy
projection/availability. Exact locate continues Q5 and shows its result in the
same pane. Render untrusted names/paths only with textContent, never HTML or
remote poster/iframe. No fabricated vendor/version, duration, codec or previews.

File details explain the next actual capability: software download requires
access grant; video needs authorized byte-range media gateway and real codec
check; documents need bounded authorized content reading/rendering. Controls
remain explicitly unavailable until those modules exist. File access/sharing
are still MVP requirements, not removed or declared complete.

Logout/session expiry clears raw resource DTOs, rows, cards, detail, count,
search and cursors. New Binding/page or failed query clears previous details;
generation checks prevent stale request results crossing identity or scope.

## Extension and acceptance

Adding a suffix is a pure mapping change with a classification test; content
preview/access is a separate authenticated port with threat limits, not a URL
field inserted into this module. Q4/Q5/Copy boundaries remain unchanged.
Required evidence: classifier/search tests with unusual names/XSS/unknown fields,
asset/HTTP/auth guards, whole Go tests/race/vet/build, exact-head CI and real
browser file/detail/category/no-result/logout/mobile checks. Fixtures are test
inputs only and never displayed as real server data.
