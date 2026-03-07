---
title: DaemonSet Ignore NotReady Nodes When Calculating MaxUnavailable
authors:
  - "@ls-2018"
reviewers:
  - "@furykerry"
  - "@zmberg"
creation-date: 2026-03-01
last-updated: 2026-03-01
status:
---

# DaemonSet Ignore NotReady Nodes When Calculating MaxUnavailable

## Table of Contents

- [Quick Reference](#quick-reference)
- [Background and Motivation](#background-and-motivation)
- [Proposal](#proposal)
- [User Scenarios](#user-scenarios)
- [Risks and Mitigations](#risks-and-mitigations)
- [Implementation History](#implementation-history)

## Quick Reference

**Enable annotation to allow DaemonSet rolling updates on healthy nodes when some nodes are NotReady:**

```yaml
annotations:
  daemonset.kruise.io/ignore-notready-nodes: "true"
```

**Effect:** `effectiveMaxUnavailable = maxUnavailable + podsOnUnschedulableNodes`

**Use when:** Planned maintenance, network partitions, node scaling, known hardware failures

**Don't use when:** Unexplained NotReady nodes, unstable cluster, already high maxUnavailable

## Background and Motivation

During DaemonSet rolling updates, `maxUnavailable` controls how many pods can be unavailable simultaneously. When nodes become NotReady, pods on those nodes can **block rollouts on healthy nodes**.

**Problem:** 10 nodes, 3 NotReady, `maxUnavailable=2` → Update blocked on 7 healthy nodes

**Solution:** `effectiveMaxUnavailable = maxUnavailable + podsOnUnschedulableNodes`

This allows updates to proceed on healthy nodes by dynamically adjusting the unavailability budget.

## Proposal

### API Definition

```yaml
apiVersion: apps.kruise.io/v1beta1
kind: DaemonSet
metadata:
  name: demo
  namespace: default
  labels:
    app: demo
  annotations:
    'daemonset.kruise.io/ignore-notready-nodes': 'true'
spec:
  selector:
    matchLabels:
      app: demo
  template:
    metadata:
      labels:
        app: demo
    spec:
      containers:
      - name: demo
        image: nginx:1.14.2
      terminationGracePeriodSeconds: 3
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/hostname
                    operator: NotIn
                    values:
                      - koord-worker4
                      - koord-worker5

```

### Behavior

**Core Mechanism:**

```
effectiveMaxUnavailable = configuredMaxUnavailable + unexpectedPodCount
```

**Unexpected pods:** Pods on nodes where `nodeShouldRunDaemonPod(node, ds)` returns `false`:
- NotReady nodes
- Nodes with untoleratable taints
- Nodes not matching NodeSelector/NodeAffinity

**Update Flow:**
1. Calculate base `maxUnavailable` from spec
2. Count pods on unschedulable nodes as `unexpectedPodCount`
3. If annotation enabled: `maxUnavailable += unexpectedPodCount`
4. Rolling update proceeds with adjusted budget

**Important:** This feature does NOT delete pods. Pods on NotReady nodes are managed by standard Kubernetes mechanisms.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Aggressive updates** | Too many pods updating if many nodes NotReady | Explicit opt-in via annotation; users understand implications before enabling |
| **Outdated pods** | Pods on long-term NotReady nodes run old versions | Expected behavior; DaemonSet manage loop updates pods when nodes recover |
| **Confusion** | Operators confused why maxUnavailable exceeded | Clear logging at verbosity level 5; comprehensive documentation |
| **Feature interaction** | Unexpected behavior with partition/selector/maxSurge | Isolated logic; only affects maxUnavailable calculation; tested combinations |

**Recommendations:**
- Only enable for known/expected NotReady conditions
- Monitor node health and address persistent NotReady states
- Test in staging with your specific configuration
- Use `kubectl logs -v=5` to observe adjustment behavior

## Implementation History

- [x] 2026-03-01: Proposal created and requirements gathered
- [x] 2026-03-02: Implementation completed (controller logic, unit tests, e2e tests)
- [x] 2026-03-06: Documentation updated to reflect actual implementation
