-- Append-only audit intent + terminal outcome. Pending intent means UNKNOWN.
ALTER TABLE product_users ADD COLUMN admin_version bigint NOT NULL DEFAULT 0 CHECK (admin_version >= 0);
CREATE TABLE product_admin_actions (
 actor_id uuid NOT NULL REFERENCES product_users(user_id),
 request_id uuid NOT NULL,
 action text NOT NULL CHECK (action IN ('CREATE_USER','SET_USER_ENABLED','PROBE_115')),
 target_id uuid,
 intent_digest bytea NOT NULL CHECK (octet_length(intent_digest)=32),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id)
);
CREATE TABLE product_admin_action_results (
 actor_id uuid NOT NULL,
 request_id uuid NOT NULL,
 outcome jsonb NOT NULL CHECK (jsonb_typeof(outcome)='object'),
 recorded_at timestamptz NOT NULL,
 PRIMARY KEY(actor_id,request_id),
 FOREIGN KEY(actor_id,request_id) REFERENCES product_admin_actions(actor_id,request_id)
);
-- Shared across processes, keyed by native product Connection (not by browser).
CREATE TABLE product_admin_probe_cooldowns (
 connection_id uuid PRIMARY KEY REFERENCES storage_connections(storage_connection_id),
 next_allowed_at timestamptz NOT NULL
);
