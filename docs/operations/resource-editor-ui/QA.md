# Resource editor browser acceptance - synthetic isolated fixture

Run node test/admin-preview.cjs --resource-editor-review --resource-editor-uncertain.
Loopback only; all identities and records are synthetic. No real DB/provider or
production connection. The real PG contract is tested separately. Screenshots
carry ISOLATED FIXTURE; ordinary product UI does not carry that banner.

- Desktop1440x1100: row edit focuses the label field; fresh Copy/version facts,
  name/category/marker form and persistent server label are visible.
- Mobile390x844: no body overflow (scrollWidth<=innerWidth); confirmation readable,
  buttons reachable. Initial focus is return/check; Shift+Tab wraps to confirm;
  Escape returns to the opener without a write.
- First human intent: exactly ONE PUT, synthetic503 after persistence -> UNKNOWN;
  explicit original-request GET -> SUCCEEDED version1 -> exact GET shows saved
  product label/category/marker, while Canonical filename/path remain unchanged.
- Second human intent: one additional confirmation and ONE PUT -> direct success
  version2 -> fresh exact GET. Total2 human intents /2 PUTs, no automatic retry.
- One expected network503 is deliberate. No browser script error was observed.
  Request log is the fixture-only method/path evidence, not production traffic.

[Desktop editor](fixture-desktop-editor.png), [mobile confirmation](fixture-mobile-confirm.png),
[mobile UNKNOWN](fixture-mobile-unknown.png), [mobile confirmed receipt](fixture-mobile-recovered.png).
