# Gate 4 - Product Control Layer

Status: Architecture direction accepted under D-042; product implementation incomplete.

Original MVP Gate 4 is login, access, share and usage/quota. The previous
observation-console blueprint did not define those product capabilities and is
superseded by this correction; a dashboard is not Gate 4 completion.

Read [domain ownership](CONTROL-PLANE-DOMAIN.md),
[execution plan](GATE4-IMPLEMENTATION-PLAN.md) and
[acceptance evidence rules](GATE4-ACCEPTANCE.md).

```text
Verified product user
        |
Product application + policy/usage/audit
        |
Existing Catalog / Acquisition / Job services
        |
Existing adapter boundaries / separately accepted access and share adapters
```

IndexCore owns physical truth. Journal Projector owns Copy availability.
Existing transactions own paired Manifest/Job writes and READY/result_copy_id.
Product auth/permission/usage never substitutes for DB-time lease authority.
Product identity never equals provider identity. UI/HTTP/Agent cannot directly
write a Job/Copy, invoke SDK side effects or verify acquisition via OpenList.

The initial product is private and authenticated, with operator-provisioned
USER/ADMIN accounts, owner-only task/source views and explicit shared Catalog
policy. No public signup, premium tier, recommendation or payment. Minimal UI
comes only after actual product contracts and adapters pass their tests.

No persistent heartbeat registry or complete event timeline exists today.
Lease and in-process events must not be presented as those capabilities. No
auth/usage/share module is declared implemented by this blueprint. Each stage
needs written contracts, real diff/tests, exact-head CI and a review record.
