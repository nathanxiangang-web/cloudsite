# Panta Gate4 Implementation Plan

Status: Architecture Roadmap

## Current bounded task - 2026-10-14

[Issue #75 - Gate 4.1](https://github.com/nathanxiangang-web/panta/issues/75)
produces [Control Plane domain architecture](CONTROL-PLANE-DOMAIN.md) only.
The domain proposal and this roadmap await explicit Architect acceptance;
document presence does not prove a contract, login or implementation is done.
No code/API/auth/UI, migration, deployment or provider operation is authorized
by this documentation task. Issue #74 is a separate implementation proposal,
not the task being executed here.

Original [MVP Gate 4](MVP-BLUEPRINT.md#gate-4--login-access-share-usage)
auth/access/share/usage is still the product acceptance boundary. Control
observation supports it; it does not replace it. AI guidance and the phases
below do not advance Agent implementation from Gate 5 or authorize premium,
recommendation, favorites or console implementation.

## Execution Order

```
G4-A01 Product Blueprint       DOCUMENT PRESENT
        |
G4-A02 User Domain Model       DOCUMENT PRESENT
        |
G4-A03 API Contract Guidance   DOCUMENT PRESENT (not a frozen route contract)
        |
Gate 4.1 Domain Architecture   PROPOSED / REVIEW PENDING
        |
Architect acceptance + one separately bounded implementation Issue
        |
Implementation (not started by this task)
```

## Development Rules

Team model:

```
Architect
    |
Codex Developer
```

Each stage:

Issue
 ↓
Implementation
 ↓
Tests
 ↓
PR
 ↓
Architect Review

## Phase Planning

The following is historical productization guidance, not the current execution
order or permission to begin these modules. Architect must reconcile it with
the original MVP, freeze required contracts and authorize one narrow task at
a time. Frontend/prototype work must not bypass domain contract acceptance.

### Phase 1

User foundation:

- authentication boundary
- account model
- permission model

### Phase 2

Catalog productization:

- resource browsing
- logical hierarchy display
- favorites

### Phase 3

Acquisition productization:

- task display
- history
- status tracking

### Phase 4

AI entrance:

- assistant UI
- intent handling
- structured requests

### Phase 5

Management:

- admin
- operations
- monitoring

## Restrictions

No phase may:

- change IndexCore ownership;
- bypass Catalog;
- bypass Job Engine;
- introduce direct OpenList verification.
