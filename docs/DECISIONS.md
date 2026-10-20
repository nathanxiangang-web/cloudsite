# cloudsite — Decision Log

> Append-only architecture/product decision memory. Do not rewrite history silently; supersede an old decision with a new entry.

## D-001 — MVP before UI

**Status:** Accepted

cloudsite MVP prioritizes functional end-to-end flows and modular architecture before UI polish.

## D-002 — OpenList is the observation/access layer

**Status:** Accepted

Existing and future storage providers are preferably exposed through OpenList for observation. cloudsite does not build another provider scanner for IndexCore.

## D-003 — IndexCore owns physical resource truth

**Status:** Accepted

IndexCore remains an independent canonical physical-resource kernel. cloudsite consumes its read API/Journal and never writes its database.

## D-004 — cloudsite owns logical resource semantics

**Status:** Accepted

cloudsite Catalog models logical identity as:

```text
Asset → Release → Variant → Copy → IndexCore resource
```

This prevents provider paths/files from becoming product identity.

## D-005 — Search is optional

**Status:** Accepted

Known-path browse/resolve, direct acquisition, and physical-resource inspection must work without Catalog search.

## D-006 — Separate observation and control planes

**Status:** Accepted

Observation:

```text
Storage → OpenList → IndexCore → Catalog
```

Control:

```text
cloudsite → Job Engine → Provider Contract → Provider Adapter → Storage
```

Provider command success does not override canonical observation.

## D-007 — 115 is the first provider, not the architecture

**Status:** Accepted

115 supplies the first storage/cloud-download/share implementations. Provider-specific behavior must remain behind replaceable contracts.

## D-008 — Cloud-download READY requires canonical confirmation

**Status:** Accepted

A completed provider download is not READY until:
1. the result is visible through OpenList;
2. IndexCore scoped refresh confirms it;
3. Journal/Projector creates or updates the Copy.

## D-009 — Agent Runtime is replaceable

**Status:** Accepted

CodeArts CLI + GLM-5.2 is the initial Agent Runtime. Deterministic product flows must work with the agent disabled.

## D-010 — Git is project memory

**Status:** Accepted

Project context, current state, accepted decisions, contracts, implementation and acceptance evidence must be committed to this repository.

## D-011 — Go is the initial cloudsite backend implementation baseline

**Status:** Accepted

cloudsite MVP backend implementation starts with **Go 1.27.1**, aligned with the current IndexCore toolchain to reduce build/runtime/tooling divergence during integration.

This is an implementation/toolchain choice, not a domain coupling decision:

- cloudsite does not import IndexCore internal packages;
- cloudsite communicates with IndexCore only through cloudsite-owned ports and supported external integration contracts;
- provider adapters remain replaceable behind cloudsite provider contracts;
- the MVP remains a single-process modular monolith until real scaling evidence justifies a split.

The cloudsite product database remains PostgreSQL as established by the MVP blueprint. Database schema and migrations are handled in a later bounded Gate 0 task, not in Gate 0.1.

## D-012 — cloudsite owns an independent PostgreSQL schema and append-only migration history

**Status:** Accepted

cloudsite product persistence uses PostgreSQL through `pgx/v5`.

Schema evolution uses ordered, append-only SQL migrations owned by cloudsite and recorded in a `schema_migrations` history. Runtime ORM auto-migration/schema generation is not used.

Compatibility is fail-closed:

- missing required migrations are incompatible;
- migrations unknown to the running binary are treated as future schema and are incompatible;
- an older cloudsite binary must not silently run against a newer product schema;
- applied known migrations must form an ordered prefix of the embedded migration history; gaps/out-of-order history are incompatible and must not be auto-repaired.

cloudsite product DB configuration remains independent from IndexCore DB configuration. cloudsite never obtains physical truth by reading or writing IndexCore PostgreSQL directly.

Catalog domain packages remain persistence-agnostic; pgx belongs in the PostgreSQL adapter/infrastructure boundary.

## D-013 — Job Engine is the durable control-plane safety boundary

**Status:** Accepted

All provider-changing operations must be represented by a durable cloudsite Job before external side effects occur.

The Gate 0 Job Engine is intentionally small and provider-neutral. It is a state/claim/recovery kernel, not a workflow engine, scheduler platform, or autonomous agent controller.

Minimum states:

```text
QUEUED
RUNNING
RETRY_WAIT
RECOVERY_REQUIRED
SUCCEEDED
FAILED
CANCELED
```

Terminal states are `SUCCEEDED`, `FAILED`, and `CANCELED`.

Claiming is PostgreSQL-atomic and lease-based. Only the active lease owner may renew, succeed, or fail a running job.

A lease expiry is ambiguous: an external side effect may already have occurred even if the worker disappeared. Therefore expired `RUNNING` work becomes `RECOVERY_REQUIRED` and is never blindly requeued/re-executed.

Retry timing is explicit durable state, not a blind sleep loop. Provider-specific reconciliation/backoff remains outside the generic Job Engine.

An optional product-level idempotency key prevents duplicate tracked command submission without embedding provider-specific identifiers in the generic job model.


## D-014 — Active job leases are fenced by claim generation and database time

**Status:** Accepted

A running Job lease is identified by more than worker name.

Every successful claim increments the Job attempt/generation. Active-lease mutations must prove:
- Job ID;
- lease owner;
- expected claim generation;
- currently unexpired lease.

This prevents a stale execution from an older claim from mutating a newer claim even when both use the same worker ID.

PostgreSQL time is authoritative for lease-expiry authorization and expired-lease recovery eligibility. Caller-supplied timestamps must not be able to revive an expired lease or force early recovery.

The existing `attempt_count` is the MVP fencing generation. A separate lease token is not required unless later provider execution proves a stronger primitive is necessary.


## D-015 — cloudsite consumes IndexCore through its external Query/Hint contracts only

**Status:** Accepted

cloudsite treats IndexCore as an independent infrastructure service and never imports IndexCore internals or accesses its PostgreSQL database directly.

For the observation plane:
- Q4 hierarchy reads back known-root browsing;
- Q5 canonical path resolution backs direct known-path access;
- Q8 per-root Journal is the canonical projection feed;
- Q9 root status may support health/generation awareness where needed.

Journal cursor semantics are owned by the IndexCore HTTP contract: `after_seq` is exclusive and the next request reuses the **last event_seq actually observed**, without incrementing it.

IndexCore Query API remains read-only from cloudsite's perspective.

Mutation Hint is a separate trusted internal transport. A successful Hint acceptance is only a durable signal and never counts as proof that a resource is canonically visible. Canonical success must still be observed through IndexCore Query/Journal.

cloudsite owns consumer DTOs, typed errors, cursors, and projection state. It does not reuse IndexCore internal Go types.


## D-016 — cloudsite preserves IndexCore query semantics instead of simplifying them away

**Status:** Accepted

cloudsite's IndexCore ports are consumer-owned, but they must faithfully preserve the semantics of the accepted external IndexCore contract.

In particular:
- Q4 is hierarchy-by-parent with opaque generation-bound pagination; it is not a path browse API and not a flattened root listing.
- Q5 path resolution returns all matches plus an ambiguity flag. cloudsite must never silently choose one canonical-path match.
- Resource presence remains an explicit canonical state rather than a boolean that would erase retained/tombstone meaning.
- Q8 Journal ordering is per root only. The HTTP `after_seq` cursor is exclusive; consumers reuse the last event sequence actually observed and never increment it.
- Stable remote error categories remain typed and distinguishable from transport and malformed-response failures.

Gate 0 placeholder contracts may be refined when real external semantics prove them insufficient. Preserving an obsolete simplified contract is not considered backward compatibility when it would hide correctness-critical behavior.


## D-017 — Journal projection commits Copy state and cursor atomically

**Status:** Accepted

cloudsite's Catalog projection is derived from IndexCore canonical Journal and must be replay-safe.

For each StorageBinding:
- the durable projection cursor is the last Journal event_seq whose Copy mutations are committed;
- IndexCore Q8 is called outside any cloudsite database transaction using exactly that cursor as after_seq;
- the fetched batch is validated before mutation;
- Copy mutations and cursor advancement commit in one PostgreSQL transaction;
- the transaction compares/locks the expected cursor so concurrent projectors cannot overwrite newer progress.

A crash or failure must leave both Copy projection and cursor at the previous committed state.

Resource-added, resource-updated, resource-renamed, and resource-moved project physical Copy availability as PRESENT. Resource-removed projects REMOVED. Root lifecycle events advance the cursor but do not bulk-change child Copy availability.

Journal payload is not authoritative for physical or logical identity. The projector uses StorageBinding root mapping, event type, and canonical resource_id only.

The projector never creates Asset/Release/Variant identity. Newly observed physical resources remain unresolved Copies with variant_id NULL.

Continuous polling/scheduling is not part of this decision or Gate 1.3.


## D-018 — IndexCore owns OpenList collection in the observation plane

**Status:** Accepted

cloudsite does not add a second production OpenList tree-walking client for Gate 1.

The observation boundary is:

```text
OpenList-compatible storage view
        ↓
IndexCore OpenList Collector
        ↓
Canonical Inventory + Journal
        ↓
cloudsite IndexCore Query client + Catalog Projector
```

IndexCore owns collection, normalization, canonical identity, completeness safety, and Journal production. cloudsite consumes canonical physical truth through IndexCore and projects product-owned Copy state.

cloudsite's existing OpenList VisibilityPort / AccessPort remains a later control/acquisition-side contract. It may be implemented when Gate 3/4 needs post-download visibility verification or access resolution, but it must not become a second canonical observation scanner.

Gate 1 closeout uses a controlled OpenList-compatible fixture and a real, externally executed IndexCore binary pinned to an accepted version. cloudsite never imports IndexCore internals to obtain this evidence.

OpenList absence is not physical deletion. The accepted IndexCore OpenList collector is additive-safe unless stronger completeness evidence exists.


## D-019 — Copy classification is an explicit monotonic binding by default

**Status:** Accepted

cloudsite classification attaches an existing unresolved Copy to an existing Variant.

The default transition is monotonic:

```text
variant_id = NULL
        ↓
explicit binding command
        ↓
variant_id = target Variant
```

Rules:
- a missing Copy is not created by classification;
- a missing Variant is not created by classification;
- repeating the same Copy → Variant binding is idempotent;
- attempting to bind an already-classified Copy to a different Variant fails closed;
- silent reclassification is forbidden;
- CopyID, IndexCore physical identity, StorageBindingID, and physical availability are preserved.

Concurrency must be fenced by the persistence boundary so two different target Variants cannot both win.

The Catalog binding operation owns VariantID. The observation projector owns physical availability. Projector replay must preserve existing classification.

Any future correction/reclassification capability requires a separately authorized operation rather than weakening this default rule.


## D-020 — Acquisition Manifest records intent; Job Engine owns execution safety

**Status:** Accepted

cloudsite separates acquisition intent from execution state.

The Acquisition Manifest records:
- what source is requested;
- which StorageBinding and target path should receive it;
- optional logical Asset / Release / Variant association;
- optional future product user linkage;
- linkage to a durable Job when execution is authorized;
- coarse acquisition milestone.

The durable Job Engine remains the sole owner of execution claim, lease, retry, recovery, and terminal execution safety.

Therefore:
- the Manifest must not duplicate Job lease/attempt/retry state;
- provider-changing side effects still require a durable Job before execution;
- Manifest existence alone never authorizes a provider side effect;
- provider task references and execution attempts are added only in later Gate 3 work;
- direct acquisition may exist without logical classification;
- classification remains optional product semantics, not a prerequisite for acquisition intent.

Gate 3.1 creates and validates acquisition intent only. It does not execute 115, OpenList, IndexCore Hint, or any other external side effect.


## D-021 — Acquisition becomes executable only after atomic Manifest + Job activation

**Status:** Accepted

A PENDING Acquisition Manifest is durable intent, not executable work.

Before any provider-changing acquisition side effect is allowed, cloudsite must commit one atomic activation that:
- creates exactly one generic durable `ACQUISITION` Job in `QUEUED`;
- links that Job to the Manifest;
- transitions the Manifest from `PENDING` to `ACTIVE`.

The commit boundary must guarantee:
- no orphan QUEUED acquisition Job;
- no ACTIVE Manifest without its durable linked Job.

The acquisition Job payload is a minimal versioned reference to `manifest_id`; it does not duplicate source_ref, target path, credentials, or provider-specific execution input.

Activation replay is idempotent by Manifest identity. Once a Manifest is ACTIVE and correctly linked, the durable linked Job is authoritative even if a retry proposes another Job ID.

The acquisition application layer may depend on cloudsite's provider-neutral Jobs domain for this orchestration. It must not duplicate Job lease/retry/recovery semantics.

Provider execution remains forbidden in Gate 3.2. Later workers may act only after this activation commit is durable.


## D-022 — StorageBinding maps provider scope, OpenList mount, and IndexCore root independently

**Status:** Accepted

A cloudsite StorageBinding connects three coordinate systems:

```text
provider target scope
OpenList mount path
IndexCore root_id
```

These values are related by explicit product configuration and must never be derived from one another.

Rules:
- OpenList mount paths retain their own normalized path semantics.
- IndexCore root IDs remain canonical physical-index identifiers.
- Provider target scope is opaque adapter-owned text and is stored separately.
- Observation-only bindings may omit provider scope.
- Acquisition requires an explicit provider scope.
- Provider adapters must not query cloudsite persistence to discover this mapping.
- StorageConnection.provider_type is the provider registry identity used for later adapter selection.

This separation prevents provider execution from depending on accidental equivalence between provider paths, OpenList mounts, and IndexCore roots.


## D-023 — Provider side effects require durable task linkage; uncertain starts fail closed

**Status:** Accepted

cloudsite crosses the external downloader boundary through a durable, provider-neutral provider-task linkage.

Rules:
- one MVP acquisition Manifest maps to at most one provider task;
- if a durable provider task reference exists, replay polls that task and never calls StartDownload again;
- provider task references remain opaque and are not parsed by the acquisition domain;
- provider task status does not replace cloudsite Job state or Acquisition Manifest milestone state;
- provider-reported success is not cloudsite READY;
- if StartDownload succeeds but the returned provider task reference cannot be durably recorded, cloudsite treats the result as an uncertain external side effect;
- uncertain external side effects must fail closed and must not be converted into blind automatic recreation.

This boundary exists so crash/restart behavior cannot accidentally duplicate provider-side downloads.


## D-024 — Provider execution is connection-scoped, not ProviderID-scoped

**Status:** Accepted

Provider implementation identity and authenticated provider session identity are different concerns.

```text
ProviderID     = which adapter implementation
ConnectionID   = which StorageConnection
CredentialRef  = which opaque secret/session reference backs that connection
```

Rules:
- ProviderID alone must never select authenticated provider state.
- The same ProviderID may serve multiple StorageConnections.
- Acquisition/domain code may carry CredentialRef only as opaque metadata; secret contents remain outside product state.
- Provider adapters must not query cloudsite persistence to discover credentials.
- Secret/session resolution happens at the composition/provider-session boundary.
- A missing or mismatched connection/credential binding fails closed before provider side effects.

This decision is required before the real 115 adapter because a real downloader is authenticated, while Gate 3.4 intentionally proved only the provider-neutral side-effect mechanics.


## D-025 — 115 Downloader maps provider scope to saveDirID and info_hash to task identity

**Status:** Accepted

The MVP concrete 115 Downloader adapter uses the pinned Go library `github.com/SheltonZhu/115driver v1.3.5`.

Mapping:

```text
ProviderID = "115"

DownloadRequest.Source.Value
    -> exact URI passed to 115 offline download

DownloadRequest.Target.Scope
    -> 115 destination directory ID (wp_path_id / saveDirID)

DownloadRequest.Target.Path
    -> cloudsite expected observation path only
       never converted into provider directory identity

TaskReference.Value
    -> 115 offline task info_hash
```

Rules:
- source URI is never normalized, truncated, or rewritten by the adapter;
- deterministic Source Resolver owns future source canonicalization;
- provider destination is never derived from OpenList mount, IndexCore root, or Target.Path;
- one StartDownload creates exactly one offline URI task;
- provider task status is mapped from 115 task state only and never implies cloudsite READY;
- cancellation deletes the offline task only, never downloaded files;
- cloudsite uses the 115driver Go library directly rather than CLI/MCP subprocesses for the runtime adapter.

This keeps 115-specific semantics fully behind the provider contract and preserves the existing control-plane/observation-plane separation.


## D-026 — Provider-stage outcome commits Manifest milestone and Job state atomically

**Status:** Accepted

The durable ACQUISITION Job spans the full acquisition workflow, not only the provider RPC.

Provider-stage outcome mapping:

```text
PROVIDER_IN_PROGRESS
  Manifest ACTIVE
  Job RUNNING -> RETRY_WAIT

PROVIDER_SUCCEEDED
  Manifest ACTIVE -> AWAITING_VISIBILITY
  Job RUNNING -> RETRY_WAIT

PROVIDER_FAILED
  Manifest ACTIVE -> FAILED
  Job RUNNING -> FAILED

PROVIDER_CANCELED
  Manifest ACTIVE -> CANCELED
  Job RUNNING -> CANCELED

UNCERTAIN / RECOVERY
  Manifest ACTIVE -> RECOVERY_REQUIRED
  Job RUNNING -> RECOVERY_REQUIRED
```

Rules:
- Manifest and linked Job are updated in one PostgreSQL transaction.
- Provider success is not Job SUCCEEDED and is never Manifest READY.
- The same ACQUISITION Job continues into visibility/canonical stages; no second visibility Job is created.
- The currently RUNNING Job remains fenced by owner, attempt generation, and database-time lease validity.
- Exact committed outcome replay is idempotent and does not rewrite timestamps.
- Conflicting outcome replay fails closed.
- RECOVERY_REQUIRED remains non-terminal and is not normally claimable.

This prevents provider-stage completion from leaving product milestone and control-plane execution state out of sync.


## D-027 — Claim generation and failure retry budget are separate Job counters

**Status:** Accepted

D-014 remains correct that active Job leases require a monotonically increasing claim generation plus database-time lease validity. Gate 3.7 refines how that generation is stored.

The two concerns are now explicitly separate:

```text
claim_attempts
    monotonically increasing claim generation
    incremented on every successful ClaimNext
    unbounded
    used as the stale-worker fencing token

attempt_count
    failure retry count (Go: jobs.Job.FailureCount)
    incremented only by RetryAt
    bounded by max_attempts
```

Consequences:
- normal provider polling, visibility polling, and later workflow-stage rescheduling do not consume failure retry budget;
- RetryAt is the only generic Job Engine operation that consumes the failure budget;
- a newer claim generation fences every older worker even when the owner string is reused;
- provider success may return the same ACQUISITION Job to RETRY_WAIT without risking exhaustion of the failure budget;
- RECOVERY_REQUIRED remains outside ordinary ClaimNext.

Migration `0010_job_claim_generation.sql` preserves upgrade-time fencing by moving the pre-v10 `attempt_count` value into `claim_attempts` and resetting the new failure counter to zero:

```sql
UPDATE jobs
SET claim_attempts = attempt_count,
    attempt_count = 0;
```

This supersedes only D-014's implementation note that the old `attempt_count` column itself was the claim-generation token. D-014's fencing principle remains unchanged.


## D-028 — Manifest target_path is binding-relative for OpenList observation

**Status:** Accepted

For acquisition visibility checks, OpenList addressing is derived only from the configured OpenList mount and the Manifest's normalized target path:

```text
OpenList request path
    = Join(StorageBinding.openlist_mount_path, Manifest.target_path)
```

Rules:
- `openlist_mount_path` is the OpenList mount coordinate;
- `Manifest.target_path` is the path inside that binding's observed namespace;
- provider_scope is never used to derive the OpenList path;
- IndexCore root identity is never used to derive the OpenList path;
- provider_scope is never derived from the OpenList path;
- the joined path must stay absolute, normalized, and beneath/equal to the configured mount.

Examples:

```text
mount "/"    + target "/downloads/item" -> "/downloads/item"
mount "/115" + target "/downloads/item" -> "/115/downloads/item"
```

This preserves D-022/D-025's separation between provider target coordinates, OpenList observation coordinates, and IndexCore canonical root identity.


## D-029 — cloudsite does not directly verify OpenList during acquisition

**Status:** Accepted — **supersedes D-028 for acquisition verification**

D-028 incorrectly elevated OpenList from IndexCore's provider/collector dependency into a direct cloudsite acquisition dependency.

The accepted observation boundary remains D-018:

```text
Storage
  -> OpenList
  -> IndexCore Collector
  -> Canonical + Journal
  -> cloudsite
```

Therefore, after a provider reports download success, cloudsite does **not** call OpenList to prove visibility.

Instead:

```text
Manifest AWAITING_VISIBILITY
        ↓
StorageBinding.indexcore_root_id
+ Manifest.target_path
        ↓
IndexCore trusted Mutation Hint
        ↓
IndexCore-owned OpenList scoped verification
        ↓
Canonical + Journal
        ↓
cloudsite confirmation
```

Exact Mutation Hint mapping:

```text
root_id   = StorageBinding.indexcore_root_id
scope_key = Manifest.target_path
reason    = POSSIBLE_CHANGE
```

The following coordinates remain separate and must not be derived from one another:

```text
provider_scope          provider-side mutation target
openlist_mount_path     IndexCore/OpenList observation configuration
indexcore_root_id       canonical IndexCore root identity
manifest.target_path    directory scope inside the selected binding/root
```

`AWAITING_VISIBILITY` now means the provider stage is complete and cloudsite is waiting to hand the affected scope to the **IndexCore-owned observation pipeline**. It does not authorize a direct cloudsite -> OpenList stat.

After IndexCore accepts the trusted Hint, cloudsite may advance to `AWAITING_CANONICAL`, but Hint acceptance is only durable verification work ingress. It is never Canonical truth and never READY.

Repeated identical Mutation Hints are intentionally safe at-least-once signals: IndexCore coalesces them through its accepted DirtyScopeWork state machine. A duplicate Hint may advance signal metadata but cannot duplicate the provider download or directly create Canonical truth.

D-008's READY principle remains unchanged in meaning: the acquired result must become observable through the OpenList-backed IndexCore pipeline and then be canonically confirmed through IndexCore Query/Journal before cloudsite marks READY. cloudsite itself does not need a separate OpenList verification hop.


## D-030 — AI-assisted development reconstructs truth from Git, not chat context

**Status:** Accepted

cloudsite is developed through long-running AI-assisted architecture and implementation sessions. Accumulated chat context is not reliable enough to act as project memory by itself.

A compact canonical memory file is therefore maintained at:

```text
docs/AI-ARCHITECTURE-MEMORY.md
```

Before designing, authorizing, implementing, or reviewing a Gate, the Architect/AI must reconstruct project truth in this order:

```text
AI-ARCHITECTURE-MEMORY
    ↓
PROJECT-STATE
    ↓
newest relevant DECISIONS
    ↓
PROJECT-CONTEXT / MVP-BLUEPRINT
    ↓
current Issue / PR
    ↓
exact code + tests + exact-head CI
```

Rules:
- chat memory never overrides current Git;
- a newer explicit accepted decision supersedes an older decision/prose/Issue/PR statement;
- contradictions are resolved explicitly and then synchronized across project-memory docs;
- an external integration must be checked against its current accepted contract before a new cloudsite Gate is written;
- Git must be searched for an existing owner before introducing a new module or responsibility;
- project memory is synchronized after accepted architecture changes and after any discovered context-drift incident;
- team topology remains exactly 1 Architect + 1 Developer unless an explicit later decision changes it.

The purpose is not to duplicate all historical detail. The compact memory keeps only current invariants, dangerous superseded assumptions, active Gate state, and the required reconstruction protocol.

This decision was introduced after a context-drift incident in which direct cloudsite -> OpenList acquisition verification was proposed despite the already accepted D-018 IndexCore-owned observation boundary. D-029 corrected the architecture; D-030 prevents the same class of drift from becoming process-normal.


## D-031 — Acquisition result identity is one direct child of target_path

**Status:** Accepted

Gate 3.8 established that `Manifest.target_path` is the directory scope handed to the IndexCore-owned scoped observation pipeline.

Canonical acquisition confirmation must not guess which child in that directory belongs to one Manifest.

The provider-neutral identity is therefore:

```text
StorageBinding.indexcore_root_id
+ Manifest.target_path
+ Manifest.expected_name
```

Semantics:

```text
Manifest.target_path
    target directory / IndexCore scoped-refresh directory

Manifest.expected_name
    exact direct-child name expected to appear under target_path
```

Rules:
- `expected_name` may be absent at initial Manifest creation;
- before provider success advances to `AWAITING_VISIBILITY`, a valid expected name must be frozen;
- if intent already supplied expected_name, a provider-reported name must match exactly;
- if intent did not supply expected_name, a provider-reported result name may fill it atomically with provider-success handoff;
- once frozen, expected_name is immutable through the acquisition workflow;
- a successful provider task with neither predeclared nor reported result name cannot advance;
- result identity is descriptive only and does not prove physical/canonical presence;
- provider file IDs, provider directory IDs, OpenList paths, timing windows, Journal ordering and "only new item" heuristics must never substitute for this identity.

A valid expected_name is one direct path segment: valid bounded UTF-8, nonblank, no NUL, no slash/backslash separator, and not `.` or `..`. Exact spelling/case is preserved.

The pinned 115 adapter may map only `OfflineTask.Name` into the provider-neutral result name. `FileId` and `DirId` remain provider-private and are not IndexCore identity.

Gate 3.10 may later resolve the exact canonical child path from this frozen identity; D-031 itself authorizes no Q5 lookup and no READY transition.


## D-031 — Canonical acquisition confirmation requires a durable result locator

**Status:** Accepted

`Manifest.target_path` is a directory scope, not the final acquired resource path. `Manifest.expected_name` is intentionally nullable. cloudsite must therefore never identify an acquisition result by listing the target directory and choosing a candidate.

Before an acquisition may leave the provider stage for IndexCore observation, cloudsite must have one durable provider-neutral top-level result name:

```text
valid provider-observed TaskStatus.ResultName
        >
Manifest.ExpectedName fallback
```

The persisted result locator is:

```text
Manifest.result_name
```

and the future exact canonical candidate path is:

```text
Join(Manifest.target_path, Manifest.result_name)
```

Rules:
- result_name is one safe basename/path segment, never a full path;
- provider FileId/DirId/task reference are not IndexCore identity;
- provider-observed result name wins over the expected-name fallback;
- an invalid provider-observed result name fails closed rather than falling back silently;
- if neither source provides a valid name, provider success cannot advance automatically to AWAITING_VISIBILITY and requires explicit recovery;
- once persisted, result_name is immutable;
- same-value replay is idempotent and different-value replay conflicts;
- result_name is locator evidence only, not Canonical truth and never READY.

The 115 adapter may source this field only from the pinned upstream offline task `Name`. It must not fabricate the name from magnet metadata, URL text, provider scope, Target.Path, FileId, or DirId.

Gate 3.9 captures this locator. Exact IndexCore Q5/Journal confirmation remains a later Gate.


## D-032 — result_name is the durable acquisition locator; expected_name remains intent/fallback

**Status:** Accepted

This decision resolves the duplicate D-031 entries and is authoritative wherever they conflict.

The earlier D-031 interpretation that reused and mutated `Manifest.expected_name` as the final acquired-result locator is superseded.

The durable model is:

```text
Manifest.expected_name
    optional request-time expectation / fallback hint

Manifest.result_name
    provider-stage durable top-level acquired-result locator
```

Canonical candidate identity for later confirmation is:

```text
StorageBinding.indexcore_root_id
+ Manifest.target_path
+ Manifest.result_name
```

Rules:

1. `target_path` remains the directory scope refreshed by IndexCore.
2. `expected_name` remains request intent and is not rewritten by provider execution.
3. `result_name` is a separate nullable persistence field introduced by Gate 3.9.
4. A valid provider-observed result name takes precedence over `expected_name`.
5. `expected_name` is used only when the provider reports no result name.
6. If the provider reports an invalid nonblank result name, fail closed; do not silently fall back.
7. If the provider reports no name and expected_name is absent/invalid, provider success cannot advance automatically.
8. Once persisted, `result_name` is immutable; same-value replay is idempotent and different-value replay conflicts.
9. `result_name` is locator evidence only. It is never Canonical truth and never READY.
10. Provider FileId/DirId, OpenList paths, timing, Journal ordering, newest-item and only-item heuristics cannot substitute for `result_name`.

For the pinned 115 adapter:
- successful `OfflineTask.Name` may populate provider-neutral `TaskStatus.ResultName`;
- a blank/absent upstream name yields SUCCEEDED with no ResultName, allowing acquisition-level `expected_name` fallback;
- invalid nonblank/unbounded names fail closed;
- FileId and DirId remain provider-private.

Persistence:
- migration v11 adds nullable `acquisition_manifests.result_name`;
- existing v10 rows are preserved with `result_name = NULL`;
- no database default is allowed;
- the provider-success transition, not a historical migration rewrite, enforces that new automatic progression to observation has a resolved result locator.

Gate 3.8 remains scoped only by `indexcore_root_id + target_path`. The Mutation Hint does not carry result_name. Exact canonical resolution using result_name is deferred to Gate 3.10.

Issue #42 is the authoritative Gate 3.9 contract. Issue #41 is superseded and must not be used for implementation or acceptance.


## D-033 — READY is anchored to one canonical projected Copy

**Status:** Accepted

Gate 3.9 provides a deterministic pre-canonical locator:

```text
StorageBinding.indexcore_root_id
+ Manifest.target_path
+ Manifest.result_name
```

Gate 3.10 turns that coordinate into one stable product result only after both IndexCore canonical truth and cloudsite Journal projection agree.

READY requires:

```text
exact default-visible Q5 match
        ↓
one PRESENT IndexCore resource
        ↓
existing Q8 Journal Projector
        ↓
exact PRESENT cloudsite Copy
        ↓
optional monotonic Copy -> Manifest.variant_id binding
        ↓
Manifest.result_copy_id = Copy.ID
Manifest READY
Job SUCCEEDED
```

Rules:
- a path is a coordinate, not identity;
- zero Q5 matches is normal pending work;
- Q5 ambiguity or multiple matches is never guessed and requires recovery;
- the canonical match must echo the exact requested root/path and be PRESENT;
- acquisition must not create or upsert Copy directly;
- D-017 remains authoritative: Q8 Journal + Projector owns Copy creation and physical availability;
- canonical confirmation may advance the existing projector by at most one bounded page per invocation;
- if the exact Copy is not yet projected/PRESENT, the same Acquisition Job returns to RETRY_WAIT without consuming failure budget;
- if Manifest.variant_id is set, the exact Copy must be monotonically bound to that Variant before READY; if no VariantID exists, unresolved Copy is allowed;
- READY atomically stores an immutable `result_copy_id` and marks the same ACQUISITION Job SUCCEEDED;
- `result_copy_id` is the durable historical acquisition result link; later Copy availability changes do not rewrite acquisition history.

Migration v12 adds nullable `acquisition_manifests.result_copy_id` referencing `copies(copy_id)`, with no default and no historical backfill. New READY writes require a result_copy_id while historical pre-Gate-3.10 rows remain upgrade-compatible.

Gate 3.10 does not add a continuous worker daemon, direct OpenList access, direct IndexCore database access, Search-based guessing, or a second Copy projection path.

## D-034 — One claimed ACQUISITION Job executes at most one persisted Manifest stage

**Status:** Accepted

Gate 3.10 completed the bounded provider-neutral, observation, and canonical stage components. Gate 3.11 wires them without replacing their owners.

Stage routing is determined exclusively by the currently persisted Acquisition Manifest:

```text
ACTIVE
  -> bounded ExecutionStepService.Execute
  -> atomic ProviderOutcomeService.Commit

AWAITING_VISIBILITY
  -> bounded RefreshStep.Submit

AWAITING_CANONICAL
  -> bounded CanonicalConfirmation.Confirm
```

Rules:
- one ACQUISITION Job spans all stages; no stage-specific duplicate Job;
- one acquired Job claim generation grants **at most one** stage invocation, regardless of how quickly that stage completes;
- the next stage requires a separately claimed Job; no same-invocation cascade or sleep/poll loop;
- stage selection is based on durable Manifest.State, not guessed provider status, elapsed time, OpenList state, or a transient in-memory cursor;
- active work requires the linked ACQUISITION Job, matching owner and claim generation and the existing database-time fenced commit;
- a terminal Job + corresponding terminal Manifest is a committed result/replay, not a fresh lease-authorized execution; it does no external work;
- provider result-name propagation and D-026 outcome translations remain closed;
- uncertain provider side effects must follow explicit recovery, never blind StartDownload replay;
- normal provider polling, Hint/IndexCore propagation and canonical pending do not consume FailureCount; ClaimAttempts remains the generation fence;
- provider execution, IndexCore Mutation Hint, Journal Projector, and Copy state retain their accepted owners;
- no direct cloudsite-to-OpenList acquisition visibility validation.

Gate 3.11 builds a bounded one-claim coordinator and controlled multi-claim evidence **without** an automatic scheduler/worker daemon. Later runtime wiring and cadence require a separately authorized Gate. No new database migration is required by this decision.

## D-035 — ACQUISITION-only claiming and atomic linked expired-lease recovery

**Status:** Accepted

Gate 3.11's one-claim dispatcher is accepted. Before enabling an autonomous worker, two Job Engine integration invariants are required:

- An acquisition worker may claim only `Job.Type == ACQUISITION`, using an atomic type-scoped claim that never leases and discards unrelated Job types. Preserve ClaimAttempts, failure-budget and DB-time lease semantics of the existing Job Engine.
- An expired RUNNING ACQUISITION Job is a **linked Manifest + Job recovery transition**, not a standalone Job status update.

Automatic expired acquisition recovery is allowed only for a fully validated `ACTIVE`, `AWAITING_VISIBILITY`, or `AWAITING_CANONICAL` Manifest linked to the correct ACQUISITION Job. Lock Manifest before Job, check expiry by PostgreSQL time, then atomically set both to `RECOVERY_REQUIRED`; leave malformed/missing/conflicting pairs unchanged for explicit diagnosis.

The generic expired-lease sweep must not independently change the state of an ACQUISITION Job, or it would violate the accepted D-034 terminal-pair contract.

No provider task reference, START_RESERVED fence, result_name, result_copy_id, claim generation or failure count is rewritten by this recovery. Nothing automatically repeats a provider mutation.

Gate 3.12 implements these two bounded prerequisites with PostgreSQL evidence. It does not authorize a polling worker daemon, process runtime wiring, OpenList verification, Source Resolver, API or UI.

## D-036 — One explicit acquisition runner tick, no autonomous scheduling

**Status:** Accepted

Gate 3.12 accepted type-scoped ACQUISITION Job claiming and atomic expired Manifest + Job lease recovery. Gate 3.13 connects these with the accepted one-stage D-034 dispatcher through one explicitly invoked application tick.

RunOnce order:
```text
bounded MarkExpiredAcquisitionRecoveryRequired
          ↓ if healthy
ClaimNextByType(ACQUISITION) at most once
          ↓ if job exists
StageDispatcher.Dispatch at most once
```

Rules:
- recovery debt or recovery store failure blocks further claims within the tick; partial recovered counts remain visible;
- ErrNoClaimableJob is IDLE, not a failure or a synthetic claimed Job;
- pass the actually claimed JobID, owner and ClaimAttempts to D-034; never use FailureCount as generation;
- a tick never chains the resulting persisted Manifest into a second stage;
- stage errors are surfaced, not converted into generic Job-only Fail/Succeed/RetryAt changes that could desynchronize the Manifest;
- provider uncertainty remains governed by START_RESERVED and the accepted D-034 paired RECOVERY_REQUIRED transition;
- caller policy time selects retry scheduling but PostgreSQL time remains the lease authority;
- no new provider task side effect permission, OpenList verification, Copy writer, Job family, queue, schema migration or background worker loop.

This Gate delivers a reusable bounded RunOnce capability. Continuous scheduling, `Application.Run` wiring, live credential configuration and deployment remain separately authorized later work.

## D-037 — Opt-in single-process acquisition worker with one in-flight RunOnce

**Status:** Accepted

Gate 3.13 provides a bounded explicit RunOnce. Gate 3.14 authorizes a controlled serial lifecycle loop without taking ownership of Job state, provider tasks or canonical truth.

Rules:
- one worker process loop executes no more than one RunOnce at a time; each tick is independent, and never cascades stages;
- acquisition worker is disabled by default. Enabled startup with missing required dependencies fails closed, never pretends to operate;
- worker identity is stable per process and unique across simultaneous replicas; no hardcoded global owner;
- validated schedule interval, tick timeout, lease duration (with safety margin), retry-at policy and per-tick recovery/projector bounds prevent busy spin or lease overrun by design;
- IDLE, RECOVERY_ONLY and successful stage handoff are normal outcomes; transient error handling must use bounded delays and must never issue Job-only Fail/Succeed/RetryAt;
- ErrLeaseRecoveryDebt is a fatal stop for claiming new work pending explicit operator repair;
- cancellation stops new ticks and propagates to an in-flight RunOnce. D-035 recovery and START_RESERVED remain the durable restart safety barriers;
- only D-033 canonical stage may mark a Manifest READY and Job SUCCEEDED;
- no new worker queue, direct OpenList verification, second Copy writer, new Job type, DB migration, or autonomous agent scheduler.

Gate 3.14 builds the opt-in lifecycle/service and safe process configuration seam; **production PostgreSQL/IndexCore/115 credential wiring and deployment are not yet authorized**. Enabling an unwired process must fail loudly. This is distinct from an operational E2E deployment.

## D-038 — Explicit fail-closed production runtime graph and injected secrets

**Status:** Accepted

Gate 3.14 accepted an opt-in serial acquisition worker but did not wire a live process. Gate 3.15 may compose **one** graph from the accepted PostgreSQL Job/Manifest/ProviderTask/Outcome/Refresh/Canonical stores, IndexCore Q5/Q8 read and trusted Mutation Hint clients, Catalog Journal Projector, exact provider-session registry, RunOnce and worker.

Rules:
- default process remains side-effect-free; enabled startup requires all mandatory dependencies and cloudsite schema v12 before claiming any Job;
- PostgreSQL product DB is distinct from IndexCore DB; no direct IndexCore DB/OpenList acquisition verification;
- Q5/Q8 read and trusted Mutation Hint have separate validated clients and trust boundaries. Hint accepted does not create READY;
- exact ProviderID + ConnectionID + CredentialRef session resolution, external secret injection, no persisted/logged plaintext cookies or Hint tokens;
- unknown/missing provider registration, credentials, unsafe Hint config, incomplete schema or failed required composition is a startup error, never a mock fallback;
- D-017 Journal Projector exclusively owns Copy creation; D-033 Q5 + exact PRESENT Copy exclusively authorizes READY;
- preserve D-034/035/036/037 lease, linked recovery, one-claim stage and one-worker semantics; shutdown cancels work and closes owned resources;
- controlled PostgreSQL + HTTP-fake integration is mandatory and may run in CI. No live 115 account access or production deployment is authorized without a later separate decision.

Scope is bounded to process/runtime composition and controlled evidence; not Source Resolver, API/UI, real account secrets provisioning, deployment images or distributed scheduling.

## D-039 — Operator-protected credentials, read-only preflight and explicit worker start

**Status:** Accepted

Gate 3.15 accepted an internally complete, fail-closed acquisition runtime graph and controlled E2E. The stock CLI must not silently enter this graph; Gate 3.16 introduces an **operator-driven** bootstrap contract.

- Default process startup stays side-effect-free; `preflight` and `start` are distinct explicit operator actions.
- Preflight must only inspect schema v12, trusted configuration/session identity and externally protected secret availability. It must never claim, migrate, start a provider task or submit Hint.
- Worker startup must reuse the one D-038 Runtime graph and D-037 serial lifecycle, with injected credentials from a restricted external SecretResolver and no command-line raw cookies or tokens.
- Malformed/missing/wrong-permission secrets, duplicate or mismatched bindings, unsafe Hint trust, missing schema and uncovered ACTIVE acquisition sessions are fail-closed startup conditions.
- Do not log or persist plaintext credentials, provider source URLs or secret bearer tokens. Secret backend and injected transports are composition-edge responsibilities.
- SIGINT/SIGTERM stops scheduling, propagates cancellation, joins Run, then closes owned cloudsite DB resources. Fatal recovery debt remains an operator-visible error.
- No live 115 account action, production deployment, API/UI, new queue, direct OpenList acquisition check or schema migration is authorized in this Gate.

Gate 3.16 evidence requires protected-secret/preflight safety tests and controlled PostgreSQL+local HTTP integration. Real 115 credentials and production deployment remain separately gated.

## D-040 — Deterministic source intake preserves exact provider input and stays side-effect-free

**Status:** Accepted

Gate 3.16 made the accepted acquisition runtime safely operable, but the product still lacks the deterministic intake boundary required by the Gate 3 blueprint. A supported source must become durable acquisition intent without hand-seeding database state and without allowing source parsing to become a second provider-specific execution layer.

Rules:
- supported deterministic MVP schemes are initially `magnet`, `http`, `https`, and `ed2k`;
- Source Resolver normalizes/classifies the scheme only; the accepted source value is persisted and later passed to DownloaderProvider exactly as supplied;
- no trim, truncation, query stripping, percent re-encoding, magnet canonicalization or provider-specific source rewriting is allowed by this decision;
- validation is structural and side-effect-free: no DNS/HTTP/provider/OpenList/IndexCore/Search/Agent call;
- HTTP(S) must be absolute and host-bearing and must reject embedded userinfo; magnet/ed2k remain bounded opaque values after scheme validation;
- ExpectedName remains explicit caller intent/fallback; Source Resolver never guesses the provider result locator;
- acquisition submission reuses existing Manifest creation and ActivationService rather than duplicating topology, lineage, target or Job-link validation;
- stable Manifest/Job identity or an equivalently deterministic injected ID seam is required for safe replay; conflicting replay fails closed;
- activation failure leaves explicit PENDING Manifest intent available for diagnosis/retry rather than deleting or hiding it;
- submission enqueues only. It must never start the Gate 3.16 worker or cross a provider/network side-effect boundary;
- source text must not be copied into operator logs/errors beyond durable product state that is explicitly designed to hold source_ref;
- no schema migration, concrete 115 credential logic, OpenList acquisition verifier, IndexCore DB client, API/UI/auth/quota/share/Agent implementation is authorized by this decision.

Gate 3.17 / Issue #58 implements and proves this boundary. Human-authorized real 115/OpenList/IndexCore staging acceptance remains a later separate gate.

## D-041 — Human-authorized real staging validates accepted ownership without bypass

**Status:** Accepted

Gate 3.17 completes deterministic source intake, so the intended acquisition path can now be validated without hand-seeding product state. Gate 3.18 authorizes one staging-only real-account acceptance under explicit human operation.

Rules:
- live provider mutation is never a CI/AI action; the human operator explicitly invokes the accepted worker start;
- use IndexCore `v0.4.0-alpha.1` at commit `6f0eec85c59bd8cbe55011b0d9e512e0cafd6615` as the fixed external baseline for this acceptance;
- IndexCore's accepted trusted Hint listener stays literal-loopback-only with >=32-byte token and the P10/P11 hybrid incremental runtime enabled;
- cloudsite and IndexCore share the same host/network namespace for this staging acceptance rather than weakening Hint trust;
- OpenList remains exclusively an IndexCore collector dependency; cloudsite never calls OpenList or its DB to establish acquisition visibility;
- preserve coordinate separation among provider_scope, OpenList collector/mount configuration, indexcore_root_id and Manifest.target_path;
- use a dedicated staging 115 target and small non-sensitive operator-controlled HTTPS test object; never publish the full source URL or credentials in Git/chat/evidence;
- preflight must remain non-mutating, submission enqueue-only, and StartDownload may occur only after explicit worker start;
- provider success and Hint 202 are intermediate evidence only; PASS requires exact Q5 PRESENT canonical resource, Q8/Projector evidence, cloudsite result_copy_id + READY and same Job SUCCEEDED;
- use bounded polling/deadlines, not fixed sleep as proof;
- START_RESERVED uncertainty, Q5 ambiguity, mapping mismatch, recovery debt or secret disclosure are STOP conditions, not reasons to force retries or bypass accepted owners;
- capture only redacted operator evidence and preserve durable failure states for diagnosis;
- no production deployment topology, API/UI, auth/quota/share, Agent, second scheduler, direct IndexCore DB access, second Copy writer or direct cloudsite OpenList verifier is authorized.

Gate 3.18 / Issue #60 may add only staging harness/runbook/evidence scaffolding unless the real run exposes a genuine accepted-contract defect. Such a defect must be reviewed explicitly before production code is changed.

### D-041 execution acceptance — 2026-10-13

Gate 3.18 is **ACCEPTED / CLOSED** under
[Architect final acceptance #6143267673](https://github.com/nathanxiangang-web/cloudsite/issues/62#issuecomment-6143267673).
This is an acceptance record, not a new architecture decision or permission to
repeat live provider mutation. The operator-authorized staging sequence used
reviewed compatibility fixes and the existing accepted owners:

- IndexCore empty-directory compatibility fix PR #110 at `7ef47217f23ee1ba921f2dcc8378c0a2a9fb84e1`;
- cloudsite known-reference recovery PR #71, reviewed HEAD `3e61483bae28a96a43faacbf9353e2bad7fb82d6`,
  merged `74e9e69e1dd89944901e1b6a7b2b42c661b897d7`, exact-head CI `38374274046` all green;
- one protected same-pair/reference continuation under GO `6142455770`;
- provider SUCCESS -> trusted loopback Hint -> unique Q5 PRESENT -> Q8
  resource-added sequence 1 -> existing Projector Copy/cursor -> result_copy_id
  + Manifest READY + same Job SUCCEEDED;
- no new source, Manifest, Job, ProviderTask or StartDownload during recovery;
- failure budget 0/max1 preserved, eight unknown reservations unchanged,
  worker joined and C3 Binding/collector permissions restored.

See [redacted completion artifact](GATE3.18-COMPLETION.md). Schema remains v12.
Gate 4/UI/production deployment remains NOT STARTED and requires a separate
architecture decision. This acceptance does not authorize unknown-task replay,
direct SQL pair-state rewriting, direct Copy creation, or relaxed Hint trust.

## D-042 - Delegated dual-role delivery of the minimum usable core MVP

**Status:** Accepted architecture/execution decision, 2026-10-14.

The human explicitly delegates Architect and Developer responsibilities to one
agent, asks it to write plans before implementation, proceed without repeated
stage authorization, and audit/correct or remove misleading recent documents.
This supersedes a separate Architect's approval for each new bounded stage;
it does not remove testing or change runtime truth.

- Use distinct planning, implementation and review passes. Call this same-agent
  review, not independent review. Keep narrow Issues/PRs and actual tests/CI;
  ordinary defects/configuration are resolved without another approval round.
- Original MVP Gate 4 remains auth/access/share/usage; console, premium, favorites
  and AI entry points cannot substitute for it. Accept the domain ownership in
  [CONTROL-PLANE-DOMAIN](CONTROL-PLANE-DOMAIN.md) and the staged
  [core MVP plan](GATE4-IMPLEMENTATION-PLAN.md), not fictional functionality.
- Auth: operator-provisioned product users, USER/ADMIN, bcrypt passwords,
  revocable opaque hashed sessions, no Provider/Hint identity substitution.
  Future product data uses append-only migrations in an isolated MVP DB.
- Private authenticated Catalog, owner-only task/source views, explicit admin
  admission; zero-price bounded operation accounting. Exact source values,
  Job/Manifest/Copy and DB-time fencing remain authoritative.
- Minimum core MVP defers optional Agent/recommendation/payment/polish; original
  full Gate 5/6 goals are preserved and not declared complete.
- Historical eight unknown reservations, staging DB, IndexCore DB, unrelated
  services and credentials are unchanged. Tests use dedicated non-sensitive
  resources, one fenced start per new operation, never blind retries.
- Production/public-network exposure, destructive data cleanup, paid
  transactions and third-party account provisioning remain outside scope.

Stage 0 corrects docs; Stage 1 executes the frozen auth/session contract. Later
slices need actual contracts and evidence recorded by the delegated Architect,
not another user authorization. A docs commit/partial test is not usable MVP.

## D-043 - Bring the bounded read-only interface forward for user inspection

**Status:** Accepted delegated architecture decision, 2026-10-14.

The human asks to see the UI after auth/read implementation. After PR #78 and
PR #80 acceptance, execute [PRODUCT-READONLY-UI](PRODUCT-READONLY-UI.md) before
Stage 2B admission. This changes presentation order, not minimum MVP scope.
Use embedded Go-served HTML/CSS/JS, existing verified auth/read ports and a new
explicit loopback-only product composition. No new frontend framework or Job
engine, provider/Hint mutation, automatic schema migration or global task view.
Protected operator-only provision, bounded login admission, request/shutdown
timeouts and safe Origin/Host/CSP are prerequisites for a running page. New
owned preview environment only; preserve old staging and historical records.
No fabricated UI progress or real-chain claim from labelled test fixtures.
Admission/usage/access/share remain required subsequent slices; readonly UI
must not be called the usable full MVP or a production deployment.

## D-044 - File deletion stays outside the product UI and API

**Status:** Accepted human scope correction, recorded 2026-10-15.

The human explicitly excludes UI file deletion and will remove files directly
in the network drive. Do not implement a file-delete button, product deletion
API or composed native file-removal capability as part of this MVP. Do not
remove Canonical/Copy rows directly to simulate a physical delete.

IndexCore still owns observation and its conservative missing/removal rules;
absence in a partial scan is not deletion. This does not imply instant UI
convergence or authorize a scanner/safety-rule bypass. Share revocation and
session logout remain distinct requirements and must preserve source files.

## D-045 - Separate file-first consumer UI from administrator UI

**Status:** Accepted human product correction, 2026-10-15; Issue #88.

Keep two interfaces: `/` is the authenticated consumer file library; `/admin/`
retains directory, canonical/Copy diagnostics and owner-only task inspection.
The admin workspace configuration is ADMIN-only server-side, not just a hidden
link. No role change or new credential grant is implied. Directory configuration
belongs to operators, not consumers. Do not delete backend functionality merely
to simplify the frontend, and do not manufacture management mutations that do
not exist yet.

Frontend directly composes real files from existing configured IndexCore read
scopes, without a per-file approval workflow. Existing product login and source
authorization remain. Bounded Q4 directory/pagination traversal is metadata
querying, not a drive scan; disclose incomplete loading/errors rather than claim
whole-drive coverage. Preserve binding+resource identity and exclude directory
or unknown-type entries from file cards. No direct OpenList calls, new provider
download or schema migration. No fabricated covers, content access or playback.
See [module contract](PRODUCT-FILE-LIBRARY.md) for bounds, testing and rollback.

## D-046 - Authenticated file bytes and one-hour server preview cache

**Status:** Accepted delegated architecture and human preview correction,
2026-10-15; Issue #90. Implementation acceptance still requires exact-head CI.

Advance the consumer content slice without changing the IndexCore observation
owner or enabling Acquisition. Existing product login and configured canonical
read scopes authorize exact binding/resource/path Q5 PRESENT files; no per-file
approval or invented Copy is required for existing physical files. An existing
REMOVED Copy refuses access. Observation-only DISABLED does not become an ACTIVE
provider Binding. No directory/delete/source submission/native provider capability.

The dedicated OpenList content adapter fetches only authorized known-file bytes,
never acquisition visibility. No client URL/token/cookie forwarding, raw URL
disclosure or third-party Office viewer. Single Range/HEAD enables native media
playback and attachments; HTML/SVG/executable formats never execute inline.

At the human's request, document preview first downloads to a private server
cache for one hour, then renders/parses on the server. Revalidate canonical
identity on EVERY hit; do not extend TTL on reads. Hash-keyed original files,
atomic bounded writes, per-minute owned-file cleanup, <=128MiB/file and <=1GiB
total. No drive originals are removed. Same-size edits may remain cached until
TTL; this is not a freshness guarantee. PDF returns bounded PNG pages through
Poppler processes with time/memory/output limits; this is not a complete OS
sandbox. Office currently returns safe labelled text, not original-layout slides
or spreadsheets. Video stays Range-streamed rather than fully cached.

Separate fileaccess/filecache/filepreview/HTTP/composition modules are replaceable
and independently tested; the parent OpenList port remains contract-only. This
supersedes D-045's metadata-only restriction only for this bounded content slice.
CloudSite sharing is a reference, NOT implemented or accepted here. Preserve
Issue #83 WIP, existing schemas, historical provider tasks and unrelated services.
See [content module contract](PRODUCT-FILE-CONTENT.md).

## D-047 - Expiring cloudsite single-file sharing and download UX

**Status:** Accepted delegated architecture decision, 2026-10-15; Issue #92.

The human prioritizes download and sharing after D-046. Implement bounded
cloudsite-hosted single-file shares, referencing inspected CloudSite expiry/code/
revoke UX, not its database or provider integration. This is NOT native 115
sharing; original ShareProvider create/inspect/revoke remains a distinct future
capability and must not be declared complete from product links.

Keep domain fileshare + Store port, PostgreSQL adapter, HTTP boundary and frontend
separate. Exact fileaccess canonical authorization before creation/download;
immutable target and current enabled owner's identity, never fabricated users or
provider IDs. Hash-only 256-bit link token, optional bcrypt code, separate random
hashed grant cookie; link secret alone cannot bypass a configured code. DB-time
expiry/revoke and owner-only management. Cap active shares/grants and redemption.
No permanent default, directory sharing, file deletion or native provider calls.

Append migration14 only to owned product DB after exact-head CI/review; original
v12 histories/IndexCore unchanged. Preserve Issue83's separate WIP; its unmerged
draft14 needs later renumber/rebase. No automatic startup migration. Back up and
stop only owned product before explicit reviewed migration. Strict-v13 binary
cannot run on v14; rollback uses reviewed v14-compatible content-only mode,
not a false promise of blindly restoring the old receipt. No public exposure.

Consumer provides stream-based downloads, file-specific share creation/copy,
owner list/revoke, and guest single-file redemption/download page. Do not buffer
entire large downloads into browser Blobs. Existing localhost-only deployment
means links require reachable cloudsite; it is not public internet distribution.
Revocation denies new requests, not bytes already sent. No full MVP claim.
See [module/deployment contract](PRODUCT-FILE-SHARING.md).

## D-048 - Complete CloudSite file scope and optional random4-character codes

**Status:** Accepted direct human correction, 2026-10-15; Issue #94.

New share forms generate exactly4 uniformly sampled alphanumeric characters
using browser crypto. Clearing the input intentionally disables code validation;
never auto-refill it on submit. New nonempty codes are exactly4 A-Z/a-z/0-9.
Existing longer hashed codes remain redeemable. Keep256-bit link/grant tokens,
bcrypt, expiry/revoke and bounded guesses; four-character codes are not standalone
access credentials. Show/copy the code with the created link and clear on logout.

The human expands consumer scope to every file under their CloudSite directory,
superseding the three-subtree-only operator scope. Read-only discovery confirms
the existing OpenList account base_path `/115open/CloudSite`; logical `/` is this
directory only, NOT all115 storage. Use one NEW Root in the isolated catalog and
one bounded existing-CLI scan, unchanged canonical/deletion rules. No native
download/share/session or source-file mutation, no original DB/Root changes.

Keep old three product bindings readable so old shares survive; add one new
observation-only DISABLED binding and select only it for the frontend. Admin and
backend read scopes are not deleted. Explicit Q6 whole-root pagination supplements
Q4 hierarchy; no direct product OpenList catalog or IndexCore DB access. Load all
canonical pages in bounded cancellable batches without old32-page/256-dir cutoff,
render120 files per UI page to keep it responsive. Disclose incomplete reads.
Images/audio/archives/unrecognized extensions use Other; no file format excluded.
Directories never become file cards. Product schema stays14, Issue83 WIP preserved.
See [contract and evidence](PRODUCT-CLOUDSITE-LIBRARY.md). No full MVP claim.

## D-049 - cloudsiteV1.0.0alpha identity and scoped HTTPS deployment

**Status:** Accepted direct human authorization, 2026-10-15; Issue #98.

Additional direct human requirements: open website registration in the protected
HTTPS deployment and reuse local product credentials on the remote instance.
Import password hashes/roles unchanged; no reset or privilege escalation. Public
registration is a separate Registrar port fixed to USER, explicit default-off
composition flag, strict login/password-only input and existing shared bounded
auth admission. No public operator Provision capability or schema migration.

The human deleted the legacy CloudSite repository. Rename the existing panta
repository (same ID/history) to cloudsite; exact release/tag cloudsiteV1.0.0alpha.
Update consumer/admin/guest branding and Markdown documentation without renaming
durable schema/cookie/config/import compatibility identifiers or fabricating MVP
completion. Version naming and functional acceptance are separate facts.

Replace only the verified target's cloudsite1.0.yml workload, preserving its
files/data/image rollback and unrelated services. Use existing HTTPS Traefik
entry, private edge and loopback product/query namespace, separate new owned
PostgreSQL16 product and PostgreSQL18 canonical-snapshot databases. Explicit
HTTPS proxy Origin only; no public database/query ports, HTTP remote credentials,
Worker/Hint/scanner or new provider acquisition. Read-only IndexCore composition
is a separate pinned-source build with its template hash, not a product import
of IndexCore internals or a falsely labelled unmodified upstream release.

Reuse the existing restricted storage application account/namespace. Protected
application-token renewal is NOT native115 login or provider execution. Reload
the protected token at the adapter boundary, reject bad replacements, keep all
auth/Q5/Range/guest-grant checks. Data restores only new owned databases; no old
history deletion. Exact-head CI, separate same-agent review and actual HTTPS/
real-file acceptance precede cutover. See [release contract](CLOUDSITE-V1-ALPHA.md).

## D-050 - Six-character product password minimum

Accepted direct human request,2026-10-15, Issue101. Replace the12-byte minimum
with at least6 Unicode characters; retain valid UTF-8, no NUL and bcrypt's72-byte
maximum. No uppercase/digit/symbol composition requirement. Domain registration,
login and operator provisioning share this policy; consumer/admin input and
documentation must agree. Existing password hashes/roles are not reset.

## D-051 - Replace public-server AList with local-config OpenList

Accepted direct human request,2026-10-15. Reuse the local OpenList configuration
and credentials, migrate them privately to the verified public server. Replace
ONLY the alist service in /opt/my-servers/docker-compose.yml with OpenList, retaining
the HTTPS storage route and existing network compatibility. Back up the exact
compose and consistent storage database/config before changing it. Stop/remove
the old alist container only after protected backups; do not delete its data,
volumes or rollback image. No whole-stack down/recreation; Traefik/Portainer/
backup services remain untouched. Pin the reused OpenList image by digest.

Do not publish config/driver cookies/JWT/passwords or copy them into Git. Preserve
logical CloudSite paths and product/index identities; validate API identity,
namespace and real bytes before product cutover. Source login is reused, not a
new native115 login or download. Application-token renewal must match the actual
OpenList identity, not assume the previous AList user ID5. No new acquisition,
Worker, Hint, scan or original historical-database mutation.

Final human clarification abandons AList credential migration and uses only the
local OpenList config/existing app login. Runtime replacement completed: v4.2.6
pinned image, old alist container removed, original data/image/YAML preserved.
Cloudsite bytes now use OpenList, not the local VM or former AList. Local user3's
existing scope/permission retained, app token renewed by actual login and the
reviewed identity-bound timer. Public real-file regression passes. See latest
alpha contract; historical initial preserve-AList instructions are superseded.

## D-052 - Authorized direct file delivery, separate from private previews

Accepted direct human correction,2026-10-15, Issue105. Both local and public
implementations currently proxy file bytes; prior local functional tests did not
prove302. Public delivery adds an avoidable server-to-client data hop. A Windows
client can read the existing115 URL from public OpenList directly with matching
Range/hash; this evidence supports an explicit redirect path, not kernel changes.

Use protected exact HTTPS CDN host allowlist, product auth/guest grant validation,
exact IndexCore Q5/PRESENT identity then known-file OpenList metadata resolution
and302. No file-byte GET on direct delivery, no secret/token/cookie forwarding,
no unsafe target, no silent proxy fallback. Bounded caller UA and narrowly scoped
media CSP. Empty list preserves legacy proxy rollback. Private one-hour document
preview/cache/rendering remains streamed on the server. Issued upstream URLs are
bearer capabilities until upstream expiry; product revoke cannot invalidate them.
No schema/IndexCore scan/Hint/Worker/native115 acquisition/historical mutation.
See [contract](PRODUCT-DIRECT-DELIVERY.md); same-agent review/CI before deployment.

## D-053 - Explicit magnet compatibility before new cloud acquisition admission

Accepted direct human direction,2026-10-16, Issue108. Inspect the MVP blueprint
and actual acquisition/product runtime before adding public cloud download. Reuse
the existing Job/Provider/Hint/Canonical/Projector boundaries, not a second engine.
Current public query-only composition is not an acquisition-ready writer runtime.

Add a separate pure PrepareSource policy magnet-btih-name-v1: one valid BTIH and
optional dn, remove advisory query parameters without arbitrary length cutting;
preserve original input and policy separately. Hex/base32 identities and retained
URL encoding validated; conflicts, so selection and unsupported v2/hybrid topics
fail explicitly. HTTP/HTTPS/ed2k and legacy ResolveSource stay exact. Source values
remain private; no human real-source fixture in Git or diagnostics.

This supersedes D-040's blanket prohibition ONLY for a future explicitly selected
new-product-input preparation step. It does not change the existing resolver,
SubmissionService, Manifest->Provider exact execution bytes, adapter or any old
task/recovery. No raw-source loss: future operation transaction must persist raw,
accepted execution and fixed policy before enabling it. Replays use stored policy
results, not a newly cleaned source. The present helper is not wired to runtime.

Preparation scope is plan/pure module/synthetic tests, no schema, public API/UI,
deployment, real provider call, credential probing, Hint/scan/Worker. Issue83's
dirty draft14 must be preserved and later rebased past merged sharing14. See
[cloud integration contract](PRODUCT-CLOUD-DOWNLOAD.md). Do not claim live cloud
download readiness from parsing tests or old Gate3 staging acceptance.

## D-054 - Product admission with frozen execution evidence and operation usage

Accepted delegated architecture direction,2026-10-16, Issue83. The human directs
implementation of cloud download; the OpenList base-path question is withdrawn.
Resume on current main while preserving the original dirty Issue83 checkout.
The first bounded slice is admission/persistence/HTTP, not live deployment.

Store raw original source, accepted execution source and frozen D-053 policy in
one transaction with product operation identity, quota and PENDING Manifest.
Server-assigned stable Manifest/Job IDs, current enabled user, immutable process
target allowlist, owner-only replay and zero-price10-lifetime/2-in-flight limits.
Digest raw intent, not normalized equality; replay reads stored policy/source
without re-preparation, including after target disablement. Manifest-to-Provider
bytes remain exact. After commit existing SubmissionService activates the pair;
uncertain commit reads durable evidence, never invents replacement IDs/downloads.

Separate product domain, persistence port, postgres adapter and strict HTTP;
no new Job engine/queue or source in DTO/logs. Source64KiB, body512KiB, accepted
execution4096bytes. Append migration15 without touching sharing14. Current public
14 and original staging12 stay unchanged; new15 binary needs explicit reviewed
upgrade/backup and a15-compatible disabled-admission fallback, no auto-migration.
Test actual isolated PostgreSQL rollback/concurrency/uncertainty/pending paths
and controlled admitted-to-READY runtime before exact-head CI/same-agent review.
Public UI, native115 session and runtime/Hint/index writer composition are next
separate integration work, not claims derived from this slice. Preserve8 unknown
reservations and original KNOWN READY evidence. See PRODUCT-ADMISSION.md.

## D-055 - Explicit managed product acquisition and consumer submission

Delegated architecture direction,2026-10-16, Issue111, frozen in the issue before
implementation. PR110/D-054 is accepted (revieweda630dafa, exact-head
CI38901805468 all3 SUCCESS, same-agent review5547872267, mergeec568ae1,
merge CI38902038377 SUCCESS). Compose it without replacing the existing engine.

Add separate serve-product-acquisition and preflight-product-acquisition actions,
enabled policy plus protected fixed target/session/bootstrap. Readonly action
rejects acquisition config. Listener precedes Worker.Run; either side exiting
cancels/joins the other before pool Close. Preflight validates construction,
not live login/health, and performs no claims/provider/Hint/cache work. The
composition-root import guard permits exactly acquisition/runtime/app, retaining
the prohibition on concrete providers/jobs and all existing domain/HTTP guards.

Consumer accepts source+operation_key+optional expected_name only, never target
or provider controls. Same-caller fresh-input preparation preview is the sole
narrow no-source-DTO exception. Stored source/task/usage/error/log output remains
redacted. Source-free owner-operation GET reconciles uncertain commits without
activation. Persist only user-bound pending key, no source; never automatic
submission/replacement IDs after failure/reload/not-found. Clear private DOM on
expiry/hide; retain reconciliation key until acknowledged or explicit logout.
Existing START_RESERVED, frozen-source replay and failure budgets stay intact.

No schema beyond15. Controlled HTTP/PG/runtime/lifecycle tests, exact-head CI and
separate same-agent review precede merge. Public14 migration needs backup and
15-compatible disabled fallback; native session and single-writer incremental
IndexCore readiness need deployment evidence. Shared loopback namespace must
not weaken Hint trust. Public native login recovery is not a download. See
[module and deployment boundaries](PRODUCT-CLOUD-RUNTIME.md).

## D-056 - Explicit readonly source preview and unavailable cloud-download UI

Superseding operational observation2026-10-17: the human subsequently completed
new native QR login; same-account/root GET succeeded, protected new cookie saved.
The OLD-session990001 below remains historical evidence, not current new-cookie
health. Runtime still not configured; no download is implied by login recovery.

Delegated direction2026-10-17, Issue114. Native directory GET on the public
server now returns HTTP200/API990001/ErrNotLogin; Issue113 has no submission or
provider start. Human changes the physical destination to existing /云下载 and
requests continued UI/testing. Never call StartDownload using a known-rejected
session or claim acceptance without actual provider/IndexCore/Copy evidence.

Allow explicit protected cloud_download_preview (label plus closed enum
LOGIN_REQUIRED/NOT_CONFIGURED) in readonly mode. It advertises only fresh-input
pure preparation and unavailable UI; it constructs no bootstrap/runtime/Worker,
and exposes no admission/operation-reconciliation/usage routes. Reject mixed
readonly-preview and managed-acquisition config. This is the sole extension to
D-055's no-preview-routes readonly contract, not permission for background work.
Current-cookie diagnosis is an operator fact, not a fabricated health probe.

Consumer can check its fresh source/view own tasks while submit stays disabled,
retaining pending keys and never probing missing write/lookup/usage routes.
Existing enabled mode and original-source policy remain unchanged. Standard
OpenList Alias may map the requested directory under the existing app namespace;
do not copy OAuth, move source files, broaden account root or bypass IndexCore.
No schema beyond15; exact-head CI and same-agent review before deployment.

## D-057 - Fixed30-day product website sessions

Issue118, human explicitly selected cloudsite WEBSITE login30 days on2026-10-17,
not115 session validity. Supersede auth module's original one-hour issue policy
only for NEW sessions: auth domain owns30*24h, PostgreSQL measures expiry from
DB clock at creation, HTTP cookie Max-Age2592000 and Expires use that result.
No sliding refresh, read-triggered writes, old-session extension or migration.
An existing1-hour session needs one new login to obtain the new lifetime.

Keep random opaque token/digest-only persistence, current account role/enabled
checks, DB expiry and revocation, immediate logout, host-only Secure/HttpOnly/
SameSite Strict cookies and Host/Origin/Fetch Metadata boundaries. Do not change
share grants or provider credentials, TTLs, polling or download budget. Native115
cookie lifetime is controlled by115 and cannot be guaranteed by this policy.
Test domain/store TTL, cookie Max-Age/Expires, actual DB duration, no sliding,
legacy expiry unchanged, expiry refusal and existing revoke/concurrency paths.
Exact-head CI and separate same-agent review before product-only deployment;
retain old image/config rollback. No schema beyond15.

## D-058 - Real cloud submission using the CloudSite directory namespace

Issue120, explicit human direction2026-10-17: enable real public submission using
the current native115 session; if save path is selectable, place it under CloudSite.
Physical /CloudSite/云下载 is the chosen target; the old root /云下载 is not moved
or deleted. ProviderScope is the independently verified native child ID, never
an OpenList virtual path. App account base stays /115open/CloudSite; canonical
and Hint target are /云下载 under the existing full-root CloudSite namespace.

Reuse accepted runtime/admission/Worker/stages/lease/projector and schema15.
Operator may create fresh logical mapping OR atomically attach a NEW115 session
Connection to the exact existing DISABLED observation Binding. CAS requires old
observation-only Connection with no credential, expected Root/mount/Connection,
nil provider scope and no acquisition history; preserve Binding/Root/mount IDs
and old Connection/credential. Guard current database/schema and empty task/
operation queue; refuse replay/adoption. New Connection+Binding change is one
transaction through storage repositories. No direct Job/Manifest/Copy state SQL.

Bounded parent-root PARTIAL Hint discovers the child directory before child Hint;
never run an unnecessary recursive full scan or reinterpret a narrow root path.
Use same identity protected OpenList auth, single IndexCore writer, loopback Hint.
Backup product config/compose and test readonly fallback; preflight constructs
but never claims. Only then use explicit managed action and one serial Worker.
Public submissions retain quotas/ownership/idempotency; no automatic browser POST.
One sealed supplied ordinary ISO smoke submission is permitted; source/name/hash/
provider ref remain private. Unknown outcome preserves reservation, never repeats
StartDownload or retries historical8 unknowns. Public managed lifecycle is now
authorized, superseding Issue113's temporary isolated-worker-only deployment limit,
not its no-duplicate provider-write rule. Actual READY acceptance needs the whole
same Job/provider/Hint/Q5/Q8/Copy transaction evidence, not merely an enabled button.

Operational evidence2026-10-17: PR121 exact c1ed10be, CI38992241383 and merge
CI38992448685 all3 SUCCESS; public f8be03c deployed with preflight, unique target
Q5 and accepted parent/child Hint. One sealed smoke POST201 has REFERENCE_KNOWN
and native destination match, still RUNNING/progress0 at01:58UTC. Budget consumed;
only observe this pair, no READY acceptance. Original service identities and public
delivery/preview/sharing regressions pass. See CLOUD-ACQUISITION-ACCEPTANCE.md.

## D-059 - Cloud-download files have a separate consumer surface

Issue122, human request2026-10-17: show acquired files directly on /downloads/,
not mixed into the main file library. This is a presentation partition using exact
Binding and canonical target subtree, including nested torrent files. No native
file move/delete or new acquisition. Share one file-browser HTML fragment and
existing content/player/download/share controller, with independent page state.
Filter before search, counts and local pagination; preserve upstream cursors.

Authenticated workspace may expose the already-browsable canonical Binding/path
as a read-only view descriptor. This supersedes earlier virtual-target redaction
only; native directory ID, reference, session and credentials stay secret. Fixed
target admission still ignores/refuses consumer injection. Owner-only tasks and
existing shared configured-file authorization are unchanged. UI filtering is not
an authorization boundary. Keep server Q5/content/share checks on every action.

READY links to the in-page files. New READY identities may refresh inventory by
GET; ordinary polling must not reset playback. Stop/expiry aborts/clears both
controllers and ignores late responses. Never fabricate empty/complete success
on errors. No migration, scanner, provider call, Copy rewrite or new smoke budget.
See FILE-SURFACES.md; tests/exact CI/separate same-agent review before deployment.

## D-060 -100-operation policy and explicit session-restoration state

Issue124, human follow-up2026-10-17: cloud list being empty is expected; no new
scan/backfill. Set new default operation quota100 and raise existing default10
configuration rows once with private backup. Preserve used operations and any
non10 custom limits, current operation keys/Job/provider records. No migration.
Remove the in-flight count from consumer text only; server concurrency2/fences
and DTO remain, not unlimited parallel downloads or reset of lifetime usage.

Main library silently resolves session state with login form hidden and no
intermediate message or popup.
Successful auth/workspace restoration opens files without a login flash;401
shows login, transient errors must not pretend the session is invalid. Preserve
30-day cookie/DB policy, logout, account and origin protections. No credential
rotation or session rewrite. Auth/canonical/presentation remain distinct modules.

Actual IndexCore PID1 environment confirms incremental runtime enabled,5s wake,
single-root writer. Accepted chain remains provider completion -> loopback Hint
-> direct-child PARTIAL incremental observation -> Q5/Q8 -> Projector/Copy.
It is not recursive/full scanning and no new scanner or observer is added here.
Full tests/CI and separate same-agent review before product-only deployment.

## D-061 - Directory result identity is not descendant file coverage

Human-reported missing files2026-10-17; verified online2026-10-18. A valid READY
directory Copy anchors the acquisition result, not a guarantee that files inside
it have been observed. Do not fabricate files, rewrite READY history, or add a
consumer-side provider scanner to hide this difference.

IndexCore PR112 owns optional bounded child-directory discovery. Its scan service
reports root-relative observed direct directory scopes only after admitted
PARTIAL observations. The executor limits expansion to an explicit configured
subtree and eight descendant directory levels, using existing per-scope entry
caps and one refresh per ExecuteOne. The store atomically merges child work and
verifies the parent claimed prefix, with root-first locking and CAS. No new queue,
schema, completeness or removal rule; no provider-native delta claim. Repeated
existing-scope hints may backfill missed files. Later changes still need a hint,
watch or explicit refresh; large trees/network failures have no universal30s SLA.

CloudSite PR127 reuses the existing visible-page10s polling cadence for GET-only
canonical inventory checks, even with unchanged READY result_copy_id. A complete
temporary catalog replaces old cloud data only when changed; selected preview,
search/page and existing cards survive ordinary polling. Session/generation
guards drop late results. No browser provider calls, hidden acquisition POST,
new login restoration message or task retry.

Evidence: IndexCore exact66c8ec8 / CI38996188340 and mergee03d14a9 /
CI38996436652 SUCCESS. CloudSite exact0f6fd5b / CI38996296258 and mergef1bceee /
CI38996449893 all3 SUCCESS;55 Node cases plus Go/real PostgreSQL tests and race.
Public authenticated canonical API shows2 cloud files/main283, unique Q5 PRESENT,
child work VERIFIED. No new download submissions/StartDownload. Browser automation
was unavailable for final visual verification; do not relabel API evidence as a
visual playback test. The original long-running ISO smoke has no READY acceptance.
Its separate known-reference pair exhausted5/5 failures; the accepted recovery
API rejected resume, so retain the pair/reservation without refunding its budget.

## D-062 - Blueprint-aligned AI planning with the existing .50 environment

2026-10-18, Issue129. Human requests AI preparation/planning and clarifies the
existing software/configuration source is192.168.178.50 beside IndexCore, not
the public product host113.20.6.57. This authorizes documentation and read-only
readiness checks, not a model request, production AI enablement or a new Gate5
implementation claim. See [modular staged plan](PRODUCT-AI-INTEGRATION.md).

Start with selected real-file metadata/semantic suggestions and verifiable
evidence, then Source/Release/enrichment/verification tasks. Deterministic file
format classification and core user flows remain independent. Model output is
advice, never permission to mutate IndexCore, provider, Job or product state.
AgentPort and strict versioned contracts isolate CodeArts from domain modules.
No chat shell, new Job Engine, autonomous downloader, drive rename/delete or
unrequested whole-library content upload. Manual suggestion application, when
implemented, revalidates authorization/current resource and domain constraints.

Keep actual model credentials on.50 and use a dedicated private authenticated
gateway connection from the product. Do not copy developer homes/IDE tokens,
repurpose115/OpenList/Hint/website keys, expose the old trusted-LAN Worker on
the internet or weaken IndexCore loopback Hint. Public-to-private routing must
be demonstrated in the actual container namespace; not an assumed reachable URL.
Existing secret references/files may be reused only after identity/type/permissions
and endpoint capability are validated. Configuration examples in the plan are
proposed, not accepted fields of the current strict product config parser.

The user's local CodeartsBridge c190fc8 is the authoritative software source.
Three private CSV profiles have populated AK/SK fields; no values are disclosed
or imported. Registry bus-w02-dev maps to.50; the matching account and live model
entitlement still require validation. Do not choose a CSV by filename, copy all
accounts or demand replacement credentials. Preserve the dirty Bridge worktree.
Its actual /api/tasks coding API and CLI JSONL parser are not a product AgentPort;
reuse only bounded protocol/diagnostic components behind a dedicated restricted
adapter. Any necessary Bridge change needs its own narrow patch and tests.

Read-only checks found CodeArts components on.50, but the existing bridge.service
working directory is absent and8080/8765 health is unreachable; common checked
CLI profiles were absent. This does not prove no credentials exist elsewhere.
Do not silently upgrade/reinstall/restart shared services in this planning task.
Restore a restricted dedicated integration separately after locating the actual
profile. Do not confuse co-location with putting LLM logic in IndexCore kernel.

Future AGENT_TASK execution reuses type-filtered Jobs and independent AI usage,
not Acquisition quota/retry state. Task/Job/result/usage finalization must use
DB-time generation-fenced atomic commits. Unknown external model-call outcomes
cannot be declared unbilled/not executed or blindly retried. No automatic budget
refund/format-repair call. AI-disabled core regressions and controlled PG/HTTP/
UI/real-model evidence precede every staged release; a docs CI is not AI acceptance.

## D-063 - Mirror confirmed source absence, not indefinite stale file cards

2026-10-18, Issue132 and IndexCore Issue113. The user deleted an entire gallery
directory. Forced parent listing succeeded without that directory, while four
canonical children remained PRESENT with zero missing evidence. No recurring
Root watch or accepted COMPLETE scan existed. Waiting24h could never converge.

IndexCore owns a new explicit opt-in direct-child coverage contract, with fresh
complete list/namespace/config/generation validation and pure Kernel subtree
absence decisions. It is not a whole-root COMPLETE claim or a change making
ordinary PARTIAL omissions destructive. Validated mirror absence does not wait
the unrelated24-hour whole-root grace. Tombstones and Q8 commit atomically in
the existing coordinator; no direct canonical UPDATE or physical drive deletion.
Existing bounded executor/watch/CAS handles recurring verification; no new engine.

The library and downloads share GET-only quiet reload of a complete temporary
catalog. Visible library polls every10s; hidden/signed-out/busy requests do not
overlap, errors/partial results never replace real cards with empty success, and
late responses cannot cross logout/manual refresh. Preserve player/details,
category/search/layout, download/share behavior and login. The product does not
infer existence by reading OpenList itself. Existing deep-tree bounds/cadence
mean this is eventual synchronization, not an unconditional30s service promise.

Keep AI disabled/planning-only. Deploy only the owned IndexCore/product after
isolated PostgreSQL/HTTP/UI tests, exact-head CI, explicit same-agent review and
protected binary/config/database backup. Preserve original provider references.
Actual API/Q8/tombstone evidence, not this document, determines acceptance.

2026-10-18 live acceptance: IndexCore code efd3cdca8d64 (CI39100197664 SUCCESS)
produced10 gallery tombstones and10 Q8 removal events in one generation. Four
image Q5 queries and the authenticated public resource inventory contain no
gallery PRESENT resources;276 files survive. Product d337ebf (CI39098778407 all3
SUCCESS,57 UI tests) is healthy with no-store quiet-refresh assets. Windows
archive CRLF initially broke all15 embedded migration checksums; rollout was
reverted, then explicit LF packaging matched existing history and passed readonly
preflight/health/public checks. Pin SQL LF with .gitattributes and exercise an
autocrlf=true archive regression. No migration/history rewrite or provider start.

## D-064 - Close only a confirmed removed or replaced selected file

**Status:** Accepted for Issue136, 2026-10-19.

IndexCore remains the only existence authority. A complete, successful current
canonical refresh may remove cards and must also close the selected detail,
player and pending preview/share callbacks if the exact Binding/resource/path
tuple is no longer present. A same-path new resource is not the old identity.
Compare the full surface inventory, not search/category/page-filtered results.
Do not interrupt surviving playback. Partial, failed, bounded/incomplete and
late refreshes cannot claim deletion or close an existing preview. Both file
surfaces share this controller; no direct OpenList reads or source mutations.
Backend deletion recovery and topology safety belong to IndexCore115/PR119.

## D-065 — Private CloudSite user folders, server-side ownership and no per-user storage quota (PROPOSED)

2026-10-20; human-requested blueprint in [Issue #138](https://github.com/nathanxiangang-web/cloudsite/issues/138), see [full design](PRODUCT-PRIVATE-CLOUD-SPACES.md). **Design proposed for review, NOT an implemented or deployed capability.** A single shared 115 account will provision a real directory for each verified CloudSite `user_id`, labeled with normalized login plus immutable UUID, while the product enforces private read/play/302/share/delete boundaries **on the server**, not just in the existing D-059 browser surface. Existing IndexCore remains Canonical/Q8 authority; provider mutations use narrow gated ports and current trusted Hint/observation. Legacy unowned files are not silently claimed or moved; original shared library behavior remains unchanged.

The user explicitly canceled *per-user storage capacity*: there will be no individual 520G quota, used-space meters, byte reservations, storage-size limits or per-user capacity enforcement. The earlier 520G is only a **global 115 account planning reference**, not a measured total or hard admission gate. Pinned `115driver v1.3.5` supports `Pan115Client.GetInfo()` returning global actual total/use/remain bytes, plus provider directory/file operations; API capability verified by reading SDK source **without live account access**. Keep existing operation-count/concurrency quotas from D-060.

Implementation order: (A) append-only user-space ownership/mapping state and mock provider contracts; (B) backend cross-user ACL including Q4/Q5/Q6/content/preview/302/anonymous share redemption **before enabling new private downloads**; (C) server-frozen native 115 saveDirID and canonical UserSpace target, original Worker/Manifest semantics and same-info_hash conflict handling; (D) independently authorized one-file physical deletion with native file/parent proof, durable idempotent ledger and IndexCore Q8 confirmation; (E) staged multi-user evidence and rollout. No live mkdir, file delete, migration, source move, provider restart, new acquisition or deploy was performed for this decision. Each implementation gate needs its own Issue/branch/PR and review before merging.

## D-066 — Website admin management console UI & functional blueprint (PROPOSED; NOT IMPLEMENTED)

2026-10-20, [Issue #141](https://github.com/nathanxiangang-web/cloudsite/issues/141): user explicitly deferred private UserSpace / D-065 Stage A coding until later; [#139](https://github.com/nathanxiangang-web/cloudsite/issues/139) execution authorization is **PAUSED/SUSPENDED** and prior approval to start is superseded. Preserve #138/#139 and all D-065 design, but perform **website management UI and feature blueprint first**. No UserSpace code, migrations, provider calls or private deletion authorized.

Proposed [management PRD/UI blueprint](PRODUCT-ADMIN-CONSOLE-BLUEPRINT.md) and [offline click-through static design](design/CLOUDSITE-ADMIN-UI-PROTOTYPE.html): side navigation overview/users/resources/downloads/storage/IndexCore/shares/audit/settings, responsive Chinese SaaS console, provenance/last-observed data and UNKNOWN/NOT_TRACKED, redacted task incident drilldown and no dangerous action without a separate gate. Existing `/admin/` enforces ADMIN only for workspace discovery, but displays **admin's own** tasks and standard resource views; it is not yet an all-user dashboard. Backend admin-wide list/summary, provider health, audit and mutation operations are **proposed, not existing**. The static concept uses invented `DEMO` data and no HTTP calls.

Keep user role `USER/ADMIN` with server-side per-action authorization for future sensitive writes, existing authentication and Origin/Host protections. IndexCore owns Canonical/Q8; native115, OpenList, website session and storage bindings remain distinct. Do not expose source URLs/credentials/native provider IDs/CDN tokens. User **has rejected all per-user GB capacity/quota functions**; preserve existing operation-count and concurrency policy (D-060). Any real admin UI implementation requires a separate, bounded Issue/PR and exact-head CI; no production work or live commands follows from design documents.
