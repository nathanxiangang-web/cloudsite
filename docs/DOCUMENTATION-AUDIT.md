# Documentation audit - 2026-10-14

Scope: recent Gate 4 documents, current execution pointers and runtime claims.
This records an audit, not a claim that MVP implementation is complete.

## Evidence checked

- Accepted Gate 3.18 runtime at `74e9e69`, PR #71 review/CI and real completion.
- Current main `564eee483ebb87fa5b56386aaa7a0f5050ed34fa` and PR #76.
- `git diff --name-status 74e9e69 origin/main`: only Markdown changes; despite
  placeholder/revert/checkpoint commit messages, final Go runtime is unchanged.
- Actual Catalog, Manifest, Job, provider session, Projector, ResourceView,
  app/operator packages and twelve SQL migrations.
- Original MVP Gate 4 login/access/share/usage and Gate 5/6 acceptance criteria.

## Findings and dispositions

| Finding | Evidence | Disposition |
| --- | --- | --- |
| Roadmap G4-A01/A02/A03 marked DONE | Guidance documents lack implemented accounts and a frozen method/DTO/error contract; no evidence of product acceptance | Replace DONE with an actual source-backed plan and explicit stage status |
| "API Contract" is only route prefixes and an AI intent example | GATE4-API-CONTRACT.md has no concrete request/response/error/auth semantics | Rewrite as an honest contract index; only individually frozen stage contracts authorize implementation |
| Product blueprint replaces Gate 4 with presentation/AI and adds Premium roles | Original MVP requires login/access/share/usage; code has no premium account/billing model | Delete GATE4-PRODUCT-BLUEPRINT.md; preserve original MVP and the reviewed domain model |
| Gate 4 AI document places recommendation/AI modules before product control foundations | Original MVP defers Agent execution to Gate 5 and recommendations beyond MVP | Delete GATE4-AI-ARCHITECTURE.md; existing MVP Agent boundaries remain intact |
| User-domain guidance lists product DB ownership as though entities are available | Only Manifest.UserID is present; no account/session/usage/audit tables | Rewrite as a verified capability index, not an implemented user model |
| PR #73 observation-console scope differs from latest product-control ADR | ADR at 564eee4 and original MVP restore login/access/share/usage | Correct blueprint/acceptance so a dashboard cannot substitute for product behavior |
| Current STATE/AI/AGENTS still say no authorized Gate 4 action | Latest human delegates Architect + Developer and instructs autonomous MVP delivery | Record D-042; synchronize active scope and staged acceptance, without pretending an independent human review occurred |

No proposal is called "fabricated" merely for being a proposal. The removed
documents were misleading duplicated **execution/product scope** and are not
needed to preserve any accepted invariant. Original commits remain recoverable:
`aaddf05` (product blueprint), `0ebb28a` (Gate 4 AI guidance).

## Preserved invariants

No changes to IndexCore truth, source value preservation, Job/Manifest lease
fences, Copy projector ownership, unknown START_RESERVED protection or secret
isolation. No data deletion, provider call, live database migration or deployment
is performed by this audit. Historical acceptance and failures remain in Git.

Every future "implemented/accepted" statement must link to source/tests and
actual test/CI evidence. Architecture decisions may describe a target contract,
but must not advertise it as a deployed capability.
