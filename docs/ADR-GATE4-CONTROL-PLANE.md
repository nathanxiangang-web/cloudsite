# ADR — Gate 4 Product Control Layer Scope

Status: Proposed

## Context

Gate 3.18 validated the complete acquisition path:

115 task -> recovery -> provider completion -> Hint -> Q5/Q8 -> Copy -> READY.

The next product evolution is controlled user-facing capability, including identity,
access control, sharing and usage management.

Previous wording using "Control Plane" was too broad and could be interpreted as
an AI orchestration platform. This ADR narrows the Gate 4 boundary.

## Decision

Gate 4 is defined as the Product Control Layer.

Initial scope:

- user identity and account boundary;
- authentication/session model;
- resource access control;
- sharing capability and access policy;
- usage tracking and quota boundary;
- audit records for product operations.

The layer must interact with existing runtime components through stable service
contracts and must not replace acquisition, provider execution or IndexCore
canonical resource responsibilities.

## Deferred Scope

The following are explicitly NOT part of Gate 4 implementation authorization:

- AI Agent platform;
- autonomous planning;
- Intent/Decision/Planner architecture;
- DAG workflow engine;
- provider orchestration replacement;
- UI implementation before API and domain boundaries are frozen.

These may be considered in future gates after product control foundations are stable.

## Non Goals

- no new acquisition architecture;
- no provider changes;
- no direct IndexCore database writes;
- no schema migration authorized by this ADR;
- no UI implementation authorization.

## Security

The product control layer must:

- preserve existing authorization boundaries;
- avoid exposing secrets or provider credentials;
- use service APIs instead of direct database writes;
- record security-sensitive operations.

## Consequence

Future Gate 4 implementation must start from this bounded product-control scope
and receive a separate implementation plan before runtime changes.
