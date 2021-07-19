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
2. Support label selector, including pod label selector and namespace label selector;
3. Support managing the resources, including synchronization and clean;
4. Be safe.

**I investigated lots of community solutions for `Resource Distribution`, and found that none of them satisfy all the requirements above. Thus, I think it's meaningful to provide this feature for kruise considering the use of `SidecarSet`.**

## Proposal
We provide a CRD solution in this proposal.
If you want to achieve `Resource Distribution` based on `Spec.Annotations`, I recommend you to use [kubernetes-replicator](https://github.com/mittwald/kubernetes-replicator).

But, compared with [kubernetes-replicator](https://github.com/mittwald/kubernetes-replicator), the unique of our design are that:
1. Support pod label selector;
2. Support automated replica cleanup.

Sure, we also have some disadvantages compared with kubernetes-replicator:
1. Cannot distribute existing resources (for safety);
2. Higher cost of use (users have to write more yaml).

In this design, we will support the distribution for the following resources:
1. Secret;
2. ConfigMap.

### User Story
```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: ResourceDistribution
metadata:
  name: secret-distribution
  namespace: users-namespace
spec: # written by user
  resource: # using runtime.RawExtension to implement
    apiVersion: apps/v1
    kind: Secret # We will validate and limit the kind of the resource
    metadata:
      ame: secret-sa-sample
      annotations:
        kubernetes.io/service-account.name: "sa-name"
    type: kubernetes.io/service-account-token
    data:
      extra: YmFyCg==
  targets: # options: ["cluster", "namespaces", "podLabelSelector", "namespaceLabelSelector"]
    all: #all namespaces will be selected, except for kube-system, kube-public and the listed namespaces.
       - exception: some-ignored-ns-1
       - exception: some-ignored-ns-2
```
The `targets` field has other three options except for `all`:
1. if choose `namespaces`, the listed namespaces will be selected.
```yaml
  targets:
    namespaces:
       - name: some-target-ns-1
       - name: some-target-ns-2
```
2. if choose `namespaceLabelSelector`, the matched namespaces will be selected.
```yaml
  targets:
    namespaceLabelSelector:
       group: seven
       environment: test
```
3. if choose `podLabelSelector`, the namespaces that contains any matched pods will be selected.
```yaml
  targets:
    podLabelSelector:
      app: nginx
```
**Special Cases:**

1. The default target is `all`.
2. If more than one `targets` were chosen, the union of their results will be applied.

### How to Implement

#### API definition

```go
package resourcedistribution

type ResourceDistribution struct{
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   ResourceDistributionSpec   `json:"spec,omitempty"`
}

type ResourceDistributionSpec struct{
	Resource runtime.RawExtension `json:"resource,omitempty"`
	Targets ResourceDistributionTargets `json:"targets,omitempty"`
}

type ResourceDistributionTargets struct{
	All []TargetException `json:"all,omitempty"`
	Namespaces []TargetNamespace `json:"namespaces,omitempty"`
	NamespaceLabelSelector map[string]string `json:"namespaceLabelSelector,omitempty"`
	PodLabelSelector map[string]string `json:"podLabelSelector,omitempty"`
}

type TargetException struct {
	Exception string `json:"exception,omitempty"`
}

type TargetNamespace struct{
	Name string `json:"name,omitempty"`
}

```
#### WebHook Validation
Check if the source resource belongs to `Secret` or `ConfigMap`.

#### Distribution and Synchronization

1. Create and Distribute
- Create the resource based on `Resouce` field.
- Parse and analyze target namespaces.
- Replicate and distribute the resource, and set their `OwnerReference` to the `ResourceDistribution`.

2. Update and Synchronize
- If `Resource` field is updated:
  - Update resource and synchronize its replicas.
- If `Targets` field is updated:
  - Parse and analyze new target namespaces;
  - Clear replicas for the old namespaces that aren't in the new targets;
  - Replicate resource to the new target namespaces.
- If New pod or namespace is created:
  - Get all `ResourceDistribution`
  - Check whether each `ResourceDistribution` is matched with the pod or namespace, if so, replica and synchronize its resource.

3. Delete
- Benefiting from `OwnerReference`, the resource and its replicas will be cleaned when the `ResourceDistribution` is deleted.

## Implementation History
- [ ] 07/19/2021: Proposal submission