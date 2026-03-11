---
title: SidecarTerminator
authors:
- "@veophi"
reviewers:
- "@furykerry"
- "@zmberg"

creation-date: 2025-11-20

last-updated: 2025-11-20

status: under review

---

## PodUnavailableBudget Group-Level Semantics for LeaderWorkerSet and Similar Workloads  
---

### Summary

This proposal extends `PodUnavailableBudget` (PUB) to understand **Pod group semantics**. When a new `podGroupPolicy` is configured, PUB will:

1. Group Pods using a user-provided label key (for example `leaderworkerset.sigs.k8s.io/group-index`).
2. Consider the group, not individual Pods, as the availability unit.
3. Evaluate `maxUnavailable`/`minAvailable` against **group counts**.

This change preserves full backward compatibility by remaining opt-in. Without `podGroupPolicy`, PUB keeps the existing per-Pod behavior.

---

### Motivation

#### 1. Gaps in Current PUB

- PUB counts Pods, but many distributed inference services treat “Leader + Workers” as a **single serving unit**.
- Taking down a single Pod inside the unit can render the entire unit unusable, yet PUB still considers remaining Pods available.
- Current `maxUnavailable=1` means “only one Pod at a time,” whereas operators need “only one serving group at a time.”

#### 2. Why LeaderWorkerSet Needs Group Protection

- `LeaderWorkerSet` (LWS) sharding schemes often require:
  - 1 Leader + N Workers Ready for a shard to serve traffic.
  - Routing uses shard identity (`group-index`), not individual Pods.
- Killing one Worker might stop the whole shard even though PUB still sees many “available” Pods.
- Rolling upgrades, auto-scaling, or node maintenance must ensure **groups** remain available, to avoid LLM serving outages.

---

### Proposal

#### User Story (YAML Example)

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: LeaderWorkerSet
metadata:
  name: llm-serving
  namespace: inference
spec:
  replicas: 3
  selector:
    matchLabels:
      app: llm-serving
  leaderTemplate:
    metadata:
      labels:
        app: llm-serving
        role: leader
    spec:
      containers:
      - name: leader
        image: example.com/leader:latest
  workerTemplate:
    metadata:
      labels:
        app: llm-serving
        role: worker
    spec:
      containers:
      - name: worker
        image: example.com/worker:latest
  # All Pods have label leaderworkerset.sigs.k8s.io/group-index = "0" / "1" / "2"
---
apiVersion: policy.kruise.io/v1alpha1
kind: PodUnavailableBudget
metadata:
  name: llm-serving-budget
  namespace: inference
spec:
  selector:
    matchLabels:
      app: llm-serving
  maxUnavailable: 1         # interpreted as groups
  podGroupPolicy:
    groupLabelKey: leaderworkerset.sigs.k8s.io/group-index
    minReadyReplicas: 3     # 1 leader + 2 workers Ready → group serves traffic
```

The PUB now ensures at most one shard goes offline at a time, aligning disruption budgets with the real service unit.

#### API Definition

```go
type PodUnavailableBudgetSpec struct {
    // existing fields: selector, targetReference, maxUnavailable, minAvailable, etc.

    // PodGroupPolicy defines the policy for grouping pods and calculating
    // availability at the group level. When nil, legacy per-pod semantics apply.
    PodGroupPolicy *PodGroupPolicy `json:"podGroupPolicy,omitempty"`
}

type PodGroupPolicy struct {
    // GroupLabelKey specifies the label key used to group Pods.
    // Pods with the same value belong to the same group.
    // Example: "leaderworkerset.sigs.k8s.io/group-index"
    GroupLabelKey string `json:"groupLabelKey"`

    // MinReadyReplicas is the minimum number of Ready pods required
    // for a group to be considered Available.
    // Default (when nil) is 1.
    MinReadyReplicas *int32 `json:"minReadyReplicas,omitempty"`
}
```

No status schema changes are required. When `podGroupPolicy` is set, the existing status fields (`currentAvailable`, `desiredAvailable`, `unavailableReplicas`) are interpreted as **group counts**.

#### Semantics

- **Grouping**: Pods are grouped by `GroupLabelKey`. Pods missing the label fall back to legacy per-Pod protection, and the PUB emits a warning condition.
- **Group Availability**: A group is Available when `ReadyPodsInGroup >= MinReadyReplicas` (default 1). The Ready determination matches existing PUB logic (PodReady true and not already disrupted).
- **Budget Interpretation**:
  - `maxUnavailable=1` → “At most 1 group Unavailable.”
  - `minAvailable=2` → “At least 2 groups Available.”
  - Both fields can coexist, enforcing both constraints.
- **Backward Compatibility**: With `podGroupPolicy` unset (default), PUB behaves exactly as today.

#### Design Details

- **Controller responsibilities**:
  - List Pods selected by the PUB.
  - Group them by `GroupLabelKey`.
  - Compute Ready counts per group (respecting disruption annotations as today).
  - Determine Available vs Unavailable groups using `MinReadyReplicas`.
  - Update status counts with group numbers.
  - Continue protecting Pods missing the label using legacy semantics and surface warnings.

- **Status updates**:
  - `currentAvailable = number of Available groups`.
  - `desiredAvailable = groups required by minAvailable or maxUnavailable`.
  - `unavailableReplicas = number of Unavailable groups`.

- **Observability**:
  - Conditions highlight missing labels or impossible `MinReadyReplicas`.
  - Metrics expose total/available groups for alerting.

#### Controller Logic (Detailed)

1. `pods := listPods(pub.selector/targetRef)`
2. If `podGroupPolicy == nil`, run legacy logic.
3. Else:
   - `groupKey := podGroupPolicy.groupLabelKey`
   - `minReady := podGroupPolicy.minReadyReplicas` (default 1)
   - Build `map[groupID][]Pod`
   - For each group:
     - `ready := countReadyPods(group)`
     - `available := ready >= minReady`
   - `updateStatus(availableGroups, desiredGroups, unavailableGroups)`
4. Handle Pods without the label using legacy accounting and emit warning conditions.

#### Webhook Logic

Admission webhook intercepts Pod eviction/deletion:

1. Identify PUBs covering the Pod.
2. For each PUB:
   - If group policy disabled or Pod lacks the label → use legacy per-Pod decision.
   - Else:
     - Determine the Pod’s group and current Ready count.
     - Simulate removing the Pod:
       - If group availability unchanged → allow (per standard logic).
       - If group transitions Available → Unavailable:
         - Compute new `unavailableGroups` / `availableGroups`.
         - Enforce `maxUnavailable` / `minAvailable` using group counts.
         - Reject if limits exceeded.
3. Request is allowed only if **every** covering PUB allows it.

This mirrors traditional PDB logic but applies it to the group unit, preventing voluntary disruptions from taking down multiple serving shards at once.

---

## Conclusion

By introducing `podGroupPolicy`, `PodUnavailableBudget` can finally express budgets that match how distributed inference workloads actually fail. The change:

- Is opt-in and backward compatible.
- Lets operators specify budgets in terms of **groups** (Leader+Workers shards).
- Keeps controller and webhook behavior tightly aligned.

This enhancement ensures LeaderWorkerSet and similar workloads receive precise availability protection without sacrificing the safety guarantees PUB already delivers to other workloads.