# INTERNAL KNOWLEDGE BASE

## OVERVIEW
`internal/` is layered shared code: domain types, app orchestration, transport adapters, Temporal integration, and executor plumbing.

## STRUCTURE
```text
internal/
├── app/              # dispatch orchestration boundary
├── config/           # env-only runtime config
├── domain/           # shared records and status strings
├── executor/         # concrete task executors/registries
├── ports/            # narrow interfaces between layers
├── temporal/         # client, queues, workflow/activity/worker wiring
└── transport/http/   # HTTP DTOs + handlers
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Change shared payload/result fields | `domain/` | Affects HTTP, workflow, activity, and tests |
| Change dispatch orchestration API | `app/` | `Dispatcher` interface is the seam |
| Change config surface | `config/config.go` | Env-only; no file-based config |
| Add executor implementations | `executor/`, `ports/` | Keep registry behind `ports.TaskExecutor` |
| Change Temporal plumbing | `temporal/` | See child `internal/temporal/AGENTS.md` |
| Change HTTP adapter | `transport/http/` | See child `internal/transport/http/AGENTS.md` |

## CONVENTIONS
- Treat `domain/` as the shared contract layer. Changes here ripple across transport, app, Temporal, executors, and tests.
- Keep `ports/` narrow. The current seam is `TaskExecutor`; do not leak transport or Temporal concerns into it.
- Keep `app/` orchestration-focused. `TemporalDispatcher` should bridge into Temporal, not own HTTP or executor details.
- Keep config loading centralized in `config.Load()`; downstream packages accept resolved config values.
- Prefer subsystem-local rules in child AGENTS files over growing this file into a duplicate root.

## ANTI-PATTERNS
- Do not move shared contracts into adapter packages.
- Do not make `domain` depend on transport, Temporal SDK, or executor packages.
- Do not skip `ports` by wiring executor-specific types directly through unrelated layers.
- Do not add subtree-local guidance here if a narrower child file can own it.
