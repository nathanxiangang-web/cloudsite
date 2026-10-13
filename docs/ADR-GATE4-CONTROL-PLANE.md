# ADR — Gate 4 Control Plane

Status: Proposed

## Context

Gate 3.18 validated the complete acquisition path:

115 task -> recovery -> provider completion -> Hint -> Q5/Q8 -> Copy -> READY.

The next problem is operational visibility and controlled interaction.

## Decision

Introduce a Control Plane as an independent layer.

The Control Plane observes and orchestrates existing runtime components without replacing them.

## Non Goals

- no new acquisition architecture
- no provider changes
- no schema migration in this decision
- no UI implementation authorization

## Security

The control plane must:

- preserve existing authorization boundaries
- avoid exposing secrets or provider credentials
- use service APIs instead of direct database writes
- record operator actions

## Consequence

Future Gate 4 implementation must start from this ADR and receive a bounded implementation plan.
