# AGENTS.md

Guidance for AI agents working on the OpenKruise project.

## Project Overview

OpenKruise is a CNCF incubating project that extends Kubernetes with advanced workload management. It runs as a set of custom controllers and CRDs on top of Kubernetes, providing five functional domains:

- **Advanced Workloads**: CloneSet, AdvancedStatefulSet, BroadcastJob, AdvancedCronJob, DaemonSet, EphemeralJob
- **Sidecar Container Management**: SidecarSet, Container Launch Priority, SidecarTerminator
- **Multi-domain Management**: WorkloadSpread, UnitedDeployment
- **Enhanced Operations**: ContainerRecreateRequest, ImagePullJob, ImageListPullJob, ResourceDistribution, PodProbeMarker, NodeImage
- **Application Protection**: DeletionProtection, PodUnavailableBudget, PersistentPodState

| Key | Value                                                                        |
|-----|------------------------------------------------------------------------------|
| Module | `github.com/openkruise/kruise`                                               |
| Language | Go 1.23+                                                                     |
| K8s Libraries | v0.32.10                                                                     |
| controller-runtime | v0.20.2                                                                      |
| controller-tools | v0.17.3                                                                      |
| API Groups | `apps.kruise.io` (workloads and operations), `policy.kruise.io` (protection) |
| K8s Compatibility | v1.18+ (v1.28+ recommended)                                                  |
| License | Apache 2.0                                                                   |

## Project Structure

```
.
├── main.go                  # kruise-manager entry point
├── apis/                    # API type definitions (CRD specs)
│   ├── apps/
│   │   ├── pub/             # Shared types (lifecycle, launch_priority, etc.)
│   │   ├── v1alpha1/        # v1alpha1 API types
│   │   └── v1beta1/         # v1beta1 API types (StatefulSet)
│   └── policy/v1alpha1/     # Policy API types (DeletionProtection)
├── pkg/
│   ├── controller/          # Controller implementations per workload type
│   ├── webhook/             # Admission webhook handlers (validating/mutating)
│   ├── daemon/              # Kruise-daemon (node agent, gRPC + CRI)
│   ├── control/             # Shared control logic (pubcontrol, sidecarcontrol)
│   ├── util/                # Utilities (expectations, feature gates, cache, etc.)
│   ├── client/              # Generated clientset, informer, lister [DO NOT EDIT]
│   └── features/            # Feature gate definitions
├── cmd/
│   ├── daemon/              # Kruise-daemon entry point
│   └── helm_hook/           # Helm hook binary
├── config/                  # Kustomize overlays (CRD, RBAC, webhook, manager)
├── test/
│   ├── e2e/                 # End-to-end tests (ginkgo/gomega)
│   └── fuzz/                # Fuzz tests
├── scripts/                 # Build and code generation scripts
├── hack/                    # Dev scripts (fmt-imports, boilerplate)
└── docs/                    # Documentation, proposals, contributing guides
```

## Commands

### Build & Generate

| Command | Description |
|---------|-------------|
| `make build` | Build `bin/manager` (runs generate + fmt + vet + manifests first) |
| `make generate` | Regenerate DeepCopy, clientset, informer, lister, OpenAPI |
| `make manifests` | Regenerate CRD YAML and RBAC manifests |
| `make generate_helm_crds` | Regenerate CRDs for Helm charts into `bin/` |
| `make docker-build` | Build Docker image `openkruise/kruise-manager:test` |
| `make docker-multiarch` | Build multi-arch image (amd64/arm64/ppc64le) |

### Test

| Command | Description |
|---------|-------------|
| `make test` | Unit tests with race detector + coverage (envtest K8s 1.32.0) |
| `make atest` | Same as `test` but skips generate/fmt/vet/manifests |
| `make kruise-e2e-test` | Full e2e: kind cluster + build + install + test + cleanup |
| `make coverage-report` | Generate `cover.html` from `cover.out` |

### Lint & Format

| Command | Description |
|---------|-------------|
| `make lint` | golangci-lint (v1.51.2, config: `.golangci.yml`) |
| `make vet` | `go vet` |
| `make fmt` | `go fmt` |
| `make fmt-imports` | goimports with local prefix grouping |
| `typos --config typos.toml` | Spell check |

## Code Style and Conventions

### Error Handling
- **Forbidden**: `github.com/pkg/errors` (enforced by depguard linter)
- **Use instead**: `fmt.Errorf("context: %w", err)` for wrapping

### Import Ordering
Use `goimports` with local prefix `github.com/openkruise/kruise`. Groups in order:
1. Standard library (`fmt`, `os`, `context`, ...)
2. Third-party (`github.com/...`)
3. Kubernetes (`k8s.io/...`, `sigs.k8s.io/...`)
4. OpenKruise (`github.com/openkruise/kruise/...`)

Blank imports must have a comment explaining the side effect.

### Boilerplate
All `.go` files must include the Apache 2.0 license header from `hack/boilerplate.go.txt`.

### Spelling
US English locale (enforced by misspell linter).

### Generated Code (DO NOT EDIT)
- `pkg/client/` — generated clientset, informer, lister
- `apis/*/zz_generated.deepcopy.go` — generated DeepCopy methods
- `apis/*/openapi_generated.go` — generated OpenAPI specs
- `config/crd/bases/` — generated CRD YAML files

Regenerate with `make generate && make manifests`.

## Architecture Patterns

### Controller
- Each workload has its own package under `pkg/controller/<workload>/` with an `Add(mgr manager.Manager) error` entry point, registered in `pkg/controller/controllers.go`
- **Reconcile loops must be idempotent**: reprocessing the same event must produce the same result. Do not assume single execution
- Use expectation tracking (`pkg/util/expectations/`) to coordinate resource creation/deletion and avoid race conditions
- Use `Status().Update()` / `Status().Patch()` for status updates, not full resource updates
- Do not perform heavy operations (locking, blocking I/O) in event handlers; move them into the reconcile loop
- Check `deletionTimestamp` and handle finalizer cleanup before applying business logic
- Use `observedGeneration` in status to track whether the controller has processed the latest spec

### Webhook
- Each workload has handlers under `pkg/webhook/<workload>/`, registered via `pkg/webhook/add_<workload>.go`
- Webhooks should only do simple mutation and validation — move heavy operations into the controller reconcile loop
- Mutating webhooks: handle CREATE and UPDATE separately
- Validating webhooks: return clear, informative error messages on rejection
- Never panic in webhook handlers; recover and return `Allowed: false`

### Daemon (Kruise-Daemon)
- Runs as a node agent (`pkg/daemon/`), handling image pulling, container recreation, and pod probes via gRPC
- Communicates with the container runtime through CRI interface only — do not talk directly to Kubelet, Docker, or Containerd
- Should only access node-local Kubernetes resources or resources in the system namespace
- Entry point: `cmd/daemon/main.go`

### Feature Gates
- New features must be gated via `pkg/features/kruise_features.go` using `utilfeature.DefaultMutableFeatureGate`
- Feature gates must have unit tests behind the gate

## Code Generation Workflow

When modifying API types under `apis/`:

```
1. Edit types in apis/apps/v1alpha1/ or apis/apps/v1beta1/
2. make generate        # DeepCopy, clientset, informer, lister, OpenAPI
3. make manifests       # CRD YAML, RBAC
```

When adding a new API type or controller:

```
1. Define types in apis/apps/v1alpha1/<type>_types.go
2. Register scheme in apis/addtoscheme_apps_v1alpha1.go (or v1beta1)
3. Register controller in pkg/controller/controllers.go
4. Register webhook in pkg/webhook/add_<type>.go
5. make generate && make manifests
```

## Common Pitfalls

- Forgetting `make generate && make manifests` after modifying API types
- Using `github.com/pkg/errors` instead of `fmt.Errorf` with `%w`
- Manually editing generated code under `pkg/client/`, `zz_generated.deepcopy.go`, or `config/crd/bases/`
- Missing license boilerplate on new `.go` files
- Import order not matching goimports local prefix convention
- Performing blocking or locking operations in controller event handlers instead of the reconcile loop
- Accessing cluster-scoped resources from daemon code
