# Panta Gate4 Product Blueprint

> Architect guidance document
>
> Status: Draft Architecture Guidance
>
> Scope: Gate4 Productization Design
>
> Owner: Architect
>
> Executor: Codex Developer

---

# 1. Gate4 Goal

Gate0-Gate3 solved the resource lifecycle reliability problem.

Gate4 does not redesign the core. Gate4 defines how Panta becomes a user-facing product while preserving existing architectural boundaries.

Core principle:

```
Product Experience
        ↓
Panta Domain Services
        ↓
Existing Catalog / Acquisition
        ↓
IndexCore / Provider
```

UI and AI must consume existing domain capabilities. They must not become the owner of resource truth.

---

# 2. Product Positioning

Panta is:

> An AI-assisted resource management and acquisition platform.

Panta provides:

- logical resource organization through Catalog;
- reliable resource acquisition through Acquisition;
- AI-assisted understanding and interaction.

Panta is NOT:

- a replacement for OpenList;
- a 115 client;
- a simple download page;
- an AI chat application.

115 remains only one provider implementation.

---

# 3. User Model

Initial product roles:

```
User
 |
 +-- Guest
 |
 +-- Registered User
 |
 +-- Premium User
 |
 +-- Administrator
```

Responsibilities:

## Guest

- browse public catalog information;
- limited AI interaction.

## Registered User

- manage personal resources;
- submit acquisition requests;
- view task status.

## Premium User

- enhanced AI capability;
- extended acquisition capability.

## Administrator

- system management;
- operational control.

---

# 4. Core User Flows

## Browse Flow

```
User
 ↓
Catalog
 ↓
Asset
 ↓
Release
 ↓
Variant
 ↓
Copy
 ↓
Access
```

Catalog remains the user-facing logical resource model.

---

## Acquisition Flow

```
User Request
 ↓
Manifest
 ↓
Job
 ↓
Provider
 ↓
IndexCore Observation
 ↓
Canonical Confirmation
 ↓
READY
```

Gate3 acquisition rules remain unchanged.

---

# 5. AI Architecture Boundary

AI is an assistant, not a domain owner.

Forbidden:

```
AI
 ↓
Database
 ↓
Modify State
```

Allowed:

```
User
 ↓
AI Assistant
 ↓
Structured Intent
 ↓
Domain Service
 ↓
Catalog / Job
```

AI output must be structured.

Example:

```json
{
  "intent": "download",
  "resource": "example",
  "quality": "1080p"
}
```

The domain layer decides whether the operation is valid.

---

# 6. Product Modules

Future product layer candidates:

```
account
catalog-ui
acquisition-ui
ai-assistant
favorites
notification
recommendation
```

These are product modules only.

They must not bypass:

- Catalog boundary;
- Job Engine boundary;
- Provider contract;
- IndexCore integration boundary.

---

# 7. Frontend Information Architecture

Initial information architecture:

```
Home
 |
 +-- Discover
 +-- Catalog
 +-- My Resources
 +-- Acquisition Tasks
 +-- AI Assistant
 +-- Settings
```

The product should emphasize:

```
Discover resources
Manage resources
Acquire resources
```

Not:

```
Search boxes + download buttons
```

---

# 8. API Boundary Planning

Future API namespace:

```
/api/v1

/auth
/catalog
/assets
/acquisition
/jobs
/ai
/user
```

This document only defines boundaries.

No API implementation is authorized by this task.

---

# 9. Data Ownership Rules

Product database may own:

- user accounts;
- preferences;
- favorites;
- AI conversations;
- product settings.

IndexCore owns:

- physical resource truth;
- canonical observation;
- Journal events.

Panta must not duplicate physical truth.

---

# 10. Gate4 MVP Scope

Included:

- user system design;
- Catalog presentation;
- acquisition task presentation;
- AI assistant entry point.

Deferred:

- recommendation algorithm;
- community features;
- comments;
- autonomous Agent execution;
- complex social functions.

---

# 11. Codex Execution Rules

Codex must:

1. Read AI-ARCHITECTURE-MEMORY.md first.
2. Follow this blueprint before implementation.
3. Create small PRs.
4. Add tests with every behavior change.
5. Wait for Architect acceptance before entering next Gate.

Forbidden:

- changing IndexCore ownership;
- adding direct OpenList verification;
- allowing AI to mutate domain state directly;
- implementing UI before contracts are accepted.

---

# Next Planned Tasks

After this document acceptance:

```
G4-A02 User Domain Model
        ↓
G4-A03 API Contract
        ↓
G4-A04 Frontend Information Architecture
        ↓
G4-A05 Prototype Design
```

Current task ends at architecture documentation acceptance.
