# TEMPORAL SUBSYSTEM KNOWLEDGE BASE

## OVERVIEW
`internal/temporal/` contains all Temporal-specific integration: client creation, queue constants, workflow/activity code, and thin worker bootstrap.

## STRUCTURE
```text
internal/temporal/
├── client/        # Temporal client dialer
├── queues/        # canonical task queue names
├── activities/    # activity entrypoints + names
├── workflows/     # workflow orchestration logic + workflow tests
└── workers/       # thin registration/bootstrap wrappers
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Change Temporal connection | `client/client.go` | Uses resolved config only |
| Change queue topology | `queues/queues.go` | Centralize queue names here |
| Change activity name or payload decoding | `activities/` | `AgentExecutor.ExecuteTask` unmarshals `DispatchRecord.Payload` |
| Change orchestration/retries/result shaping | `workflows/service_dispatch.go` | Owns retry policy and result ID formation |
| Change worker registration | `workers/agent`, `workers/service` | Keep these wrappers thin |
| Change workflow test style | `workflows/service_dispatch_test.go` | Uses Temporal test suite |

## CONVENTIONS
- Keep queue names centralized in `queues/queues.go`; starter and worker sides must agree.
- Keep workflow code side-effect-light. External execution should stay behind activities.
- Keep activity logic as adapter code: decode payload, delegate to registry/port, return structured outcome.
- Keep worker packages bootstrap-only: register workflows/activities, then `Run(worker.InterruptCh())`.
- Preserve the current split: service worker polls `service-worker`, agent executor polls `agent-executor`.
- Preserve workflow result shaping: `ResultID = dispatchID + ":result"`.

## ANTI-PATTERNS
- Do not inline queue names in workflows, entrypoints, or tests when constants already exist.
- Do not stuff business logic into `workers/*`; those leaves are thin wrappers by design.
- Do not let activities bypass `ports.TaskExecutor` and reach directly into transport-facing types.
- Do not change retry policy or workflow/activity queue routing without updating the whole dispatch path.
- Do not add leaf `AGENTS.md` files under `workers/agent` or `workers/service`; those directories are too small and should stay covered here.
