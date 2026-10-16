# Product cloud-download admission and zero-price usage

2026-10-16, Issue83 / D-054, following D-042 and D-053. Development slice, not
public cloud-download enablement or full MVP acceptance. Original isolated dirty
Issue83 files remain untouched; selected concepts are rebased on sharing14.

## Modules and dependency direction

`product.AdmissionService -> AdmissionStore + acquisition.SubmissionService`.
The product domain owns identity, operation intent, safe DTOs and immutable target
allowlist. PostgreSQL owns atomic quota/operation/PENDING persistence. The HTTP
adapter owns strict input and the existing cookie/Host/Origin protection. Only
the accepted SubmissionService/ActivationService creates the stable queued Job.
No new queue, provider session, scanner, listener, Worker, Copy writer or frontend
activation in this PR. The existing readonly runtime does not register these new
routes. Later composition explicitly opts in only after dependencies are ready.

## Raw intent, source policy and replay

Input: operation_key (1..64 ASCII letters/digits/dot/underscore/hyphen), source,
binding_id, target_path and optional expected_name. Owner, roles, credentials,
Manifest/Job IDs, attempts, limits and source policy cannot be supplied by HTTP.
Target is normalized by existing NormalizeTargetPath. ExpectedName remains nil
or valid nonblank UTF-8 <=512bytes without NUL; magnet dn never becomes this field.
Source is private raw input <=64KiB; HTTP body<=512KiB; execution<=4096bytes.

For a NEW operation, the server freezes magnet-btih-name-v1: magnet retains
validated BTIH+optional dn, other accepted schemes remain byte-exact. Persist
original_source, execution_source, source_policy, stable random Manifest/Job IDs,
fixed5-attempt budget and length-framed raw-intent SHA256 together. Manifest uses
execution_source, thereafter unchanged all the way to DownloaderProvider.

The immutable ledger key is (user_id,operation_key), not global source dedupe.
Same key and raw intent replays stored IDs/source/policy; changed trackers still
conflict even if preparation would produce the same short magnet. Replay does
NOT re-run source preparation, consume another usage unit or require Binding to
remain ACTIVE. It validates ledger/Manifest identity and exact execution bytes;
stored execution/policy is not accepted blindly. This service does not update
ledger rows. Corruption returns uncertain rather than inventing another pair.
ADMIN follows exactly the same ownership/quota rules as USER; legacy unowned
Manifests are neither adopted nor counted. All source values remain private DB
data, excluded from response DTOs/Stringer/logs; source retention follows the
explicit D-053 operation contract, not secret storage in Job payloads.

## Transaction, limits and uncertainty

Reserve verifies the current enabled user/login/role FOR SHARE, serializes on
that user's quota row FOR UPDATE, then checks replay before capacity or topology.
New reservation locks Binding then Connection FOR SHARE, delegates target policy
to transaction-bound ManifestService, writes one PENDING Manifest plus one usage
operation and commits. Do not wrap whole SubmissionService in this transaction.
After commit, activate the stable pair through the existing submission service.

Ten lifetime operations, at most2 nonterminal operations by default; zero price,
not payments/billing. Limits are DB-owned, never request-controlled. PENDING,
ACTIVE, AWAITING_VISIBILITY, AWAITING_CANONICAL and RECOVERY_REQUIRED hold capacity.
READY/FAILED/CANCELED release concurrency, not lifetime usage. Claims/polls/replay
cost zero additional units. No automatic quota replenishment or cancellation API.
Usage uses one consistent statement for the current enabled owner's operations,
including targets later removed from a process allowlist. A new user sees defaults.

Failed Commit is not proof of rollback: a fresh bounded5-second cancellation-
detached read checks durable operation+Manifest evidence. Identical evidence can
be reused; absent/unreadable evidence returns ADMISSION_UNCERTAIN. The caller may
retry only the same key+intent. Proven PENDING activation failure returns202 with
the reserved IDs and PENDING_ACTIVATION; replay activates that same pair. Request
cancellation does not refund or create a replacement operation.

## Explicit transport, safe DTOs and extensibility

NewAdmissionHandler opens no listener. It provides authenticated, no-query routes:

- POST /api/v1/acquisitions: strict JSON; required4fields above, optional name;
  reject unknown/duplicate/null/nonstring/trailing/malformed/oversized input.
- GET /api/v1/usage: owner-only OPERATION/ZERO, limits/used/remaining/in_flight.

Submission DTO contains only ManifestID, reserved JobID and status. CREATED201,
REPLAY200, proven pending202. Static errors: INVALID_REQUEST400, AUTH_REQUIRED401,
BINDING_NOT_AVAILABLE404, OPERATION_CONFLICT409, OPERATION_LIMIT or CONCURRENCY_LIMIT
429, ADMISSION_UNAVAILABLE or ADMISSION_UNCERTAIN503. No raw causes/source/ref.
Same-origin mutation protection, exact Host and valid session remain mandatory.

Future source policies extend a versioned admission policy, not worker rewriting.
Future UI selects a server-defined target without exposing directories or scopes.
Future quota administration, lineage or paired cancellation needs a separate
documented port/PR; never add them as unvalidated JSON backdoors. Existing read,
share, preview and direct302 modules are unchanged.

## Schema and deployment boundary

Append migration15: product_operation_quotas + product_operations; no modifications
to migrations1..14, no defaults/legacy backfill. New binaries require exact15 and
keep all gap/future/checksum guards. This PR does NOT migrate public schema14 or
original staging12. A later reviewed rollout must back up the owned product DB
and prepare a15-compatible disabled-admission fallback; the old strict14 binary
cannot roll back directly onto15. Never auto-migrate on process startup.

## Acceptance

Unit and HTTP tests cover private DTOs, identity, long input/body limits, Origin,
Host, status/error categories and malformed input. Isolated real PostgreSQL tests
cover raw/execution/policy atomic persistence, exact provider execution input,
pinned replay despite topology/policy changes, cross-user/conflicting intent,
concurrent same-key races and quotas, rollback/cancellation/backend termination,
lost commit acknowledgement and PENDING activation retry. Controlled fake-provider
runtime E2E starts from actual admission through READY+result_copy_id+same Job
SUCCEEDED, verifies one provider start and one operation usage. This is NOT a
real-account download or public runtime acceptance. Full CI and a separate
same-agent review pass (not independent review) precede merge.
