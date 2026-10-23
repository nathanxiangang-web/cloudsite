-- Append-only: extend the audited admin action allowlist with share revocation.
-- No table/column/index change; the existing (actor_id,request_id) audit ledger
-- already carries intent and terminal outcome for REVOKE_SHARE.
ALTER TABLE product_admin_actions DROP CONSTRAINT product_admin_actions_action_check;
ALTER TABLE product_admin_actions ADD CONSTRAINT product_admin_actions_action_check
 CHECK (action IN ('CREATE_USER','SET_USER_ENABLED','PROBE_115',
 'RENAME_USER','RESET_USER_PASSWORD','DELETE_USER','RESTORE_USER','REVOKE_SHARE'));