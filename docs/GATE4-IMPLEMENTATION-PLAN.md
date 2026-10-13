# Panta Gate4 Implementation Plan

Status: Architecture Roadmap

## Execution Order

```
G4-A01 Product Blueprint       DONE
        |
G4-A02 User Domain Model       DONE
        |
G4-A03 API Contract            DONE
        |
G4-A04 Frontend Architecture
        |
G4-A05 Prototype
        |
Implementation
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
