CREATE TABLE product_users (
    user_id uuid PRIMARY KEY,
    login text NOT NULL UNIQUE CHECK (login ~ '^[a-z0-9][a-z0-9._-]{2,63}$'),
    password_hash bytea NOT NULL CHECK (octet_length(password_hash) = 60),
    role text NOT NULL CHECK (role IN ('USER', 'ADMIN')),
    enabled boolean NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE product_sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id uuid NOT NULL REFERENCES product_users(user_id),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL CHECK (expires_at > created_at),
    revoked_at timestamptz NULL CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);
CREATE INDEX product_sessions_user_id ON product_sessions(user_id);
