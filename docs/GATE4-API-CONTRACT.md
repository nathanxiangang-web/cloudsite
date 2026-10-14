# Product API contract index

Status: Stage 1 handlers implemented under Issue #77; no product listener/deployment yet.

The former file was a route-prefix sketch with an AI example, not a frozen API
contract. It did not establish working endpoints, DTOs, authentication or errors.

| Stage | Contract source | Implementation status |
| --- | --- | --- |
| Auth/session | [Stage 1](GATE4-IMPLEMENTATION-PLAN.md#stage-1-frozen-product-auth-contract) and [implemented module](PRODUCT-AUTH.md): login/me/logout, input/DTO/error/session/Origin behavior | Domain/store/handlers tested; startup integration not yet implemented |
| Product reads/submission/usage | Must be frozen from existing source before its bounded implementation | Missing |
| Access/share | Must define capability binding, exact resource mapping, tracked side effects, safe results and failure semantics | Missing |
| UI/Agent | UI consumes accepted APIs; Agent optional and separately validated | Missing / deferred |

Transport -> application service -> domain/ports -> adapters. No direct UI/HTTP
SQL writes, SDK mutation, IndexCore DB client or acquisition OpenList verifier.
An authorized access adapter is distinct from canonical observation and may use
OpenList for access as allowed by the original MVP. IDs/source/credential data
must follow policy and DTO allowlists; route names alone are not contracts.
