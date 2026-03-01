---
title: Proposal Template
authors:
- "@FillZpp"
reviewers:
- "@Fei-Guo"
- "@furykerry"
creation-date: 2021-03-31
last-updated: 2021-03-31
status: implementable
---

# Cascading deletion protection

Provide a safety policy to protect Kubernetes resources from the cascading deletion mechanism.

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Disable cascading deletion](#disable-cascading-deletion)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
    - [Requirements](#requirements)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Implementation History](#implementation-history)

## Motivation

Currently, there are so many risks can be caused by cascading deletion mechanism:

1. delete a CRD mistakenly, all CR disappeared
2. delete a Workload mistakenly, all Pods belongs to it deleted
3. delete all Workloads mistakenly in batches, all applications in the cluster unavailable
4. delete a namespace, all resources in it deleted

Kruise should provide a safety policy which could help users protect Kubernetes resources and
applications' availability from the cascading deletion mechanism.

## Proposal

API definition:

- a new feature-gate named `ResourcesDeletionProtection`
- a label named `policy.kruise.io/delete-protection`, the values can be:
  - `Always`: this object will always be forbidden to be deleted, unless the label is removed
  - `Cascading`: this object will be forbidden to be deleted, if it has active resources owned

If the feature-gate has enabled, the resources below with this label will be validated for deletion operation:

| Kind                        | Group                  | Version            | Cascading judgement                                |
| --------------------------- | ---------------------- | ------------------ | ----------------------------------------------------
| `Namespace`                 | core                   | v1                 | whether there is active Pods in this namespace     |
| `CustomResourceDefinition`  | apiextensions.k8s.io   | v1beta1, v1        | whether there is existing CRs of this CRD          |
| `Deployment`                | apps                   | v1                 | whether the replicas is 0                          |
| `StatefulSet`               | apps                   | v1                 | whether the replicas is 0                          |
| `ReplicaSet`                | apps                   | v1                 | whether the replicas is 0                          |
| `CloneSet`                  | apps.kruise.io         | v1alpha1           | whether the replicas is 0                          |
| `StatefulSet`               | apps.kruise.io         | v1alpha1, v1beta1  | whether the replicas is 0                          |
| `UnitedDeployment`          | apps.kruise.io         | v1alpha1           | whether the replicas is 0                          |

### User Stories

1. delete a CRD mistakenly, all CR disappeared
2. delete a Workload mistakenly, all Pods belongs to it deleted
3. delete all Workloads mistakenly in batches, all applications in the cluster unavailable
4. delete a namespace, all resources in it deleted

### Requirements

If users enable `ResourcesDeletionProtection` feature-gate when install or upgrade Kruise,
Kruise will require more authorities:

1. Webhook for deletion operation of namespace, crd, deployment, statefulset, replicaset and workloads in Kruise.
2. By default, clusterRole for reading all resource types, because CRD validation needs to list the CRs of this CRD.
   Optionally, users can use `manager.role.groupsAuthorization` helm parameter to only limit which groups can be listed by Kruise.

### Implementation Details/Notes/Constraints

Intercept deletion operation of the resources with `policy.kruise.io/delete-protection`:

1. For a `Namespace`, list the active Pods in it
2. For a workload, just decide by its replicas
3. For a `CRD`, list the CRs of it using unstructured client

### Risks and Mitigations

If all kruise-manager Pods are crashed or in other abnormal states, the deletion webhook will fail for the resources above,
which means these resources can not be deleted temporarily.

## Implementation History

- [ ] 31/03/2021: Proposal submission
