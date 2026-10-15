# Gate 4.1 - Control Plane Domain Architecture

Status: **DOMAIN OWNERSHIP ACCEPTED under D-042; implementation incomplete**

Tracking: [Issue #75](https://github.com/nathanxiangang-web/cloudsite/issues/75).
Source baseline: `3f6d9cc20dbee8aa7cde02dcbfc89cfcd9071e71`.
This document describes design, not working auth/API/UI, provider access, audit
or quota. D-042 authorizes staged implementation under the separate execution
plan, not fictional capabilities.
The design baseline is schema v12 and the accepted Gate 3.18 runtime is
unchanged. Issue #77 subsequently implements the [auth module](PRODUCT-AUTH.md)
with schema head v13 in isolated tests; statements below about missing auth
describe the pinned design baseline, not this later implementation.

## 1. Product scope and terminology

[MVP Gate 4](MVP-BLUEPRINT.md#gate-4--login-access-share-usage) still requires
minimal auth, usage ledger/quota, access gateway and the first 115 Share adapter.
The [PR #73 Control Plane guidance](GATE4-BLUEPRINT.md) supports those behaviors;
an observation console cannot substitute for their acceptance criteria.

Here, **product control plane** means authenticated application commands,
policy and usage decisions over the existing domains. **Operator observation**
means protected, redacted reads of existing facts; **operator mutation** means
separately authorized application-service commands. None owns physical truth.
These are modules in the existing modular monolith, not new microservices.

Misleading duplicated product/AI roadmaps were removed by the documentation
audit. Premium, favorites, recommendations, payment and polish are deferred.
Agent remains optional in the core MVP; never call it implemented without
evidence. Issue #74's vague skeleton is superseded by the Stage 1 contract,
not implemented by this documentation PR.

## 2. Verified baseline and missing capabilities

| Responsibility | Existing evidence | What it does NOT establish |
| --- | --- | --- |
| Logical resource identity | [Catalog](../internal/catalog/catalog.go): Asset -> Release -> Variant -> Copy, HierarchyReader, PhysicalIdentityReader | User ownership, public visibility or permission policy |
| Acquisition intent | [Manifest](../internal/acquisition/manifest.go): optional UserID, target Binding, result_copy_id, linked Job | User account, verified login or access entitlement; a nil historical UserID is not anonymous product authorization |
| Job execution | [Job model](../internal/jobs/jobs.go), [stage dispatcher](../internal/acquisition/stage_dispatch.go) | Permission to mutate a Job alone, worker liveness or billing by claim count |
| Provider credential identity | [Storage topology](../internal/storage/storage.go), [session registry](../internal/providers/session/registry.go) | Product user identity or authorization; the registry resolves Downloader sessions, not every provider capability |
| Canonical Copy projection | [Projector](../internal/projector/projector.go), [canonical finalization](../internal/acquisition/canonical.go) | A new Control Plane Copy writer or physical-presence authority |
| Resource reads | [Logical ResourceView](../internal/resourceview/resourceview.go), [physical ResourceView](../internal/resourceview/physical.go) | Authentication, ownership or an access URL; ambiguity and lineage checks are preserved |
| Provider capabilities | [Provider contracts](../internal/providers/contracts/contracts.go): StorageProvider, DownloaderProvider, ShareProvider | A concrete accepted 115 Storage/Share adapter; current accepted concrete slice is Downloader |
| Worker events | [Worker](../internal/app/acquisition_worker.go): in-process WorkerEvent sink | Persistent audit/event history or a heartbeat registry |
| IndexCore Journal | [Query contracts](../internal/integrations/indexcore/contracts.go) and Projector | Complete product command history; resource events are not login, quota or task-stage events |
| Process composition | [Application](../internal/app/app.go), [operator](../internal/platform/operator/operator.go) | A product HTTP listener; ordinary Application.Run waits for cancellation when no injected Worker exists |

The checked package tree and [migrations](../migrations) contain no product
account/session/permission store, usage ledger/quota store or durable operator
audit store. Naming a package, an ID type or a future endpoint in guidance does
not implement it. Future implementation must recheck these facts at its head.

## 3. Module ownership and proposed ports

All new names in this section are **conceptual proposals**, not Go interfaces,
tables, route contracts or permission to add them in this PR.

| Module | Sole responsibility | Proposed boundary / dependencies |
| --- | --- | --- |
| Account and Authentication | Product accounts, verified user sessions and authentication result | Authenticator port yields a validated Principal; credential mechanism and store belong to a later auth decision |
| Authorization | Decide actor/action/target access from product policy | Policy reader + authorization decision; not provider capability detection or Job-state mutation |
| Access Gateway | Resolve an authorized logical resource into an allowed access result | Authorization + Catalog/ResourceView + separately accepted access adapter; no physical scan or guess |
| Share Application | Own product share intent and coordinate create/inspect/revoke | Authorization + Usage policy + future job-backed share execution through ShareProvider; not direct SDK calls |
| Usage and Quota | Reservations, limits and idempotent product accounting | Usage/Quota ports; does not modify ClaimAttempts, FailureCount or provider retry policy |
| Task/Resource Query | Present bounded, authorized projections of persisted facts | Narrow read ports over Catalog and linked Manifest/Job/ProviderTask; missing list/snapshot ports need separate implementation |
| Audit | Durable records of actors, operations, targets and outcomes | Audit append/read ports; does not serve as a second Job engine or IndexCore Journal |
| Operator Commands | Admission for specific administrative actions | Auth/Authorization/Audit plus existing application services; not generic SQL, a second scheduler or automatic unknown-task retry |
| Transport and Composition | DTO allowlists, request validation, wiring and lifecycle | Calls application ports; adapters implement those ports; no domain dependency on HTTP, PostgreSQL or provider SDK |

No module receives the full PostgreSQL repository or provider SDK merely for
convenience. Read and command interfaces are separate. Core domain models stay
provider-neutral; one module cannot import another's concrete adapter/store.

```text
Transport / future UI / future advisory Agent
                    |
        Authentication -> Principal
                    |
          Application use case
          /         |          \
 Authorization   Usage/Quota    Audit
          \         |          /
       Existing domain-owned operations
       Catalog / Acquisition / Job ports
                    |
       Existing persistence/integration adapters

Observation remains: Storage -> OpenList -> IndexCore -> Projector -> Copy
```

## 4. Identities and conceptual relationships

| Identity | Meaning | Must not be substituted by |
| --- | --- | --- |
| Product UserID | Stable internal account identity; proposed account authority not yet implemented | Provider account ID, StorageConnectionID or client-supplied header |
| Actor | Verified user, authorized operator, or explicitly trusted internal system actor | An arbitrary role string or unauthenticated request |
| UserSession / Principal | Verified product session and request identity context | 115 Cookie, Hint bearer token or provider-session registry Binding |
| StorageConnectionID + CredentialRef | Integration configuration and protected credential binding | Product login, ownership or entitlement |
| StorageBindingID + IndexCore root/resource | Explicit storage/canonical mapping | Guessed path prefixes, user scope or provider file ID |
| CopyID | Product link to one canonical physical identity | A download URL, provider ref or newest file in a directory |
| ManifestID / JobID | Durable intent and its existing execution identity | UI-local status or a replacement control-plane task ID |

Proposed account/session association: UserAccount owns UserSession; successful
verification yields a Principal. PermissionDecision references Actor, action
and target. AccessGrant and product ShareRecord reference an authorized CopyID
and actor; provider share references remain below redacted product boundaries.
UsageReservation/UsageEntry reference actor, operation ID and explicitly chosen
accounting units. AuditEvent references actor/operation/target and safe outcome.
These relationships do not create rows, foreign keys or a new resource domain.

Guest is an unauthenticated actor category, not a stored privileged account.
Registered/premium/admin labels in draft guidance do not automatically grant
permissions. System actors need explicit admission; historic nil Manifest.UserID
does not make those tasks readable or controllable by any user. Ownership of
shared/global Catalog entries and access to historic unattributed tasks require
an explicit policy; do not backfill ownership from a provider connection.

## 5. Read, access and command boundaries

**Read:** validate request -> authenticate -> authorize target -> bounded read
port -> consistent projection -> allowlisted DTO. Resource visibility/ownership
policy must be settled before granting guest or cross-user reads. Raw source,
provider task/share refs, credentials, target paths, arbitrary LastError and Job
payload are not ordinary response fields. Product IDs are still authorized,
not inherently public. Internal clocks, lease metadata and observation time
must not be presented as a durable heartbeat or complete timeline.

**Access:** authenticate -> authorize logical target -> select an eligible
PRESENT Copy under accepted ResourceView semantics -> accepted access port.
Historical READY/result_copy_id is a stable acquisition result, not permanent
proof that a Copy remains PRESENT or that an access URL remains valid. Missing
or removed resources and unsupported access capabilities fail explicitly.
AccessGrant is a policy result, not permission to create a share behind a read.

**Acquisition command:** authenticated actor -> authorization/quota admission
-> existing submission/activation -> same Manifest + ACQUISITION Job -> existing
Worker. Source.Value remains exact. This proposed product admission layer must
not retroactively change the accepted protected operator/staging entrypoints.

**Share mutation:** authorized intent -> durable job/command admission ->
separately accepted share execution -> ShareProvider -> outcome + audit/usage.
Inspect/read differs from create/revoke. No share Job type or atomic linkage
exists yet; future implementation must specify it, not reuse ACQUISITION or
generic Job-only transitions as a shortcut. Revoke does not delete Copy/files.

**Operator recovery/cancel:** authorization is necessary but cannot bypass the
existing operation's stronger exact-pair/reference/CAS/database-time fences.
There is no generic public resume or acquisition cancellation authority. Eight
unknown START_RESERVED records stay frozen; no policy grants a second start.

## 6. Failure, accounting and evidence semantics

- Unwired auth, missing Principal, invalid credentials or ambiguous policy
  fails closed; never synthesize a user from a Provider credential or client ID.
- Transport distinguishes unauthenticated, denied, unsupported, unavailable,
  invalid input and conflicting durable state. HTTP codes/DTOs must be fixed in
  a later bounded contract, not guessed by the Worker from this domain document.
- Project DB/IndexCore failure is unavailable/degraded evidence, not invented
  absence, presence, success or a healthy Worker. No fallback direct DB/OpenList
  probe may create canonical truth.
- Future mutation admission needs durable operation identity before side
  effects. An unknown provider outcome remains uncertainty, not failure proof
  permitting recreate. Audit failure cannot be hidden as a successful command.
- Quota reservation/settlement/release must be idempotent for one product
  operation. Polls and claims are not separate billable operations. Units,
  charging point, cancellation/refund and atomic reservation/enqueue semantics
  require explicit decisions before migrations or execution; this document
  does not refund an uncertain reservation or invent usage from task progress.
- Audit records intention, result and uncertainty with verified actor, target,
  operation ID and trustworthy observation/commit time. It excludes secrets and
  raw response/error bodies. In-process logs and IndexCore Journal cannot be
  relabelled as a durable complete Audit ledger.

## 7. Extension rules and independent acceptance tests

Each later module needs one bounded Issue, reviewed ports/DTOs, explicit failure
semantics and one narrow PR. No provider implementation imports into domain,
general workflow engine, distributed queue, new scheduler or microservice split.
Use fake auth/policy/usage/audit/provider implementations only in isolated tests;
an unwired deployed component must not silently select those fakes.

Future contract tests must prove:

1. Product user identity cannot be asserted through Cookie/Hint token/Connection
   or request UserID/role fields; unwired/invalid/expired auth denies admission.
2. Guest, own-user, other-user, operator and explicit system actor policies;
   denied requests make zero provider/mutation calls and leak no sensitive DTO.
3. Read snapshots never fabricate timeline/heartbeat; absence differs from
   unavailable; pagination/snapshot boundaries and provenance are explicit.
4. Same operation replay does not enqueue, reserve quota, charge or mutate twice;
   crashes preserve operation identity and uncertain outcomes without refunds.
5. Stale owner/generation/expired DB-time lease fails paired mutation; no lone
   Job write, direct Copy write or extra StartDownload.
6. Audit and usage failures before/after side effects have explicit recoverable
   outcomes; provider-neutral mock and unsupported capabilities remain usable.
7. Access uses authorized logical identity and current eligibility; share
   create/revoke uses tracked execution; no direct acquisition OpenList probe.

These are design acceptance obligations, **not tests added or already passed**.
This documentation PR instead checks sources, links, scope and CI, with no live
provider call, production connection, schema migration or service modification.

## 8. Decisions still owned by the Architect

Before implementation authorize one slice and settle its required choices:

- real account/session mechanism, storage lifecycle and revocation semantics;
- ownership/visibility policy and distinct operator/system admission;
- access mechanism, share command/job protocol and capability binding;
- usage units, charging/settlement point and enqueue/reservation transaction;
- durable audit admission/outcome protocol and retention/redaction;
- snapshot/list read ports and DTOs, without claiming missing runtime evidence;
- exact product Gate 4 sequencing versus optional console/AI guidance.

The delegated Architect resolves these choices per slice in
[the MVP plan](GATE4-IMPLEMENTATION-PLAN.md), starting with the frozen auth/session
contract. This domain document does not itself implement any module.
