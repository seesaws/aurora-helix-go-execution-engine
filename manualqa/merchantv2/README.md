# Isolated merchant SOP completion runtime

This helper owns only the Go/Temporal portion of the 2026-10-21 local completion
acceptance. It starts real Temporal with a private persistent SQLite database,
then the existing three Go processes. It creates no callback substitute, Auth
substitute, Nacos registration, shared database access or business writes.
The parent acceptance task owns real isolated Java/Auth services and all business
requests. No credentials are stored in this repository.

The fixed private directory is
`/tmp/aurora-merchant-sop-completion-20261021/go-runtime` (mode 700):

- `bin/`: freshly built Go entrypoints and a private copy of the cached Temporal CLI.
- `temporal/temporal.sqlite`: dedicated persistent Temporal state; never a shared server's database.
- `secrets/execution-service-token`: machine Bearer for Java → Go ingress, mode 600.
- `secrets/sop-callback-token`: machine Bearer for Go → Java callback, mode 600.
- `processes.json`, `logs/`, `configuration.json`: owned PIDs, private logs and configuration without credentials.

From the Go repository, prepare using an existing real Temporal CLI path:

```sh
rtk proxy python3 manualqa/merchantv2/runtime.py prepare --temporal-cli /absolute/path/to/temporal
rtk proxy python3 manualqa/merchantv2/runtime.py temporal
```

Preparation refuses occupied dedicated ports. Temporal binds gRPC
`127.0.0.1:17243` and UI `127.0.0.1:18243`; CLI-created internal listeners also
bind loopback. Its actual cluster health check must succeed before readiness is
reported. No Go workers start during this phase.

Configure the isolated Java SOP process with:

- `EXECUTION_SERVICE_BASE_URL=http://127.0.0.1:18096`.
- `EXECUTION_SERVICE_TOKEN`: read the private execution-service-token file.
- `SOP_SERVICE_TOKEN`: read the private sop-callback-token file.
- Real isolated Auth/UC configuration from the parent runner. These human identity endpoints and tokens do not enter Go workflow input.

Do not print the token files or embed their values in command arguments. They are
distinct synthetic machine credentials, scoped by the fixed Go route surface
and fixed Java callback surface, rather than human access tokens. Local direct
machine-token validation does not establish external machine IAM integration.

After the parent reports real Java SOP ready at `127.0.0.1:59203`, start workers:

```sh
rtk proxy python3 manualqa/merchantv2/runtime.py workers --callback-base-url http://127.0.0.1:59203/sop
rtk proxy python3 manualqa/merchantv2/runtime.py status
```

The resulting Go ingress binds `127.0.0.1:18096`; `/healthz` must return 200.
The production executor accepts only `system.echo`. The configured callback
prefix produces
`http://127.0.0.1:59203/sop/internal/sop/service-dispatches/{id}/result`.
Secrets are read into subprocess environments, never workflow input or history.
Business acceptance remains in the Java parent runner's
`deploy/local/merchant-sop-completion/acceptance.py`.

For cleanup:

```sh
rtk proxy python3 manualqa/merchantv2/runtime.py stop
```

The helper checks each recorded PID's command before signalling its process
group, stops only its own processes, and checks the three dedicated ports. It
writes private `cleanup.json`. SQLite, evidence and credential files remain in
the private task directory for the parent to review and remove; the helper never
removes sibling task files or stops unrelated listeners.

Preparation evidence (2026-10-21): all three Go binaries built, real Temporal
cluster health succeeded, persistent SQLite exists, credential files have mode
600, and all actual listeners of Temporal PID 84050 were loopback. At this stage
Go 18096 remains free pending the parent Java readiness signal. Runtime state
and final acceptance evidence take precedence over this initial preparation
snapshot.

## Isolated SQL-backed image preflight

After building `aurora-go-execution:sop-preflight` with the repository Dockerfile:

```sh
rtk proxy python3 manualqa/merchantv2/sql_images.py
```

This separate helper creates a fresh local PostgreSQL 18.4 and pinned Temporal
1.32 on an internal Docker network, publishes no ports and never connects to
dev/shared PostgreSQL. It initializes two dedicated schemas with the actual
review Job script, verifies non-root read-only server health, creates a 7d
namespace, completes a legacy-branch echo workflow without a callback substitute,
and records actual connection counts with per-store pool caps of one. It removes
its owned containers/network and temporary SQL credential files in `finally`.
The existing native loopback fixture is unaffected. See `deploy/dev/README.md`
for the security boundary and remaining dev acceptance requirements.

Actual loopback callback recovery (2026-10-21): after Java's callback-token
validation fix, original dispatch `2d0a3936-ccda-47f9-afc8-5e41de3ebd4f`
completed in its original Temporal run at `2026-10-21T02:51:30.456597Z`.
The callback activity's eleventh natural attempt succeeded after prior HTTP 401
responses. No administrative reset, synthetic callback, database edit or Go
restart was used. This establishes isolated wire recovery, not final dev
existing-object acceptance or external machine IAM.
