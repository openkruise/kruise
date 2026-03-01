---
title: Resource Distribution
authors:
- "@veophi"
reviewers:
- "@Fei-Guo"
- "@furykerry"
- "@FillZpp"
creation-date: 2021-07-19
last-updated: 2021-07-28
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
      - [Resource Conflict](#resource-conflict)
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
2. Support namespace label selector;
3. Support managing the resources, including synchronization and clean;
4. Be safe.

**I investigated lots of community solutions for `Resource Distribution`, and found that none of them satisfy all the requirements above. Thus, I think it's meaningful to provide this feature for kruise considering the use of `SidecarSet`.**

## Proposal
We provide a CRD solution in this proposal.
If you want to achieve `Resource Distribution` based on `Spec.Annotations`, I recommend you to use [kubernetes-replicator](https://github.com/mittwald/kubernetes-replicator).

But, compared with [kubernetes-replicator](https://github.com/mittwald/kubernetes-replicator), the uniques of our design are:
1. More flexible support for adding other types of resources (if you need);
2. Support for automated resource cleanup.

Sure, we also have some disadvantages compared with kubernetes-replicator:
1. Cannot distribute existing resources (for safety);
2. Higher cost of use (users have to write more yaml).

In this design, we will support the distribution for the following resources (we may support more types of resources in the future
):
1. Secret;
2. ConfigMap.

By the way, why we don't support distribute existing resources:
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
  targets: # options: ["cluster", "namespaces", "namespaceLabelSelector"]
    excludedNamespaces: #all namespaces will be selected, except for kube-system, kube-public and the listed namespaces.
       - Name: some-ignored-ns-1
       - Name: some-ignored-ns-2
- Status: #written by `ResourceDistribution` automatically if resource distribution failed.
  description: "Resource distribution failed: Name Conflict." # logs: "Resource %s has existed in some namespaces, please rename your resource or adopt [`overwrite`, 'ignore'] writePolicy."
  conflictingNamespaces:
    - name: some-conflict-ns-1
    - name: some-conflict-ns-2
```
#### Annotations
Record the source and version of resource.

#### Resource Conflict

Resource will conflict with the resources with the same name that already exist in target namespaces.
 - ResourceDistribution is created only when no conflict occurs.
 - The resource will be distributed **if and only if** there is no conflict (distribute to all, or none of them);
 - All conflicting namespaces will be detected and listed when creating ResourceDistribution;

#### Targets
The `targets` field has other two options except for `excludedNamespaces`:
1. If choose `includedNamespaces`, the listed namespaces will be selected.
```yaml
  targets:
    includedNamespaces:
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

**Special Cases:**
1. The default target is `excludedNamespaces`, if no targets option is enabled, it will distribute to all namespaces.
3. If more than one `targets` options were enabled, **the intersection** of their results will be distributed to.

#### Synchronization
1. When the `ResourceDistribution` is updated, the resource and its replica will be synchronized.
2. After ResourceDistribution creation, you can image that ResourceDistribution is `Deployment`, and the Resource is `Pod`. If `Pod` (`Resource`) is modified directly, `Deployment`(`ResourceDistribution`) will not restore it back when reconciling.

### How to Implement

#### API definition
```go
package resourcedistribution

type ResourceDistribution struct {
  metav1.TypeMeta   `json:",inline"`
  metav1.ObjectMeta `json:"metadata,omitempty"`

  Spec   ResourceDistributionSpec   `json:"spec,omitempty"`
  Status ResourceDistributionStatus `json:"status,omitempty"`
}
// ResourceDistributionSpec defines the desired state of ResourceDistribution.
type ResourceDistributionSpec struct {
  // INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
  // Important: Run "make" to regenerate code after modifying this file

  // Resource must be the complete yaml that users want to distribute
  Resource runtime.RawExtension `json:"resource,omitempty"`

  // Targets defines the namespaces that users want to distribute to
  Targets ResourceDistributionTargets `json:"targets,omitempty"`
}

// ResourceDistributionTargets defines the targets of Resource.
// Four options are provided to select target namespaces.
// If more than options were selected, their **intersection** will be selected.
type ResourceDistributionTargets struct {
  // Resource will be distributed to all namespaces, except listed namespaces
  // Default target
  // +optional
  ExcludedNamespaces []ResourceDistributionNamespace `json:"excludedNamespaces,omitempty"`

  // If it is not empty, Resource will be distributed to the listed namespaces
  // +optional
  IncludedNamespaces []ResourceDistributionNamespace `json:"includedNamespaces,omitempty"`

  // If NamespaceLabelSelector is not empty, Resource will be distributed to the matched namespaces
  // +optional
  NamespaceLabelSelector metav1.LabelSelector `json:"namespaceLabelSelector,omitempty"`
}

type ResourceDistributionNamespace struct {
  // Namespace name
  Name string `json:"name,omitempty"`
}

// ResourceDistributionStatus defines the observed state of ResourceDistribution.
// ResourceDistributionStatus is recorded by kruise, users' modification is invalid and meaningless.
type ResourceDistributionStatus struct {
  // INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
  // Important: Run "make" to regenerate code after modifying this file

  // Describe ResourceDistribution Status
  Description string `json:"description,omitempty"`

  // DistributedResources lists the resources that has been distributed by this ResourceDistribution
  // Example: "ns-1": "12334234", "ns-1" denotes a namespace name, "12334234" is the Resource version
  DistributedResources map[string]string `json:"distributedResources,omitempty"`
}


```
#### WebHook Validation
**Create:**
1. Check whether Resource belongs to `Secret` or `ConfigMap`;
2. Check the conflicts with existing resource;

**Update:**
1. Check whether the Resource apiVersion, kind, and name are changed.

#### Distribution and Synchronization

1. Create and Distribute
- Parse and analyze the resource and the target namespaces.
- Create the resource based on `Resource` field.
- Replicate and distribute the resource, and set their `OwnerReference` as the `ResourceDistribution`.

2. Update and Synchronize
- When `Resource` field is updated:
  - Update resource and synchronize its replicas.
- When `Targets` field is updated:
  - Parse and analyze new target namespaces;
  - Clear replicas for the old namespaces that aren't in the new targets;
  - Replicate or update resource for the new target namespaces.
- When a new namespace is created:
  - Get all `ResourceDistribution`
  - Check whether each `ResourceDistribution` is matched with the namespace, if so, distribute this resource.

#### Deletion
- Benefiting from `OwnerReference`, the replicas will be cleaned when the `ResourceDistribution` is deleted.

## Implementation History
- [ ] 07/28/2021: Proposal submission
