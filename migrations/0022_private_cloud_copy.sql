CREATE TABLE cloud_user_spaces (
    space_id uuid PRIMARY KEY,
    owner_user_id uuid NOT NULL,
    binding_id uuid NOT NULL REFERENCES storage_bindings(storage_binding_id),
    provider_connection_id uuid NOT NULL REFERENCES storage_connections(storage_connection_id),
    indexcore_root_id text NOT NULL CHECK (btrim(indexcore_root_id) <> ''),
    native_parent_id text NOT NULL CHECK (btrim(native_parent_id) <> ''),
    native_dir_id text NULL CHECK (native_dir_id IS NULL OR btrim(native_dir_id) <> ''),
    canonical_prefix text NOT NULL,
    directory_name text NOT NULL CHECK (
        btrim(directory_name) <> '' AND char_length(directory_name) <= 512
        AND position('/' IN directory_name) = 0 AND position(E'\\' IN directory_name) = 0
        AND directory_name NOT IN ('.', '..')
    ),
    status text NOT NULL CHECK (status IN (
        'NOT_PROVISIONED', 'PROVISIONING', 'ACTIVE', 'RECONCILE_REQUIRED', 'BLOCKED', 'UNAVAILABLE'
    )),
    owner_state text NOT NULL CHECK (owner_state IN ('ACTIVE', 'OWNER_DELETED')),
    provisioning_key text NOT NULL CHECK (char_length(provisioning_key) BETWEEN 1 AND 128),
    version bigint NOT NULL CHECK (version >= 0),
    last_verified_at timestamptz NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT cloud_user_spaces_prefix_normalized CHECK (
        char_length(canonical_prefix) <= 2048
        AND left(canonical_prefix, 1) = '/' AND right(canonical_prefix, 1) <> '/'
        AND position('//' IN canonical_prefix) = 0
        AND position(E'\\' IN canonical_prefix) = 0
        AND canonical_prefix !~ '(^|/)\.{1,2}(/|$)'
    ),
    CONSTRAINT cloud_user_spaces_active_directory CHECK (status <> 'ACTIVE' OR native_dir_id IS NOT NULL),
    CONSTRAINT cloud_user_spaces_owner_binding_unique UNIQUE (owner_user_id, binding_id),
    CONSTRAINT cloud_user_spaces_binding_prefix_unique UNIQUE (binding_id, canonical_prefix)
);
CREATE UNIQUE INDEX cloud_user_spaces_connection_native_dir_unique
    ON cloud_user_spaces(provider_connection_id, native_dir_id) WHERE native_dir_id IS NOT NULL;

ALTER TABLE acquisition_manifests
    ADD COLUMN user_space_id uuid NULL REFERENCES cloud_user_spaces(space_id),
    ADD COLUMN target_provider_connection_id uuid NULL REFERENCES storage_connections(storage_connection_id),
    ADD COLUMN target_provider_scope text NULL,
    ADD CONSTRAINT acquisition_manifests_private_target_shape CHECK (
        (user_space_id IS NULL AND target_provider_connection_id IS NULL AND target_provider_scope IS NULL)
        OR (
            user_space_id IS NOT NULL AND target_provider_connection_id IS NOT NULL
            AND target_provider_scope IS NOT NULL AND btrim(target_provider_scope) <> ''
        )
    );

CREATE TABLE acquisition_result_object_evidence (
    manifest_id uuid PRIMARY KEY REFERENCES acquisition_manifests(manifest_id),
    provider_connection_id uuid NOT NULL REFERENCES storage_connections(storage_connection_id),
    provider_object_ref text NOT NULL CHECK (btrim(provider_object_ref) <> ''),
    provider_parent_scope text NOT NULL CHECK (btrim(provider_parent_scope) <> ''),
    result_name text NOT NULL CHECK (
        btrim(result_name) <> '' AND char_length(result_name) <= 512
        AND position('/' IN result_name) = 0 AND position(E'\\' IN result_name) = 0
        AND result_name NOT IN ('.', '..')
    ),
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    hash_algorithm text NOT NULL CHECK (hash_algorithm = 'SHA1'),
    content_hash text NOT NULL CHECK (content_hash ~ '^[0-9A-Fa-f]{40}$'),
    observed_at timestamptz NOT NULL,
    CONSTRAINT acquisition_result_object_unique UNIQUE (provider_connection_id, provider_object_ref)
);

CREATE TABLE acquisition_copy_operations (
    copy_operation_id uuid PRIMARY KEY,
    manifest_id uuid NOT NULL UNIQUE REFERENCES acquisition_manifests(manifest_id),
    job_id uuid NOT NULL UNIQUE REFERENCES jobs(job_id),
    donor_manifest_id uuid NOT NULL REFERENCES acquisition_result_object_evidence(manifest_id),
    provider_connection_id uuid NOT NULL REFERENCES storage_connections(storage_connection_id),
    request_digest bytea NOT NULL CHECK (octet_length(request_digest) = 32),
    destination_before jsonb NOT NULL CHECK (jsonb_typeof(destination_before) = 'array'),
    status text NOT NULL CHECK (status IN (
        'COPY_RESERVED', 'PROVIDER_ACCEPTED', 'PROVIDER_CONFIRMED', 'PROVIDER_UNKNOWN', 'FAILED'
    )),
    provider_result_ref text NULL CHECK (provider_result_ref IS NULL OR btrim(provider_result_ref) <> ''),
    last_error_kind text NULL CHECK (last_error_kind IS NULL OR last_error_kind IN (
        'AUTH_OR_PERMISSION', 'THROTTLED', 'TRANSIENT_PROVIDER', 'SCOPE_TOO_LARGE',
        'INVALID_SCOPE', 'CONFIG_INVALID', 'ROOT_INACTIVE', 'INTERNAL'
    )),
    provider_accepted_at timestamptz NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT acquisition_copy_confirmed_result CHECK (
        (status = 'PROVIDER_CONFIRMED' AND provider_result_ref IS NOT NULL)
        OR (status <> 'PROVIDER_CONFIRMED' AND provider_result_ref IS NULL)
    ),
    CONSTRAINT acquisition_copy_accepted_time CHECK (
        (status = 'COPY_RESERVED' AND provider_accepted_at IS NULL)
        OR (status = 'PROVIDER_ACCEPTED' AND provider_accepted_at IS NOT NULL)
        OR status IN ('PROVIDER_CONFIRMED', 'PROVIDER_UNKNOWN', 'FAILED')
    )
);