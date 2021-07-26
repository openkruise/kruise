---
title: WorkloadSpread
authors:
  - "@BoltsLei"
reviewers:
  - "@Fei-Guo"
  - "@furykerry"
  - "@FillZpp"
creation-date: 2021-06-28
last-updated: 2021-07-19
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

Not like the workload type, WorkloadSpread can be used with those stateless workloads, such as CloneSet, Deployment, ReplicaSet and even Job.

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
#   preferredNodeSelectorTerms:
#   - weight: 50
#     preference:
#     matchExpressions:
#     - key: topology.kubernetes.io/zone
#       operator: In
#       values:
#       - zone-a
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
      unschedulable: false
      unscheduledTime: 2021-04-27T08:34:11Z
      failedCount: 1
  - name: subset-b
    missingReplicas: -1
```

**spec**:
- `targetRef`: reference to a specific workload, it supports `Deployment` in `apps` group and `CloneSet` in `apps.kruise.io` group
- `subsets`: list of subsets which defines different topologies
  - `name`: name of this subset
  - `requiredNodeSelectorTerm`: the additional node selector of this subset, it will be injected into each term of the `pod.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms`
  - `preferredNodeSelectorTerms`: the additional node selector of this subset, it will be appended into the `pod.spec.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution`
  - `tolerations`: the additional tolerations of this subset, it will be injected into `pod.spec.tolerations`
  - `patch`: the specified strategic patch body of this subset, it will be patched to Pods that belong to this subset
     > use [runtime.RawExtension](https://github.com/kubernetes/apimachinery/blob/03ac7a9ade429d715a1a46ceaa3724c18ebae54f/pkg/runtime/types.go#L94) type
    ```yaml
    # patch metadata:
    patch:
      metadata:
        labels:
          xxx-specific-label: xxx
    ```
    ```yaml
    # patch container resources:
    patch:
      spec:
        containers:
        - name: main
          resources:
            limit:
              cpu: "2"
              memory: 800Mi
    ```
    ```yaml
    # patch container environments:
    patch:
      spec:
        containers:
        - name: main
          env:
          - name: K8S_CONTAINER_NAME
            value: main
    ```
  - `maxReplicas`: the maximum number of Pods in this subset, it can be an absolute number or percentage string. maxReplicas can be null represents that there is no replicas limits in this subset.
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
        If a Pod was scheduled failed and still in a pending status over rescheduleCriticalSeconds duration, the controller will reschedule it to the suitable subset.

**status**:
- `subsetStatuses`: statuses of subsets defined in spec
  - `missingReplicas`: the missing number of this subset, most of the time it is (maxReplicas - the number of active Pods)
    > missingReplicas > 0 indicates the subset is still missing `missingReplicas` pods to create. \
    missingReplicas = 0 indicates the subset already has enough pods, and there is no need to create. \
    missingReplicas = -1 indicates the subset's maxReplicas not set, then there is no limit for pods number.
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
       > Controller will set status to unschedulable if finding some unscheduled Pods belongs to this subset. However,
         it is temporary when unschedulable is true, and controller will recover subset to schedulable after 10 minutes.
     - `unscheduledTime`: last time for unscheduled.
     - `failedCount`: the number of subset was marked with unschedulable.
       > failedCount just records the number of failures. Every failure can lead subset unschedulable for 10 minutes.

## Requirements

### Pod Webhook
WorkloadSpread requires Pod webhook to inject rules into Pods and notice the deletion and eviction of Pods.

So if the `PodWebhook` feature-gate is set to `false`, WorkloadSpread will also be disabled.

### deletion-cost feature

CloneSet has supported deletion-cost feature in the latest versions.

Since Kubernetes 1.21, there is a new annotation definition on Pod:

```yaml
    controller.kubernetes.io/pod-deletion-cost: -100 / 100 / 0
```

(In 1.21, users need to enable `PodDeletionCost` feature-gate, and since 1.22 it will be enabled by default)

## Implementation Details/Notes/Constraints

WorkloadSpread has both webhook and controller. Controller should collaborate with webhook to maintain WorkloadSpread's status together.

The webhook is responsible for injecting rules into pod, updating `missingReplicas` and recording the creation or deletion entry of Pod into map.

The controller is responsible for updating the missingReplicas along with other statics, and cleaning `creatingPods` and `deletingPods` map.

### Replicas control

#### Pod creation

When a Pod is creating and its workload has a related WorkloadSpread, webhook will check the subsetStatuses in the WorkloadSpread one by one:
1. If `missingReplicas` > 0 or `missingReplicas` = -1 and this subset is not unschedulable, choose this subset temporarily. \
   (1). Update `missingReplicas` -= 1 and put this pod into `creatingPods` map and try to update subsetStatus. \
   (2). If update to WorkloadSpread conflicts with other updates, get the WorkloadSpread again and go back to step (1). \
   (3). if update to WorkloadSpread successfully, inject the rules of this subset into the Pod.
2. If `missingReplicas` = 0, go to the next subset.
3. If there is no available subset left, just let the Pod go.

#### Pod deletion

Also, when a Pod that belongs to a subset is being deleted or evicted, webhook will put it into the `deletingPods` map in subsetStatus and update `missingReplicas` += 1.

#### Update Pod Status

When a Pod that belongs to a subset changes status phase to 'succeed' or 'failed', which means Pod has been terminated lifecycle, webhook will update `missingReplicas` += 1.

### Reschedule strategy

Reschedule strategy will delete unscheduled Pods that still in pending status. Some subsets have no sufficient resource can lead to some Pods unscheduable.
WorkloadSpread has multiple subset, so the unschedulable pods should be rescheduled to other subsets.

Controller will mark the subset containing unscheduable pods to unscheduable status and Webhook cannot inject Pod into this subset by check subset's status.
And then controller cleans up all unscheduable Pods to trigger workload creating new replicas, and webhook will skip unscheduable subset and chose suitable subset to inject.

The unscheduled subset can be kept for 10 minutes and then should be recovered schedulable to schedule Pod again by controller.

### Deletion priority
We use deletion-cost feature to restrict replica numbers of each subset when scaling down.
If `controller.kubernetes.io/pod-deletion-cost` annotation is set, then the pod with the lower value will be deleted first.

We have four types for subset's Pod deletion-cost
1. The number of active Pods in this subset <= maxReplicas, deletion-cost = 100. indicating the priority of Pods \
   in this subset is higher than other Pods in workload.
2. The subset's maxReplicas is nil, deletion-cost = 0.
3. The number of active Pods in this subset > maxReplicas, two class: (a) deletion-cost = -100, (b) deletion-cost = +100. \
   indicating we prefer deleting the Pod have -100 deletion-cost in this subset in order to control the instance of subset \
   meeting up the desired maxReplicas number.
4. The remaining pods do not belong to any subset, deletion-cost = -100.

If WorkloadSpread changes its subset maxReplicas, Pods will not be recreated, but WorkloadSpread will adjust deletion-cost annotation through the above algorithm.

To change scheduleStrategy of WorkloadSpread will keep the deletion-cost annotation of Pod.

## Caution
When you adjust the subset's maxReplicas, you need to trigger the workload's rollout to make the existing Pods meeting the new topology spread.

The workload scales up when it's maxReplicas type is percent, but the new Pods don't meet the desired spread.
The possible reason is the race between the kruise informer and the controller-manager informer.
The controller doesn't update the latest workloadReplicas, leading to the corresponding missingReplicas can't be scaled up in time.

## Alternative Considered

### UnitedDeployment

Both UnitedDeployment and WorkloadSpread allow the workload to be distributed in different subsets, **but the WorkloadSpread requires no workload api changes**.
One can use vanilla CloneSet even Deployment while attaching extra topology constraints.

WorkloadSpread can also support the elastic scenario. For example, user has two topology subset: ECS and virtual-kubelet.
ECS should hold on 30 replicas, and the extra Pods should be scheduled to virtual-kubelet.
```yaml
  subsets:
    - name: ecs
      requiredNodeSelectorTerm:
        matchExpressions:
          - key: type
            operator: NotIn
            values:
              - virtual-kubelet
      maxReplicas: 30
    - name: vk
      requiredNodeSelectorTerm:
        matchExpressions:
          - key: type
            operator: In
            values:
              - virtual-kubelet
      tolerations:
        - key: virtual-kubelet.io/provider
          operator: Exists
```
**If the workload instance is scaled out, but the WorkloadSpread CR is not adjusted accordingly, unexpected behaviors may happen.**

### NodeAffinity

Add nodeAffinity to the workload template, such as preferredDuringSchedulingIgnoredDuringExecution, which can be scheduled in multiple regions.
However, it cannot limit the replica numbers of a subset, and it is only effective when scaling out the workload, not effective when scaling in.

The internal implementation of WorkloadSpread is also based on nodeAffinity, but it will provide richer control for multiple subsets.

### PodTopologySpread

You can use topology spread constraints to control how Pods are spread across your cluster among failure-domains such as regions, zones, nodes, and other user-defined topology domains.
This can help to achieve high availability as well as efficient resource utilization.

1. WorkloadSpread's subset maxReplicas can be a percentage string, which defines the percentage spread in multiple topology, but PodTopologySpread doesn't support this feature.
2. For PodTopologySpread, there's no guarantee that the constraints remain satisfied when Pods are removed.
   For example, scaling down a Deployment may result in imbalanced Pods distribution.
   For WorkloadSpread, controller will guarantee the workload replicas meeting up maxReplicas by deletion-cost feature.