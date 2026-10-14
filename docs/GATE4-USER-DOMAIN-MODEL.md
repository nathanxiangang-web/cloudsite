# Product identity: verified facts and approved target

Status: Design index, NOT implemented user persistence.

Verified baseline: [Manifest](../internal/acquisition/manifest.go) has nullable
UserID attribution; [StorageConnection](../internal/storage/storage.go) and
[provider sessions](../internal/providers/session/registry.go) identify integration
credentials. Neither supplies product accounts, password checks or user sessions.
The twelve current migrations contain no product users/session/usage/audit store.

The D-042 Stage 1 target is operator-provisioned product accounts with immutable
UUID, USER/ADMIN roles, bcrypt passwords, enabled status and revocable hashed
opaque sessions. Read [the frozen contract](GATE4-IMPLEMENTATION-PLAN.md#stage-1-frozen-product-auth-contract)
before coding. These are design choices, not existing tables or permissions.

Catalog continues to own Asset/Release/Variant/Copy; IndexCore owns physical
truth. Product ownership and authorization cannot be guessed from storage paths,
Provider accounts, connection IDs, client role headers or historic nil UserID.

Premium, favorites, AI conversation storage and product-history tables are not
current MVP models. Proposed domain relationships and later policy choices live
in [CONTROL-PLANE-DOMAIN](CONTROL-PLANE-DOMAIN.md), not a parallel invented schema.
