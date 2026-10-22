# 07 · Issue #153 live preflight — NO-GO, no production cutover

2026-10-22 UTC. This is a fresh authenticated **read-only preflight**, not a
cutover receipt or full production acceptance. No product stop was attempted.

## Access and verified artifacts

Password SSH authenticated as the existing privileged operator. An earlier
BatchMode failure was public-key authentication failure, not evidence that the
supplied password failed. Strict host-key validation remains enabled; only the
previously human-pinned host entry was updated, with its prior file backed up.
No safety guard was bypassed. Prior guard refusal is historical; this pass did
not test a stop command or establish that the old refusal is lifted.

- Current product: `cloudsite:deleted-preview-bf89f5a`, healthy,
  `serve-product-acquisition`, Worker enabled, tick timeout60s/lease2m.
- Product PostgreSQL schema: head15/15 rows. Five READY/SUCCEEDED pairs and
  five RECOVERY_REQUIRED pairs; no RUNNING, QUEUED or RETRY_WAIT Job observed.
- Candidate image ID:
  `sha256:caa56df2e44fdc65387f0980d931541c9af29e1ae52c0632599b699d80547cf0`.
  Embedded revision `c43408bb61722fb3d575063b8da79c04d736abdc`.
- Image binary SHA-256:
  `6686cd9184543cc33feeb8792d3ab200953e5364a7fa62fe9cef7ec64866648b`.
- Separate migrator SHA-256:
  `db43dc06031b8ea6119bc2caa173f96bbaff2ff061dd1d12ef51423dff048bea`.
  Both match `01-source-and-image.md`; no rebuild or relabel was performed.
- Product/runtime/acquisition protected files have restrictive modes; no
  credential values were printed or exported. About88GB free disk observed.
- Existing isolated dry-run containers remain retained and untouched.
- Public HTTPS returned200; main CI39728366064 independently checked3/3 SUCCESS.
  This does not prove authenticated resource,302,share,Q4/Q5/Q9 acceptance.

## Actual failed dependency gate

IndexCore/cloud-catalog reports healthy between failures, but observed Docker
restart count increased from the prior report60 to61. Its restart policy is
`unless-stopped`; OOMKilled=false. Fresh logs show repeated
`incremental_runtime_fatal`, `phase=cycle`, `error_class=executor`, followed by
runtime stop and single-writer reacquisition on restart.

Observed fatal times include04:17:47Z,04:53:31Z,05:44:26Z and06:05:53Z.
Immediately before the latest fatal, a scoped scan reported
`error_class=reconcile`; at06:05:55Z single-writer ownership was acquired again.
The fatal record contains only time/level/msg/phase/error_class, not a specific
underlying error. **This establishes repeated fatal exits, not their root cause.**
The inspected storage-token refresher is not a catalog restart mechanism.
No memory-pressure cause or periodic-maintenance explanation is established.
Do not turn the current healthy flag into sustained-stability acceptance.

Issue153 explicitly requires dependency stability before interrupting product.
This gate is **NO-GO**. IndexCore repair is outside this rollout's scope; no
IndexCore restart,configuration,queue or database change was performed.

## Machine-readable gate snapshot

```json
{
  "issue": 153,
  "result": "NO_GO_DEPENDENCY_STABILITY",
  "password_ssh": "PASS",
  "strict_host_key": "PASS",
  "production_schema": 15,
  "candidate_image_and_binary_hash": "PASS",
  "migrator_hash": "PASS",
  "legacy_history_read": "PASS_15_ROWS",
  "migration16_runtime_status": "NOT_RUN",
  "active_acquisition_jobs": 0,
  "historical_pair_count": 10,
  "disk_capacity": "PASS",
  "maintenance_window": "HUMAN_APPROVED_NOW",
  "indexcore_stability": "FAIL_REPEATED_FATAL_EXIT",
  "indexcore_underlying_cause": "UNKNOWN",
  "production_stop_safety_capability": "NOT_RETESTED",
  "writer_freeze": "NOT_RUN",
  "frozen_backup_and_fresh_restore": "NOT_RUN",
  "production_rollback_connection_switch": "NOT_VERIFIED_THIS_PASS",
  "rollback_custodian_rto_rpo": "NOT_FINALIZED_THIS_PASS",
  "protected_probe_binding_validation": "NOT_COMPLETED_THIS_PASS",
  "approved_disposable_user_identity": "NOT_FINALIZED_THIS_PASS",
  "protected_audit_export": "NOT_RUN",
  "migration": "NOT_RUN",
  "image_switch": "NOT_RUN",
  "live_115_probe": "NOT_RUN",
  "live_user_canary": "NOT_RUN",
  "authenticated_regressions": "NOT_RUN"
}
```

## Scope and next action

Only a nonsecret read-check helper was staged under host `/tmp` for this pass;
production services/configuration/data and all historical reservations remain
unchanged. No backup was exported, no retained data was deleted, no live115
request or USER command occurred. No claim of PROD_UPGRADED is made.

Resolve/review the IndexCore fatal-exit risk under its own bounded authorization
before returning to Issue153's release gate. Do not request another maintenance
clock time or password approval. Then repeat fresh live baselines and complete
all frozen-backup/restore,rollback,credential,canary and safety gates; this
preflight does not waive any of them. Issue153 stays OPEN.
