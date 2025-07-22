---
title: volumeClaimTemplate for Advanced DaemonSet
authors:

- "@chengjoey"

reviewers:

- "@ChristianCiach"

- "@furykerry"

- "@ABNER-1"

creation-date: 2025-07-22
last-updated: 2025-07-22
status: implementable
---

# volumeClaimTemplate for Advanced DaemonSet
Add volumeClaimTemplate to Advanced DaemonSet

## Table of Contents

- [volumeClaimTemplate for Advanced DaemonSet](#volumeClaimTemplate-for-Advanced-DaemonSet)
    - [Table of Contents](#table-of-contents)
    - [Motivation](#motivation)
    - [Proposal](#proposal)
        - [User Stories](#user-stories)
            - [Story 1](#story-1)
        - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [Implementation History](#implementation-history)

## Motivation

Now, Most of the daemon set specs I have seen so far use hostpath volumes for storage. I would like to use local persistent volumes (= LPV) 
instead of hostpath volumes. The problem is that you need a PVC to get a LPV. 
Daemon sets do not allow you to dynamically create a PVC that claims a LPV by using a dedicated storage class like 
stateful sets do (using volumeClaimTemplates).

We hope to add volumeClaimTemplate to Advanced DaemonSet like stateful sets do.

## Proposal

add volumeClaimTemplate to Advanced DaemonSet like stateful sets do.

### API Definition

```
// DaemonSetSpec defines the desired state of DaemonSet
type DaemonSetSpec struct {
	// volumeClaimTemplates is a list of claims that pods are allowed to reference.
	// The DaemonSet controller is responsible for mapping network identities to
	// claims in a way that maintains the identity of a pod. Every claim in
	// this list must have at least one matching (by name) volumeMount in one
	// container in the template. A claim in this list takes precedence over
	// any volumes in the template, with the same name.
	// TODO: Define the behavior if a claim already exists with the same name.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	VolumeClaimTemplates []corev1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}
```

```
apiVersion: apps.kruise.io/v1alpha1
kind: DaemonSet
metadata:
  name: app-ds
spec:
  selector:
    matchLabels:
      app: app-ds
  template:
    metadata:
      labels:
        app: app-ds
    spec:
      containers:
      - name: nginx
        image: nginx:latest
        imagePullPolicy: IfNotPresent
        volumeMounts:
        - mountPath: /var/lib/nginx
          name: nginx-data
  volumeClaimTemplates:
    - apiVersion: v1
      kind: PersistentVolumeClaim
      metadata:
        name: nginx-data
      spec:
        accessModes:
        - ReadWriteOnce
        resources:
          requests:
            storage: 5Gi
        volumeMode: Filesystem
```

### User Stories

#### Story 1

there are many people are looking for a DaemonSet-like Workload type that supports defining a PVC per Pod.

[Feature Request : volumeClaimTemplates available for Daemon Sets](https://github.com/kubernetes/kubernetes/issues/78902)

[feature request: Add volumeClaimTemplate to advanced DaemonSet](https://github.com/openkruise/kruise/issues/2112)

### Implementation Details/Notes/Constraints

We should consider whether to delete the PVC when a node is deleted, and whether to delete the PVC when the ads is deleted.
We can add a `PersistentVolumeClaimRetentionPolicy`, with optional values of `Retain` and `Delete`, the default being `Retain`.

## Implementation History

- [ ] 07/22/2025: Proposal submission, implement VolumeClaimTemplates create
