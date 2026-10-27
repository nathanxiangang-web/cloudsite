-- Append-only action allowlist extension. No acquisition/provider schema change.
ALTER TABLE product_admin_actions DROP CONSTRAINT product_admin_actions_action_check;
ALTER TABLE product_admin_actions ADD CONSTRAINT product_admin_actions_action_check
 CHECK (action IN ('CREATE_USER','SET_USER_ENABLED','PROBE_115',
 'RENAME_USER','RESET_USER_PASSWORD','DELETE_USER','RESTORE_USER','REVOKE_SHARE',
 'CANCEL_PENDING_MANIFEST'));
