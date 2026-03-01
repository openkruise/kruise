---
title: Proposal Template
authors:
- "@FillZpp"
reviewers:
- "@Fei-Guo"
- "@furykerry"
creation-date: 2021-03-26
last-updated: 2021-03-26
status: implementable
---

# Pre-download image during in-place update

If a workload is going to update Pods in-place,
the controller could pre-download the new image on the Nodes of all Pods in the workload.

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Pre-download image during in-place update](#pre-download-image-during-in-place-update)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
    - [Requirements](#requirements)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Implementation History](#implementation-history)

## Motivation

If the image has been pre-downloaded on Nodes, we can significantly accelerate the speed of in-place update,
for Kubelet has not to pull the image again.

## Proposal

Add a new feature-gate: `PreDownloadImageForInPlaceUpdate`.

If it is opened, workloads like CloneSet, Advanced StatefulSet or SidecarSet will create one or more `ImagePullJobs` when it starts a new in-place update.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: ImagePullJob
metadata:
  namespace: cloneset-namespace
  name: cloneset-name-{random-id}
spec:
  image: nginx:1.9.1
  parallelism: 1
  podSelector:
    matchLabels:
      {the selector of CloneSet}
    matchExpressions:
      - key: controller-revision-hash
        operator: NotIn
        values:
        - {the updateRevision of CloneSet}
  completionPolicy:
    type: Always
  pullPolicy:
    backoffLimit: 1
    timeoutSeconds: 300
```

Default values:

- The requested parallelism `parallelism` defaults to `1`.
- The timeout for pulling image `timeoutSeconds` defaults to `300`.

Also, users can configure them in their workloads:

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
metadata:
  annotations:
    apps.kruise.io/image-predownload-parallelism: 10
    apps.kruise.io/image-predownload-timeout-seconds: 600
```

### User Stories

#### Story 1

Image pulling is the most time-consuming stage in an in-place update progress.

If the new image has already been downloaded on the Node, it makes in-place update more efficient.

### Requirements

It relies on `ImagePullJob` and `NodeImage` to pre-download images.

So if the `KruiseDaemon` feature-gate is closed, `PreDownloadImageForInPlaceUpdate` can neither be opened.

### Implementation Details/Notes/Constraints

1. User updates an image in CloneSet template.
2. If the cloneset-controller finds it is an in-place update,
  the controller will create an ImagePullJob to pull the new image on the Nodes of its Pods.
3. When sorting Pods to update, CloneSet controller will firstly sort them by the image has already downloaded on their Nodes.
  Since there is many sorting criteria of pods to update, the image-downloaded sorting will be the lowest priority.
4. After all Pods updated or the template been changed, CloneSet controller will delete the previous ImagePullJob.

Since different workloads have different sorting logics to update Pods in-place, so imagepulljob-controller will get the workload
from the owner reference of ImagePullJob, and call the sorting function of the workload controller to sort the Pods before pulling
images on their Nodes.

### Risks and Mitigations

- The ImagePullJob may caused the image downloading parallelly on multiple Nodes. So we have to limit the `parallelism` number.

## Implementation History

- [ ] 26/03/2021: Proposal submission
