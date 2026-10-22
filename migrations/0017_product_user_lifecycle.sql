-- Product account tombstones preserve immutable ownership and all history.
ALTER TABLE product_users
 ADD COLUMN deleted_at timestamptz,
 ADD COLUMN deleted_by uuid REFERENCES product_users(user_id),
 ADD CONSTRAINT product_users_deleted_pair CHECK ((deleted_at IS NULL) = (deleted_by IS NULL)),
 ADD CONSTRAINT product_users_deleted_disabled CHECK (deleted_at IS NULL OR NOT enabled);

ALTER TABLE product_admin_actions DROP CONSTRAINT product_admin_actions_action_check;
ALTER TABLE product_admin_actions ADD CONSTRAINT product_admin_actions_action_check
 CHECK (action IN ('CREATE_USER','SET_USER_ENABLED','PROBE_115',
 'RENAME_USER','RESET_USER_PASSWORD','DELETE_USER','RESTORE_USER'));
CREATE INDEX product_users_lifecycle ON product_users(deleted_at,enabled,user_id);
