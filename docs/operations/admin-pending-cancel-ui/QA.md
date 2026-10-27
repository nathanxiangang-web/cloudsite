# Isolated synthetic browser QA - Issue172

All screenshots and requests use `node test/admin-preview.cjs
--pending-cancel-review --pending-cancel-uncertain`, loopback ephemeral port only.
The server uses in-memory synthetic rows and receipts; no PostgreSQL,115,
IndexCore or production access. These are real Chromium screenshots, not live
acceptance, mock image generation or deployment evidence.

| Viewport | Evidence | Verified |
| --- | --- | --- |
| 1440x1000 | fixture-desktop-confirm.png | Exact target, explicit destructive confirmation, return-first focus |
| 390x844 | fixture-mobile-confirm.png | Labeled fixture, wrapped text/UUID, visible buttons and bounded dialog |
| 390x844 | fixture-mobile-unknown.png | Simulated503, result unconfirmed, original request ID and explicit receipt GET |

Keyboard Shift-Tab from return focused confirm; unit DOM tests also cover forward
trap, Escape restoration and stale-confirmation expiry. The final mobile run
focused pending-cancel-action after submission; body scrollWidth <= innerWidth.
Task table retains its own horizontal scroll region without body overflow.

Browser network observation: exactly ONE cancel-pending POST -> synthetic503,
then manual GET /api/v1/admin/actions/{same request_id} ->200. Subsequent GETs
refreshed downloads, authoritative overview and exact detail; list/detail showed
CANCELED with no Job. No repeated cancel POST. Expected console HTTP503 only;
no unhandled script error. Invalid/auth/stale/race variants are exercised by the
DOM fixtures, not claimed as live-provider browser acceptance.
