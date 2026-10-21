# Dev Go/Temporal deployment readiness — read-only assessment

2026-10-21. This assessment changes no GitOps resource, cluster object, database,
image repository or live configuration. OA/OpenIM resources are outside this
task. The existing dev-specific test merchant/user objects remain the final
acceptance target; the local stack is preflight evidence only.

## Observed infrastructure

Verified CA/SNI Kubernetes access through the parent-provided Rancher endpoint;
no TLS verification was disabled and no Secret content was read.

| Observation | Actual read-only result |
| --- | --- |
| Namespaces | `aurora-dev`, `aurora-test`, `aurora-prod` already exist |
| Temporal reuse | No Temporal-named Service or Temporal image in Deployment/StatefulSet across all namespaces |
| Current dev runtime | No SOP or Go execution-engine Service/Deployment; GitOps has an older SOP template using `latest`, not a deployed v2 runtime |
| Nodes | Four Linux amd64 nodes, allocatable CPU 8/10/10/10 cores and about 11.7 GiB RAM each |
| Momentary usage | `kubectl top nodes`: CPU 1–13%, memory 30–67%; this is not reserved-resource or scheduling capacity evidence |
| Storage classes | `manual`: no provisioner, WaitForFirstConsumer; `nfs-client`: dynamic NFS, Immediate; no default storage class observed |
| Available PVs | Existing manual 10 GiB PostgreSQL PV is Bound; no Available PV observed |
| Existing PostgreSQL | `postgresql.aurora.svc.cluster.local:5432`, image PostgreSQL 18.4; database access/free space/Temporal compatibility were not tested in this read-only phase |
| Registry | `192.168.0.31/aurora-helix-dev`; unauthenticated `/v2/` returns 401 with authentication challenge, confirming reachability only |
| Image builder | Local Docker daemon arm64, BuildKit supports linux/amd64 |
| Go build | All three entrypoints successfully cross-built with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`; `file` identifies statically linked ELF x86-64 binaries |

The cross-built files are private preparation under
`/tmp/aurora-merchant-sop-completion-20261021/go-runtime/bin-linux/`.
No image was built, pushed, tagged or deployed in this assessment. The Go repo
currently has no Dockerfile/Jenkinsfile, so image packaging/pipeline is a concrete
remaining task. Use Go 1.26 (the module directive), a linux/amd64 builder and a
non-root static runtime with CA certificates. One image containing the three
binaries can serve three independent Deployments using distinct commands.
Pin the pushed immutable image digest in the eventual manifests.

## Recommended minimal dev topology

Use a persistent SQL-backed Temporal Server. Prefer new dedicated Temporal
history and visibility databases plus a dedicated role on the existing
PostgreSQL service, subject to DBA approval, version/schema checks and a measured
connection/disk budget. Do not reuse any business schema or business credentials.
SQL visibility avoids an additional Elasticsearch dependency. Temporal documents
PostgreSQL 12+ SQL visibility in its [visibility guide](https://docs.temporal.io/visibility).

Do not put the local CLI's SQLite database on `nfs-client`. SQLite explicitly says
WAL does not work on a network filesystem in its [WAL documentation](https://www.sqlite.org/wal.html).
The local CLI dev server is appropriate for preflight. For sustained dev service,
use `temporalio/server` with matching schema tools, as described by the official
[self-hosted deployment guide](https://docs.temporal.io/self-hosted-guide/deployment).
If new dedicated PostgreSQL databases are unavailable, provision a dedicated
block/local storage path and evaluate its recovery model first; no suitable free
PV currently exists. Do not borrow the existing PostgreSQL or OpenIM volumes.

For the smallest non-HA dev footprint, one SQL-backed Temporal Server pod can
host frontend/history/matching/internal-worker roles; choose and validate its
single-pod service binding/configuration with the pinned release before rollout.
This is a dev availability tradeoff. The standard Helm topology separates those
roles; it costs more pods and is the preferred later scaling/availability path.
Temporal 1.30+ requires compatible Helm chart configuration (0.73.1+); do not
mix arbitrary chart/image tags. No Helm values or server version have been
selected/validated against the shared PostgreSQL 18.4 in this phase.

Suggested initial resource budgets, to be measured during acceptance:

| Component, replicas | Request CPU / memory | Limit CPU / memory | Service / probe |
| --- | --- | --- | --- |
| Temporal SQL server, 1 | 500m / 1Gi | 2 / 2Gi | ClusterIP 7233, actual gRPC health; startup grace for schema readiness |
| Go event-worker, 1 | 100m / 64Mi | 500m / 256Mi | ClusterIP 8081 → 8081; `/healthz` |
| Go service-worker, 1 | 100m / 64Mi | 500m / 256Mi | No Service/HTTP; process check plus Temporal task-queue poller observation |
| Go agent-executor, 1 | 100m / 64Mi | 500m / 256Mi | No Service/HTTP; process check plus Temporal task-queue poller observation |
| Temporal UI | Omit initially | — | If required later, internal ClusterIP 8080; no public ingress |

These are proposed low-volume starting values, not load-test results. Go
`/healthz` is a process/HTTP probe and does not establish Temporal or Java callback
readiness. Verify those separately. Allow graceful termination for workers and
callback delivery, and avoid a liveness probe that restarts them during a
temporary Temporal/database outage. Go pods need no PVC. Set explicit bounded
ephemeral storage, `automountServiceAccountToken: false`, non-root UID and a
read-only root filesystem.

## Manifest and configuration contract

The existing GitOps AGENTS requires one syncable directory per service,
`<environment>/<source-project>/<service-name>/deploy.yml`, explicit namespace,
matching labels, probes/resources, Secret references and no business Namespace
objects. Recommended future paths (not created here):

- `dev/aurora-platform/aurora-temporal/deploy.yml`: Temporal Deployment/Service, configuration and schema/bootstrap Job references.
- `dev/aurora-helix-go-execution-engine/aurora-go-event-worker/deploy.yml`: ingress Deployment/Service.
- `dev/aurora-helix-go-execution-engine/aurora-go-service-worker/deploy.yml`: workflow Deployment.
- `dev/aurora-helix-go-execution-engine/aurora-go-agent-executor/deploy.yml`: activity Deployment.

All Go Deployments:

```text
TEMPORAL_HOST_PORT=aurora-temporal.aurora-dev.svc.cluster.local:7233
TEMPORAL_NAMESPACE=aurora-merchant-sop-dev
```

Create that Temporal namespace explicitly, with a documented history retention
greater than the Java dispatch replay horizon. History/memo deduplication expires
after Temporal retention; this is not indefinite exactly-once execution.

Go ingress:

```text
HTTP_HOST=0.0.0.0
PORT=8081
EXECUTION_SERVICE_TOKEN=<Secret key shared with Java ingress client>
```

Go agent executor:

```text
SOP_BASE_URL=http://aurora-sop-service.aurora-dev.svc.cluster.local:8080/sop
SOP_SERVICE_TOKEN=<Secret key accepted only by Java SOP result callback>
```

Java SOP:

```text
EXECUTION_SERVICE_BASE_URL=http://aurora-go-event-worker.aurora-dev.svc.cluster.local:8081
EXECUTION_SERVICE_TOKEN=<same ingress Secret key>
SOP_SERVICE_TOKEN=<same callback Secret key>
```

Use fresh dev-specific machine credentials; local preflight files do not become
dev secrets. No human Bearer enters Go input/history. The current Temporal
client supports host/namespace only, without TLS/auth configuration. Therefore
the minimum current dev boundary is internal ClusterIP plus restrictive
NetworkPolicies: Go pods → Temporal 7233; Temporal → dedicated PostgreSQL 5432;
Java SOP → Go ingress 8081; Go activity worker → Java SOP 8080; explicitly allow
DNS and Temporal internal membership traffic. No NodePort/Ingress is needed for
Go or Temporal. Protect callback/dispatch paths from unrelated pods; do not alter
existing OA/OpenIM policies. External Temporal machine IAM/mTLS support would be
a separate client/config change if the environment requires it.

## Remaining concrete work before deployment

1. Approve/create dedicated Temporal databases/role using a private channel, confirm PostgreSQL 18.4 compatibility and free disk/connection budget, and run version-matched schema setup/migrations as a controlled Job. Runtime roles must not hold business DB authority.
2. Select/pin server/admin-tool images and render/validate SQL/server bindings; retain a tested namespace/retention bootstrap action. The old auto-setup examples are not a substitute for a verified schema lifecycle.
3. Package/sign or otherwise verify the linux/amd64 Go image, authenticate a publisher privately, push to the requested registry and record its digest. Current registry check proves neither push permission nor successful pull of a new image.
4. Prepare service-scoped GitOps manifests/Argo Applications and new machine Secret references. Root must coordinate those writes/deployment separately.
5. Deploy Java SOP v2 with its own DB migrations/Auth/candidate integrations and correct gateway routes; existing SOP `latest` template lacks this contract.
6. Exercise the existing dev-specific test merchant/account through real Gateway/Auth: dispatch, completed replay/changed-input conflict, callback failure/retry, cancellation and worker restart recovery. Local preflight does not satisfy this final dev acceptance.

No database, registry, GitOps or cluster mutation was performed for this report.

## Follow-up packaging and isolated verification

The subsequent Go-owned delivery now supplies `Dockerfile`, the static probe
utility and the review draft in `deploy/dev/`; see that directory's README and
preflight evidence for pinned images, explicit schema/namespace jobs, all three
Go Deployments and restrictive self-owned NetworkPolicies. Earlier discovery
findings above describe the repository/cluster before this delivery. Nothing has
been pushed or deployed by this lane. Actual local PostgreSQL 18.4/Temporal 1.32
checks passed with all SQL pool caps at 1 and namespace retention 7d; observed
connections after a completed task were eight (four per store). Shared dev
capacity and final existing-object acceptance remain operator-owned gates.
