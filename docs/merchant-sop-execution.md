# Merchant SOP service-task execution — local implementation evidence

Date: 2026-10-21. Baseline `10dda3e`, branch
`codex/sop-M0-001-merchant-owned`. Worktree:
`/Users/wick/Desktop/project/aurora/.sop-worktrees/aurora-helix-go-execution-engine`.

Scope follows Java `docs/contracts/merchant-sop-v2.md` and the approved
`aurora-ai-service/exec-plans/merchant-owned-sop-20261019.md` M3 contract.
The HTTP dispatch/result/cancel, Temporal workflow and callback HTTP seams were
authorized by the parent implementation task; TDD used those boundaries.

## Progress and decisions

- [x] Read root/internal/HTTP/Temporal AGENTS and M0 contract; coordinate Java wire types (`tenantId` string, generation integer, `status` callback field).
- [x] Machine-only route authorization; fail closed when ingress secret missing.
- [x] Strict bounded dispatch validation, production `system.echo` allowlist.
- [x] Canonical semantic hash and immutable memo; reject workflow ID reuse including completed runs.
- [x] Terminal query state before detached durable callback activity; runtime service token lookup and fixed callback origin.
- [x] Typed permanent/transient execution errors, retry/backoff, total timeout, cancellation heartbeat and terminal fence.
- [x] Temporal GetVersion gate preserves the unmarked scaffold command path without inventing tenant identity/callback.
- [x] HTTP and workflow tests, plus real Temporal callback retry/worker recreation and terminal-state checks.
- [x] Real Java HTTP gateway, PostgreSQL application service and callback controller exercised against standalone Go ingress and Temporal.
- [ ] Parent cross-repository review; no commit or deployment performed by this worker.

Each slice first exercised an absent behavior: machine credential constructor,
identity envelope/replay, terminal result during callback retry, callback HTTP
transport, and Temporal reuse policy. The first real Temporal test caught a
critical SDK distinction: `REJECT_DUPLICATE` alone returns the old workflow handle
without error. `WorkflowExecutionErrorWhenAlreadyStarted=true` is required to
enter semantic-hash comparison. The failing real test returned 200 for a changed
completed request; the corrected test returns 409.

## Validation evidence

- `rtk go test ./...`: 18 tests passed at the initial full validation (real server tests skipped unless explicitly enabled).
- Real dev server launched through `rtk go run ./manualqa/temporaldev`; listener `127.0.0.1:7233` was separate from any pre-existing server. Downloaded CLI 1.9.1 started Server 1.32.0 with in-memory persistence. The helper was stopped after validation and no 7233 listener remained.
- `TEMPORAL_INTEGRATION=1 ... TestRealTemporalCallbackRecoveryAndCompletedReplay`: passed. Actual workflow `recovery-1792545312038939000`; one task execution, two callback attempts (503 then accepted), one accepted delivery after both workers were stopped and recreated. Unchanged completed request 200; changed request 409.
- `TEMPORAL_INTEGRATION=1 ... TestRealTemporalCancellationAndTimeout`: passed. Actual workflow `terminal-CANCELLED-1792545395101837000` returned and delivered CANCELLED; `terminal-TIMEOUT-1792545395927370000` returned and delivered TIMEOUT. Tenant/generation matched.
- These worker recreations occurred in one test OS process against the real Temporal server. They do not claim OS-process crash, Temporal-server restart, actual Java callback, external IAM, real business effects or deployment evidence.
- Synthetic test credentials appear only in test source/runtime; no real credentials were read, persisted or logged.

Final local checks:

- `rtk go test ./...`: 21 tests passed; ordinary runs skip the real-server tests. The final suite includes server-configured callback context-prefix validation.
- `rtk proxy env TEMPORAL_INTEGRATION=1 go test -race ./... -count=1`: all packages passed after the version gate; real callback recovery/cancel/timeout included and raw history payloads checked for synthetic machine credentials. The final rerun also included the real legacy-history replay test.
- All three entrypoints built successfully with `rtk go build -o /tmp/aurora-sop-{service-worker,agent-executor,event-worker} ./cmd/<entrypoint>` (three separate commands).
- `TestRealTemporalReplaysScaffoldHistory`: actual baseline-shaped unmarked history `legacy-replay-1792545595325999000`, 11 events, replayed successfully by the v2 workflow. This proves workflow command compatibility, not completion of old pending demo activities.
- `rtk git diff --check` passed. Parent review and final cross-repository checks remain separate.

## Java–Go wire validation

The Java selector `MerchantSopV2Test.realGoTemporalDispatchReturnsToAuthenticatedJavaCallback`
passed against standalone Go service-worker, agent-executor and event-worker
processes with a real local Temporal dev server. Isolated listeners were Temporal
`127.0.0.1:17233`, Go ingress `127.0.0.1:18095`, and the Java callback receiver
`127.0.0.1:18094`. `HTTP_HOST=127.0.0.1` keeps this fixture on loopback, and
`SOP_BASE_URL=http://127.0.0.1:18094/sop` supplies the server-owned context prefix.
The callback URL remains fixed by server configuration, with no caller URL or
credential in workflow input.

Java used the real `HttpSopDispatchGateway`, PostgreSQL store/application service
and `SopDispatchCallbackController`. A local JDK HTTP server adapted incoming
callback requests to that controller; it deliberately returned 503 for the first
callback, then accepted the authenticated retry. Assertions found one PostgreSQL
INBOX row, one next human task, and an APPROVED outcome after the fixture human
action. This verifies the local wire contract, including the `/sop` context path;
it does not establish a full Spring/Gateway deployment, external IAM or Mall
business integration.

Final passing rerun recorded instance `29fc1e1e-8b2e-40c9-8f42-0d90f18a4844`,
dispatch `d0038cd9-9251-4778-8e91-9175d9083b1e`:

- [Authenticated Go result](evidence/java-go-wire-result.json): SUCCEEDED, with matching tenant, instance, node and generation.
- [Temporal execution summary](evidence/java-go-wire-temporal.json): COMPLETED, 19 history events, zero pending activities.
- [Filtered Go process evidence](evidence/java-go-wire-process.log): task/callback scheduling and the initial callback HTTP 503.
- [Fixture cleanup evidence](evidence/java-go-wire-cleanup.json): owned processes exited, no listeners on 17233/18095, temporary fixture directory and launch helper removed.
- Java evidence lives in the sibling repository at `aurora-sop-service/evidence/merchant-v2/java-go-temporal.log`; it records two callback attempts and the passing selector.

Only synthetic fixture credentials were used; none are included in these evidence
files. The fixture uses in-memory Temporal persistence and temporary binaries/logs.
The fixture was stopped after the final passing rerun; no other service was
changed. This Go worktree is frozen for parent review, with no commit or deployment.

## Operational bounds

Temporal history retention bounds Go dispatch deduplication; Java remains the
permanent dispatch ledger and its replay horizon must be shorter than configured
Temporal retention. Callback delivery retries indefinitely, so workflow liveness
is independent of terminal task result and backlog must be monitored. A live
service worker is needed to answer running-workflow queries; restart reconstructs
state from history. Administratively terminating a workflow bypasses callback
recovery and is not the cancellation API. Before rollout, drain old demo/RFQ activities or retain a pinned legacy worker build; the new production worker only accepts `system.echo`, so a completed-history replay does not prove pending legacy task compatibility. Future external business tasks require
an idempotent authorized Java adapter before catalog admission.
