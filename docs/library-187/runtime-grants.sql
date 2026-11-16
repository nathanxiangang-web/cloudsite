-- Run as the relevant table/grant owner after Migration0021.
-- psql -v runtime_role=YOUR_EXISTING_RUNTIME_ROLE -f runtime-grants.sql
-- This augments existing product permissions; it grants no provider authority.
BEGIN;
GRANT USAGE ON SCHEMA public TO :"runtime_role";
GRANT SELECT, INSERT, UPDATE ON public.product_library_snapshots TO :"runtime_role";
GRANT SELECT ON public.storage_bindings TO :"runtime_role";
-- PostgreSQL requires one UPDATE column for SELECT ... FOR SHARE.
-- updated_at avoids granting Root/status/identity-column update permission.
GRANT UPDATE (updated_at) ON public.storage_bindings TO :"runtime_role";
COMMIT;
