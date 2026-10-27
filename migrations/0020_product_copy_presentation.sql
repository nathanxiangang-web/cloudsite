-- Product-owned overlay only. Copy identity and availability remain projector-owned.
CREATE TABLE product_copy_presentation (
 copy_id uuid PRIMARY KEY REFERENCES copies(copy_id),
 binding_id uuid NOT NULL REFERENCES storage_bindings(storage_binding_id),
 display_name text NOT NULL CHECK (octet_length(display_name)<=640 AND char_length(display_name)<=160),
 category text NOT NULL CHECK (category IN ('AUTO','SOFTWARE','VIDEO','DOCUMENT','IMAGE','AUDIO','ARCHIVE','OTHER')),
 hidden boolean NOT NULL,
 admin_version bigint NOT NULL CHECK (admin_version BETWEEN 1 AND 9007199254740991),
 updated_at timestamptz NOT NULL,
 updated_by uuid NOT NULL REFERENCES product_users(user_id)
);
ALTER TABLE product_admin_actions DROP CONSTRAINT product_admin_actions_action_check;
ALTER TABLE product_admin_actions ADD CONSTRAINT product_admin_actions_action_check
 CHECK (action IN ('CREATE_USER','SET_USER_ENABLED','PROBE_115',
 'RENAME_USER','RESET_USER_PASSWORD','DELETE_USER','RESTORE_USER','REVOKE_SHARE',
 'CANCEL_PENDING_MANIFEST','SET_COPY_PRESENTATION'));
