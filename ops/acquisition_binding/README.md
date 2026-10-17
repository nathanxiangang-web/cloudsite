# Acquisition topology operator — Issue120 / D-058

This operator owns only protected logical storage configuration. Dependencies:
operator -> bootstrap file reader + storage repository + schema checker; no HTTP,
provider SDK, acquisition services, source submission, scanner or Worker. The
runtime/public UI remain the already accepted D-055 modules.

Invocation requires `PANTA_CONFIGURE_ACQUISITION_GO=new-logical-mapping` and exactly
one absolute protected plan path. Plan contains root_id, connection_id (NEW),
binding_id, provider_scope (opaque native directory ID), credential_ref (reference
only), mount_path, database_url_file, expected_database, and optionally
previous_connection_id. Never place credential values or download sources in it.
All IDs/paths/references are validated; unknown/duplicate fields reject. DSN lives
in a separate protected file; actual database and schema must match. Errors do
not print underlying DB diagnostics or private plan values.

Without previous_connection_id create a new Connection/Binding atomically. With
it, attach the fresh native session Connection to the exact existing DISABLED
observation-only Binding. Preserve Binding/Root/mount identities and old Connection.
CAS checks previous Connection, Root/mount, nil scope and disabled state, old
credential-free observation Connection, fresh active115 Connection and absence
of acquisition history. Refuse stale/active mappings and repeat execution; do
not silently adopt or reset state. An advisory transaction lock serializes this
operator; nonempty Manifest/Job/ProviderTask/operation queue refuses before writes.

The surrounding operator transaction is mandatory for AttachObservationSession;
it rolls back the new Connection when attaching/creating the Binding fails.
No Job, Manifest, Copy, old credential, canonical identity or source file changes.
This is not a credential-validity check or authorization to start a download.

## Deployment mapping and recovery

Human-selected physical destination: /CloudSite/云下载. Verify native parent/child
ID with the current same-account session; no guessing from virtual paths. Existing
OpenList account base /115open/CloudSite exposes it as /云下载. Existing full-root
adapter path / and canonical target /云下载 agree with the Manifest/Hint scope.
Keep root-level original /云下载 data and old observation Binding as historical
configuration, without keeping that old directory as the new destination.

Use IndexCore administrative adapter API only to bind existing app credentials
without changing the path namespace. One bounded root PARTIAL Hint discovers the
new canonical directory; then child Hint verifies scope readiness. Preserve all
other canonical resource IDs; no full recursive scan or deletion-rule change.
Protected bootstrap mounts only the verified native cookie and trusted Hint token.
Graph preflight must pass before changing product action to managed mode. Keep
readonly config/compose/image backup. Rollback stops managed process, joins Worker,
restores readonly action/config and leaves all Job/reservation/mapping data intact;
do not reverse bindings or destroy evidence while a task could be in flight.

Public submit activation is not completed download acceptance. Controlled tests
plus one sealed real ordinary-source test cover separate contracts. No direct SQL
repair of task states, duplicate source attempts, historical replay or new engine.
Future providers extend existing ports; operator must stay independent of SDKs.
