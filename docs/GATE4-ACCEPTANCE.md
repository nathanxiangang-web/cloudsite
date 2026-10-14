# Gate 4 acceptance and core MVP evidence

Status: Required acceptance criteria; NOT a completed product acceptance.

| Capability | Required evidence | Current baseline |
| --- | --- | --- |
| Product login | Protected provisioning, password verification, enabled-account policy, session expiry/revocation and safe cookies; HTTP + isolated PG tests | Not implemented |
| Policy | Provider identity cannot impersonate a user; cross-user task/source reads denied; admin/system privileges explicit | Not implemented |
| Resource browse/access | Authorized Catalog/known-path flow, exact PRESENT Copy, unavailable/removed/ambiguous fail closed, real access result | Read domains exist; product gateway missing |
| Acquisition | Authenticated exact-source submission, ledger/quota admission, same Job through Hint/Q5/Q8/Copy/READY, no duplicate start | Gate 3 core accepted; product admission missing |
| Share | Authenticated tracked create/inspect/revoke through ShareProvider, capability/uncertainty handling, revoke preserves file | Port exists; concrete product flow missing |
| Usage/quota | Operation-level durable idempotent accounting, concurrency limits, zero-price display, no charge per claim/poll | Not implemented |
| Usable interface/runtime | Actual browser flow, explicit protected startup, owned process shutdown and reproducible runbook | Not implemented |

Update "Current baseline" only with linked source and actual tests/CI/deployment
evidence. Design docs do not prove a model, endpoint or UI is implemented.

Before core MVP acceptance demonstrate all required flows on isolated product
data, retain controlled real evidence, prove history/credentials/old services
unchanged, and record limitations. Preserve architecture import guards and
provider-replacement/mock tests. No payment, recommendation, autonomous Agent
or polished UI is required; do not claim optional untested Agent capability.

The delegated same-agent review is stated honestly. Exact-head CI is required
for merges; passing docs CI is not runtime or product acceptance. Original full
Gate 5/6 product goals remain separately documented, not falsely closed.
