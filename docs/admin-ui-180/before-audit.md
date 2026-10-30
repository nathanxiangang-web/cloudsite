# Issue180 screenshot audit - before implementation

Baseline: main750f5281217d92bc2ed48b71bb28256bd0101e82; merged-main CI40652403381 SUCCESS3/3.
All twelve captures are isolated browser fixtures with synthetic data, not production screenshots. Desktop viewport1440x900, mobile390x844. Screenshots were captured through the connected Chrome tab, saved and inspected before this inventory was accepted.

| # | Evidence | Finding and action | Source |
|---|---|---|---|
| 1 | 01,10 | Nine peer navigation entries mix business, diagnostics and an unfinished settings page. Keep five daily tasks and a secondary diagnostics group. | index.html sidebar |
| 2 | 01,02,09 | Schema badge, control-plane eyebrow and repeated admin branding occupy task space. Remove developer chrome. | index.html header/login |
| 3 | 01-09,11,12 | The same connection caveat banner appears on unrelated pages. Keep contextual warnings only at the relevant action. | index.html scope banner |
| 4 | 01,09 | Two different task totals and many low-level phase counts are equally prominent. Retain distinct denominators, show key counts, disclose detailed phase breakdowns. | overview-panel, admin-overview.js |
| 5 | 02,09 | Endpoint names, aggregation contract and two clocks compete with list contents and user actions. Show business totals and disclose provenance. | admin-lists.js, index.html |
| 6 | 03,11 | Resource toolbar repeats query terminology and scope paragraphs; on390px the first file is below the first screen. Shorten controls and disclose exact-path tools. | resources-panel, app.js |
| 7 | 04 plus same-state DOM | Ten resource identity/evidence rows precede the editor. A disabled document preview placeholder has no accepted implementation. Move technical facts to disclosure and remove placeholder. | file-detail, app.js |
| 8 | 05,12 | Ten task columns contain raw UUIDs and protocol states, forcing horizontal navigation before meaningful status. Use a compact business list and retain the evidence in details. | admin-lists.js |
| 9 | 06 | Connection configuration, manual-probe results and untracked metrics are mixed; repeated health caveats do not help choose the next step. Keep one precise caveat at the manual diagnostic action. | storage-panel, admin-observe.js |
| 10 | 07,08 | Audit rows lead with request UUID and machine result code; share rows expose full owner IDs. Prioritize action/result/file and make raw identifiers expandable. | admin-audit.js, admin-shares.js |

Strengths to preserve: real read/empty/error distinctions; separate product request and execution counts; role/session fences; deliberate write confirmations; existing keyboard focus and mobile drawer; the deep blue navigation shell. Screenshots do not establish full accessibility compliance. Keyboard, focus, error recovery and mutation safety require regression checks in the implementation pass.