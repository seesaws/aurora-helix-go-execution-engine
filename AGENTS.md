# PROJECT KNOWLEDGE BASE

**Generated:** 2026-04-18T11:34:52
**Commit:** ad02974
**Branch:** master

## OVERVIEW
Temporal-based Go execution engine scaffold with three runnable processes: HTTP ingress/client, workflow worker, and activity worker.
Single Go module. Runtime flow is HTTP request -> dispatcher -> Temporal workflow -> Temporal activity -> worker result.

## STRUCTURE
```text
aurora-helix-go-execution-engine/
├── cmd/                    # process entrypoints only; thin wiring
├── internal/               # shared models, adapters, Temporal integration, HTTP transport
├── manualqa/               # local dev helper for Temporal dev server
├── test/                   # top-level HTTP/integration-style tests
├── README.md               # run/verify commands and role split
└── go.mod                  # single Go module
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Run a process | `cmd/*/main.go` | Three binaries only: `event-worker`, `service-worker`, `agent-executor` |
| Change dispatch contract | `internal/domain`, `internal/transport/http` | Preserve camelCase JSON fields |
| Change HTTP behavior | `internal/transport/http` | Routes, validation, response shape |
| Change workflow/activity flow | `internal/temporal` | Queue names, workflow/activity registration, retry behavior |
| Change dispatch orchestration | `internal/app` | `Dispatcher` abstraction + Temporal-backed implementation |
| Add task types | `internal/executor/demo` | Registry keyed by dotted `taskType` strings |
| Understand test coverage | `internal/temporal/workflows/*_test.go`, `test/worker_integration_test.go` | Workflow test is colocated; HTTP integration test is top-level |
| Start local Temporal dev server | `manualqa/temporaldev/main.go` | Manual helper, not production runtime |

## CODE MAP
| Symbol | Type | Location | Role |
|--------|------|----------|------|
| `main` | function | `cmd/event-worker/main.go` | Boot HTTP ingress + Temporal client |
| `main` | function | `cmd/service-worker/main.go` | Boot workflow worker |
| `main` | function | `cmd/agent-executor/main.go` | Boot activity worker |
| `Dispatcher` | interface | `internal/app/dispatcher.go` | App boundary for dispatch/result lookup |
| `TemporalDispatcher` | struct | `internal/app/temporal_dispatcher.go` | Starts workflows and reads results |
| `ServiceDispatchWorkflow` | workflow | `internal/temporal/workflows/service_dispatch.go` | Dispatch orchestration workflow |
| `AgentExecutor.ExecuteTask` | activity method | `internal/temporal/activities/agent_executor.go` | Decode payload and invoke registry |
| `Server.Handler` | method | `internal/transport/http/server.go` | Exposes `/healthz`, `/dispatches`, `/results/` |
| `Registry` | struct | `internal/executor/demo/demo_executor.go` | Maps `taskType` -> executor |

## CONVENTIONS
- Keep executable entrypoints thin. `cmd/*` should compose dependencies and call package-level runners, not hold business logic.
- Keep HTTP ingress only in `event-worker`. Temporal workers do not serve HTTP.
- Keep Temporal strings centralized. Queue names live in `internal/temporal/queues`; activity name lives in `internal/temporal/activities/names.go`.
- Preserve camelCase JSON names: `processInstanceId`, `nodeKey`, `taskType`, `dispatchId`, `resultStatus`.
- Runtime config is env-only via `internal/config/config.go`; visible vars are `PORT`, `TEMPORAL_HOST_PORT`, `TEMPORAL_NAMESPACE`.
- Task types use dotted strings such as `demo.echo`.

## ANTI-PATTERNS (THIS PROJECT)
- Do not add HTTP handlers to `service-worker` or `agent-executor`; HTTP belongs in `event-worker`.
- Do not bypass `Dispatcher -> workflow -> activity` when servicing request-triggered execution.
- Do not inline queue names or activity names in random files; reuse centralized constants.
- Do not silently coerce unknown task types. Current contract treats unsupported task types as explicit failure.
- Do not treat `PLACEHOLDER_*` status strings as mature domain language without intentional schema work.

## UNIQUE STYLES
- Role split is explicit and part of the project model: event worker = ingress/client, service worker = workflow host, agent executor = activity host.
- Workflow result IDs are derived as `<dispatchId>:result`.
- Dispatch IDs are generated in HTTP as `dispatch-<unixnano>`.
- Tests are intentionally mixed: colocated workflow tests plus top-level HTTP integration tests.

## COMMANDS
```bash
go run ./cmd/service-worker
go run ./cmd/agent-executor
go run ./cmd/event-worker

go test ./...
go test -race ./...

go build ./cmd/service-worker
go build ./cmd/agent-executor
go build ./cmd/event-worker

go run ./manualqa/temporaldev
```

## NOTES
- Runtime commands expect a local Temporal dev server first; default target is `localhost:7233`.
- No checked-in CI/workflow files or task runner files were found. README is the primary source of run/verify commands.
- There were no pre-existing `AGENTS.md` or `CLAUDE.md` files in this repository when this knowledge base was generated.
