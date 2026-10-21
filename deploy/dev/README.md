# Merchant SOP execution: dev deployment review draft

These manifests are a reviewable starting point for the existing `aurora-dev`
GitOps owner. They have not been applied. They contain no Secret resources,
credentials, live database creation, registry push, or automatic migration hooks.
Replace the Go image placeholder with a published immutable digest before use.
Final acceptance must use the existing dedicated dev merchant/user objects;
local loopback and isolated container checks are preflight evidence only.

## Image

From the repository root:

```sh
rtk proxy docker buildx build --platform linux/amd64 --load --provenance=false \
  -t aurora-go-execution:sop-preflight .
```

The multi-stage Dockerfile pins the Dockerfile frontend and Go 1.26 builder by
SHA-256, verifies `go.sum`, disables CGO and embedded build paths/VCS IDs, and
normalizes output timestamps with `SOURCE_DATE_EPOCH` (default 0). The scratch
runtime includes only the three service binaries, a small probe executable and
pinned-builder CA certificates. UID/GID 65532 needs neither a shell nor a writable
root filesystem. The probe is an executable utility, not a fourth service.
Build and validation evidence is recorded alongside this draft; the local image
has not been pushed to `192.168.0.31/aurora-helix-dev`.

## Operator prerequisites and sequence

1. Review current PostgreSQL capacity with its owner. Provision **two new
   dedicated databases**, `aurora_temporal_dev` and
   `aurora_temporal_visibility_dev`; never use business schemas. Give a separate
   schema role DDL rights only in these databases. Give the runtime role CONNECT,
   schema USAGE and required DML/sequence privileges, including default privileges
   on objects subsequently created by the schema role. Do not grant superuser or
   CREATE DATABASE. This repository does not create these live databases.
2. Provision the Secret references below through the approved secret mechanism.
   Keep schema and runtime credentials separate; do not put credentials in Git,
   image layers, shell arguments, or evidence logs. Review registry pull access.
3. Integrate the NetworkPolicies and schema Job into the actual GitOps layout.
   Execute `11-schema-job.yml` deliberately once with the pinned admin tools;
   require successful completion before starting the server. Review schema
   upgrades and backup/restore independently; the Job is not a sync hook.
4. Start `10-temporal.yml`, wait for its real gRPC health, then execute the initial
   namespace Job. It creates `aurora-merchant-sop-dev` with **7 days retention**.
   Existing namespaces deliberately make this create fail so an operator reviews
   retention instead of silently replacing it. Keep Java's authoritative dispatch
   ledger and outbox/reconciliation horizon shorter than retained history. After
   Temporal history expiry Go's dispatch-ID deduplication guarantee expires.
5. Replace the Go image placeholder, verify actual Java SOP labels/port/context,
   configure Java's gateway and machine credentials, then start `20-go.yml`.
   Run authenticated dispatch/result/callback checks and the user's existing dev
   object acceptance. A YAML client dry-run does not prove cluster scheduling,
   NetworkPolicy enforcement, service discovery or real business integration.

| Secret | Keys | Consumer |
|---|---|---|
| `aurora-temporal-schema` | `username`, `password` | explicit schema Job only |
| `aurora-temporal-runtime` | `username`, `password` | Temporal SQL persistence |
| `aurora-sop-execution-machine` | `execution-service-token`, `sop-callback-token` | Go ingress and callback activity; Java must use matching respective values |

Java configuration must target
`http://aurora-go-event-worker.aurora-dev.svc.cluster.local:8081`, send the
execution service token, and accept the callback token on
`/sop/internal/sop/service-dispatches/{id}/result`. The Go callback target is
fixed server configuration
`http://aurora-sop-service.aurora-dev.svc.cluster.local:8080/sop`.
Tokens are machine credentials; neither human Bearer nor merchant password is
passed to Go or Temporal workflow history.

## Capacity and persistence boundary

One SQL-backed Temporal pod runs frontend, history, matching and internal worker
services. Both persistence pools have max connections **1** and max idle **1**
per service/store. Four roles and two stores give an estimated eight steady SQL
connections; the isolated actual-server evidence records observed counts. This
is a deliberately small dev acceptance setting, not a throughput/capacity result.
The coordinating Java draft reserves four SOP connections, targeting at most
12 steady connections plus the short schema Job. Recheck shared PostgreSQL
headroom immediately before rollout and monitor actual `pg_stat_activity`.

The Temporal pod requests 500m CPU/1Gi RAM and limits 2 CPU/2Gi; each Go process
requests 100m/64Mi and limits 500m/256Mi. Jobs have separate short-lived budgets.
One replica with Recreate is **not HA**. State lives in the two PostgreSQL
Databases; no SQLite, NFS volume, new PVC or borrowed business volume is used.
No UI or externally published Temporal endpoint is included. Dev PostgreSQL
connection/disk headroom is a shared operator constraint; local fresh PostgreSQL
18.4 tests cannot establish its remaining disk budget or sustained workload.

## Network and authentication boundary

Temporal 1.32.0 is explicitly started with `TEMPORAL_ALLOW_NO_AUTH=true`.
Current Go Temporal clients use plaintext gRPC without server auth/TLS. This
choice is limited to a trusted internal dev network with enforcing
NetworkPolicy, controlled pod-label/namespace administration and no external
Temporal route. It provides no protection from cluster/node administrators,
allowed compromised workloads, stolen pod labels or plaintext network observers.
Before crossing this boundary add Temporal server/client mTLS or auth and
certificate/credential rotation; environment names alone do not enable TLS.

`30-networkpolicy.yml` defaults both ingress and egress to deny **only** pods
labelled `app.kubernetes.io/part-of=aurora-sop-execution`. Explicit allowances are:
DNS to actual kube-system CoreDNS; Java SOP to Go HTTP 8081; the three Go clients
and namespace Job to Temporal 7233; activity worker to Java SOP 8080; Temporal
internal TCP/membership ports to its own pods; Temporal/schema Job to the
existing `aurora` namespace PostgreSQL pods (`app=postgresql`) on 5432.
Go has no PostgreSQL or Dubbo allowance and only executes `system.echo`.

These policies do not select Java SOP, shared PostgreSQL, OA or OpenIM pods.
They preserve the Java owners' existing Auth/UC/Mall/Factory/Dubbo egress and
shared business database access. If the Java owner separately defaults SOP
ingress/egress to deny, it must allow agent callback ingress, Go dispatch egress,
its own business dependencies and kubelet health. Confirm actual selectors when
integrating; NetworkPolicy cannot restrict a permitted HTTP connection to a URL
path. Kubernetes probes and Calico behavior require actual cluster validation.

## Health probes and evidence limits

Ingress `/healthz` checks HTTP process availability. Go readiness uses the small
`health-probe worker <binary>` executable: it verifies PID 1 identity and actual
Temporal WorkflowService gRPC health within three seconds. Worker liveness only
checks PID 1 to avoid restart storms during dependency outages. The probe does
not prove task-queue polling or successful callbacks; acceptance must observe
those separately. Temporal readiness is its named WorkflowService gRPC health;
its TCP startup probe allows schema/bootstrap time. No dependency-driven Temporal
liveness restart is configured.

To reproduce the isolated SQL check after building the local Go image:

```sh
rtk proxy python3 manualqa/merchantv2/sql_images.py
```

It requires Docker plus Ruby YAML support, refuses a pre-existing owned fixture
network, and uses only a private Docker network with no published ports.

The isolated container check executes the actual schema Job script against a
fresh local PostgreSQL 18.4, starts the pinned server as non-root with read-only
root filesystem, verifies real gRPC health and namespace creation, and exercises
an actual single-task workflow while observing SQL connections. It does not use
a callback substitute and does not establish authenticated Java callback delivery;
the separate loopback Java/Go acceptance covers that wire boundary. Private
container/network resources and temporary SQL credential files are removed after
the check. See `preflight-evidence.json` and `SHA256SUMS` for the reviewed draft.
