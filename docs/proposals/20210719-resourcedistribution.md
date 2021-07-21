---
title: Resource Distribution
authors:
- "@veophi"
reviewers:
- "@Fei-Guo"
- "@furykerry"
- "@FillZpp"
creation-date: 2021-07-19
last-updated: 2021-07-19
status: implementable
---

# Resource Distribution
Provide a way to distribute and synchronize some kinds of resources across namespaces.

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Resource Distribution](#resource-distribution)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [What should it look like](#what-should-it-look-like)
  - [Proposal](#proposal)
    - [User Story](#user-story)
      - [Annotations](#annotations)
      - [WritePolicy](#writepolicy)
      - [Targets](#targets)
      - [Synchronization](#synchronization)
    - [How to Implement](#how-to-implement)
      - [API definition](#api-definition)
      - [WebHook Validation](#webhook-validation)
      - [Distribution and Synchronization](#distribution-and-synchronization)
  - [Implementation History](#implementation-history)

## Motivation
`Resource Distribution` provides a service that is responsible for distributing and synchronizing some kinds of resources, such as `Secret` and `ConfigMap` across namespaces.
For example, in the following scenarios, the `Resource Distribution` may be helpful:
1. Pull private images for `SidecarSet` using `ImagePullSecret`;
2. Distribute ConfigMaps for `SidecarSet` across namespaces;

## What should it look like
In our mind, the `Resource Distribution` should:
1. Support distributing resources to all namespaces or listed namespaces;
2. Support label selector, including workload label selector and namespace label selector;
3. Support managing the resources, including synchronization and clean;
4. Be safe.

**I investigated lots of community solutions for `Resource Distribution`, and found that none of them satisfy all the requirements above. Thus, I think it's meaningful to provide this feature for kruise considering the use of `SidecarSet`.**

## Proposal
We provide a CRD solution in this proposal.
If you want to achieve `Resource Distribution` based on `Spec.Annotations`, I recommend you to use [kubernetes-replicator](https://github.com/mittwald/kubernetes-replicator).

But, compared with [kubernetes-replicator](https://github.com/mittwald/kubernetes-replicator), the uniques of our design are that:
1. Support workload label selector;
2. Support automated replica cleanup.

Sure, we also have some disadvantages compared with kubernetes-replicator:
1. Cannot distribute existing resources (for safety);
2. Higher cost of use (users have to write more yaml).

In this design, we will support the distribution for the following resources:
1. Secret;
2. ConfigMap.

By the way, why we cannot support distribute existing resources:
1. Pull secret from other namespace is very unsafe and dangerous;
2. When the source is deleted, its replicas may be no longer cleaned.

### User Story
```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: ResourceDistribution
metadata:
  name: secret-distribution
spec:
  resource:
    apiVersion: v1
    kind: Secret # We will validate and limit the kind of the resource
    metadata:
      name: secret-sa-sample
      ownerReferences: # written by `ResourceDistribution` automatically
        ...
      annotations: # written by `ResourceDistribution` automatically
        openKruise.io/resource-distributed-by: "resource-distribution:secret-distribution"
        openKruise.io/resource-distributed-time: "2021-07-20T03:00:00"
    type: kubernetes.io/service-account-token
    data:
      extra: YmFyCg==
  wirtePolicy: strict #strict for default, or use [`overwrite`, 'ignore']
  targets: # options: ["cluster", "namespaces", "workloadLabelSelector", "namespaceLabelSelector"]
    all: #all namespaces will be selected, except for kube-system, kube-public and the listed namespaces.
       - exception: some-ignored-ns-1
       - exception: some-ignored-ns-2
- Status: #written by `ResourceDistribution` automatically if resource distribution failed.
  description: "Resource distribution failed: Name Conflict." # logs: "Resource %s has existed in some namespaces, please rename your resource or adopt [`overwrite`, 'ignore'] writePolicy."
  conflictingNamespaces:
    - name: some-conflict-ns-1
    - name: some-conflict-ns-2
```
#### Annotations
Record the source of resource and creation time.

#### WritePolicy
The `writePolicy` specifies the write operation when the resource conflict with existing resources.

1. If `writePolicy` is `strict`:
 - The resource will be distributed **iff** there is no conflict (distribute to all, or none of them);
 - All conflicting namespaces will be listed in `conflictingNamespaces`;
 - Users can get `Name Conflict` in kruise log.
2. If `writePolicy` is `overwrite`:
 - The existing resources with the same name will be overwritten;
 - All conflicting namespaces will be listed in `conflictingNamespaces`;
3. If `writePolicy` is `ignore`:
 - The resource will not be distributed to the conflicting namespaces (but will be distributed to the others);
 - All conflicting namespaces will be listed in `conflictingNamespaces`;

#### Targets
The `targets` field has other three options except for `all`:
1. If choose `namespaces`, the listed namespaces will be selected.
```yaml
  targets:
    namespaces:
       - name: some-target-ns-1
       - name: some-target-ns-2
```
2. If choose `namespaceLabelSelector`, the matched namespaces will be selected.
```yaml
  targets:
    namespaceLabelSelector:
      matchLabels:
        group: seven
        environment: test
```
3. If choose `workloadLabelSelector`, the namespaces that contains any matched workload will be selected.
```yaml
  targets:
    workloadLabelSelector:
      kind: CloneSet
      matchLabels:
        app: nginx
```

#### Synchronization
1. Only when the `ResourceDistribution` is updated, the resource and its replica will be synchronized.
2. When replicated resource is deleted or updated alone, the `ResourceDistribution` controller will do nothing.

#### Special Cases
1. The default target is `all`.
2. The default writePolicy is `strict`.
3. If more than one `targets` were chosen, the intersection of their results will be applied.

### How to Implement

#### API definition

```go
package resourcedistribution

type ResourceDistribution struct{
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   ResourceDistributionSpec   `json:"spec,omitempty"`
	Status ResourceDistributionStatus 'json:"status,omitempty"'
}

type ResourceDistributionSpec struct{
	Resource runtime.RawExtension `json:"resource,omitempty"`
	WritePolicy string `json:"writePolicy,omitempty"`
	Targets ResourceDistributionTargets `json:"targets,omitempty"`
}

type ResourceDistributionTargets struct{
	All []TargetException `json:"all,omitempty"`
	Namespaces []NamespaceName `json:"namespaces,omitempty"`
	NamespaceLabelSelector ResourceDistributionNamespaceLabelSelector `json:"namespaceLabelSelector,omitempty"`
	WorkloadLabelSelector  ResourceDistributionWorkloadLabelSelector `json:"workloadLabelSelector,omitempty"`
}

type ResourceDistributionNamespaceLabelSelector struct{
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

type ResourceDistributionWorkloadLabelSelector struct{
	Kind string `json:"kind,omitempty"`
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

type ResourceDistributionStatus struct{
	Description string `json:"description,omitempty"`
	ConflictedNamespaces []NamespaceName `json:"exception,omitempty"`
}

type TargetException struct {
	Exception string `json:"exception,omitempty"`
}

type NamespaceName struct{
	Name string `json:"name,omitempty"`
}
```
#### WebHook Validation
Check if the resource belongs to `Secret` or `ConfigMap`.

#### Distribution and Synchronization

1. Create and Distribute
- Parse and analyze the resource and the target namespaces.
- Check conflicts and `WritePolicy`.
- Create the resource based on `Resouce` field.
- Replicate and distribute the resource, and set their `OwnerReference` to the `ResourceDistribution`.

2. Update and Synchronize
- When `Resource` field is updated:
  - Update resource and synchronize its replicas.
- When `Targets` field is updated:
  - Parse and analyze new target namespaces;
  - Clear replicas for the old namespaces that aren't in the new targets;
  - Replicate resource to the new target namespaces.
- When a new workload or namespace is created:
  - Get all `ResourceDistribution`
  - Check whether each `ResourceDistribution` is matched with the workload or namespace, if so, replica and synchronize its resource.

3. Delete
- Benefiting from `OwnerReference`, the replicas will be cleaned when the `ResourceDistribution` is deleted.

## Implementation History
- [ ] 07/19/2021: Proposal submission