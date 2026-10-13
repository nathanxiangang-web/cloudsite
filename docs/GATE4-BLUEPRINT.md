# Gate 4 Control Plane Blueprint

Status: ARCHITECTURE REVIEW DRAFT

## 1. Objective

Gate 4 builds a safe control plane on top of the validated Gate 3 runtime. It does not replace acquisition, Job, Manifest, Worker, IndexCore or Copy semantics.

Primary goal:

```
User / Operator
        |
        v
Control Plane
        |
        v
Existing Runtime
(Job / Manifest / Worker / IndexCore / Copy)
```

## 2. Boundary

Included:

- runtime observation
- task visibility
- execution timeline
- worker health visibility
- audit events
- safe operator workflows

Excluded:

- new provider implementation
- new download workflow
- schema redesign
- bypassing state machines
- direct database mutation from UI
- public product launch

## 3. Core Principles

1. Runtime remains the source of truth.
2. UI never writes directly to storage.
3. All mutations must pass existing service boundaries.
4. Read-only observation precedes control actions.
5. Every operator action requires auditability.

## 4. Initial Modules

### Control API

Provides controlled access to:

- tasks
- jobs
- manifests
- workers
- execution events

### Console

Provides:

- dashboard
- task detail
- state visualization
- worker status

### Audit Plane

Records:

- actor
- operation
- timestamp
- target object
- result

## 5. Development Order

Phase 0: ADR and contracts

Phase 1: Read-only Control API

Phase 2: Console visualization

Phase 3: Restricted operator actions

Each phase requires independent review.

## 6. Acceptance

Gate 4 is not started until this blueprint receives architecture approval and a bounded implementation issue is created.
