-- Owned admission ledger. No legacy ownership adoption or provider side effect.
CREATE TABLE product_operation_quotas (
    user_id uuid PRIMARY KEY REFERENCES product_users(user_id),
    operation_limit integer NOT NULL CHECK (operation_limit BETWEEN 0 AND 1000000),
    concurrency_limit integer NOT NULL CHECK (concurrency_limit BETWEEN 1 AND 1000)
);
CREATE TABLE product_operations (
    user_id uuid NOT NULL REFERENCES product_operation_quotas(user_id),
    operation_key text NOT NULL CHECK (operation_key ~ '^[A-Za-z0-9._-]{1,64}$'),
    intent_digest bytea NOT NULL CHECK (octet_length(intent_digest) = 32),
    original_source text NOT NULL CHECK (octet_length(original_source) BETWEEN 1 AND 65536),
    execution_source text NOT NULL CHECK (octet_length(execution_source) BETWEEN 1 AND 4096),
    source_policy text NOT NULL CHECK (source_policy IN ('source-exact-v1','magnet-btih-name-v1')),
    manifest_id uuid NOT NULL UNIQUE REFERENCES acquisition_manifests(manifest_id),
    reserved_job_id uuid NOT NULL UNIQUE,
    max_attempts integer NOT NULL CHECK (max_attempts = 5),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, operation_key)
);
