# Aurora service-task execution engine

Go executes one authorized service task per dispatch. Java SOP owns the graph,
approvals, business authorization, outbox/inbox and final business state. Go has
no business database or Dubbo client. The production executor currently admits
only `system.echo`; the RFQ/demo executors are explicit local demonstrations
and perform no real business mutation.

## Processes and configuration

- `cmd/event-worker`: authenticated HTTP ingress and Temporal client.
- `cmd/service-worker`: `service-worker` queue; single-task workflow.
- `cmd/agent-executor`: `agent-executor` queue; task and durable callback activities.

Environment only:

| Variable | Meaning |
|---|---|
| `PORT` | HTTP port, default `8081` |
| `HTTP_HOST` | Optional bind host; use `127.0.0.1` for isolated local fixtures |
| `TEMPORAL_HOST_PORT` | Temporal target, default `localhost:7233` |
| `TEMPORAL_NAMESPACE` | Namespace, default `default` |
| `EXECUTION_SERVICE_TOKEN` | Fixed machine ingress Bearer shared with Java SOP; missing value denies every protocol request |
| `SOP_BASE_URL` | Fixed SOP HTTP base, e.g. `http://127.0.0.1:8080/sop`; optional clean context prefix, no credentials, query or fragment |
| `SOP_SERVICE_TOKEN` | Outgoing machine Bearer; resolved at each callback activity attempt, never included in workflow arguments/history |

These machine credentials are separate from human access tokens. Supply them
through the process environment or a credential provider; do not check them in.
Token provider errors, callback transport errors and rejection responses are
sanitized. Callbacks never follow redirects. No deployment or tenant grants are
part of this repository change.

## Internal HTTP protocol

`GET /healthz` is public. Every request to the following routes requires
`Authorization: Bearer <configured EXECUTION_SERVICE_TOKEN>`:

- `POST /dispatches`
- `GET /results/{dispatchId}`
- `POST /dispatches/{dispatchId}/cancel`

Example dispatch (all IDs supplied by the trusted Java caller):

```json
{
  "dispatchId": "dispatch-1",
  "tenantId": "tenant-1",
  "processInstanceId": "process-1",
  "nodeInstanceId": "node-1",
  "nodeKey": "echo",
  "executionGeneration": 1,
  "taskType": "system.echo",
  "idempotencyKey": "tenant-1:process-1:node-1:1",
  "input": {"message": "hello"},
  "metadata": {"traceId": "trace-1"},
  "retry": {"maxAttempts": 3, "backoff": "PT1S"},
  "timeoutSeconds": 30
}
```

ACK is `{ "dispatchId": "dispatch-1", "status": "DISPATCHED" }`.
Unknown JSON fields, extra JSON values and bodies over 1 MiB are rejected.
All identity fields and a positive generation are required. Metadata admits
only string `traceId` and `correlationId`. Credential/callback fields in input
are rejected. Identifiers are limited to 200 ASCII letters, digits, `_ . : -`.
Input nesting is bounded to 128; numeric text to 256 characters and exponents
between -1000 and 1000.

Timeout is the whole task budget, including queueing and retries (1–3600s,
default 30). Retries are 1–10 attempts (default 3) with exponential backoff
(default 1s, accepted as ISO time duration such as `PT1S` or Go `1s`, bounded
1ms–1h). Permanent typed executor errors stop retries; transient errors retry.
`system.echo` has no external effect; future business adapters must enforce the
stable idempotency key at the actual business transaction boundary.

Workflow changes use a Temporal version gate. Unmarked scaffold histories replay
through their original command sequence and never receive an invented tenant or
merchant callback. Before rollout, drain old `demo.echo`/simulated RFQ activities
or retain a correctly pinned legacy worker build: the new production activity
worker deliberately does not execute those types. Completed-history replay does
not prove pending legacy task compatibility.

The workflow ID is the dispatch ID with Temporal reuse rejected after completion.
An immutable memo holds a canonical hash of the complete execution envelope,
input and normalized policy. Object order and equivalent number spellings
(e.g. `1`, `1.0`) replay the original ACK; changes return 409. Default values and
equivalent backoff spellings are canonicalized. Never reuse a dispatch ID for a
new generation. This deduplication depends on Temporal history retention;
configure retention to exceed the Java outbox/reconciliation horizon and retain
Java's authoritative dispatch ledger. Requests after history expiry are outside
this engine's deduplication guarantee.

Results include dispatch, tenant, process, node, generation, status, output,
message and completion time. Status is `RUNNING`, then exactly one of
`SUCCEEDED`, `FAILED`, `TIMEOUT`, `CANCELLED`. Cancellation returns
`CANCEL_REQUESTED` while stopping a running task, or the existing terminal
status. Task adapters must cooperate with context cancellation; activity
heartbeats propagate cancellation. Once a terminal result is recorded,
cancellation cannot overwrite it. Java must independently check the complete
envelope/generation and ignore late results for a cancelled SOP instance.

The terminal result is queryable before callback delivery. A detached Temporal
activity retries the callback without an attempt limit, at most every 5 minutes,
even for cancelled tasks. Its fixed destination is:

`POST <SOP_BASE_URL>/internal/sop/service-dispatches/{dispatchId}/result`

The JSON body uses `status` (not `resultStatus`) and the full trusted identity
envelope. A non-2xx response retries; Java's inbox must atomically deduplicate
`dispatchId + executionGeneration`. Workflow queries require a live service
worker; state replays from Temporal history when the worker restarts. The
workflow remains open while its callback is undelivered, making callback backlog
visible separately from the terminal task outcome. Unexpected administrative
workflow termination is queryable but cannot promise callback delivery.

## Run and verify

Start an isolated Temporal server (`rtk go run ./manualqa/temporaldev` provides
a loopback local dev-server helper; `TEMPORAL_HOST_PORT` selects a separate port), configure environment, then run the three commands:

```bash
rtk go run ./cmd/service-worker
rtk go run ./cmd/agent-executor
rtk go run ./cmd/event-worker
```

```bash
rtk go test ./...
rtk go test -race ./...
rtk go build ./cmd/service-worker
rtk go build ./cmd/agent-executor
rtk go build ./cmd/event-worker
```

Actual Temporal recovery tests are opt-in and use an isolated server:

```bash
rtk proxy env TEMPORAL_INTEGRATION=1 TEMPORAL_HOST_PORT=127.0.0.1:7233 go test ./test -run TestRealTemporal -count=1 -v
```

They start real workers against a real server, force callback HTTP 503, recreate
the workers, verify one execution and restored delivery, and verify completed
replay/conflict, cancellation, timeout and replay of an unmarked scaffold history. This is distinct from Temporal's
in-process `WorkflowTestSuite`; it is still a local callback HTTP substitute,
not Java/Go deployed integration. See [implementation evidence](docs/merchant-sop-execution.md).
