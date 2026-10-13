# Panta Gate4 API Contract Guidance

Status: Architecture Guidance

## API Boundary

Future product API:

```
/api/v1

/auth
/user
/catalog
/assets
/acquisition
/jobs
/ai
/admin
```

## Rules

API exposes product capabilities only.

Forbidden:

```
API -> OpenList
API -> IndexCore DB
API -> Provider SDK
```

Required:

```
API
 ↓
Application Service
 ↓
Domain Module
 ↓
Existing Adapter Boundary
```

## Resource API

Catalog APIs operate on logical identity:

```
Asset
Release
Variant
Copy
```

They do not expose physical storage implementation details.

## Acquisition API

Acquisition APIs create user intent:

```
Request
 ↓
Manifest
 ↓
Job
```

Execution remains owned by existing Gate3 flow.

## AI API

AI endpoint accepts intent requests only.

Example:

```
POST /api/v1/ai/intents
```

AI output must be validated by domain services.

No direct state mutation.

## Acceptance

This document defines contracts only. No implementation.
