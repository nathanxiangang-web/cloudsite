# Synthetic browser evidence - Issue174

Run `node test/admin-preview.cjs --audit-review`. Loopback only, ephemeral port,
in-memory fake audit data. No database, provider,115, IndexCore or production call.
These are real Chromium screenshots, explicitly labeled ISOLATED FIXTURE.

| Viewport | Evidence | Verification |
| --- | --- | --- |
| 1440x1100 | [Desktop list](fixture-desktop-list.png) | Own actor, action/status and full UTC date/time inputs, all three outcomes, no total claim |
| 390x844 | [Mobile evidence](fixture-mobile-detail.png) | UNKNOWN has no terminal timestamp, target redaction, wrapped IDs/code and readable details |

Real browser read flow: audit list GET, one exact evidence GET, Escape returned
focus to request99500000-0000-4000-8000-000000000004, status UNKNOWN submit reduced
the fixture page from3 to1. Body scrollWidth <= innerWidth at390px; table owns
its horizontal scroll region. Requests remained GET only; no command/replay call,
no browser script error. DOM fixtures exercise authentication, stale responses,
missing capability, malformed/foreign DTOs, microsecond ordering and read deadlines.
No live-user/production acceptance or comprehensive security audit is claimed.
