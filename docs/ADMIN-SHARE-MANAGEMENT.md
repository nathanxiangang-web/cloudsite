# Admin share management - Issue155 / PR-D1

This slice implements backend share listing and audited revocation. It does not
implement the administrator shares UI or certify a production schema upgrade.

## Module boundaries

- `adminread.ShareReads` owns administrator authorization, bounded query/response
  validation and the safe share directory DTO. `ShareReader` is a replaceable
  persistence port; list totals are database counts, not page estimates.
- `adminops.ShareApplier` is an additive write port, separate from USER lifecycle
  mutation. The existing operation service owns request IDs, intent digests,
  unknown-result behavior and terminal audit receipts.
- PostgreSQL adapters own repeatable-read listing and row-locked revocation.
  They reuse fresh administrator/session checks and the locked action ledger.
- `producthttp` owns same-origin command admission, strict JSON/query decoding,
  safe error codes and no-store responses. Runtime composition uses the existing
  admin services/repositories; no provider or IndexCore operation is involved.

## API contracts

`GET /api/v1/admin/shares` accepts `limit` (1..50), UUID `cursor`, UUID `owner`
and `status` (`ALL`, `ACTIVE`, `REVOKED`, `EXPIRED`). Count and keyset page use
one repeatable-read snapshot and transaction-start expiry evaluation. The scope
is `FILTERED_PRODUCT_SHARES`; read time is explicit. Empty/failed reads differ.
The DTO includes file metadata and an `active` flag, not token/code hashes or a
redeemable secret. Inactive alone does not distinguish revoked from expired.

`POST /api/v1/admin/shares/{share_id}/revoke` accepts only `request_id`.
An authenticated ADMIN and matching Origin are required. The normal successful
receipt is `SUCCEEDED / SHARE_REVOKED`; an already revoked or missing target is
`REJECTED / SHARE_ALREADY_REVOKED` or `REJECTED / SHARE_NOT_FOUND`. Repeating the
same actor/request/intent returns its original outcome. A mismatched intent is
rejected; a reserved request with no committed result remains UNKNOWN and is
never blindly executed a second time. Unsupported explicit Accept headers are
rejected; an absent Accept retains the existing handler's JSON default.

## Transactions and data preservation

The apply transaction revalidates and locks the administrator/session, locks the
reserved intent, locks the target share, stamps database-time `revoked_at`, and
commits the terminal audit result atomically. Failure rolls back the share and
result while retaining the separately durable intent. Owner revoke/grant issue
use the same share-row serialization. Existing secret/grant read paths require
an unrevoked parent, so revocation denies subsequent reads/redemptions; it cannot
recall an already issued CDN URL or already transferred bytes.

Append-only migration18 extends only the admin action CHECK allowlist. Migrations
1..17 and existing users, sessions, provider reservations, physical files,
Canonical resources and Journal history are not rewritten. Production migration
and rollback validation remain a separate release task.

## Tests and extension rules

HTTP tests check anonymous/USER/Origin/query/body/media boundaries and exact
command/receipt identity. PostgreSQL tests cover owner/status filters, aggregate
counts, keyset completeness, revoke/replay/history, old/new grant denial,
transaction rollback, changed intent and session revocation before apply.
Shared architecture, race, migration-byte and frontend suites remain required.

Future UI must bind the exact share ID and receipt action/status/code, confirm
revocation explicitly, query uncertain outcomes instead of re-posting, render
literal metadata, and fence stale responses by session and selected target.
Do not widen this port into generic SQL, credentials, native deletion, provider
downloads or IndexCore mutations. A management UI is a separate reviewable slice.
