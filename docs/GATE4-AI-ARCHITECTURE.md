# Panta Gate4 AI Architecture

Status: Architecture Guidance

## Position

AI is an assistant layer.

AI does not own business state.

## Architecture

```
User
 ↓
AI Assistant
 ↓
Intent Understanding
 ↓
Structured Command
 ↓
Domain Service
 ↓
Catalog / Acquisition
```

## AI Responsibilities

Allowed:

- natural language understanding;
- resource classification assistance;
- recommendation assistance;
- workflow guidance.

Forbidden:

- direct database writes;
- bypassing Job Engine;
- direct Provider execution;
- becoming canonical resource truth.

## Agent Boundary

Agent output is advisory structured input.

The domain layer decides:

- validity;
- permission;
- execution;
- state transition.

## Future AI Modules

```
ai-assistant
ai-classifier
ai-recommendation
ai-search-helper
```

Implementation requires separate approval.
