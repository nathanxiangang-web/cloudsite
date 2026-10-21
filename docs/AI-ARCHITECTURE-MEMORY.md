# cloudsite — AI Architecture Memory

**Latest user priority (2026-10-20; DESIGN ONLY):** Website Admin UI/management feature blueprint [Issue #141](https://github.com/nathanxiangang-web/cloudsite/issues/141), [proposed D-066](DECISIONS.md) and [management UI spec](PRODUCT-ADMIN-CONSOLE-BLUEPRINT.md). Earlier private-user-space Stage A [Issue #139](https://github.com/nathanxiangang-web/cloudsite/issues/139) is **PAUSED by user, authorization suspended**; D-065 remains in Git and parent #138 stays open for later resumption only after a fresh explicit human instruction. Current `/admin/` is ADMIN-gated but only offers read-only resources and own tasks; no admin-wide data, provider status, audit or mutating admin API may be claimed. The new static prototype is offline mock data, not deployed. NO personal storage GB limits, no code/production/provider/IndexCore changes. The D-064 live status pointer below remains true and independent of design priority.

**New proposal (2026-10-20, DESIGN ONLY; no deployment):** User requested private 115-backed CloudSite folders and corrected earlier capacity assumption: **NO personal 520G quota, user usage balances or user byte cap**. The prior 520G is only a shared-account global reference, not a verified total/hard gate. Pinned 115driver v1.3.5 exposes `GetInfo()` for actual **account-level** all_total/all_use/all_remain; only SDK source was checked, no real account API invoked. See [Issue #138](https://github.com/nathanxiangang-web/cloudsite/issues/138), [proposed D-065](DECISIONS.md#d-065--private-cloudsite-user-folders-server-side-ownership-and-no-per-user-storage-quota-proposed) and [private-cloud blueprint](PRODUCT-PRIVATE-CLOUD-SPACES.md). One real folder per verified UUID/login label, server-side private-file ACL and gated single-file physical deletion; legacy files remain unclaimed and the ordinary public library stays unchanged. This is a **docs-only PR proposal**, not accepted implementation, live mkdir, migration, new provider call, role bypass, IndexCore write or production authorization. Gate A (mock/schema only) may be planned under a separate bounded Issue after design review; user capacity metrics are OUT OF SCOPE. Current D-064 live product pointer below remains the running system of record.

> Canonical short-form project memory for Architect/AI sessions.

Latest execution pointer2026-10-20: Issue136 / D-064 runtimebf89f5a deployed,
CI39261559684 SUCCESS/60 Node tests PASS. At02:15Z public API+asset verification
passed:274 library files/cloud0, deleted legacy/gallery absent, download302.
IndexCore115/PR119 runtime2d9068e restores three fresh trusted root scans and
six confirmed Q8 removals; unrelated identities and all4 copy-conflict paths
preserved. Empty Alias disabled only; metadata/history and eight reservations
retained. No frontend scanner, fabricated absence or live-browser playback claim.
Schema15/AI-disabled/provider reservations unchanged. Older blocks are historical.
>
> Active incident: **IndexCore Issue115 / PR116 mitigated, not full parity. Prior Issue132/PR133 acceptance was insufficient: virtual-only success removed749 rows. Guard399eca4/full CI39123818119 SUCCESS is deployed.300 blockers repaired,14 due INTERNAL retries explicitly resumed once,7 parents re-observed through existing CAS APIs. At11:53Z actual public272 = library270/downloads2; gallery PRESENT0, no removals since containment, sample download302 without CDN follow; product healthy/other services unchanged. Native30s root watch restored positive-only: no live topology ref/credential, latest3 root snapshots have no absence authority.4 source-present same-hash/different-path files remain unresolved under R3, open IndexCore117; no hash stripping or SQL resurrection. Full parity/automatic root deletion NOT accepted. No physical deletion/new Provider start. AI #131 remains unstarted.**
>
> Last architecture-memory synchronization: **2026-10-18**
> Current status: **Issue129 / D-062: AI planning only; read PRODUCT-AI-INTEGRATION.md. User identifies.50 (IndexCore development host) as the existing AI config source; model secrets stay there. Old Bridge is currently broken and callable credentials unverified; no AI module/model request/feature activation. First proposed slice is strict Agent contracts/mock and real-file metadata suggestions, not autonomous provider actions. Existing publicf1bceee/IndexCoree03d14a9/schema15 unchanged, last canonical proof2 cloud/main283. Original ISO remains known-reference with5/5 exhausted failures; do not resubmit or reset. No Gate5/full-MVP acceptance.**
> Governing corrections: **D-029** (IndexCore-owned observation), **D-030** (Git-first AI reconstruction), **D-032** (durable result_name locator), **D-033** (READY anchored to one canonical projected Copy), **D-034** (one-claim stage routing), **D-035** (type-scoped claiming and atomic recovery), **D-036** (one explicit runner tick), **D-037** (opt-in worker lifecycle), **D-038** (fail-closed runtime graph and secret/trust boundaries), **D-039** (protected operator bootstrap/preflight), **D-040** (deterministic source intake with exact value preservation), and **D-041** (human-authorized live staging acceptance without architectural bypass).
>
> **Read this file before planning, reviewing, or authorizing any new Gate.**
>
> If chat context, an old issue, an old PR description, README text, or an older decision conflicts with this file plus the latest accepted Decision Log / Project State, **Git wins and the newer accepted decision wins**.

## 1. Source-of-truth order

Current Issue129 / D-062 prepares optional AI behind a replaceable Agent Port,
with.50 server-side credential resolution and private authenticated gateway to
public113.20.6.57. IndexCore remains physical truth, not an LLM host module.
No AI packages/routes/tables exist on this baseline; all new names/contracts in
the plan are proposed. The user's local CodeartsBridge c190fc8 is the software
source; its private CSV seed has three populated AK/SK profiles, not imported.
Registry bus-w02-dev maps to.50; account mapping/live entitlement is unverified.
Preserve the dirty Bridge worktree and never publicly proxy its coding /api/tasks;
use a dedicated restricted adapter. CodeArts components exist on.50, but an old systemd
Bridge points at missing/home/nathan/bridge-python;8080/8765 health unavailable.
Readiness, actual model profile, restricted execution and network must be verified
before enabling. Do not revive broad shell-capable development Workers for users.
Core runtime/history remains as recorded in current PROJECT-STATE/D-061.

### Earlier execution snapshots (not the current task)

Current Issue124 / D-060:100 operations, hide UI concurrency count, preserve
server concurrency2. Fix login-form flash with silent session restoration,
401 vs transient failures distinguished. Existing default10 quota config backed
up then raised; used operations untouched, no schema/provider/session change.

Issue122 / PR123 / D-059 accepted/publice2576dc. Exact CI38994379661 and merge
CI38994534211 all3 SUCCESS. Human confirms empty cloud list is expected; no
backfill/scan needed. Main283-file list/shared markup verified; config/service
identities preserved. Actual IndexCore process incremental=true, wake5s, one
root writer. Existing scoped refresh only observes direct children, no recursive
or full-scan claim. Separate cloud files on /downloads/, not the main feed.
Use one shared file template/controller and exact Binding/canonical-path partition
before search/count/pagination. New authenticated canonical view descriptor is
read-only; no native scope/credentials or caller-selected target. Existing file
authorization and owner-only tasks unchanged. No downloads, migration or file move.
See FILE-SURFACES.md; this surface code is already deployed and accepted.

Issue120 / PR121 / D-058 accepted/deployed f8be03c. Exact c1ed10be, CI38992241383
and merge CI38992448685 all3 SUCCESS, separate same-agent review. Target physical
/CloudSite/云下载, canonical /云下载; old root directory/files unchanged. Reused
CloudSite Root/Binding/mount IDs via guarded CAS attaching a NEW native Connection;
old observation Connection remains DISABLED without credentials. Parent/child
PARTIAL Hint accepted, Q5 PRESENT directory unique. Preflight passed before the
managed serial Worker/public form enabled. Original283-file library and public
delivery/preview/share regressions pass; product/new catalog healthy, original
service identities/PIDs preserved. Website fixed30-day policy remains.

One sealed ordinary ISO smoke POST returned201 at01:54UTC on2026-10-17. Durable
REFERENCE_KNOWN, native destination matches and status RUNNING/progress0 at01:58UTC;
ACTIVE/RETRY_WAIT and zero recorded failures. No Copy/READY acceptance yet. Budget
CONSUMED; no new smoke pair or repeated start. Continue observing the same pair.
See CLOUD-ACQUISITION-ACCEPTANCE.md. No schema/queue/engine/scanner/provider rewrite.

### Historical deployment snapshots

Statements below about284-file alias visibility, NOT_CONFIGURED/LOGIN_REQUIRED,
disabled managed Worker, zero submissions or unconsumed smoke budget are preserved
history, NOT current instructions. Current native session is verified; do not
infer old990001 applies to it. Eight historical unknown reservations stay frozen.

Accepted Issue118/PR119/D-057: human selects WEBSITE fixed30-day login. Domain TTL,
DB-time persistence and cookie expiry agree; reads never slide existing expiry,
logout/revocation remains immediate, no backfill/migration or provider change.
Native115 QR recovery now SUCCESS on public server: same-account/root GET verified,
new credential protected0600; old rejected cookie retained. Historical990001 below
does not describe the new session. No submission/Worker/download. Still verify
native cloud-directory ID and Hint/Q5 namespace before managed activation;
readonly preview reason now NOT_CONFIGURED rather than LOGIN_REQUIRED. Native
cloud-directory ID/parent read verified and privately recorded; Hint/Q5 namespace
agreement remains. Reviewed2bee9a6b, exact CI38991088165 all3 SUCCESS; mergecaabc0a0,
merge CI38991282378 all3 SUCCESS. Public Cookie/DB TTL/expiry, unchanged old expiry
and logout401 verified. Recovery banner was a declared config, not live health;
it is now synchronized after verified login. Do not claim automatic health checks.

Accepted Issue114/PR115/D-056: readonly source preview and truthful login-
unavailable UI publicly deployed; exact-head CI38967538356 and merge38967953503
all3 SUCCESS, same-agent acceptance, merge894962f6bb5a. Public DB15 with backed-up
14 and tested15-compatible readonly fallback. See CLOUD-DIRECTORY-ACCEPTANCE.md.
Issue113 has zero submissions/Worker/StartDownload; historical OLD native
GET returns HTTP200/API990001/ErrNotLogin. Do not run downloads with this known-
rejected session. Human target now /云下载, standard OpenList Alias mapped into
existing CloudSite namespace without moving files or changing OAuth/account root.
Observation/public UI/API regression passed:283 original+1 cloud file, persistent
single writer and strict loopback Hint. New DISABLED observation Binding has no
native scope/credential and is not acquisition-ready. Real download awaits native
session and target-coordinate verification. Issue116/PR117 scope-only follow-up
accepted: exactf0807cc0 CI38970141986 all3 SUCCESS, merge987adb3af391,
merge CI38970604155 all3 SUCCESS,44 Node cases, public browser/API/audit pass.
Preview configuration is not an acquisition graph or health service.

Accepted Issue111 / PR112 / D-055 composes the managed product acquisition action
and fixed-target consumer task/submission UI. Reviewed0555d3b4,
CI38907187975 all3 SUCCESS, separate same-agent acceptance, squash51211b553bac.
Read PRODUCT-CLOUD-RUNTIME.md. Next is bounded public integration on113.20.6.57.
Readonly
action stays separate; owner-key GET reconciles uncertain submissions without
provider calls. Issue83/PR110/D-054 is accepted at ec568ae1: exact-head
CI38901805468 and merge CI38902038377 all3 SUCCESS, same-agent review5547872267.
Public15 supersedes former public14; historical12 unchanged. Historical native
login success is superseded by990001. Public managed runtime remains off; pure
preview UI is enabled. Keep backed-up14,15-compatible fallback and old catalog. Preserve
the old dirty checkout and8 historical unknown reservations.

Accepted Issue108 / PR109 / D-053 prepared cloud acquisition: see
[verified gaps and modular sequence](PRODUCT-CLOUD-DOWNLOAD.md). Pure PrepareSource
adds explicit magnet-btih-name-v1 (BTIH+optional dn) while retaining raw input.
D-054 uses it only on new durable product reservations. ResolveSource, old
submissions and adapter remain byte-exact. No native call/public migration/
Worker/UI activation in this slice. Old dirty Issue83 draft14 remains unchanged;
new development uses append-only15 after sharing14, never deploys the old draft.

Current Issue105 / D-052 supersedes server-byte-proxy-only delivery, not IndexCore
truth: protected direct_link_hosts opt in to authorized302 for ordinary/guest
content; document previews remain private streamed/cache/rendered. Existing
local and public code both proxied bytes; local functional acceptance was not
public302 acceptance. No silent fallback or bypass of auth/Q5/grants. Issued
upstream bearer URLs remain usable until upstream expiry after product revoke.
See [module contract](PRODUCT-DIRECT-DELIVERY.md) and
[actual acceptance](DIRECT-DELIVERY-ACCEPTANCE.md). PR106 reviewed2f873bfa03a4,
CI38762045631 all3 SUCCESS, same-agent review5535806545, squash0b29ecc88faff5f9,
merge CI38762378328 all3 SUCCESS. Public product image carries that revision;
binary SHA256dbdfb6e5baebd0fb352e756b24bd691283a41573b9860a4930158dfeb7baa922.
Actual browser observes302->115206, playback through149s with seek/error=null,
and a fresh15423-byte native downloaded DOCX with matching SHA256. Download-event
automation returned no event, so completion rests on the verified new local file.
Candidate stopped; other services/DB/catalog unchanged. Old image/config retained.

Current superseding runtime: PR102 reviewed22705d031adc, CI38748673995 all3
SUCCESS, review5534538189, merge68d945793b8f, merge CI38749342436 SUCCESS.
Minimum6-character public registration/login verified;5 rejected; old hashes
unchanged. PR103 reviewedcfc283178bb5, CI38754182448 all3 SUCCESS,
review5535047887, merge7bfe63d7af4c. Application renewal follows actual protected
OpenList identity, not the previous AList user5 assumption.

The human explicitly expanded service scope: original root-compose alist is now
replaced by local-config OpenList v4.2.6, pinned digestc555c6e1c8af2aead38ed12ec761ac077fdf046d19cf033414be8e056aec6b64.
Old AList data/image/YAML remain recoverable; container removed. Other three
root-stack containers unchanged. Local user3/CloudSite namespace/permission65341
preserved, NOT falsely called permission0. Copied application JWT was rejected;
existing local app username/password successfully obtain a new app token.
No forged token, password reset, native115 login or new acquisition. Four copied
OpenList transfer-queue rows omitted ONLY from the new clone before startup;
original local DB and historical provider reservations unchanged. Actual public
OpenList and cloudsite real-file regression pass. Latest release BUILD-MANIFEST
anchors final build; no full-MVP/realtime-indexing claim.

Latest deployment: Issue98 / PR99 reviewed93606e67ccf2, CI38745269165 all3 SUCCESS,
same-agent review5534142444, merge7368671aa0af. PR100 quoted tmpfs fix reviewed
7a8ab8fe1694, CI38746403395 all3 SUCCESS, review5534272835, merge841c4d14fb54;
merge CI38746768026 all3 SUCCESS. Candidate/public TLS and real-byte acceptance
passed. Local USER hashes/password unchanged; new public USER registration,
duplicate rejection and ordinary admin denial verified.283 canonical files,
DOCX15423-byte download/text preview, video206 Range, both share-code modes pass;
temporary shares revoked. Original four public-server services untouched.
Human then requests minimum6-character passwords and a local-OpenList credential/
configuration migration to independent remote OpenList. Preserve public AList,
credentials and8 historical reservations; no provider/Worker/Hint/scanner.
Final accepted release code is identified by GitHub release BUILD-MANIFEST, not
inferred from the earlier initial-deployment SHA. [Alpha contract](CLOUDSITE-V1-ALPHA.md).

Latest: Issue #96 / PR #97 reviewed HEAD `d5d056b0e1e2`, CI `38736852134`
all3 SUCCESS, same-agent review `5533296802`, merge `57a9d626cd62`, merge CI
`38737121369` all3 SUCCESS. Real-link native attachments replace asynchronous
HEAD/synthetic clicks, no fake completion claims. Explicit media controls and
event-based state. Actual user Chrome had stopped candidate8182; formal8180
restored MP4/MKV metadata, native download human-confirmed and reference video
playback beyond37s/pause verified. Automated download event NOT observed; full
formal DOCX bytes/hash and HEAD200/Range206 independently verified.28 Node tests
and full CI pass. Only product replaced; schema14/source/original DB/catalog/
services unchanged. Binary SHA256
`84b5c2cda9b5778433c87ec4a9b84e9d026d7b03828d1e5799a9185b3e19c194`.
Old14 receipt/binary retained; no provider/scan/Hint/session or codec guarantees.

Previous: Issue #94 / PR #95, exact HEAD `e5961f8fe0603eb8e2ee0c862abfbdd68d79c6b2`,
CI `38733576527` all three SUCCESS, same-agent review `5533047604`, merge
`9c902a3650df17c8202e3d04985a20c562389087`, merge CI `38734637547` all three SUCCESS.
D-048 [contract](PRODUCT-CLOUDSITE-LIBRARY.md): full CloudSite logical root,
one new isolated IndexCore Root and one explicit scan;283 files/474 directories,
42software/188video/6document/47Other. All16 Q6 pages verified; UI120/120/43,
all files reachable and directories hidden. Four backend read Bindings preserve
old shares, consumer selects only new observation-only DISABLED Binding.
Random4-character A-Z/a-z/0-9 on new form; clearing keeps empty/no code. Browser
and formal API verify both modes, copied code, old longer-code share and original
DOCX download hash. All test shares revoked/candidate stopped. Product-only
cutover, schema14 unchanged; old14 binary/config/launcher retained for rollback.
Deployed SHA256 `983e10735db46b94cfd5d404372e8e97c788bfd05ff8aa58d832e4348d287f67`.
Original DB histories/query PID/old root generations/unrelated services unchanged.
No source mutations/native115 download/share/login/scheduled scan/Hint. Issue83
WIP preserved; no full MVP claim. Old six-file/three-subtree defaults below are
historical, not current constraints.

Previous: Issue #92 / PR #93, exact HEAD `d98f79a2d39f6add0e6d7d4bb98126516623bfa7`,
CI `38702187329` all three SUCCESS, same-agent review `5529930361`, merge
`49b98b21bff0c6be3e11f05a6292ba6c383da3af`, merge CI `38702621096` SUCCESS.
D-047 [product sharing contract](PRODUCT-FILE-SHARING.md): cloudsite-hosted single
canonical file, hash-only link/grant tokens, optional bcrypt code, DB-time expiry,
owner management/revoke, guest grants/downloads. Not native115 ShareProvider or
public internet distribution; current localhost service must be reachable.
Ordinary/guest browser downloads hash-match; wrong code and revoked link denied.
Separate anonymous formal client has no product session; download/HEAD/Range
and revoked redemption denied. Test shares revoked. Content regression passed.
Owned product DB14 only, protected13 backup. Reviewed14 content-only disabled-
sharing fallback started/verified; NEVER point strict13 binary at14. Binary
SHA256 `38d8f0c5c75053b263c98c8cd0e0997f9c7be074b2eaa8584583d6e5f2eb951c`.
Original staging12 histories/query/generations/services unchanged; zero native
provider/share/download/login/scan/Hint. Issue83 WIP unchanged, draft14 must be
renumbered later. Its older 'no v14 deployed' pointers below are now historical.

Previous accepted content slice: Issue #90 / PR #91, exact reviewed HEAD
`34863813a53f8b1be187bfb9ceaecafbab4cb886`, CI `38699810879` all three SUCCESS,
same-agent review `5529707246`, merge `e5783b5fcef27f3a6f520b08d5773819d3f6aea5`.
D-046: known-file bytes may use the server-only OpenList content adapter AFTER
exact Q5/PRESENT/size/directory authorization; acquisition visibility remains
IndexCore-owned. Every one-hour private document cache hit revalidates identity.
PDF server-rendered PNG pages, Office/text safe text-only, native Range video and
attachments; no raw URL/Token or third-party viewer. Formal8180 real downloads,
three-page PDF/paging, Office/text, MP4 playback/seek verified. Binary
`34863813a53f` SHA256 `3625b49c028962faacd492642385b6e760897946dced92d3ab3ad700c3116c2c`.
Only product restarted; query PID/generations/original DB hashes/unrelated
services unchanged. No provider start/native login/scan/Hint/migration/role change.
Rollback receipt/launcher/binary retained. [Contract](PRODUCT-FILE-CONTENT.md).
Sharing is reference-only; preserved Issue #83 and current-DB Copy work remain.
The previous file-library slice below is historical, not a ban on accepted content.

Previous accepted slice: Issue #88 / PR #89, reviewed HEAD
`8b7d1c23feabc0a76100d975b1ae9ebe1b4ec16b`, CI `38673309830` all three SUCCESS,
same-agent review `5529290784`, merge `963c66d9c0d7735fbf8ae0a8ab0c7a4000d6bb1d`
and merge CI `38695548823` SUCCESS. D-045: retain two UIs, file-first `/` with no
directory/topology controls and ADMIN operational `/admin/`. Existing real Q4
sources are composed directly with bounded internal folder traversal, no manual
per-file approval or new data source. Six actual files verified at formal8180.
Only owned product service replaced with accepted `8b7d1c23feab`; IndexCore query
PID/generations, original staging database hashes and unrelated services unchanged.
No role/credential/provider/scan/migration change. Playback/access/Issue #83 remain.
See [contract and actual evidence](PRODUCT-FILE-LIBRARY.md). Source scope below
remains PR #87; its old product binary default is superseded by PR #89.

Previous accepted source/lifecycle slice: Issue #86 / PR #87, exact HEAD
`d693878c7791b592fa21668a920abd322df9aa37`, CI `38669580171` all three jobs
successful, same-agent review `5527219011`, merge
`ddecd51c19b7431d9ca3611add3304eea725240a`. The formal 8180 scope now uses three
new observation-only DISABLED Bindings in an isolated IndexCore catalog; this
supersedes the previous one-document default described below. Five real document
files/three directories and one MP4 are visible; selected software directory is
empty, not fabricated. See [actual evidence](PRODUCT-REAL-CATALOG.md). Two enabled
owned user-systemd services had a controlled graceful restart test, not a full VM
reboot test. No new provider start, content read or scheduled scan. Deployed product
binary remains PR #85, schema v13; preserve separate Issue #83 WIP. D-044 forbids
file delete UI/API; drive deletion does not bypass conservative canonical rules.

Current authority: D-042 delegates Architect and Developer phases to one agent,
with contracts, tests, CI and honest same-agent review before each merge. Ordinary
in-scope work needs no repeated authorization. Start with the
[documentation audit](DOCUMENTATION-AUDIT.md), Issue #75 / PR #76 (merged `03719b5`, exact-head CI `38501814871` successful), then
[core MVP plan](GATE4-IMPLEMENTATION-PLAN.md). The
[domain model](CONTROL-PLANE-DOMAIN.md) is design, not implemented capability;
accepted domain/runtime ownership is unchanged.
Original Gate 4 auth/access/share/usage is not replaced by the PR #73 control
console or later product/AI guidance. Issue #77 now adds product auth/session
domain, persistence and handlers; [PRODUCT-AUTH](PRODUCT-AUTH.md) records their
scope/tests/limits. Permission/usage/audit persistence and heartbeat/full task-history stores remain missing;
proposed entities and ports must not be reported as implemented capabilities.
Issue #74's vague skeleton is superseded; frozen stage contracts govern coding.
Auth acceptance: exact HEAD `95ca01a6cf450b20f4f2e357587cbeb31b84b78a`, CI
`38506025321` all three groups successful, PR #78 merge `eeedd3af242353822c0541cd1e74e48475fe596d`.
Read acceptance: PR #80 merge `574124aa997464eb3305fcc6ce2d5ddcb9ff28f7`,
exact HEAD `f8c0607c73cbe640aeec06be0d39c11cd9296bd1`, CI `38507300691` all successful.
Readonly UI acceptance: PR #82 merge `8b86ae336b8b88b00bb0a9b6ab787695126eb060`,
exact HEAD `e82d2fed1fa92ac68702a4189a336f2134f8f61c`, CI `38537678423` all successful.
[PRODUCT-READONLY-UI](PRODUCT-READONLY-UI.md) records actual scope, real-data
connection and rollback. D-043 brought presentation forward; Issue #83 is next
for exact-source admission/usage, with contracts before coding. No full MVP.

Historical PR #85 presentation baseline, superseded in source scope by PR #87:
the user's software/video/document display request advanced one bounded
readonly follow-up: Issue #84 / PR #85, merge `0ddf314554cd5b252969e9e1d82a06f9b36469c8`,
exact HEAD `af23cf2746a0fa3f2266a8d7c95f4a0be550ec6d`, CI `38575852403` all three
groups successful, same-agent review `5519125136`. Pure extension hints,
current-page counts/search, list/cards and structured detail are deployed to
the owned .50 loopback UI. One real 1955-byte Markdown file was browser-tested;
software/video have no records in this scoped Root. No whole-drive/content/
playback claim. [Module contract](PRODUCT-RESOURCE-DISPLAY.md) preserves access
and Copy boundaries. Preview stays v13; unreviewed admission v14 WIP is separate.
After the VM reboot, only readonly processes/required databases were resumed,
original hashes and unrelated service states unchanged. Resume Issue #83, then
real Copy projection/access rather than continuing metadata-only substitutes.

Readonly UI runs on .50 and the user accesses it via a loopback SSH forward.
Historical initial real-data hookup (the selected source is now the isolated
PR #87 catalog above): the explicit real-data request superseded synthetic preview:
fixture stopped, original staging PostgreSQL restored without migration, owned
IndexCore QueryReader/Readiness/httpapi composition from pinned 7ef47217 with
DB read-only sessions, no writer/Hint/scanner. Original public-table and provider
history hashes compare unchanged; unrelated services unchanged. Actual existing
MCP-Test resource/Q5 verified. New product DB lacks projection so UI does not
claim file access or that the original accepted Copy disappeared.
No full MVP acceptance has happened. New migration
head v13 is tested only in owned disposable PostgreSQL; live Gate 3.18 v12 is not upgraded. Historical unknown tasks and unrelated
services stay protected. One agent, separate planning/development/review passes.

Current acceptance: [Architect #6143267673](https://github.com/nathanxiangang-web/cloudsite/issues/62#issuecomment-6143267673)
supersedes all older NO-GO, OPEN and recovery-development pointers below.
PR #71 merged at `74e9e69` after exact HEAD `3e61483b`, review `5501296768`
and CI `38374274046` (all three groups SUCCESS). Under GO `6142455770`, the
original KNOWN task completed reviewed paired recovery and poll-only execution:
provider SUCCESS -> Hint -> unique Q5 PRESENT -> Q8 resource-added sequence 1
-> existing Projector PRESENT Copy/cursor 1 -> `result_copy_id` + Manifest READY
+ same Job SUCCEEDED. Claim 22 -> 25; failure 0/max1 and reference preserved.
Resume 1 / Worker 1; new Submit/Manifest/Job/ProviderTask/StartDownload all 0.
Final Worker/runnable 0, Binding DISABLED, collector permission 0. Old eight
unknown reservations and unrelated Root/service records unchanged. Schema v12.
See [redacted completion artifact](GATE3.18-COMPLETION.md) and
[recovery contract](KNOWN-REFERENCE-RECOVERY.md). No further real 115 test is
required for auth; no old GO/unknown reservation replay or manual pair requeue.
D-042 and Issue #83 supersede the former documentation-only handoff.
Chronological snapshots below retain historical evidence, not current authority.

When reconstructing project truth, read in this order:

1. `docs/AI-ARCHITECTURE-MEMORY.md` — compact current invariants and anti-drift rules.
2. `PROJECT-STATE.md` — current accepted gates and exactly one authorized next task.
3. `docs/DECISIONS.md` — append-only architecture decisions; later decisions may explicitly supersede earlier ones.
4. `PROJECT-CONTEXT.md` — durable product/architecture context.
5. `docs/MVP-BLUEPRINT.md` — end-to-end product blueprint.
6. Current GitHub Issue / PR for the active Gate.
7. Code + tests + CI on the exact PR head.

Do **not** infer current architecture from chat memory alone.

## 2. Project identity

cloudsite is a provider-neutral resource acquisition + logical catalog platform.

115 is the first provider, not the product architecture.

Logical identity:

```text
Asset
  ↓
Release
  ↓
Variant
  ↓
Copy
  ↓
IndexCore resource_id
```

Search is optional. Known-root/path flows and acquisition must work without Search.

## 3. Hard architecture boundary

### Observation plane

```text
Storage / 115
    ↓
OpenList
    ↓
IndexCore OpenList Collector
    ↓
IndexCore Canonical + Journal
    ↓
cloudsite IndexCore Query / Journal consumer
    ↓
cloudsite Catalog Projector / Copy
```

**IndexCore owns physical truth.**

cloudsite does not:
- scan OpenList;
- query OpenList to decide canonical visibility during acquisition;
- read OpenList DB;
- read/write IndexCore DB;
- invent physical presence independently of IndexCore.

OpenList is an upstream dependency of IndexCore's observation pipeline, not a cloudsite acquisition truth source.

### Control plane

```text
cloudsite command
    ↓
Job Engine
    ↓
Provider Contract
    ↓
Provider Adapter
    ↓
Storage / 115
```

Provider success never overrides observation truth.

## 4. Acquisition synchronization — current canonical flow

```text
Acquisition Manifest
    ↓
durable ACQUISITION Job
    ↓
connection-scoped Downloader session
    ↓
115 adapter
    ↓
provider task
    ↓
provider reports SUCCEEDED
    ↓
durable Manifest.result_name
    provider ResultName > ExpectedName fallback
    ↓
Manifest AWAITING_VISIBILITY
    ↓
cloudsite sends trusted IndexCore Mutation Hint
    root_id   = StorageBinding.indexcore_root_id
    scope_key = Manifest.target_path
    reason    = POSSIBLE_CHANGE
    ↓
IndexCore-owned incremental runtime
    ↓
IndexCore OpenList scoped verification
    ↓
IndexCore Canonical + Journal
    ↓
cloudsite canonical confirmation / projector
    ↓
Copy becomes PRESENT / READY workflow completes
```

Critical meanings:

- `PROVIDER_SUCCEEDED` != Job `SUCCEEDED`.
- `PROVIDER_SUCCEEDED` != Manifest `READY`.
- IndexCore Hint `202 Accepted` != canonical confirmation.
- cloudsite does **not** add a direct OpenList visibility verifier.
- Same ACQUISITION Job spans provider, observation-handoff, and canonical stages.

## 5. Coordinate separation

Never derive one coordinate from another:

```text
provider_scope
    provider-side mutation target, e.g. 115 saveDirID

openlist_mount_path
    IndexCore/OpenList observation configuration

indexcore_root_id
    canonical IndexCore root identity

manifest.target_path
    normalized directory scope inside the selected binding/root
```

Current Mutation Hint mapping:

```text
root_id   = indexcore_root_id
scope_key = manifest.target_path
reason    = POSSIBLE_CHANGE
```

Do not use `openlist_mount_path` or `provider_scope` to build the Hint.

## 5.1 Deterministic acquisition result locator

Before provider success may advance automatically beyond the provider stage, cloudsite must durably resolve one top-level locator:

```text
valid provider-observed TaskStatus.ResultName
        >
Manifest.ExpectedName fallback
        ↓
Manifest.result_name
```

Canonical candidate identity for the later gate is:

```text
StorageBinding.indexcore_root_id
+ Manifest.target_path
+ Manifest.result_name
```

Semantics:
- `target_path` remains the directory scope refreshed by IndexCore;
- `expected_name` remains optional request-time intent/fallback and is not rewritten by provider execution;
- `result_name` is the immutable durable acquired-result locator;
- a valid provider-observed name wins over ExpectedName;
- blank/absent provider name may fall back to a valid ExpectedName;
- an invalid nonblank provider name fails closed instead of silently falling back;
- no provider FileId/DirId, OpenList path, Journal ordering, timing window, newest-item or only-item heuristic may replace result_name;
- result_name is locator evidence only, never Canonical truth or READY.

Gate 3.10 uses the frozen locator for exact canonical confirmation before READY.

## 5.2 Canonical READY anchor

Gate 3.10 finalizes one acquisition only after the pre-canonical locator has been resolved through IndexCore and the existing Journal Projector:

```text
indexcore_root_id + target_path + result_name
        ↓ Q5
one PRESENT IndexCore resource
        ↓ Q8 Journal / existing Projector
one PRESENT cloudsite Copy
        ↓ optional monotonic Variant bind
Manifest.result_copy_id = Copy.ID
Manifest READY
Job SUCCEEDED
```

Rules:
- zero Q5 matches is pending, not failure;
- Q5 ambiguity/multiple matches is recovery, never guesswork;
- acquisition never creates/upserts Copy directly;
- one invocation may advance the existing projector by at most one bounded page;
- unresolved Copy is sufficient when the Manifest has no Variant intent;
- if VariantID is present, the exact Copy must be bound to that Variant before READY;
- result_copy_id is the stable historical acquisition result link.

## 5.3 One-claim stage routing

Gate 3.11 connects accepted bounded services into one state-driven application coordinator:

```text
already claimed RUNNING ACQUISITION Job (owner, ClaimAttempts)
     ↓ fetch durable linked Manifest
ACTIVE             → one ExecutionStep + one atomic ProviderOutcome
AWAITING_VISIBILITY→ one trusted IndexCore Mutation Hint handoff
AWAITING_CANONICAL → one bounded canonical confirmation
terminal pair      → no-op/replay, no provider/IndexCore work
```

Never chain into a second stage in the same claim; a new claim is required.
Do not consume FailureCount for ordinary stage waiting, do not bypass provider START_RESERVED, do not convert uncertain provider effects into blind retry.
Gate 3.11 does not authorize an automatic scheduler, a continuous worker daemon, new stage Jobs, OpenList acquisition observation, Source Resolver parsing, or public API/UI.

## 5.4 Acquisition lease recovery safety

D-035 / Gate 3.12 closes two runtime prerequisites before an automatic worker may operate:

1. Claiming must be type-scoped: acquisition workers only lease ACQUISITION Jobs and must not claim/discard other future Job types.
2. Expired RUNNING ACQUISITION leases require an **atomic paired transition** of the exact linked Manifest and Job to RECOVERY_REQUIRED, not a Job-only update.

Lock order: Manifest then Job. PostgreSQL time decides lease expiry; revalidate linkage and states under locks. Never erase START_RESERVED / provider task identity, retry StartDownload, reset failure budget, or fabricate terminal Manifest state.

The existing generic expired-Job sweep must not independently place an ACQUISITION Job into RECOVERY_REQUIRED. No continuous scheduling/runtime is authorized by this Gate.

## 5.5 One-shot runner boundary

D-036 / Gate 3.13 connects D-035 and D-034 in one explicit bounded tick:

```text
bounded paired expired acquisition recovery
    → if no recovery debt: type-scoped ClaimNextByType(ACQUISITION)
    → if eligible Job: one StageDispatcher.Dispatch
    → return IDLE / recovery / one-stage outcome
```

Never claim an unrelated Job; never perform two Manifest stages in one tick; never convert a stage error into a generic Job-only Fail/Succeed/RetryAt mutation; never automatically repeat a provider side effect.

Gate 3.13 does not start an autonomous background worker, timer, process runtime, or live credential integration. Later runtime wiring requires a new Gate.

## 5.6 Opt-in worker lifecycle

D-037 / Gate 3.14 makes D-036 RunOnce repeatable under one controlled, opt-in serial worker:

```text
disabled-by-default worker, validated unique owner and bounded policy
    → one RunOnce at a time
    → interruptible bounded wait
    → next distinct tick or graceful shutdown
```

IDLE/RECOVERY_ONLY/STAGE_COMPLETE are normal; errors must not bypass stage stores or mutate Job-only terminal state; ErrLeaseRecoveryDebt must stop new work. Bound tick duration below lease with a documented margin. Cancellation propagates to in-flight work while START_RESERVED remains the durable duplicate-start barrier.

Do not claim enabled production service until real PostgreSQL, IndexCore and 115 authenticated runtime composition is complete in a separately accepted Gate. A process enabled without dependencies must fail closed; default process remains side-effect-free.

## 5.7 Runtime composition boundary

D-038 / Gate 3.15 may compose one accepted runtime graph of cloudsite PostgreSQL stores, IndexCore Q5/Q8 read client plus distinct trusted Hint port, exact provider-session registry, D-017 Journal Projector, D-036 RunOnce and D-037 serial Worker.

Require validated schema v12, complete mandatory clients, injected secrets and exact ProviderID/ConnectionID/CredentialRef match *before* enabling worker; no fallback to mocked integrations. The main cloudsite DB is never IndexCore's DB. Hint acceptance never implies READY; only D-033 canonical Copy confirmation may finalize.

Default disabled process must remain side-effect-free. Controlled PostgreSQL+HTTP-fake E2E is in scope; real 115 account secrets and live deployment are not yet authorized. No direct cloudsite OpenList verification, Job Engine duplication, or new schema is allowed.

## 5.8 Protected operator bootstrap

D-039 / Gate 3.16 is accepted: stock CLI remains disabled and side-effect-free; an explicit read-only preflight validates externally protected credentials, trusted Hint configuration, schema v12 and exact active ProviderID/ConnectionID/CredentialRef coverage **without** claiming Jobs, submitting hints, migrating, contacting 115, or mutating any data.

Explicit operator worker start reuses exactly one accepted D-038 runtime graph with a restricted external SecretResolver, D-037 serial Worker and SIGINT/SIGTERM cancellation. No raw cookie/token in CLI process args, logs, Git or cloudsite tables; no silent credential fallback.

Gate 3.16 does not authorize AI to run a real 115 account download or deploy to production. Only local controlled PostgreSQL/HTTP fakes were accepted as evidence.

## 5.9 Deterministic source intake and submission

D-040 / Gate 3.17 owns the missing deterministic intake boundary before any human-authorized live-account acceptance.

Supported MVP schemes are initially `magnet`, `http`, `https`, and `ed2k`. Resolution is validation/classification only: it performs no DNS, HTTP, provider, OpenList, IndexCore, Search or Agent work.

The resolver may normalize the **scheme/type** but must preserve the accepted source value exactly:

```text
input source string
  -> validate/classify scheme
  -> Manifest.source_ref = exact accepted input
  -> ExecutionInput.Download.Source.Value = same exact value
```

Do not trim, truncate, rebuild magnet query parameters, percent-reencode, strip suffixes or invent a provider-specific canonical source. HTTP(S) userinfo is rejected. ExpectedName remains explicit caller intent; never guess result_name from the source.

Submission composes the already accepted Manifest creation and activation services. Stable IDs/idempotency must make replay deterministic; an activation failure leaves explicit PENDING intent rather than hidden cleanup. Submission itself never starts the worker or crosses a provider/network side-effect boundary.

Gate 3.17 is accepted.

## 5.10 Human-authorized real staging acceptance

D-041 / Gate 3.18 authorizes one bounded **operator-run staging validation**, not a new product architecture.

Phase A staging harness is accepted and merged:
- PR #61 reviewed exact HEAD `7424e5322fbb061fd14472dc0becde03b9fa6272`;
- Architect Review `5465144212`;
- squash `7a507a2411a26f8e36b0abf70271b3a8417942d7`;
- exact-head CI `37874559508` all green.
The merged harness is read-only evidence collection only. **Gate 3.18 real Phase B is accepted and closed**, as recorded by the final acceptance and completion artifact above. The following diagnostic/preparation history is preserved; old OPEN/STOP/NO-GO instructions are no longer current authority.

Live Phase B first attempt (operator report, 2026-10-09) passed cloudsite preflight and Job claim, but the first real 115 StartDownload did not leave a durable task reference: linked Manifest/Job RECOVERY_REQUIRED and provider task START_RESERVED. The upstream failure cause and whether 115 actually created a remote task **remain unknown**. Issue #62 is the bounded diagnostic under Issue #60: read-only remote task inquiry, first error categorization, minimal fix and exact-head CI only if warranted, then a separate human-initiated fresh test with distinct IDs. Never clear the original fence or blindly replay; do not mark Gate 3.18 ACCEPTED or authorize Gate 3.19 on partial evidence.

**Issue #62 diagnostics PR #63 ACCEPTED / MERGED (Gate 3.18 still BLOCKED):**
- exact reviewed HEAD `39a3e02751f1fdcb9384c8fef04fc00085f22f1a`, Architect Review `5477994000`, squash `b0b91701e15409e7ff5cd573def5883e14b29052`, CI `38031086983` green;
- accepted safe provider category/elapsed/status/type diagnostics after durable recovery and 115 SDK User-Agent initialization-order correction;
- separate new-ID live attempts still show HTTP 200 with `json.SyntaxError` even after that correction; original failure cause remains UNKNOWN;
- remote task inventory `NOT_FOUND_WITH_LIMITATIONS` does not establish absence of a provider side effect; historical START_RESERVED stays frozen;
- next: pinpoint pinned SDK outer/encrypted JSON decoding seam with redacted protocol shape and a synthetic reproducer, followed by minimal evidence-based fix; no real retry until then;
- Issues #62 and #60 stay OPEN; Gate 3.19 not authorized.
### Historical live checkpoint (superseded by 2026-10-13 final acceptance)

- PR #64 parser-boundary diagnostics and PR #65 protected single-attempt plan
  are merged. PR #65 main baseline is
  `f28f3a05759a6e4a9c7f01895e58df8db9829f33`. Diagnostic/plan acceptance is not
  actual download or terminal acquisition acceptance.
- [C1 acceptance](https://github.com/nathanxiangang-web/cloudsite/issues/62#issuecomment-6096681761):
  isolated clean cloudsite candidate built, not deployed; prepared Hint address
  inactive; historical five recovery/reservation pairs preserved.
- [C1.5 acceptance](https://github.com/nathanxiangang-web/cloudsite/issues/62#issuecomment-6097060575):
  configuration-first investigation selected. Current non-root OpenList
  collector prefixes canonical Q5 paths, while Hint interprets target_path as
  root-relative. Simply setting target_path to the prefix duplicates the Hint
  prefix and is forbidden. Conditional remedy: a genuinely restricted view
  exposing only the approved destination as virtual `/`, with a NEW approved
  IndexCore Root collector path `/` and NEW cloudsite Binding. Independently verify
  provider scope, OpenList view, Root ID and Manifest target_path; never infer IDs.
- [C2-A report](https://github.com/nathanxiangang-web/cloudsite/issues/62#issuecomment-6097687709)
  and [architect acceptance](https://github.com/nathanxiangang-web/cloudsite/issues/62#issuecomment-6098683022):
  independent synthetic-only Local OpenList instance actually runs on
  `127.0.0.1:5248`, restart=no, fresh 0700/0600 state, pinned v4.2.6. No real 115
  credentials/mount, new canonical Root/Binding, real Hint or acquisition.
  Missing-path/share 500 is not a proven 403 boundary; the separate bridge is
  not a formal egress-isolation guarantee. Collector login needs re-login on
  restart; never replace restricted credentials with global ADMIN token.
- **Actual synthetic PASS; real 115 / Hint / Q5 / E2E NOT TESTED in C2-A.**
  Full live restricted-view isolation is still unproven. Five historical
  RECOVERY_REQUIRED / START_RESERVED pairs stay frozen; schema v12.
- Historical [C2-B0 instruction](https://github.com/nathanxiangang-web/cloudsite/issues/62#issuecomment-6098855717)
  is completed: [actual report](https://github.com/nathanxiangang-web/cloudsite/issues/62#issuecomment-6098957134)
  and [architect acceptance](https://github.com/nathanxiangang-web/cloudsite/issues/62#issuecomment-6099065863).
  **READ-ONLY EVIDENCE ACCEPTED / C2-B ADMISSION NO-GO**. In measured scope,
  synthetic instance `127.0.0.1:5248`, original nine containers, pinned single
  IndexCore writer, schema v12 and five recovery/reservation pairs stayed
  unchanged. Old native directory is still nonempty (marker retained); new
  provider/view/Root/Binding coordinates remain UNSET. No real mount, Hint or
  new-root Q5/Q8/acquisition validation. Next action is **environment-owner
  handoff**, not another worker code/plan task or repeat B0: privately select a
  NEW named independently empty native 115 destination; complete privileged
  root/other-user service/drop-in/timer/cron/restart audit and identify the exact
  IndexCore launcher; explicitly approve each real change, operator and
  outage/freeze/rollback scope, including any fixture/Hint writes. Privileged
  audit remains UNKNOWN; destination/credentials/change scope not approved.
  Retain C2-A and original services; do not restart or alter them without approval.
- **C2-B NOT AUTHORIZED:** real mount/credential import, new Root/Binding,
  pinned existing single-writer Hint enable/restart, sentinel lifecycle,
  baseline/scan and diagnostic Hint require concrete owner approval and a
  verified outage/freeze/rollback boundary. OpenList remains IndexCore-owned;
  no direct cloudsite visibility verifier or speculative prefix patch.
- **C3 NOT AUTHORIZED:** separate exact-source UTF-8 SHA-256 (no rewriting),
  protected request, fresh IDs, verified empty destination/four coordinates,
  executable provenance, single Worker and bounded human-start window required.
  No submit/start, provider POST, replay or recovery bypass during B0.

Dependency chain: **C2-A accepted -> C2-B0 evidence accepted / admission NO-GO ->
resource-specific C2-B owner approval/implementation -> separate C3 one-shot
approval -> Gate 3.18 final E2E closeout -> Gate 4**. Gate 5 Agent
and Gate 6 replacement/MVP closeout follow blueprint acceptance, not parallel
implementation authority. Issues #60/#62 stay open; no Gate 3.19.

Pinned external IndexCore baseline:

```text
v0.4.0-alpha.1
6f0eec85c59bd8cbe55011b0d9e512e0cafd6615
```

The staging path is:

```text
cloudsite protected submission
  -> real 115 provider mutation
  -> provider result_name
  -> trusted loopback IndexCore Mutation Hint
  -> IndexCore hybrid incremental runtime
  -> IndexCore OpenList collector
  -> Canonical + Journal
  -> cloudsite Q5/Q8 Projector
  -> exact PRESENT Copy
  -> READY + result_copy_id + Job SUCCEEDED
```

cloudsite still never queries OpenList for acquisition truth. Provider success and Hint 202 remain non-terminal. Because IndexCore's accepted Hint listener is literal-loopback-only, cloudsite and IndexCore share one host/network namespace for this acceptance rather than weakening trust for separate containers.

Real secrets stay outside Git/chat/reporting. CI/AI must not use the real account. A human explicitly starts the worker. If START_RESERVED becomes uncertain, stop and preserve evidence; never force another StartDownload.

Acceptance uses bounded polling of durable state and IndexCore Query/Journal, not blind sleep. A failure is diagnostic evidence, not permission to bypass D-029/D-033 or add a second observer.

## 6. Job Engine invariants

The ACQUISITION Job is the durable execution safety boundary.

Current counter split:

```text
claim_attempts
    monotonic claim generation
    increments on every successful claim
    stale-worker fencing token
    unbounded

attempt_count / FailureCount
    genuine failure retry budget
    increments only through RetryAt
    bounded by max_attempts
```

Active lease validity requires:
- Job is RUNNING;
- owner matches;
- claim generation matches;
- database-time lease is unexpired.

Expired RUNNING side-effecting work never blind-retries; it goes to explicit recovery semantics.

## 7. Provider-side-effect invariant

Before external `StartDownload`:

```text
persist START_RESERVED
    ↓
exactly one caller may cross provider side-effect boundary
```

Then:

```text
StartDownload succeeds
    ↓
persist REFERENCE_KNOWN + opaque provider_task_ref
```

If reference persistence is uncertain:

```text
START_RESERVED remains
    ↓
future execution must NOT call StartDownload again
    ↓
operator recovery required
```

This fence is for provider side effects only.

IndexCore Mutation Hint is intentionally at-least-once/coalescing and does **not** use the provider-task fence.

## 8. 115 adapter invariants

Pinned runtime library:

```text
github.com/SheltonZhu/115driver v1.3.5
```

Frozen mapping:

```text
ProviderID             = "115"
Source.Value           = exact URI
Target.Scope           = 115 saveDirID / provider scope
Target.Path            = cloudsite observation scope only
TaskReference.Value    = 115 info_hash
```

No source normalization/truncation inside the adapter.

Cancel uses:

```text
DeleteOfflineTasks([info_hash], false)
```

Never delete downloaded files as part of task cancellation.

## 9. Provider session / secret boundary

Authenticated provider execution is selected by:

```text
ProviderID + ConnectionID + CredentialRef
```

ProviderID alone must never select authenticated state.

Secrets:
- never enter Manifest;
- never enter Job payload;
- never enter provider-task persistence;
- never enter Agent prompts;
- never be logged;
- are resolved only at the composition/provider-session boundary.

## 10. Current Gate status

Accepted:
- Gate 0 — skeleton/contracts — CLOSED
- Gate 1 — observation plane — CLOSED
- Gate 2 — resource semantics — CLOSED
- Gate 3.1 — Manifest
- Gate 3.2 — activation + Job
- Gate 3.3 — execution input/provider scope
- Gate 3.4 — durable provider-task fence
- Gate 3.5 — connection-scoped session/credential boundary
- Gate 3.6 — concrete 115 Downloader
- Gate 3.7 — atomic provider-stage outcome handoff
- Gate 3.8 — IndexCore trusted Mutation Hint and observation handoff
- Gate 3.9 — durable acquisition result locator
- Gate 3.10 — canonical Q5 + projected Copy + atomic READY finalization
- Gate 3.11 — one-claim acquisition stage dispatcher and controlled E2E
- Gate 3.12 — acquisition-only claims and atomic expired-lease recovery
- Gate 3.13 — bounded one-shot acquisition RunOnce
- Gate 3.14 — opt-in serial acquisition worker lifecycle
- Gate 3.15 — fail-closed runtime composition and controlled PostgreSQL+HTTP E2E
- Gate 3.16 — protected operator bootstrap, read-only preflight and explicit worker startup
- Gate 3.17 — deterministic Source Resolver and acquisition submission
- Gate 3.18 — real 115 + OpenList + IndexCore + cloudsite READY/same Job SUCCEEDED — CLOSED

Authorized now:
- D-042: audit docs, then bounded core MVP slices after written contracts and validated prior merges, without repeated user approval.
- Frozen Stage 1: product accounts/revocable sessions, not yet complete. Preserve Gate 3.18 evidence and historical unknown reservations.

Not authorized yet:
- direct cloudsite OpenList acquisition verifier;
- direct acquisition Copy creation/upsert;
- production deployment, public API/UI, or Gate 4+ implementation without a separate architecture decision;
- weakening IndexCore loopback Hint trust to accommodate deployment topology;
- provider-specific source rewriting/truncation without a new decision;
- auth/quota/share/access/usage;
- Agent implementation;
- generalized workflows/microservices/distributed queue.

## 10A. Acquisition result locator

Before provider success may advance to observation, cloudsite must durably know one top-level result basename.

```text
provider-observed TaskStatus.ResultName
        >
Manifest.ExpectedName fallback
        ↓
Manifest.result_name
```

Rules:
- target_path remains a directory scope;
- result_name is one safe path segment, not a path;
- never infer the acquired result by listing the target directory or choosing a new/sole object;
- provider FileId/DirId/info_hash are not IndexCore physical identity;
- missing result identity requires explicit recovery;
- future exact canonical candidate path is `Join(target_path, result_name)`;
- result_name itself is not Canonical truth and never READY.

## 11. Superseded / dangerous historical statements

These statements may still appear in old issues/commits and must NOT be revived:

### Superseded
```text
"cloudsite verifies OpenList visibility directly after provider success."
```

Superseded by D-029.

### Wrong
```text
"cloudsite should call /api/fs/get during acquisition."
```

Wrong for production acquisition architecture.

### Wrong
```text
"OpenList is cloudsite's canonical physical truth."
```

IndexCore is canonical truth.

### Wrong
```text
"provider task SUCCEEDED means READY."
```

Canonical confirmation is still required.

### Wrong
```text
"attempt_count is still the claim-generation token."
```

Since migration 0010, `claim_attempts` is generation and `attempt_count` is failure budget.

## 12. Anti-hallucination preflight

Before proposing or implementing a Gate:

1. Read this file.
2. Read `PROJECT-STATE.md`.
3. Read the newest relevant decisions in `docs/DECISIONS.md`.
4. Inspect the actual current code/contracts in Git.
5. If integrating an external repo, inspect that repo's current accepted interface before writing the Issue.
6. Search for an existing accepted module before proposing a duplicate implementation.
7. Check whether the proposed dependency direction violates an accepted boundary.
8. Write the bounded Issue only after those checks.
9. Do not authorize the next Gate until the current Gate is accepted and merged.

Before accepting a PR:

1. inspect exact head SHA;
2. inspect diff;
3. inspect relevant implementation, tests, migrations, and architecture guards;
4. inspect exact-head CI;
5. verify behavior against the current Issue and the newest decisions;
6. check replay/restart/concurrency/failure semantics where durability is claimed;
7. update `PROJECT-STATE.md` and architecture memory only after acceptance.

## 13. Conflict rule

If two documents disagree:

```text
newer explicit accepted decision
    > older decision
    > blueprint prose
    > old issue/PR description
    > chat memory
```

Never silently choose between contradictory documents.

Instead:
1. identify the conflict;
2. resolve it with an explicit Architect decision;
3. update this memory file + Project State + affected blueprint/context docs in the same architecture synchronization pass.

## 14. Team / scope rule

Team model is exactly:

```text
1 Architect
1 Developer
```

Do not invent additional workers.

The Developer implements only the currently authorized bounded Gate.

The Architect owns:
- boundaries;
- contracts;
- acceptance;
- decisions;
- project memory synchronization.
