# AGENTS.md — Panta AI/Developer Entry Point

This repository uses AI-assisted development.

Before planning, changing, or reviewing code, **read these files first**:

1. `docs/AI-ARCHITECTURE-MEMORY.md`
2. `PROJECT-STATE.md`
3. newest relevant entries in `docs/DECISIONS.md`
4. current authorized GitHub Issue
5. actual current contracts/code/tests

Chat history is not a source of truth.

## Current execution pointer — 2026-10-15

Latest accepted/deployed Issue #94 / PR #95, exact HEAD `e5961f8fe060`,
CI `38733576527` all three SUCCESS, same-agent acceptance `5533047604`, merge
`9c902a3650df`; merge CI `38734637547` all three SUCCESS. D-048 and
[complete CloudSite contract](docs/PRODUCT-CLOUDSITE-LIBRARY.md): all283 actual
files (software42/video188/document6/Other47) through one new isolated Q6 Root.
474 directories remain metadata, not consumer cards. One consumer Binding,
four backend read Bindings so old shares survive. Observation-only mapping is
DISABLED, no provider_scope/credential_ref. Old three root generations0/1/1 and
original staging DB histories unchanged. One explicit initial scan, no daemon.
New forms auto-fill crypto-random4 A-Z/a-z/0-9; clearing means no code. Legacy
longer-code share from old binary/Binding verified against candidate AND formal.
UI pagination120/120/43, all counts and both redemption modes verified. Test
shares revoked; candidate stopped. Schema14 unchanged. Product-only cutover,
query PID/unrelated services unchanged; rollback old14 receipt/config/binary.
Deployed SHA256 `983e10735db46b94cfd5d404372e8e97c788bfd05ff8aa58d832e4348d287f67`.
Three-subtree-only/six-file defaults below are historical, NOT current limits.
Issue83 WIP unchanged. No native115 share/download/login or full MVP claim.

Previous accepted/deployed Issue #92 / PR #93, exact HEAD `d98f79a2d39f`,
CI `38702187329` all three SUCCESS, same-agent review `5529930361`, merge
`49b98b21bff0`; merge CI `38702621096` SUCCESS. D-047 and
[sharing contract](docs/PRODUCT-FILE-SHARING.md): Panta-hosted single-file links,
expiry/code/grant/owner revoke and streamed download UX. Not native115 shares or
public exposure. Real ordinary/guest downloads hash-match; anonymous grant,
wrong-code rejection and revoke-denial verified. Temporary test shares revoked.
Owned product DB14 only, protected13 backup, reviewed14 sharing-disabled fallback
started and verified. Old13 binary is incompatible with14: do not use old receipt
blindly. Binary SHA256 `38d8f0c5c75053b263c98c8cd0e0997f9c7be074b2eaa8584583d6e5f2eb951c`.
Original staging12/query/generations/services unchanged; zero native provider
calls/scan/Hint. Issue83 WIP preserved; unmerged draft14 needs later renumbering.
Older no-v14/sharing-not-implemented pointers below are historical; native115
ShareProvider, admission/usage/current-DB Copy still remain. No full MVP claim.

Previous accepted/deployed content is Issue #90 / PR #91, exact HEAD
`34863813a53f`, CI `38699810879` all three SUCCESS, same-agent review
`5529707246`, merge `e5783b5fcef27`. D-046 and
[content contract](docs/PRODUCT-FILE-CONTENT.md) govern server-only known-file
OpenList bytes AFTER IndexCore exact Q5 authorization, every cache hit included.
One-hour private original-document cache -> server PDF PNG pages or Office/text;
Office TEXT ONLY, not original layout. Range media and attachments verified with
real files. Product binary SHA256 `3625b49c028962faacd492642385b6e760897946dced92d3ab3ad700c3116c2c`.
Only owned product restarted; query/generations/original histories/services
unchanged. No new source/Job/Manifest/ProviderTask/download/scan/Hint/migration.
Sharing is NOT implemented; Issue #83 WIP and current-DB Copy remain separate.
Older metadata-only statements below are historical and superseded for this
bounded content slice. D-044 no deletion and dual UI remain in force.

Previous accepted/deployed UI is Issue #88 / PR #89, exact HEAD `8b7d1c23feab`,
CI `38673309830` all three SUCCESS, review `5529290784`, merge `963c66d9c0d7`.
D-045 requires TWO UIs: file-first consumer `/`, ADMIN `/admin/` retaining the
operational workspace. Frontend directly reads configured real IndexCore files,
no per-file approval, directory controls or fake files. Six real file cards;
backing sources are unchanged PR #87 scopes. No user role was escalated. Only
owned product UI service restarted, original data/query/unrelated services
unchanged. [Module contract](docs/PRODUCT-FILE-LIBRARY.md). No playback/content
access or full MVP acceptance. D-044 no deletion; preserve isolated Issue #83 WIP.

Previous accepted source/lifecycle slice is Issue #86 / PR #87: exact HEAD `d693878c7791`,
CI `38669580171` all three successful, merge `ddecd51c19b7`, same-agent review
`5527219011`. [Real catalog contract](docs/PRODUCT-REAL-CATALOG.md) governs the
three new isolated observation-only DISABLED Bindings. Formal .50 loopback 8180
now shows five real document files/three directories, one MP4 and an empty
software scope, superseding the old one-document default described below.
Two owned readonly user-systemd services are enabled; controlled restart tested,
full VM reboot untested. Panta uses IndexCore HTTP, never direct OpenList reads.
D-044: DO NOT implement file-delete UI/API; users delete directly in their drive.
No playback/content/access/Copy projection or full MVP acceptance is implied.
Resume isolated Issue #83 admission/usage WIP without overwriting it; no v14
migration has been deployed. No acquisition, Hint or scheduled scan is enabled.

Gate 0 through Gate 3 are ACCEPTED / CLOSED. Gate 3.18 real Phase B passed
[Architect final acceptance](https://github.com/nathanxiangang-web/panta/issues/62#issuecomment-6143267673):
original KNOWN task -> Hint -> Q5/Q8 -> PRESENT Copy/cursor -> READY + same Job
SUCCEEDED, with no additional StartDownload. Preserve the eight historical
unknown reservations and protected evidence. No further live 115 validation is
needed. Documentation Issue #75 / PR #76 is merged. Current task:
auth Issue #77 / PR #78 merged. Current implementation:
[Issue #79](https://github.com/nathanxiangang-web/panta/issues/79) / PR #80 and Issue #81 / PR #82 are accepted and merged. Initial readonly UI used real MCP-Test data; PR #87's isolated catalog now supersedes that default scope. Next bounded [Issue #83](https://github.com/nathanxiangang-web/panta/issues/83): freeze and implement exact-source admission and operation usage. [Read-only UI/runbook](docs/PRODUCT-READONLY-UI.md) remains governed by D-043.
User-prioritized real resource presentation Issue #84 / PR #85 is accepted and
deployed after exact-head CI `38575852403`. See [display module](docs/PRODUCT-RESOURCE-DISPLAY.md).
That PR #85 baseline was metadata classification/list/cards/detail over the
MCP-Test Root (one document), not whole-drive indexing, content preview/video
playback or file access. Issue #83 WIP remains isolated on its own branch and
unreviewed; no v14 migration has been deployed. Do not overwrite that work or
mix it into readonly deployment. Resume admission/usage and subsequent actual
Copy/access/share modules; MVP is still incomplete. Preserve restored readonly
processes and historical data; no new provider download is needed for UI work.

Current authority is D-042: the human delegates Architect and Developer roles
to one agent through minimum usable core MVP delivery. Finish the documentation
audit (complete), then execute [the staged plan](docs/GATE4-IMPLEMENTATION-PLAN.md).
Each bounded slice follows written contracts, tests, exact-head CI and a separate
same-agent review pass. Do not call it independent review. Issue #77 implements
auth domain/store/handlers; [PRODUCT-AUTH](docs/PRODUCT-AUTH.md) records limits.
Issue #79 adds owner-only task and allowlisted resource read handlers, with no
provider mutation. Issue #81 implemented read-only UI/listener/provisioning;
admission/usage/access/share is NOT implemented and full MVP is not accepted. Schema head v13
does not upgrade the existing Gate 3.18 v12 database/binary. Issue #74's vague
old skeleton is superseded.
Original MVP auth/access/share/usage remains required; new guidance is not
implicit permission to replace it or move Agent implementation ahead of Gate 5.

Gate-labelled constraints below describe the scope when each capability was
introduced, not an instruction to reopen old Gates. The current Project State,
AI Architecture Memory and newest accepted decision govern execution.

## Current non-negotiable boundaries

### Observation

```text
Storage / 115
  -> OpenList
  -> IndexCore Collector
  -> IndexCore Canonical + Journal
  -> Panta
```

IndexCore owns physical truth.

**Do not add direct Panta -> OpenList acquisition visibility verification.**

After provider completion, Panta sends a trusted IndexCore Mutation Hint:

```text
root_id   = StorageBinding.indexcore_root_id
scope_key = Manifest.target_path
reason    = POSSIBLE_CHANGE
```

Then Panta waits for IndexCore canonical Query/Journal evidence.

### Control

```text
Panta
  -> Job Engine
  -> Provider Contract
  -> Provider Adapter
  -> Storage
```

Provider success is never READY by itself.

## Coordinate separation

Never derive these from each other:

- `provider_scope` — provider mutation target;
- `openlist_mount_path` — IndexCore/OpenList observation configuration;
- `indexcore_root_id` — IndexCore canonical root;
- `manifest.target_path` — scope inside the selected root/binding.


## Acquisition result identity

Before provider success may advance automatically beyond the provider stage:

```text
valid provider-observed TaskStatus.ResultName
        >
Manifest.ExpectedName fallback
        ↓
Manifest.result_name
```

Canonical candidate identity is later formed from:

```text
StorageBinding.indexcore_root_id
+ Manifest.target_path
+ Manifest.result_name
```

- `target_path` is the directory scope refreshed by IndexCore.
- `expected_name` is request-time intent/fallback and must not be rewritten by provider execution.
- `result_name` is the immutable durable provider-stage locator.
- A valid provider result wins; blank/absent provider result may use ExpectedName fallback.
- Invalid nonblank provider result fails closed; do not silently fall back.
- Never use provider FileId/DirId, OpenList paths, Journal order, timing, newest-item, or only-item heuristics as a substitute.
- This locator still does not prove canonical presence; IndexCore confirmation remains required.

## Canonical completion

READY is not authorized by provider success or Hint acceptance.

For Gate 3.10:
```text
root_id + target_path + result_name
  -> exact Q5 PRESENT resource
  -> existing Q8 Journal Projector
  -> exact PRESENT Copy
  -> optional monotonic Variant binding
  -> result_copy_id + READY + Job SUCCEEDED
```

- zero Q5 matches is pending;
- Q5 ambiguity/multiple matches fails closed;
- acquisition must never create/upsert Copy directly;
- reuse the existing projector in bounded one-page steps;
- `result_copy_id` is the durable final acquisition result reference.

## One-claim acquisition dispatcher

Current authorized Gate 3.11: Issue #46 / D-034.

Dispatch exactly ONE stage per claimed ACQUISITION Job using persisted Manifest.State:

```text
ACTIVE              -> provider Execute + atomic ProviderOutcome
AWAITING_VISIBILITY -> trusted IndexCore Mutation Hint step
AWAITING_CANONICAL  -> exact canonical confirmation step
terminal pair       -> committed no-op / replay
```

- Use Job owner and ClaimAttempts fencing, with PostgreSQL database-time check at every durable stage write.
- After an accepted handoff return; never execute the next stage in the same claim.
- Never convert provider uncertainty into blind StartDownload retry.
- Normal pending status does not consume failure budget.
- Do not implement a continuous worker daemon / automatic ClaimNext scheduler yet.

## Type-scoped claim and linked recovery

Active Gate 3.12: Issue #48 / D-035.

- A future acquisition worker must only claim ACQUISITION Jobs using a database-atomic type filter. Never claim arbitrary Jobs and then discard mismatched types.
- Expired RUNNING ACQUISITION Jobs require a PostgreSQL-time-fenced atomic transition of both linked Manifest and Job to RECOVERY_REQUIRED.
- Lock Manifest before Job, recheck exact idempotency/payload/reverse link under lock.
- Generic Job-only expired-lease sweep must not mutate ACQUISITION Jobs independently.
- Do not drop START_RESERVED / task references or automatically repeat 115 StartDownload.
- Maintain ClaimAttempts generation and FailureCount semantics, no worker daemon or process runtime in Gate 3.12.

## One-shot runner

Current Gate 3.13 / Issue #50 / D-036 allows a single explicit application RunOnce tick:

```text
bounded expired-acquisition linked-pair recovery
  → if healthy: ClaimNextByType(ACQUISITION) at most once
  → StageDispatcher.Dispatch at most once
  → return a typed IDLE / RECOVERY / STAGE result
```

Recovery debt stops new claims in the tick. Stage errors must not be translated into generic Job-only Fail/Succeed/RetryAt writes; that would desynchronize the Manifest. Pass the actual claimed owner/ClaimAttempts. No autonomous worker daemon, timer, process startup wiring or live provider-credential integration yet.

## Opt-in acquisition worker lifecycle

Active Gate 3.14 / Issue #52 / D-037: serial opt-in lifecycle around the accepted RunOnce.

- Worker disabled by default; an enabled process with no real injected dependencies must fail closed at startup.
- Use exactly one worker loop and one in-flight RunOnce per process; a slow tick cannot overlap another.
- Owner stable for the process and unique across replicas; bounded interval, deadline, retry and lease safety margin; never busy spin.
- Treat IDLE/recovery-only/completed tick as normal, pace transient errors, stop on corrupt linked-pair recovery debt.
- Propagate cancellation, no detached task goroutines, no new claim after stop.
- Do not mutate Jobs on runner errors, recreate provider tasks or claim canonical READY from a worker-level result.
- Real 115 credential/IndexCore/PostgreSQL startup graph and deployment remain later authorized work.

## Runtime composition and secrets

Active Gate 3.15 / Issue #54 / D-038.

- Construct a single complete real-protocol graph from accepted Panta PostgreSQL stores, IndexCore Q5/Q8, separate trusted Mutation Hint, provider-session registry, Journal Projector, RunOnce and worker.
- Worker-enabled startup must fail closed if any dependency, schema-v12 guard, trusted Hint configuration or exact provider/connection credential binding is absent.
- Resolve 115 credentials through an externally injected SecretResolver; never put cookies, tokens or user magnets/URLs in Git, Panta DB, logs or diagnostic errors.
- Never query IndexCore DB or inspect OpenList directly from Panta for acquisition visibility; Hint 202 is NOT canonical READY.
- No new acquisition Copy writer; Q8 Projector owns Copy and D-033 canonical transaction owns READY.
- Controlled PostgreSQL + HTTP fake E2E is authorized, live 115 account acceptance / deployment remains a later Gate.

## Protected operator bootstrap and preflight

Current Gate 3.16 / Issue #56 / D-039.

- Default `cmd/panta` mode must not resolve provider secrets, claim jobs, submit Hint, or make provider requests.
- Provide distinct explicit read-only preflight and opt-in worker start using one accepted D-038 runtime graph.
- Inject raw 115 cookies and trusted Hint tokens only via externally protected secret sources with strict file/path permission checks; never put secrets into CLI arguments, Git, logs, Panta tables or error messages.
- Preflight must be verifiably read-only: schema v12/active session identities/config and secret availability, no DB migration, no ClaimNext, no external provider mutation.
- Explicit start must fully validate all required exact sessions before claims, preserve one D-037 serial Worker, signal-based cancellation and owned pool Close-after-Run ordering.
- Live 115 account downloads, production deployment, public API/UI and additional schedulers are not authorized in this Gate.

## Job safety

- `claim_attempts` = monotonic claim-generation fence.
- `attempt_count` / `FailureCount` = bounded failure retry budget.
- stale claim generations fail closed.
- provider StartDownload requires the durable START_RESERVED fence.
- START_RESERVED with unknown provider reference must never auto-recreate.

## Provider safety

Current 115 downloader:
- `github.com/SheltonZhu/115driver v1.3.5`;
- Source.Value passed unchanged;
- Target.Scope is the 115 saveDirID;
- task reference is `info_hash`;
- cancel never deletes downloaded files.

Secrets never enter Manifest, Job payload, provider-task persistence, logs, or Agent prompts.

## Team / scope

One agent performs Architect and Developer roles in separate phases under
D-042. No imaginary parallel Worker team.

The Developer implements only the currently authorized bounded Gate.

Validate and merge each bounded stage before advancing. Ordinary in-scope stages
need no repeated user authorization; preserve D-042 risk limits.

## Conflict rule

If old prose/Issue/PR/chat conflicts with a newer accepted Git decision:

```text
newer accepted decision wins
```

Stop, identify the conflict, and synchronize project memory before continuing.
