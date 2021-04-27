---
title: UnitedTopology
authors:
- "@FillZpp"
reviewers:
- "@Fei-Guo"
- "@furykerry"
creation-date: 2021-04-27
last-updated: 2021-04-27
status: implementable
---

# UnitedTopology

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.


## Motivation

Define a CRD named UnitedTopology, which can support configuration and management for elastic and multi-domain applications,
even more powerful and flexible than [UnitedDeployment](https://openkruise.io/en-us/docs/uniteddeployment.html) in some ways.

Not like the workload type, UnitedTopology can be used with those stateless workloads, such as CloneSet, Deployment and ReplicaSet.

So users needn't learn the usage of a new workload and how to deploy and update their application by it.
They only have to create UnitedTopology CRs related to their existing workloads. Everything is just great!

## Proposal

UnitedTopology definition looks like this:

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: UnitedTopology
metadata:
  namespace: ...
  name: ...
  # ...
spec:
  targetRef:
    apiVersion: apps/v1 | apps.kruise.io/v1alpha1
    kind: Deployment | CloneSet
    name: workload-xxx
  subsets:
  - name: subset-a
    nodeSelectorTerm:
      matchExpressions:
      - key: topology.kubernetes.io/zone
        operator: In
        values:
        - zone-a
    maxReplicas: 3
    patch:
      metadata:
        labels:
          xxx-specific-label: xxx
  - name: subset-b
    nodeSelectorTerm:
      matchExpressions:
      - key: topology.kubernetes.io/zone
        operator: In
        values:
        - zone-b
  scheduleStrategy:
    type: Adaptive | Fixed
    adaptive:
      disableSimulationSchedule: false
      rescheduleCriticalSeconds: 30
status:
  observedGeneration: 12
  subsetStatuses:
  - name: subset-a
    missingReplicas: 2
    creating:
      pod-xxx: "2021-04-27T08:34:11Z"
    deleting:
      pod-yyy: "2021-04-27T08:34:11Z"
  - name: subset-b
    missingReschedule: 1
```

**spec**:
- `targetRef`: reference to a specific workload, it supports `Deployment` in `apps` group and `CloneSet` in `apps.kruise.io` group
- `subsets`: list of subsets which defines different topologies
  - `name`: name of this subset
  - `nodeSelectorTerm`: the additional node selector of this subset, it will be injected into each term of `pod.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms`
  - `tolerations`: the additional tolerations of this subset, it will be injected into `pod.spec.tolerations`
  - `patch`: the specified strategic patch body of this subset, it will be patched to Pods that belong to this subset
  - `maxReplicas`: the maximum number of Pods in this subset, it can be an absolute number or a percentage string.
- `scheduleStrategy`: strategies for schedule
  - `type`: can be `Fixed` or `Adaptive`
    - `Fixed` means always choose the former subset for Pods unless the subset's maxReplicas has been satisfied
    - `Adaptive` means choose the right subset by sequence of subsets and also enough resources
  - `adaptive`:
    - `disableSimulationSchedule`: whether to disable the simulation schedule in webhook
    - `rescheduleCriticalSeconds`: the critical duration that controller will trigger a Pod to reschedule to another subset

**status**:
- `subsetStatuses`: statuses of subsets defined in spec
  - `missingReplicas`: the missing number of this subset, most of the time it is (maxReplicas - the number of active Pods)
  - `creating`: the map of Pods that just allocated to this subset by webhook
  - `deleting`: the map of Pods been deleted or evicted that just noticed by webhook
  - `missingReschedule`: the number of Pods that should be rescheduled to this subset

### Requirements

UnitedTopology requires Pod webhook to inject rules into Pods and notice the deletion and eviction of Pods.

So if the `PodWebhook` feature-gate is set to `false`, UnitedTopology will also be disabled.

### Implementation Details/Notes/Constraints

UnitedTopology have both webhook and controller.

#### Replicas control

When a Pod is creating and its workload has a related UnitedTopology,
webhook will check the subsetStatuses in the UnitedTopology one by one:

1. for each subset, calculate the `delta` number by `missingReplicas - len(creating) + len(deleting)`
2. if `delta` > 0, choose this subset temporarily
   1. try to update subsetStatus and put this pod into `creating` map
   2. if it conflicts, get the UnitedTopology again and go back to step 1
   3. if it succeeds, inject the rules of this subset into the Pod
3. if `delta` <= 0, go to the next subset
4. if there is no available subset left, just let the Pod go

Also, when a Pod that belongs to a subset is deleting or evicting, webhook will put it into the `deleting` map in subsetStatus.

#### Deletion priority

Since Kubernetes 1.21, there is a new annotation definition on Pod:

```yaml
    controller.kubernetes.io/pod-deletion-cost: -1
```

(In 1.21, users need to enable `PodDeletionCost` feature-gate, and since 1.22 it will be enabled by default)

The integer value is the deletion cost of the Pod. A Pod without this annotation will be considered as deletion-cost `0`.

When ReplicaSet(Deployment) tries to scale in, it will sort its Pods by following sequence:

1. Pods that have not been scheduled
2. Pods with `Pending` and `Unknown` phases
3. Pods in not-ready state
4. Pods with **lower deletion-cost**
5. Pods that have deployed on the same Node
6. Pods with higher restart count
7. Pods that have newer creationTimestamp
8. ...

CloneSet will also support this annotation to implement deletion priority.

And then, UnitedTopology will define three levels for the Pods belongs to subsets:

1. `Guaranteed` for Pods that in the `maxReplicas` of its subset, deletion-cost will be set bigger than `0`
2. `BestEffort` for Pods that belong to subsets without `maxReplicas`, no deletion-cost will be set (defaults to `0`)
3. `Redundant` for Pods that exceed the `maxReplicas` of its subset, deletion-cost will be set lower than `0`

### User Stories

#### A fixed number or percentage of Pods in zone-a, zone-b, zone-c

```yaml
  subsets:
  - name: subset-a
    nodeSelectorTerm:
      matchExpressions:
      - key: topology.kubernetes.io/zone
        operator: In
        values:
        - zone-a
    maxReplicas: 10
  - name: subset-b
    nodeSelectorTerm:
      matchExpressions:
      - key: topology.kubernetes.io/zone
        operator: In
        values:
        - zone-b
    maxReplicas: 30%
  - name: subset-a
    nodeSelectorTerm:
      matchExpressions:
        - key: topology.kubernetes.io/zone
          operator: In
          values:
            - zone-c
```

#### A fixed number of Pods on ECS, the other Pods on VK

```yaml
  subsets:
  - name: ecs
    nodeSelectorTerm:
      matchExpressions:
      - key: type
        operator: NotIn
        values:
        - virtual-kubelet
    maxReplicas: 3
  - name: vk
    nodeSelectorTerm:
      matchExpressions:
      - key: type
        operator: In
        values:
        - virtual-kubelet
    tolerations:
    - key: virtual-kubelet.io/provider
      operator: Exists
```

#### Prefer to create Pods on ECS, use VK only if there is no enough resources on ECS 

```yaml
  subsets:
  - name: ecs
    nodeSelectorTerm:
      matchExpressions:
      - key: type
        operator: NotIn
        values:
        - virtual-kubelet
  - name: vk
    nodeSelectorTerm:
      matchExpressions:
      - key: type
        operator: In
        values:
        - virtual-kubelet
    tolerations:
    - key: virtual-kubelet.io/provider
      operator: Exists
  scheduleStrategy:
    type: Adaptive
    adaptive:
      rescheduleCriticalSeconds: 30
```


### Risks and Mitigations

The webhooks will retry on conflict when update the subset status, this might cause the Pod creation to be slower,
but it can be guaranteed to finish in one seconds.

## Implementation History

- [ ] 27/04/2021: Proposal submission

