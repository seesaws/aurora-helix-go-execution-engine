# Go image CI

The Go repository had no checked-in CI pipeline. The Java release Jenkinsfile
has a Java-only service list and Maven build, so it cannot build this repository.
The root `Jenkinsfile` adds a separate **dev image publication** pipeline. No job
has been created or run by this change, and no source/image has been pushed.

Configure a Pipeline-from-SCM job using the reviewed Go revision and script path
`Jenkinsfile`, with the existing remote:

```text
git@github.com:seesaws/aurora-helix-go-execution-engine.git
```

The pipeline reuses that job's single SCM credential reference and enforces the
exact remote; it does not accept arbitrary repository URLs or credential IDs
from build parameters. The Java job's SSH credential reference is
`687e7a60-3502-45cd-bc95-ae478ec5f414`, used there for Codeup. Its authorization to
this GitHub repository is **unverified**; select a credential with confirmed Go
read access. Retain normal Jenkins SSH host-key verification. Do not read/export
credential values or disable host-key checks to make checkout pass.

Use an isolated Linux amd64 Jenkins worker with Docker CLI/daemon, Buildx, Git
plugin (`scmGit`), Docker permissions and Credentials Binding. This job can run
trusted reviewed source with Docker access; it is not a sandbox for untrusted
pull requests. The worker needs network access to the Git remote, pinned public
builder/frontend images, Go modules and the dev Harbor registry. Follow existing
node registry trust/insecure-registry configuration; the pipeline changes none.
It does not need Maven, kubectl, GitOps credentials or runtime machine tokens.

Provide these parameters explicitly:

| Parameter | Value |
|---|---|
| `SOURCE_REF` | Reviewed, reachable full lowercase 40-character commit SHA |
| `EXPECTED_COMMIT` | Exactly the same full SHA |
| `BUILD_ONLY` | `true` (default and only accepted mode) |

`BUILD_ONLY` follows the Java release job's meaning: **test, build and push the
image without deploying**. It is not a dry run. Setting it false fails before
checkout. Pipeline/job setup and the actual publication build require a separate
operator action; merely committing this file triggers neither action.

The pipeline rejects missing/mismatched SHA parameters, checks the actual
checkout before any compilation/login, runs `go mod verify`, unit tests and the
race detector in the same digest-pinned Go builder as the Dockerfile, then builds
the existing Dockerfile with amd64, fixed epoch and provenance disabled. The
result is checked for non-root UID/GID 65532 and a read-only, network-isolated
process probe. This is an image smoke check; it does not exercise real Temporal,
Java callbacks, deployed NetworkPolicies or business acceptance.

Publication reuses the existing Java Harbor username/password credential ID
`d0278fbe-ed06-43e6-82d3-7c622f896195`, scoped only to the push stage. It targets
only `192.168.0.31/aurora-helix-dev/aurora-go-execution`, with tag
`git-<full-source-sha>-<build-number>`. Password input uses stdin with shell tracing
disabled; temporary Docker login configuration is removed by a trap and never
archived. Verify that the Jenkins credential has the intended project push scope;
its presence in the Java file does not prove current access.

Successful builds archive `image-digest.txt`, `image-manifest.json`, the local
image ID and Dockerfile SHA with Jenkins fingerprints. The repository-qualified
`@sha256:` digest is the deployment input; a tag alone is insufficient. Build
metadata maps the reviewed commit to the digest without embedding credentials.
There is no GitOps checkout/write, Argo sync, Kubernetes call, schema migration,
runtime secret handling or deployment stage. Security/authentication filters and
running local workflows remain untouched.

Local validation passed Groovy compilation/AST parsing, four shell syntax checks,
six SHA/mode guard cases and equality of the CI/Dockerfile builder pins. The
pipeline was not executed. Validation remains static until a Jenkins
validation/build record is provided separately; these checks do not prove
Jenkins plugin availability, sandbox approval, GitHub checkout access, registry
push permission or runtime image pullability from the cluster.
