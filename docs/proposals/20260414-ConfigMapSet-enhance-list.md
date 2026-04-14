# ConfigMapSet Incremental Change Proposal Notes

## Document Purpose

This document compares the following two proposal versions and provides a reviewer-oriented summary of incremental changes, focusing on design improvements, semantic convergence, and implementation value introduced by the new proposal compared with the old one.

- Old proposal: `20250306-ConfigMapSet.md`
- New proposal: `20250306-ConfigMapSet-enhance.md`

This document does not repeat the full solution. It only describes the key changes in terms of what was added, adjusted, converged, or removed.

---

## I. Overall Conclusion

Compared with the old proposal, the biggest change in the new proposal is not simply “adding more implementation details,” but upgrading the solution from a controller-internal design to a more productized, user-configurable solution. The overall improvements are reflected in the following aspects:

1. **Clearer capability abstraction**: The old theme of “ConfigMap management + sidecar injection + restart logic” is decomposed into clearer capability modules such as version management, container selection, reload sidecar injection, configuration effect strategy, and rollback.
2. **More complete user-facing interface**: Key fields such as `reloadSidecarConfig`, `effectPolicy`, and `matchLabelKeys` are added, turning the solution from a single-path design into a composable multi-mode capability.
3. **Richer configuration effect mechanisms**: The old proposal mainly focused on “restart to take effect,” while the new proposal expands this into three models: `ReStart`, `PostHook`, and `HotUpdate`, covering more business scenarios.
4. **More complete release and rollback closed loop**: The new proposal explicitly fills in rollback flow, stacked release, uninstall flow, and scenario constraints, making the design more reviewable and more actionable for implementation.
5. **More reasonable platform collaboration boundary**: Many implementation details in the old proposal that were tightly coupled with Daemon, Webhook, Pod annotations, and cold-start decisions are deemphasized, while CRD semantics and control-plane behavior are emphasized instead, reducing design-stage coupling.

---

## II. Key Improvements

## 1. Reload Sidecar evolved from “fixed injection” to “multi-mode injection”

### Old proposal

The old proposal injects a special sidecar (`reload-sidecar`) by default through an admission webhook. The design essentially follows only one path:

- inject `emptyDir`
- inject `reload-sidecar`
- let `reload-sidecar` parse the RMC
- make configuration take effect by restarting the sidecar / business container

This approach is feasible, but lacks flexibility.

### New proposal improvement

The new proposal adds `reloadSidecarConfig`, explicitly supporting three injection modes:

1. `type: k8s`
   Inject the container directly by `ConfigMapSet` when the Pod is created.

2. `type: sidecarset`
   Inject by referencing a container defined in OpenKruise `SidecarSet`.

3. `type: custom`
   Reference a `ConfigMap` to customize reload-sidecar configuration and enable cross-namespace reuse.

### Value of the improvement

- **Injection method becomes configurable**: webhook direct injection is no longer the only option.
- **Reuses existing SidecarSet capability**: naturally supports independent canary rollout and independent evolution of the sidecar image.
- **Better suited for platform scenarios**: supports centrally maintained reload container templates, reducing repetitive configuration across business lines.
- **Reduced coupling**: `ConfigMapSet` no longer has to fully own every detail of reload-sidecar management.

---

## 2. Configuration effect strategy evolved from “restart-centric” to “three `effectPolicy` modes”

### Old proposal

The core effect strategy in the old proposal mainly revolves around:

- restarting `reload-sidecar`
- optionally restarting business containers injected with configuration
- controlling rolling updates through `restartInjectedContainers` and `maxUnavailable`

In essence, configuration effect is driven by restart.

### New proposal improvement

The new proposal introduces `effectPolicy`, supporting three configuration effect modes:

1. `ReStart`
   - restart `reload-sidecar` first
   - then restart business containers
   - suitable for the safest strong-consistency scenarios

2. `PostHook`
   - `reload-sidecar` restarts and prepares the new configuration
   - business containers are notified through HTTP/TCP callback to reload configuration
   - suitable when the business already has a reload interface and container restart is undesired

3. `HotUpdate`
   - neither the reload container nor the business container restarts
   - `reload-sidecar` hot-updates the shared configuration files
   - business containers implement their own hot reload
   - suitable for online scenarios that are highly sensitive to interruption

### Value of the improvement

- **Significantly broader capability coverage**: expands from a single “restart to take effect” model to three coexisting models: restart / notify / hot update.
- **Closer to real business differences**: different services load configuration differently, and the new proposal no longer forces a single path.
- **Reduced release disturbance**: `PostHook` and `HotUpdate` provide explicit support for low-disruption scenarios.
- **Better decoupling of configuration release and image release**: configuration updates can choose an independent effect mechanism instead of being tied to container restart strategy.

---

## 3. Update strategy is further enhanced to support grouped rolling updates

### Old proposal

The rolling update behavior in the old proposal mainly depends on:

- `partition`
- `maxUnavailable`
- `restartInjectedContainers`
- `injectUpdateOrder`

Among them, `injectUpdateOrder` requires cooperation from the workload or the platform control plane through `priorityStrategy`, which introduces collaboration complexity.

### New proposal improvement

The new proposal retains and strengthens the rolling semantics, while adding:

- `matchLabelKeys`

This field is used to group matched Pods by labels, and each group performs rolling updates independently according to the semantics of `partition` and `maxUnavailable`.

### Value of the improvement

- **Finer-grained canary capability**: instead of using one global rolling window for all Pods, rollout can proceed independently by group.
- **Better suited for multi-AZ / multi-shard / multi-tenant scenarios**: for example, grouping by `app-group`, IDC label, or business partition.
- **Reduced external platform dependency**: the old collaboration pattern based on `injectUpdateOrder + workload priorityStrategy` is weakened in favor of more direct controller-owned grouped rolling.
- **More unified semantics**: the update strategy is expressed directly in the `ConfigMapSet` spec, reducing cross-object mental overhead.

---

## 4. Rollback capability evolved from “implicitly supported” to “explicitly designed flow”

### Old proposal

Although the old proposal mentioned version management, historical revision retention, and rollback scenarios, rollback was mostly described through platform-collaboration flowcharts and scenario discussion, without a concise and unified solution statement.

### New proposal improvement

The new proposal adds a dedicated “Rollback Flow” section and clearly states:

- rollback can be done by simply re-applying `ConfigMapSet`
- rollback to `currentRevision` is supported
- rollback to `historyRevision` is supported
- the rollback flow reuses the normal rolling-update process
- existing not-yet-updated Pods are not unnecessarily disturbed, and non-`currentRevision` Pods are prioritized

### Value of the improvement

- **Rollback path becomes clearer**: instead of requiring additional platform orchestration knowledge, it becomes a standard flow supported by the CRD itself.
- **Lower operational complexity**: rollback can be performed by re-applying, which aligns with Kubernetes conventions.
- **Tighter closed loop with version management**: `revisionHistoryLimit`, RMC, rolling update strategy, and rollback now work together naturally.
- **Higher design credibility**: a configuration release solution without a rollback closed loop is usually hard to land in production; the new proposal addresses this gap.

---

## 5. Uninstall flow is formally designed to avoid risks caused by Pod self-healing

### Old proposal

The old proposal mentioned in the deletion logic that:

- deleting `ConfigMapSet` does not clean up or recreate Pods

But it did not fully design how to uninstall safely.

### New proposal improvement

The new proposal adds an “Uninstall Flow” design:

- block deletion with a `finalizer`
- stop injecting into new Pods
- wait for the business side to complete a full workload update
- confirm that `ConfigMapSet` no longer affects any Pod
- then clean up the related ConfigMap and remove the finalizer

### Value of the improvement

- **Completes the operational lifecycle**: it considers not only how to create and update, but also how to exit safely.
- **Avoids self-healing anomalies**: prevents inconsistent behavior when Pods are recreated without reload container injection or mount injection.
- **More production-ready**: once uninstall is clearly defined, the platform can expose the feature to users with more confidence.

---

## 6. Status definition is more release-observability oriented instead of controller-internal oriented

### Old proposal

The old proposal status mainly includes:

- `matchedPods`
- `updatedPods`
- `readyPods`
- `updatedReadyPods`
- `currentRevision`
- `updateRevision`
- `lastContainersTimestamp`

These fields are more oriented toward underlying controller state statistics.

### New proposal improvement

The new proposal evolves Status into:

- `currentCustomVersion`
- `updateCustomVersion`
- `currentRevision`
- `updateRevision`
- `replicas`
- `readyReplicas`
- `updatedReplicas`
- `updatedReadyReplicas`
- `expectedUpdatedReplicas`

### Value of the improvement

- **Closer to release perspective**: not only revision but also user-friendly `customVersion` is visible.
- **Better suited for control-plane display**: replica semantics are more aligned with common observability models used by Kruise / Deployment.
- **Easier release progress judgment**: `expectedUpdatedReplicas` makes the intended rollout scope clearer.
- **Lower understanding threshold**: status fields are more suitable for users, control planes, and operational visualization.

---

## 7. Semantics for non-config changes are more converged, avoiding overpromising dynamic capability

### Old proposal

The old proposal specifically discussed “cold start”:

- changing `mountPath` requires a cold start
- `spec.containers` changes are judged through `lastContainersTimestamp`
- Pod creation time compared with the timestamp determines whether a Pod participates in the update
- when Daemon restarts business containers, it must also judge whether the old container definition is still valid

The solution is complete, but complex to implement and difficult to explain at the boundary.

### New proposal improvement

The new proposal directly converges the behavior into:

- **for modifications to non-data parts of `ConfigMapSet`, no dynamic handling is performed**
- they take effect only when the business redeploys and Pods are recreated next time

### Value of the improvement

- **Clearer boundary**: only dynamic updates to configuration content are promised; online effect for mount structure / injection structure changes is not promised.
- **Lower implementation complexity**: reduces dependence on timestamps, Pod creation time, and special handling for existing instances.
- **More stable behavior**: avoids inconsistency between controller and Daemon in complex scenarios.
- **Better for phase-one landing**: prioritize delivering the core capability first, then gradually expand dynamic change support.

This is a typical “capability convergence optimization”: it may look like a restriction on the surface, but in practice it improves implementability.

---

## 8. The proposal adds “stacked release” and “scenario constraints,” making risk boundaries clearer

### Old proposal

Although the old proposal discussed many flows, it did not centrally describe questions such as “what if a new release is initiated in the middle of an existing one?” or “what is explicitly unsupported?”

### New proposal improvement

The new proposal explicitly adds:

- **Stacked release**
  - if a new version update is initiated during an ongoing update, a new round of rolling update is started
  - Pod selection prioritizes non-`currentRevision` Pods

- **Scenario constraints**
  - injecting multiple reload containers is not supported
  - multiple `ConfigMapSet`s controlling the same path on the same Pod is not supported
  - simultaneous modification of configuration and workload spec is not supported for now
  - hot update is not supported for Pods on `virtual-kubelet` nodes

### Value of the improvement

- **More transparent design boundaries**
- **More controllable risk expectations**
- **Clearer future iteration path**
- **Easier to determine MVP scope during review**