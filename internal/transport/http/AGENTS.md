# HTTP TRANSPORT KNOWLEDGE BASE

## OVERVIEW
This directory owns the public HTTP contract: DTOs, route wiring, validation, and response shaping for the event worker.

## STRUCTURE
```text
internal/transport/http/
├── dto.go      # HTTP request/response DTOs backed by domain types
└── server.go   # route registration, validation, dispatch/result handlers
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Change routes | `server.go` | Current routes: `GET /healthz`, `POST /dispatches`, `GET /results/` |
| Change request/response JSON | `dto.go` + `server.go` | Preserve domain-backed camelCase fields |
| Change validation | `server.go` | Required fields are enforced in handler |
| Change server-side ID generation | `server.go` | Dispatch IDs generated as `dispatch-<unixnano>` |

## CONVENTIONS
- Keep camelCase JSON fields aligned with current DTO/domain tags.
- Keep `processInstanceId`, `nodeKey`, and `payload.taskType` as required fields unless the dispatch contract is intentionally redesigned.
- Keep this layer dependent on `app.Dispatcher`; it should not know workflow/activity internals.
- Keep route wiring in `Handler()` and response writing through `writeJSON`/`writeError` helpers.

## ANTI-PATTERNS
- Do not bypass `app.Dispatcher` to call executors or Temporal SDK directly from handlers.
- Do not move HTTP ingress into other processes; this transport belongs to `event-worker`.
- Do not invent a second JSON naming style or mix snake_case with existing camelCase fields.
- Do not silently swallow malformed payloads; invalid JSON and missing required fields are explicit 400s.

## NOTES
- HTTP behavior is black-box tested from `test/worker_integration_test.go`, not with package-local tests in this directory.
- `GET /results/` is implemented as a prefix route; handler code trims `/results/` from the path before lookup.
