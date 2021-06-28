---
title: WorkloadSpread
authors:
  - "@BoltsLei"
reviewers:
  - "@Fei-Guo"
  - "@furykerry"
  - "@FillZpp"
creation-date: 2021-06-28
last-updated: 2021-06-28
status: implementable
---

# WorkloadSpread

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

## Motivation

Define a CRD named WorkloadSpread, which can support configuration and management for elastic and multi-domain applications,
even more powerful and flexible than [UnitedDeployment](https://openkruise.io/en-us/docs/uniteddeployment.html) in some ways.

Not like the workload type, WorkloadSpread can be used with those stateless workloads, such as CloneSet, Deployment and ReplicaSet.

So users needn't learn the usage of a new workload and how to deploy and update their application by it.
They only have to create WorkloadSpread CRs related to their existing workloads. Everything is just great!

## Proposal

WorkloadSpread definition looks like this:

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: WorkloadSpread
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
    requiredNodeSelectorTerm:
      matchExpressions:
      - key: topology.kubernetes.io/zone
        operator: In
        values:
        - zone-a
    preferredNodeSelectorTerms:
    - weight: 50
      preference:
      matchExpressions:
      - key: topology.kubernetes.io/zone
        operator: In
        values:
        - zone-b
    maxReplicas: 3
    tolertions: []
    patch:
      metadata:
        labels:
          xxx-specific-label: xxx
  - name: subset-b
    requiredNodeSelectorTerm:
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
    creatingPods:
      pod-xxx: "2021-04-27T08:34:11Z"
    deletingPods:
      pod-yyy: "2021-04-27T08:34:11Z"
    subsetUnscheduledStatus:
      unschedulable: true
      unscheduledTime: 2021-04-27T08:34:11Z
      failedCount: 1
  - name: subset-b
    missingReplicas: -1
```

**spec**:
- `targetRef`: reference to a specific workload, it supports `Deployment` in `apps` group and `CloneSet` in `apps.kruise.io` group
- `subsets`: list of subsets which defines different topologies
  - `name`: name of this subset
  - `requiredNodeSelectorTerm`: the additional node selector of this subset, it will be injected into each term of `pod.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms`
  - `preferredNodeSelectorTerms`: the additional node selector of this subset, it will be injected into each term of `pod.spec.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution`
  - `tolerations`: the additional tolerations of this subset, it will be injected into `pod.spec.tolerations`
  - `patch`: the specified strategic patch body of this subset, it will be patched to Pods that belong to this subset
  - `maxReplicas`: the maximum number of Pods in this subset, it can be an absolute number or percentage string. maxReplicas 
    can be null represents that there is no replicas limits in this subset.
- `scheduleStrategy`: strategies for schedule
  - `type`: can be `Fixed` or `Adaptive`
    - `Fixed` means always choose the former subset for Pods unless the subset's maxReplicas has been satisfied
    - `Adaptive` means choose the right subset by sequence of subsets and also enough resources
  - `adaptive`:
    - `disableSimulationSchedule`: whether to disable the simulation schedule in webhook
      > webhook can take a simple general predicates to check whether Pod can be scheduled into this subset, \
        but it just considers the Node resource and cannot replace scheduler to do richer predicates practically.
    - `rescheduleCriticalSeconds`: the critical duration that controller will trigger a Pod to reschedule to another subset.
      > rescheduleCriticalSeconds indicates how long controller will reschedule a schedule failed Pod to the subset that has
        redundant capacity after the subset where the Pod lives. \
        If a Pod was scheduled failed and still in a pending status over RescheduleCriticalSeconds duration, the controller will reschedule it to the suitable subset.

**status**:
- `subsetStatuses`: statuses of subsets defined in spec
  - `missingReplicas`: the missing number of this subset, most of the time it is (maxReplicas - the number of active Pods)
    > missingReplicas > 0 indicates the subset is still missing MissingReplicas pods to create \
    missingReplicas = 0 indicates the subset already has enough pods, there is no need to create \
    missingReplicas = -1 indicates the subset's MaxReplicas not set, then there is no limit for pods number
  - `creatingPods`: the map of Pods that just allocated to this subset by webhook
    > creatingPods contains information about pods whose creation was processed by
      the webhook handler but not yet been observed by the WorkloadSpread controller.
      A pod will be in this map from the time when the webhook handler processed the
      creation request to the time when the pod is seen by controller. \
      The key in the map is the name of the pod and the value is the time when the webhook
      handler process the creation request. If the real creation didn't happen and a pod is
      still in this map, it will be removed from the list automatically by WorkloadSpread controller
      after some time.\
      If everything goes smooth this map should be empty for the most of the time.
      Large number of entries in the map may indicate problems with pod creations.
  - `deletingPods`: the map of Pods been deleted or evicted that just noticed by webhook
    > deletingPods is similar with creatingPods and it contains information about pod deletion.
  - `subsetUnscheduledStatus`: contains the details for the unscheduled subset.
     - `unschedulable`: unschedulable is true indicates this Subset cannot be scheduled. The default is false.
     - `unscheduledTime`: last time for unscheduled.
     - `failedCount`: the number of subset was marked with unschedulable.

### Requirements

WorkloadSpread requires Pod webhook to inject rules into Pods and notice the deletion and eviction of Pods.

So if the `PodWebhook` feature-gate is set to `false`, WorkloadSpread will also be disabled.

### Implementation Details/Notes/Constraints

WorkloadSpread have both webhook and controller. Controller should collaborate with webhook to maintain WorkloadSpread status together. \
The controller is responsible for calculating the real status, and the webhook mainly counts missingReplicas and records the creation or deletion entry of Pod into map.

#### Replicas control

##### Pod creation 
When a Pod is creating and its workload has a related WorkloadSpread, webhook will check the subsetStatuses in the WorkloadSpread one by one:

1. if `missingReplicas` > 0 or `missingReplicas` = -1 and this subset is not unschedulable, choose this subset temporarily \
   (1). update `missingReplicas` -= 1 and put this pod into `creatingPods` map and try to update subsetStatus. \
   (2). if it conflicts, get the WorkloadSpread again and go back to step (1). \
   (3). if it succeeds, inject the rules of this subset into the Pod. 
2. if `missingReplicas` = 0, go to the next subset
3. if there is no available subset left, just let the Pod go

##### Pod deletion 
Also, when a Pod that belongs to a subset is deleting or evicting, webhook will put it into the `deletingPods` map in subsetStatus and update `missingReplicas` += 1.

##### Update Pod Status
When a Pod that belongs to a subset changes status phase to 'succeed' or 'failed', which means Pod has been terminated lifecycle, \
webhook will clean it from `creatingPods` map and update `missingReplicas` -= 1.

#### Reschedule strategy
rescheduleSubset will delete some unscheduled Pods that still in pending status. Some subsets have no sufficient resource can lead to some Pods scheduled failed.

WorkloadSpread has multiple subset, so some Pods scheduled failed should be rescheduled to other subsets. \
controller will mark the subset contains Pods scheduled failed to unscheduable status and Webhook cannot inject \
Pod into this subset by check subset's status. Controller cleans up Pods scheduled failed to trigger workload create new replicas, \
webhook will skip unscheduable subset and chose suitable subset to inject. 

The unscheduled subset can be kept for 10 minutes and then should be recovered schedulable status to schedule Pod again by controller.

#### Deletion priority

We have three types for subset's Pod deletion-cost
1. the number of active Pods in this subset <= maxReplicas, deletion-cost = 100. indicating the priority of Pods \
   in this subset is more higher than other Pods in workload.
2. subset's maxReplicas is nil, deletion-cost = 0. indicating the priority of Pods in this subset is lower than \
   other subsets that have a not nil maxReplicas.
3. the number of active Pods in this subset > maxReplicas, two class: (a) deletion-cost = -100, (b) deletion-cost = +100. \
   indicating we prefer deleting the Pod have -100 deletion-cost in this subset in order to control the instance of subset \
   meeting up the desired maxReplicas number.

CloneSet has supported deletion-cost feature in the recent versions.

Since Kubernetes 1.21, there is a new annotation definition on Pod:

```yaml
    controller.kubernetes.io/pod-deletion-cost: -100 / 100 / 0
```

(In 1.21, users need to enable `PodDeletionCost` feature-gate, and since 1.22 it will be enabled by default)

### Comparison with PodTopologySpread

#### PodTopologySpread

You can use topology spread constraints to control how Pods are spread across your cluster among failure-domains such as regions, zones, nodes, and other user-defined topology domains. 
This can help to achieve high availability as well as efficient resource utilization.

#### For WorkloadSpread
1. WorkloadSpread's subset maxReplicas can be a percentage string, which defines the percentage spread in multiple topology, but PodTopologySpread doesn't support this feature.
   
2. Only if the Pods number is equal with maxReplicas of this subset, WorkloadSpread inject Pod into the next available subset. Therefore, workload spread has an order attribution, and the order of subset influence creation and deletion order
   
3. For PodTopologySpread, there's no guarantee that the constraints remain satisfied when Pods are removed.
   For example, scaling down a Deployment may result in imbalanced Pods distribution.
   For WorkloadSpread, controller will guarantee the workload replicas meeting up maxReplicas by deletion-cost feature.
   
4. WorkloadSpread support subset changes desired Pod numbers by configuring maxReplicas for elastic.