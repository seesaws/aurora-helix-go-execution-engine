# aurora-helix-go-execution-engine

Temporal-based Go execution engine scaffold for the workflow example.

## Role split

- `cmd/service-worker`: Temporal workflow worker, hosts the service-dispatch workflow
- `cmd/agent-executor`: Temporal activity worker, executes service tasks
- `cmd/event-worker`: HTTP ingress + Temporal client, starts workflows and reads results

## Temporal mapping

- **Agent Executor** → activity worker on `agent-executor` task queue
- **Service Worker** → workflow worker on `service-worker` task queue
- **Event Worker** → Temporal client process with HTTP endpoints

## Endpoints

- `GET /healthz`
- `POST /dispatches`
- `GET /results/{dispatchId}`

## Run

Start a local Temporal dev server first, then run:

```bash
go run ./cmd/service-worker
go run ./cmd/agent-executor
go run ./cmd/event-worker
```

Default HTTP port: `8081`
Default Temporal target: `localhost:7233`

## Verify

```bash
go test ./...
go test -race ./...
go build ./cmd/service-worker
go build ./cmd/agent-executor
go build ./cmd/event-worker
```
