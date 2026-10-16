# Cloud directory observation and unavailable UI — 2026-10-17

## Scope and acceptance

PR115 / Issue114 / D-056: reviewed exact HEAD
`bbb38ae1e00271b1c7c8454594755f55bfce9f80`, exact-head CI38967538356 all3 SUCCESS,
separate same-agent review (not independent), merge
`894962f6bb5aa2a24e8ffc172eda06738fe6b6a2`, merge CI38967953503 SUCCESS.
Full Go/race/vet/build,43 Node cases and owned PostgreSQL/runtime/CLI tests passed.
Issue116/PR117 changes only the folder-neutral footer and adds one Node regression.
Reviewedf0807cc014231b36e24c16e07e28c844a7d5d51a, exact-head CI38970141986
all3 SUCCESS, separate same-agent review, merge987adb3af3910ff3d634b2b0b699304b8b237f40,
merge CI38970604155 all3 SUCCESS;44 Node cases. Product-only follow-up publicly
deployed as cloudsite:cloud-scope-987adb3, replacing the product894962f image below.
Post-switch health/audit/284-file/login/prepare/admission404 pass; original service
PIDs/root generations unchanged. Browser verifies footer and1440/390 layouts
without horizontal overflow. Temporary processes stopped, all records retained.

Public deployment: https://cloud.netioi.com/downloads/ on113.20.6.57.
Product image `cloudsite:cloud-preview-894962f`; public DB explicitly14 -> 15.
This is pure source preparation plus truthful unavailability, NOT native download
or full MVP acceptance. Managed runtime, admission and Worker remain disabled.

## Module and data boundaries

- Existing physical115 directory `/云下载` is exposed by standard OpenList Alias
  inside the existing application account scope. No source file moved, no
  OAuth/native credential changed, no account root expanded.
- IndexCore owns observation/canonical facts. Its new dedicated DB starts from
  an original-catalog backup, retaining old resource/root identities/generations.
  Original catalog/query remains intact for recovery.
- New cloud root scans only the alias subtree. Initial broad scan timed out;
  its FAILED record retained, NOT accepted as success. Corrected bounded full
  scan succeeded at generation1:2 resources,1 non-directory file.
- Persistent `catalog-cloud` runs accepted IndexCore7ef47217 with a single writer.
  Owned temporary writer stopped. Query/trusted Hint remain loopback-only.
  Minimal protected credentials exclude native115 cookies; OpenList app login
  uses existing credentials rather than a static copied JWT.
- New Panta Connection/Binding remain observation-only and DISABLED, with no
  native scope/credential. Read-only library explicitly includes the binding;
  this does not authorize acquisition or prove Hint/canonical namespace agreement.
  Verify native target ID and path contract before selecting an acquisition Binding.
- Old283 files and new1 cloud file accessible through IndexCore:284 total.
  UI pagination/partial-load counts are not the full count.

## Actual public tests

Existing account login/logout; authenticated inventory284; supplied magnet pure
preparation retains BTIH/optional name; readonly admission404; own tasks0.
Browser confirms LOGIN_REQUIRED, disabled submit, preparation without download,
and the consumer entry. Available in-app browser used because Chrome disconnected;
do not label this Chrome evidence.

Original library stays283 files. Download302 ->115 CDN200 with exact15423-byte
DOCX hash; video302 ->CDN206 exact2MiB range/hash; HEAD302; anonymous401;
invalid query400; invalid Range416; private document preview. Code/no-code5-minute
guest share downloads/revoke404 pass; all test shares revoked. Protocol/byte
checks are not fresh browser playback or all-codec acceptance.

## Native blocker — Issue113 stays OPEN

Fresh native directory GET: HTTP200, valid JSON, API990001, ErrNotLogin, superseding
historical successful login receipt. No further native retry, submission, Manifest,
Job, ProviderTask, Worker or StartDownload. Authorized one-start budget unconsumed.
8 historical unknown reservations/prior accepted results untouched. Human QR/device
confirmation is needed for native session restoration; then verify native target
ID and Hint/Q5 contract. Never use the old MCP-Test tuple or guess native IDs from
virtual OpenList paths. No READY claim.

## Recovery and extension constraints

Protected server receipts/backups stay in the private live113 directory; no cookie,
password, supplied source/name/hash or provider reference belongs in Git.
Original product14 dump/config, compose and catalog retained. Tested fallback uses
reviewed15-compatible image on SAME current15 product DB with old readonly config/
query. Never run strict14 on15 or restore stale DB over new users/shares. Check
product/catalog health after switching. Original edge/OpenList/old catalog not
restarted. Changes confined to owned alias, new catalog and product composition.

Future work reuses existing admission/stage/lease machinery; no parallel Jobs,
scanner, direct OpenList canonical truth, hidden Worker or repeated StartDownload.
Directory observation, pure preparation and native execution remain separate,
replaceable, independently testable modules with explicit documented interfaces.
