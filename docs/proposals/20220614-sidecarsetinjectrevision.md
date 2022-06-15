---
title: SidecarSet Inject Historical Version 
authors:
- "@veophi"
reviewers:
- "@zmberg"
- "@furykerry"
- "@FillZpp"
creation-date: 2021-06-15
last-updated: 2021-06-15
status: implementable
---

# SidecarSet Inject Historical Version

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [SidecarSet Inject Historical Version](#sidecarset-inject-historical-version)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Proposal](#proposal)
  - [Way 1: always inject historical revision of sidecar](#way-1-always-inject-historical-revision-of-sidecar)
    - [Advantage:](#advantage)
    - [Disadvantage:](#disadvantage)
  - [Way 2: inject historical/latest revision of sidecar in proportion](#way-2-inject-historicallatest-revision-of-sidecar-in-proportion)
    - [With Random Algorithm](#with-random-algorithm)
      - [Random With Probability](#random-with-a-probability)
      - [Same As ResourceQuota](#same-as-resourcequota)


## Motivation
Currently, SidecarSet always injects the latest revision sidecar container to Pod, which will lead to some troubles. 
For example, if deployment is upgraded when SidecarSet is upgrading, all sidecars managed by the SidecarSet will be upgraded even if SidecarSet `Partition` is set.

Therefore, we should provide a way to allow SidecarSet inject historical/specific revision.

## Proposal
There are two possible ways to address this problem.

## Way 1: Always inject specific revision of sidecar
```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
....
spec:
  injectStrategy:
    # Default to the latest controller-revision
    controllerRevision: xxxxxx #controller-revision-name of this SidecarSet
```

If `InjectStrategy.controllerRevision` is set, SidecarSet will always inject this specific revision of sidecar to newly-created pods. 
If so, the old sidecar of these newly-created pods will be upgraded by SidecarSet according to its `updateStrategy`.

### Advantage:
- Very easy to implement and understand.

### Disadvantage:
- If users have upgraded 90%(even 100%) sidecar to a matched Deployment, if the Deployment is upgraded at this moment, 
all pods' sidecar will be rollback to specific revision due to the pod rebuilding and this `injectStrategy`.

## Way 2: Inject historical/latest revision of sidecar in proportion
### With Random Algorithm
```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
....
spec:
  injectStrategy:
    # Default to the latest controller-revision
    revision: xxxxxx #controller-revision-name of this SidecarSet
```

The configuration is same as ## Way 1, but SidecarSet will consider the `Partition` field to decide which revision should be injected to newly-created Pods.

### Algorithm
#### Random With Probability
suppose that `proportion = Partition / MatchedReplicas`, `X ~ uniform(0, 1.0)`:
- if `X <= proportion`, SidecarSet will inject the specific revision sidecar to pod;
- if `X > proportion`, SidecarSet will inject the latest revision sidecar to pod.

**Advantage:**
- Very easy to implement.

**Disadvantage:**
- Unstable;
- It may not make much sense if replicas is not very large.

#### Same As ResourceQuota
we treat the number of specific revision sidecar as a kind of Resource Quota, and we just refer to the same algorithm as [ResourceQuota](https://kubernetes.io/zh-cn/docs/concepts/policy/resource-quotas/) to solve this problem.

**Advantage:**
- Stable.
- Much more in-line with user expectations;

**Disadvantage:**
- Hurt to performance;
- Hard to implement. 