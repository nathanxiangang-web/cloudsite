CREATE TABLE product_file_shares (
 share_id uuid PRIMARY KEY,
 owner_id uuid NOT NULL REFERENCES product_users(user_id),
 binding_id uuid NOT NULL REFERENCES storage_bindings(storage_binding_id),
 resource_id text NOT NULL CHECK (length(resource_id) BETWEEN 1 AND 512),
 canonical_path text NOT NULL CHECK (length(canonical_path) BETWEEN 1 AND 2048),
 file_name text NOT NULL,
 size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
 token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash)=32),
 code_hash bytea NULL CHECK (code_hash IS NULL OR octet_length(code_hash)=60),
 created_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL CHECK (expires_at>created_at AND expires_at<=created_at+interval '7 days'),
 revoked_at timestamptz NULL
);
CREATE INDEX product_file_shares_owner ON product_file_shares(owner_id,created_at DESC);
CREATE TABLE product_file_share_grants (
 grant_hash bytea PRIMARY KEY CHECK (octet_length(grant_hash)=32),
 share_id uuid NOT NULL REFERENCES product_file_shares(share_id),
 created_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL CHECK (expires_at>created_at)
);
CREATE INDEX product_file_share_grants_share ON product_file_share_grants(share_id);
