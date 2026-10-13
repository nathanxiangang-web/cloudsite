# Panta Gate4 User Domain Model

Status: Architecture Guidance

## Goal

Define product user ownership without changing existing Catalog, IndexCore or Acquisition boundaries.

## Domain Boundary

```
User Product Domain
        |
        +-- Account
        +-- Permission
        +-- Preference
        +-- Favorite
        +-- AI Session
        +-- Acquisition History
```

## Ownership

Panta Product DB owns:

- users
- accounts
- permissions
- preferences
- favorites
- AI conversation metadata
- product history

Catalog owns:

- Asset
- Release
- Variant
- Copy

IndexCore owns:

- physical resource truth
- canonical identity
- Journal observation

## Rules

1. User state never enters IndexCore.
2. AI session state never becomes resource truth.
3. Favorites reference Catalog identity, not physical paths.
4. Acquisition history references Job/Manifest results.

## Future Entities

```
User
 |
 +-- Account
 +-- Role
 +-- Preference
 +-- Favorite
 +-- AIConversation
 +-- AcquisitionRecord
```

No implementation is authorized by this document.
