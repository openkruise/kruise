# Agent Prompt: Promote PersistentPodState from v1alpha1 to v1beta1

You are promoting **PersistentPodState** (`persistentpodstates.apps.kruise.io`) from `v1alpha1` to `v1beta1` in [openkruise/kruise](https://github.com/openkruise/kruise).

**This is NOT a small task.** A promotion is only complete when every section below is done, verified, and tests pass. Do not stop after API types or CRD YAML — agents consistently miss controller/webhook/client/e2e/conversion-webhook wiring.

Before starting, read these merged promotion PRs as reference:


| Resource                     | PR                                                      | Notes                                                               |
| ---------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------- |
| CloneSet                     | [#2225](https://github.com/openkruise/kruise/pull/2225) | Full controller + webhook + e2e                                     |
| SidecarSet                   | [#2222](https://github.com/openkruise/kruise/pull/2222) | Pod webhook cross-references                                        |
| DaemonSet                    | [#2211](https://github.com/openkruise/kruise/pull/2211) | Validating webhook dual-version decode                              |
| ImagePullJob/NodeImage       | [#2201](https://github.com/openkruise/kruise/pull/2201) | Multi-resource promotion                                            |
| BroadcastJob/AdvancedCronJob | [#2186](https://github.com/openkruise/kruise/pull/2186) | CRD webhook patches baseline                                        |
| UnitedDeployment             | [#2396](https://github.com/openkruise/kruise/pull/2396) | Validating: decode both versions → single v1beta1 validate fn       |
| PodUnavailableBudget         | [#2397](https://github.com/openkruise/kruise/pull/2397) | Policy group promotion pattern                                      |
| ResourceDistribution         | [#2445](https://github.com/openkruise/kruise/pull/2445) | **Conversion webhook patch was commented out — caused E2E failure** |
| ContainerRecreateRequest     | (in-repo branch)                                        | Recent apps-group promotion with typed status                       |


---

## Resource metadata


| Field                  | Value                                                             |
| ---------------------- | ----------------------------------------------------------------- |
| Kind                   | `PersistentPodState`                                              |
| Plural                 | `persistentpodstates`                                             |
| Singular               | `persistentpodstate`                                              |
| API group              | `apps.kruise.io`                                                  |
| Short name             | *(none)*                                                          |
| Scope                  | Namespaced                                                        |
| Has status subresource | Yes                                                               |
| Has mutating webhook   | No (validating only)                                              |
| Has daemon             | No                                                                |
| Cross-references       | `pkg/webhook/pod/mutating/persistent_pod_state.go` reads PPS spec |


---

## Maintainer-approved API change for v1beta1

Rename field inside `NodeTopologyTerm`:

```go
// v1alpha1
type NodeTopologyTerm struct {
    NodeTopologyKeys []string `json:"nodeTopologyKeys"`
}

// v1beta1 — "Node" is redundant inside NodeTopologyTerm (upstream K8s pattern)
type NodeTopologyTerm struct {
    Keys []string `json:"keys"`
}
```

After rename, YAML looks like:

```yaml
spec:
  requiredPersistentTopology:
    keys:
    - kubernetes.io/hostname
  preferredPersistentTopology:
  - weight: 100
    preference:
      keys:
      - topology.kubernetes.io/zone
```

**Conversion MUST map explicitly in both directions:**

- `ConvertTo`: `dst.Keys = src.NodeTopologyKeys`
- `ConvertFrom`: `dst.NodeTopologyKeys = src.Keys`

Do **not** use blind `DeepCopy` for conversion.

---

## Step 0 — Understand what promotion means

`v1beta1` is the **Hub / storage version** in etcd. `v1alpha1` objects are converted on read/write via a CRD conversion webhook.

- API server calls `ConvertTo` / `ConvertFrom` for every stored v1alpha1 object
- Field renames **require** conversion functions — no exceptions
- CRD must have `conversion.strategy: Webhook` — verify patch is **uncommented** in `config/crd/kustomization.yaml`
- Unit tests calling Go conversion functions directly **do not** prove the webhook works in-cluster

---

## Step 1 — Define v1beta1 types

Create `apis/apps/v1beta1/persistent_pod_state_types.go`:

- Copy v1alpha1 types as base
- Apply `NodeTopologyKeys` → `Keys` rename with updated JSON tags
- Keep annotation constants in v1alpha1 (they are used by workload annotations, not PPS CRD spec)
- Add `// +kubebuilder:storageversion` on `PersistentPodState`
- Add `// +kubebuilder:resource` and `// +kubebuilder:subresource:status` (match v1alpha1)
- Add `// +kubebuilder:validation:Enum=WhenScaled;WhenDeleted` on `PersistentPodStateRetentionPolicyType`
- Add `// +kubebuilder:validation:Minimum=0` on `PreferredTopologyTerm.Weight` if applicable
- Register with `SchemeBuilder.Register(&PersistentPodState{}, &PersistentPodStateList{})`

Create `apis/apps/v1beta1/persistent_pod_state_conversion.go`:

```go
func (in *PersistentPodState) Hub() {}
```

Nothing else in the v1beta1 conversion file.

Ensure types are registered in `apis/apps/v1beta1/groupversion_info.go` via `init()` in types file.

---

## Step 2 — Write conversion functions

Create `apis/apps/v1alpha1/persistent_pod_state_conversion.go`:

```go
func (src *PersistentPodState) ConvertTo(dstRaw conversion.Hub) error { ... }
func (dst *PersistentPodState) ConvertFrom(srcRaw conversion.Hub) error { ... }
```

Map **every** spec and status field explicitly:


| Field                                    | Notes                                                     |
| ---------------------------------------- | --------------------------------------------------------- |
| `Spec.TargetReference`                   | Direct copy                                               |
| `Spec.PersistentPodAnnotations`          | Direct copy                                               |
| `Spec.RequiredPersistentTopology`        | **Rename keys**                                           |
| `Spec.PreferredPersistentTopology`       | **Rename keys in each preference**                        |
| `Spec.PersistentPodStateRetentionPolicy` | Direct copy                                               |
| `Status.ObservedGeneration`              | Direct copy                                               |
| `Status.PodStates`                       | Deep copy map (nodeName, nodeTopologyLabels, annotations) |


Write tests in `apis/apps/v1alpha1/persistent_pod_state_conversion_test.go`:

- `TestPersistentPodState_ConvertTo` — table-driven: all fields, empty/nil topology, multiple preferred terms
- `TestPersistentPodState_ConvertFrom` — table-driven: all fields, keys round-trip
- `TestPersistentPodState_RoundTrip` — v1alpha1 → v1beta1 → v1alpha1, no data loss
- `FuzzPersistentPodStateConversion` — fuzz test (see RD/PUB/UD patterns)

---

## Step 3 — Regenerate everything

```bash
make manifests   # CRD YAML from +kubebuilder markers
make generate    # DeepCopy, clientset, informer, lister, OpenAPI
```

Never hand-edit:

- `apis/apps/v1beta1/zz_generated.deepcopy.go`
- `pkg/client/**`
- `config/crd/bases/apps.kruise.io_persistentpodstates.yaml`

---

## Step 4 — CRD and conversion webhook (CRITICAL)

### 4.1 CRD YAML

`config/crd/bases/apps.kruise.io_persistentpodstates.yaml` must have **both** versions:

```yaml
versions:
- name: v1alpha1
  served: true
  storage: false
  schema: ...
- name: v1beta1
  served: true
  storage: true
  schema: ...
```

v1beta1 schema must use `keys` not `nodeTopologyKeys`.

### 4.2 Conversion webhook patch

Create `config/crd/patches/webhook_in_persistentpodstates.yaml` (copy from `webhook_in_containerrecreaterequests.yaml`, change resource name).

**Uncomment** in `config/crd/kustomization.yaml`:

```yaml
- patches/webhook_in_persistentpodstates.yaml
```

If this line stays commented, conversion silently breaks at runtime — this caused multi-hour E2E debugging in RD PR#2445.

### 4.3 Validating webhook marker

Update `pkg/webhook/persistentpodstate/validating/webhooks.go`:

- Add second `+kubebuilder:webhook` marker for `versions=v1beta1`
- Update `config/webhook/manifests.yaml` (regenerated via `make manifests` or controller-gen)

---

## Step 5 — Generated client / informer / lister

After `make generate`, verify these exist and are wired:


| File                                                                                | Check                                                  |
| ----------------------------------------------------------------------------------- | ------------------------------------------------------ |
| `pkg/client/clientset/versioned/typed/apps/v1beta1/persistentpodstate.go`           | Get/List/Watch/Create/Update/UpdateStatus/Delete/Patch |
| `pkg/client/clientset/versioned/typed/apps/v1beta1/apps_client.go`                  | `PersistentPodStates()` method                         |
| `pkg/client/clientset/versioned/typed/apps/v1beta1/fake/fake_persistentpodstate.go` | Fake client                                            |
| `pkg/client/informers/externalversions/apps/v1beta1/persistentpodstate.go`          | Informer                                               |
| `pkg/client/informers/externalversions/apps/v1beta1/interface.go`                   | `PersistentPodStates()`                                |
| `pkg/client/informers/externalversions/generic.go`                                  | case for `persistentpodstates`                         |
| `pkg/client/listers/apps/v1beta1/persistentpodstate.go`                             | Lister                                                 |
| `pkg/client/listers/apps/v1beta1/expansion_generated.go`                            | Expansion interfaces                                   |


**Common mistake:** `generic.go` imports a package that doesn't exist yet → build failure.

---

## Step 6 — Update controller

Files to update (currently all v1alpha1):


| File                                                                        | Changes                                                                                                                     |
| --------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `pkg/controller/persistentpodstate/persistent_pod_state_controller.go`      | Import `appsv1beta1`; `KruiseKindPps` uses v1beta1 GVK; reconcile reads/writes v1beta1; replace `NodeTopologyKeys` → `Keys` |
| `pkg/controller/persistentpodstate/persistent_pod_state_event_handler.go`   | v1beta1 lister/client                                                                                                       |
| `pkg/controller/persistentpodstate/persistent_pod_state_controller_test.go` | v1beta1 types; use `Keys` field                                                                                             |


Key controller references to update:

- `persistentPodState.Spec.RequiredPersistentTopology.NodeTopologyKeys` → `.Keys`
- `item.Preference.NodeTopologyKeys` → `.Keys`
- Auto-generate PPS from workload annotations still creates v1beta1 objects

---

## Step 7 — Update webhooks

### 7.1 Validating webhook (follow UnitedDeployment PR#2396)

File: `pkg/webhook/persistentpodstate/validating/persistent_create_update_handler.go`

- `decodeObject` / `decodeOldObject`: decode **both** v1alpha1 and v1beta1 requests into `*appsv1beta1.PersistentPodState`
- Single `validatePersistentPodState(obj, old *appsv1beta1.PersistentPodState)` — no version branches
- No `strict bool` flag — same rules for both versions
- Update field paths in validation to use `.Keys`

### 7.2 Pod mutating webhook (cross-reference)

File: `pkg/webhook/pod/mutating/persistent_pod_state.go`

- Reads PPS spec to inject node affinity — update to v1beta1 client/types
- Replace `NodeTopologyKeys` → `Keys`
- Update `pkg/webhook/pod/mutating/persistent_pod_state_test.go`

---

## Step 8 — Tests (comprehensive — do not skip)

### 8.1 Conversion tests

`apis/apps/v1alpha1/persistent_pod_state_conversion_test.go`:

- All spec fields populated
- Required + preferred topology with multiple keys
- Empty/nil topology terms
- Full status with multiple PodStates
- Round-trip preserves all v1alpha1-compatible fields
- Fuzz conversion test

### 8.2 Validating webhook tests

Rewrite/extend `pkg/webhook/persistentpodstate/validating/persistent_validating_test.go`:

- Table-driven validation cases using v1beta1 types
- New `persistent_create_update_handler_test.go` (UD pattern):
  - Real JSON admission requests for v1alpha1 create
  - Real JSON admission requests for v1beta1 create
  - Update immutability rejection (both versions)
  - Duplicate PPS rejection
  - Invalid targetRef rejection
  - Custom workload whitelist validation

### 8.3 Controller tests

Update `pkg/controller/persistentpodstate/persistent_pod_state_controller_test.go`:

- All struct literals use v1beta1 + `Keys`
- Test topology key collection logic
- Test auto-generate from workload annotations
- Test retention policy (WhenScaled / WhenDeleted)

### 8.4 Pod webhook tests

Update `pkg/webhook/pod/mutating/persistent_pod_state_test.go`:

- v1beta1 PPS objects with `Keys` field
- Node affinity injection from required/preferred topology

### 8.5 E2E framework

Create `test/e2e/framework/v1beta1/persistent_pod_state_util.go`:

- Mirror `test/e2e/framework/v1alpha1/persistent_pod_state_util.go`
- Use `kc.AppsV1beta1().PersistentPodStates(...)`
- Helper: `CreatePPSViaV1alpha1ReadViaV1beta1()` for cross-version test

### 8.6 E2E tests

Create `test/e2e/apps/v1beta1/persistent_pod_state.go`:

Mirror and extend v1alpha1 scenarios:


| Scenario                        | Description                                                                                                       |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| `statefulset node topology`     | Auto-generate PPS from kruise StatefulSet annotations; verify podStates, node affinity, scale down/up persistence |
| `statefulsetlike node topology` | Custom workload via dynamic watch whitelist                                                                       |
| Cross-version                   | Create PPS as v1alpha1 with `nodeTopologyKeys`; read as v1beta1; verify `keys` populated                          |
| Validation webhook              | Duplicate PPS rejected                                                                                            |
| Retention WhenDeleted           | PPS deleted when workload deleted                                                                                 |
| Persistent pod annotations      | Annotations preserved across rebuild                                                                              |
| Preferred topology weight       | Weighted node affinity applied                                                                                    |


Register suite: ensure ginkgo imports include v1beta1 test package.

---

## Step 9 — Run before pushing

```bash
go build ./...
go test ./apis/apps/v1alpha1/... -run PersistentPodState
go test ./pkg/webhook/persistentpodstate/...
go test ./pkg/webhook/pod/mutating/... -run PersistentPodState
go test ./pkg/controller/persistentpodstate/...
gofmt -l ./apis/... ./pkg/... ./test/...
make lint   # if available
```

Optional but recommended — in-cluster conversion verification:

```bash
make docker-build
kind load docker-image openkruise/kruise-manager:test
kubectl apply -f config/crd/bases/apps.kruise.io_persistentpodstates.yaml
# Create v1alpha1 object, read via v1beta1 client, verify keys field
```

---

## Completion checklist — DO NOT STOP UNTIL ALL PASS

Use this as a gate. Mark each item only after reading/grepping the actual file.

### API & conversion

- `apis/apps/v1beta1/persistent_pod_state_types.go` exists with `+kubebuilder:storageversion`
- `NodeTopologyTerm.Keys` (not `NodeTopologyKeys`) in v1beta1
- `apis/apps/v1beta1/persistent_pod_state_conversion.go` has only `Hub()`
- `apis/apps/v1alpha1/persistent_pod_state_conversion.go` has ConvertTo/ConvertFrom with explicit key mapping
- Conversion tests + fuzz test pass

### CRD

- CRD has v1alpha1 (storage: false) AND v1beta1 (storage: true)
- CRD has `conversion.strategy: Webhook`
- `config/crd/patches/webhook_in_persistentpodstates.yaml` exists
- Patch line **uncommented** in `config/crd/kustomization.yaml`

### Generated code

- `make generate && make manifests` run; no hand-edits in generated files
- v1beta1 clientset/informer/lister exist
- `generic.go` case for `persistentpodstates` compiles

### Runtime code

- Controller uses v1beta1 exclusively
- All `NodeTopologyKeys` references in controller updated to `Keys`
- Validating webhook decodes both versions → single v1beta1 validate
- Validating webhook has v1beta1 marker
- Pod mutating webhook uses v1beta1 PPS client/types

### Tests

- Conversion unit tests (table + fuzz)
- Validating webhook tests (v1alpha1 + v1beta1 admission requests)
- Controller unit tests updated
- Pod webhook unit tests updated
- E2E framework v1beta1 helpers
- E2E test file with cross-version scenario
- `go build ./...` clean
- All targeted `go test` packages pass

---

## Common mistakes (from prior promotions)


| Mistake                                                      | Where caught        |
| ------------------------------------------------------------ | ------------------- |
| Conversion webhook patch commented out in kustomization.yaml | RD PR#2445 E2E      |
| CRD only has v1alpha1 — promotion is a no-op                 | Audit               |
| `generic.go` import of non-existent package                  | RD PR#2445 build    |
| `zz_generated.deepcopy.go` not regenerated                   | RD PR#2445 compile  |
| `strict` flag with stricter v1beta1 validation               | RD PR#2445 review   |
| Missing `+kubebuilder:validation:Enum` on enum types         | PUB PR#2447         |
| Missing `+kubebuilder:validation:Minimum` on integers        | PUB PR#2447         |
| `make manifests` not run after adding markers                | PUB PR#2447         |
| Copyright year wrong on new files (use current year)         | RD PR#2445          |
| Stopping after API types without controller/webhook/e2e      | Every agent mistake |


---

## Audit mode (run before opening PR)

When asked to audit, run EVERY check below against the actual codebase. Report PASS / FAIL / MISSING for each. Do not stop at first failure.

```
Resource: PersistentPodState
Plural: persistentpodstates
```

### Section 1 — API types

1.1 `apis/apps/v1beta1/persistent_pod_state_types.go`: storageversion, resource, subresource:status, SchemeBuilder.Register, Keys field

1.2 `apis/apps/v1beta1/persistent_pod_state_conversion.go`: only Hub()

1.3 `apis/apps/v1alpha1/persistent_pod_state_conversion.go`: ConvertTo/ConvertFrom with explicit Keys ↔ NodeTopologyKeys mapping

### Section 2 — DeepCopy

2.1 Every v1beta1 struct has DeepCopy in `zz_generated.deepcopy.go`

### Section 3 — CRD (CRITICAL)

3.1 Both versions in CRD YAML with correct storage flags
3.2 conversion.strategy: Webhook present
3.3 webhook patch file exists
3.4 kustomization.yaml patch uncommented

### Section 4 — Client/informer/lister

4.1–4.9 All v1beta1 client files exist and wired (see Step 5 table)

### Section 5 — Controller

5.1 Controller imports v1beta1, uses v1beta1 GVK, reads/writes v1beta1, uses Keys
5.2 Event handler uses v1beta1

### Section 6 — Webhooks

6.1 Validating webhooks.go: two markers (v1alpha1 + v1beta1)
6.2 Handler decodes both versions to v1beta1, single validate fn
6.3 Pod mutating webhook updated to v1beta1 + Keys

### Section 7 — Tests

7.1 Conversion tests (ConvertTo, ConvertFrom, RoundTrip, Fuzz)
7.2 Webhook tests with real admission JSON (both versions)
7.3 Controller tests with v1beta1
7.4 E2E framework v1beta1
7.5 E2E tests including cross-version
7.6 E2E registered in suite

### Section 8 — Build

8.1 `go build ./...` passes
8.2 `gofmt -l` clean

Print summary: PASS: N  FAIL: N  MISSING: N. List CRITICAL FAILURES (3.x, 7.x). Ask user which failures to fix first.

---

## Definition of done

The promotion is **complete** only when:

1. A v1alpha1 PPS with `nodeTopologyKeys` stored in etcd converts to v1beta1 with `keys` via the API server webhook
2. Controller reconciles v1beta1 objects and persists pod topology across rebuilds
3. Validating webhook accepts/rejects identically for both API versions
4. Pod mutating webhook injects affinity from v1beta1 PPS spec
5. All unit tests and e2e tests pass
6. Audit checklist shows zero CRITICAL failures

**Do not open a PR or report completion until all six conditions are met.**