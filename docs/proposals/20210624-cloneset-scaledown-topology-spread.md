---
title: Proposal Template
authors:
- "@FillZpp"
  reviewers:
- "@Fei-Guo"
- "@furykerry"
  creation-date: 2021-06-24
  last-updated: 2021-06-24
  status: implementable
---

# CloneSet scale-down supports topology spread

## Motivation

Currently, the sequence of CloneSet scale-down is:

1. Node unassigned < assigned
2. PodPending < PodUnknown < PodRunning
3. Not ready < ready
4. Lower pod-deletion cost < higher pod-deletion-cost
5. Been ready for empty time < less time < more time
6. Pods with containers with higher restart counts < lower restart counts
7. Empty creation time pods < newer pods < older pods

What's more, CloneSet should also choose pods to delete according to topology constraints.

## User Stories

1. Pods stacking on the same node should be deleted preferentially.
2. Should delete Pods according to topology spread constraints.

## Implementation Details/Notes/Constraints

Add a rank step between (4) and (5). We can now support two Ranker types:

- `SameNodeRanker` for same node spread.
- `SpreadConstraintsRanker` for [Pod Topology Spread Constraints](https://kubernetes.io/docs/concepts/workloads/pods/pod-topology-spread-constraints/).

Note that if there are Pod Topology Spread Constraints defined in CloneSet template,
controller will use `SpreadConstraintsRanker` to get ranks for pods, but it will still sort pods in the same topology by `SameNodeRanker`.
Otherwise, controller will only use `SameNodeRanker` to get ranks for pods.

### SameNodeRanker

SameNodeRanker helps calculate pod ranks according to same node stacking.

- Rank of those pods that are alone on their nodes is `0` by default.
- For a node that has multiple pods stacking on it, SameNodeRanker will sort these pods by active and use their indexes as ranks.

For example:

```
node-a: pod-0, pod-1
node-b: pod-2
node-c: pod-3, pod-4, pod-5
```

The result will be:
- Rank of `pod-2` is definitely `0`.
- Sort pods for `node-a`, if they are `[pod-0, pod-1]` after sorting, their ranks will be `{pod-0: 1, pod-1: 0}`.
- Sort pods for `node-c`, if they are `[pod-5, pod-3, pod-4]` after sorting, their ranks will be `{pod-5: 2, pod-3: 1, pod-4: 0}`.

### SpreadConstraintsRanker

SpreadConstraintsRanker is a little more complicated than SameNodeRanker.

First, we should define the constraint rule that the ranker should care about:

```golang
type PodSpreadConstraint struct {
	TopologyKey     string `json:"topologyKey"`
	LimitedValues []string `json:"limitedValues,omitempty"`
}
```

- `TopologyKey` is the key of node labels. Nodes that have a label with this key and identical values are considered to be in the same topology.
- `LimitedValues` defines the specific values of this topology key. By default, all values of this topology key are included.

Note that:
1. The constraint rules will be got from CloneSet template (such as `cloneset.spec.template.spec.topologySpreadConstraints`),
   users have no need to configure them additionally.
2. `LimitedValues` is currently not provided in [Pod Topology Spread Constraints](https://kubernetes.io/docs/concepts/workloads/pods/pod-topology-spread-constraints/),
   reserve for future extensions

SpreadConstraintsRanker sorting logic:

1. Split nodes and pods into different values of such topology key.
2. For all pods in one topology value, SpreadConstraintsRanker firstly sorts them by active and SameNodeRanker,
   then calculates their ranks as their index of this topology value.

For example:

```
zone-x:
  node-a: a-0, a-1
  node-b: b-0
  node-c: c-0, c-1, c-2

zone-y:
  node-d: d-0, d-1
  node-e: e-0, e-1

Constraint topologyKey: topology.kubernetes.io/zone
```

After sorting in same topology value, the sequence of pods in zone-x and zone-y will be:

```
zone-x: c-2, a-1, c-1, a-0, b-0, c-0
zone-y: d-1, e-1, d-0, e-0
```

*Note that pods in same zone are sorted by same node spread.*

So the final pod sequence by topology spread rank will be:
`d-1, c-2, e-1, a-1, c-1, d-0, a-0, b-0, e-0, c-0`
