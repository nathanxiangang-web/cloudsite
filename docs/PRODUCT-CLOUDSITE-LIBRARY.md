# Complete CloudSite library and four-character share codes

Human correction, 2026-10-15: automatically fill a random4-character A-Z/a-z/0-9
share code; clearing it means no code. Show ALL CloudSite files in the consumer
UI. Software/video/document categories retain extension rules; all other formats
(including images/audio/archives/unknown extensions) belong to Other.

## Boundaries and modules

- Sharing domain: new codes are exactly4 alphanumeric characters or empty;
  existing6-32-character hashed codes remain redeemable. Keep token entropy,
  bcrypt, separate grants, expiry/revoke and bounded guesses unchanged.
- Frontend sharing: browser cryptographic randomness, unbiased62-symbol sampling;
  populate only on opening a new form, never refill a deliberately cleared input.
  Show/copy the code with the created link, clear secret UI state on close/logout.
- IndexCore operator: existing OpenList user base_path is `/115open/CloudSite`.
  Its logical `/` is therefore the requested CloudSite tree, not whole115 drive.
  Create ONE NEW Root for that `/` in isolated `indexcore_panta_mvp_catalog_20261014`.
  Serial one-shot existing CLI scan, bounded30min, refresh=false, no Hint/daemon,
  download, source upload/delete or original DB change. No scanner reimplementation.
- New observation-only DISABLED Connection/Binding uses existing product ports,
  no credential_ref/provider_scope. Keep the three old bindings readable for old
  shares, but consumer selects only the new CloudSite binding to avoid duplicates.
- Query/UI: explicit whole-root mode uses existing IndexCore Q6 `/active`, not
  direct OpenList or IndexCore DB. Existing Q4 hierarchy/admin behavior unchanged.
  Q6 generation-bound opaque pagination, identity/presence validation and safe
  cancellation remain. No32-page/256-directory permanent truncation in this mode;
  bounded batches and explicit continuation can reach every canonical file.
  Directories are internal metadata, never consumer file cards. Counts disclose
  whether more pages remain. No fabricated files/covers or deletion feature.

## Verification and rollout

Freeze contracts before implementation. Unit/HTTP/DOM tests cover4-character
generation/clear/no-code/legacy redemption, Q6 route and hierarchy isolation,
pagination beyond former cap, cancellation, and Other classification. All3
exact-head CI groups, then separate same-agent review and scoped product deploy.
Source scan and Q4/Q5/Q8 evidence recorded privately, public counts only. A failed
scan never switches the UI or invents success. Confirm complete canonical file
count matches fully paged Q6 and UI traversal. No original source file changes.

Owned product DB stays14. Existing protected credentials reused; no login/role
change. Product config allows four read bindings, one consumer selection and
explicit whole-root mode. Guard validates count/selection rather than weakening
DSN/loopback/binary checks. Retain old receipt/config/binary for rollback; no
database removal, historical task changes or unreviewed Issue83 integration.

## Actual source result before code review

The one-shot full CloudSite scan completed successfully (generation1) on
2026-10-15. Fully paged Q6 contains283 files and474 directories. Classification
by the unchanged extension sets:42 software,188 video,6 documents,47 Other.
Q5 uniquely resolves the real test document; Q8 first page has100 added events.
These are actual existing source files, not generated fixtures. Existing root
generations0/1/1 remain. Source files, original databases and provider tasks are
not rewritten. New product mapping and formal UI cutover still require code
review/exact-head CI and deployment evidence, not inferred from scan completion.

## Accepted and deployed

Issue #94 / PR #95 exact HEAD `e5961f8fe0603eb8e2ee0c862abfbdd68d79c6b2`;
CI `38733576527` all three SUCCESS. Same-agent review `5533047604`, not an
independent audit; squash merge `9c902a3650df17c8202e3d04985a20c562389087`;
merge CI `38734637547` all three SUCCESS. Full local Go/race/vet/build,
27 Node,11 Python and isolated real PostgreSQL sharing regression pass.

Candidate AND formal8180 fully paged16 authenticated requests, exactly matching
283 IndexCore file IDs +474 directories. Browser shows283/42/188/6/47 counts,
120/120/43 cards across three pages, Other labels and no directory cards.
Actual random4-character code, clear/no-code redemption, protected redemption
and copying original code verified. Real DOCX bytes in both new-code modes and
legacy eight-character old-binary/Binding share match SHA256
`2bc8090eca36718b3c682ba66f9b3f1a97b88be0d850a1f7c55f1ffedf298925`.
Wrong code and revoked downloads denied. Formal document text preview works.
All temporary test links revoked, candidate stopped, private evidence retained.

Only owned product service restarted. Binary SHA256
`983e10735db46b94cfd5d404372e8e97c788bfd05ff8aa58d832e4348d287f67`.
Schema14 unchanged; four read Bindings/one consumer selection. New mapping is
DISABLED with no provider_scope/credential_ref. Query PID, all four Root
generations, original staging whole-table hashes and unrelated containers stayed
unchanged during cutover. Preserve old14 config/receipt/launcher/binary for
rollback; no DB restore needed. No source-file mutation/native115 call/login,
new provider task, additional scan or Hint. Old Issue83 WIP untouched.
