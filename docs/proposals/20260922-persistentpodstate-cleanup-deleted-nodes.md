---
title: Clean Up PersistentPodState After Node Deletion
creation-date: 2026-09-22
last-updated: 2026-10-15
status: provisional
---

# Clean Up PersistentPodState After Node Deletion

## Summary

Add an Alpha feature gate `PersistentPodStateAutoCleanup` (off by default).
When it is on, PersistentPodState removes a saved PodState entry once its Node is deleted from the cluster.
It applies only to required persistent topology. No API or CRD changes are needed.

## Motivation

GPU workloads may use `kruise.io/required-persistent-topology: kubernetes.io/hostname`
so that a restarted Pod returns to the Node that already caches its models.
PersistentPodState saves the hostname and adds it to the new Pod's `nodeSelector`.

During Node replacement the Node is deleted, but its saved state is not:
today state is removed only on scale-down or workload deletion.
New Pods then require a hostname that no longer exists and cannot be scheduled.

### Why Not Preferred Topology

`preferred-persistent-topology` is only a scheduler hint. Even with `weight: 100`,
a Pod can land on another Node while the saved Node still exists.
This use case needs to wait for the saved Node (for example, for a free GPU)
to avoid downloading large models again, and to move only when the Node is gone.

| Saved Node | Preferred topology | Required + this feature |
| --- | --- | --- |
| Exists and can run the Pod | May pick another Node | Keep the saved Node |
| Exists, cannot run the Pod now | Uses another Node | Wait for the saved Node |
| Deleted | Uses another Node | Drop the state, use another Node |

### Example

```yaml
# PersistentPodState status (not Pod status)
status:
  podStates:
    gpu-worker-0:
      nodeName: compute-xxx
      nodeTopologyLabels:
        kubernetes.io/hostname: compute-xxx
```

When `compute-xxx` is deleted, the `gpu-worker-0` entry is removed.
The next `gpu-worker-0` can run on any suitable GPU Node, and its new state is saved when it becomes Ready.

## Goals and Non-Goals

Goals: remove state only after the API confirms that the Node is gone,
never give new Pods rules for a deleted Node, and keep current behavior when the gate is off.

Non-goals:

- Cleanup for NotReady, cordoned, tainted, or full Nodes.
- Fixing Pods that are already Pending (see [Risks and Limitations](#risks-and-limitations)).
- Detecting a new Node with the same name (PodState saves no Node UID).

## Proposal

### Feature Gate

Add the gate to `pkg/features/kruise_features.go` (Alpha, default `false`):

```text
--feature-gates=PersistentPodStateAutoCleanup=true
```

It applies to every PersistentPodState with at least one `requiredPersistentTopology` key,
whether created by users or from workload annotations, and works with both `WhenScaled` and `WhenDeleted`.

### Cleanup Rules

The Node is found by `status.podStates[podName].nodeName`, not by the hostname label (they can differ).

| Node check result | Action |
| --- | --- |
| Node exists (in any condition, including terminating) | Keep the entry |
| Direct API read returns NotFound | Remove the whole entry |
| Timeout, access error, or other error | Keep the entry. The controller retries, the webhook returns an error |
| `nodeName` is empty | Keep the entry |

### Controller

- Runs cleanup in the reconcile loop and writes through the status subresource.
- Confirms a cache miss with a direct API read, and checks each Node once per loop.
- Rechecks matching objects once per minute to cover missed events and controller downtime.
  Rechecks stop when no saved Node names remain.
- Is idempotent: on a status conflict it reads the latest object and checks again, never overwriting newer state.

### Pod Webhook

The webhook covers the gap before the controller finishes cleanup.
Before adding a saved entry to a new Pod, it reads the Node from the API:

- NotFound: skip the entry. Rules from the Pod template and other webhooks still apply.
- Other errors: reject the request so that the workload controller retries.

It never changes PersistentPodState or Pods, including on dry-run.

## Risks and Limitations

- **Pods created before Node deletion** (for example, during a drain) keep the hostname rule and stay Pending.
  An operator must recreate them. Automatic recovery needs a separate design.
- **The whole entry is removed**, so the Pod may move to another zone and saved annotations are lost.
- **A new Node with the old name** is treated as the saved Node.
- **API load:** each matching Pod creation and each cache miss adds one direct Node read. This must be measured before Beta.

## Alternatives

- **Preferred topology:** does not wait for the saved Node.
- **Manual cleanup:** slow, and new Pods can still get old rules before it is done.
- **Watch Node deletion events:** faster, but needs a Node-to-PersistentPodState lookup and still has to handle missed events.
  Can be added later if large cluster tests show a need.
- **Per-object policy field:** finer control, but requires API and CRD changes.

## Upgrades

Gate off: no change. Gate on: cleanup also covers Nodes deleted before the upgrade;
all kruise-manager replicas should use the same gate value.
Turning the gate off stops cleanup but does not restore removed entries.

## Open Questions

1. Apply to all required topology keys, or only to `kubernetes.io/hostname`?
2. Remove the whole entry, or keep annotations and shared topology values such as zone?
3. Is one global gate enough, or is a per-object opt-in needed?
4. Are periodic rechecks enough for large clusters, or should the first version watch Node deletions?
5. Should recovery of already Pending Pods be in the first version or a follow-up?

## References

- [Original PersistentPodState proposal](20220421-persistent-pod-state.md)
- [PersistentPodState custom workload support](20220824-persistentpodstate-custom-workload-support.md)
- [PersistentPodState user documentation](https://openkruise.io/docs/user-manuals/persistentpodstate)
- [Kubernetes node affinity weight](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity-weight)
- [PersistentPodState controller](../../pkg/controller/persistentpodstate/persistent_pod_state_controller.go)
- [PersistentPodState Pod admission handler](../../pkg/webhook/pod/mutating/persistent_pod_state.go)
