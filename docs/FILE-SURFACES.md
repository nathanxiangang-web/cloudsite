# D-059: separate cloud-download file surface

## Responsibilities and dependencies

- Authenticated workspace supplies a read-only canonical view descriptor for the
  configured cloud target. It never accepts a target from the consumer.
- FileSurface is a pure Binding/path partition policy. The exact target subtree
  belongs to downloads; other resources belong to the main library. Sibling
  prefixes and different Bindings must not be mistaken for descendants.
- One shared HTML fragment and existing file controller serve both pages. They
  retain search, classification, cards/list, cursor loading, preview/player,
  trusted attachment link and share/revoke behavior. No duplicated player or
  download implementation. Source submission/task reconciliation stay separate.
- IndexCore Q6 remains the read authority; Q5 and existing content/share services
  still authorize every file operation. Filtering is presentation, NOT a new
  authorization boundary. Tasks remain owner-only; files retain existing shared
  configured CloudSite visibility. No provider list, new writer or schema change.

## UI behavior and extension constraints

/downloads/ shows files under the configured target including nested torrent
contents. Main / excludes that same subtree before search/count/pagination.
READY links stay on this page. Only task-state changes to a new READY result
trigger automatic inventory refresh; all refreshes are GET only. Failures stay
visible and never become fabricated empty/complete success. Stop/expiry clears
both controllers and invalidates late responses. Preview and playback must not
be interrupted on each ordinary task poll. Existing arbitrary resource API
browsing is unchanged; no file move/delete, new acquisition or historical retry.

Future surfaces reuse the controller/policy rather than copy content actions.
Issue136/D-064: preserve details only while the exact Binding/resource/path
identity still exists in the complete refreshed surface inventory. Confirmed
removal/replacement closes the stale player/detail and invalidates callbacks;
failed/partial/late reads never do. Search/category filtering is not absence.
Canonical paths use exact slash boundaries, no decoding, case folding or provider
native IDs. Cursor iteration continues through excluded records without changing
the upstream cursor; category counts reflect only the active surface.

## Validation and deployment

Oct20 at02:15Z: runtimebf89f5a deployed, CI39261559684 SUCCESS,60 Node tests
PASS; live served asset matches source. Authenticated public inventory excludes
confirmed deleted legacy/cloud files and gallery, preserves274 remaining files;
IndexCore three fresh trusted scans and Q8 prove backend convergence. DOM tests
cover both surfaces/player cleanup and failed/partial retention; live browser
playback was not separately replayed. No provider starts/schema changes.

Boundary and controller tests, shared rendered markup/unique IDs, expiry/racing
reads, pagination/late READY, and preview/download/share regressions precede CI.
Product-only rollout preserves accepted schema15/config/graph and one serial
Worker. Keep the old image/compose; stop/join before replacing product. Use only
existing real task/files for live verification, zero new StartDownload.
