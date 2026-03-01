---
title: ContainerPriority
authors:
  - "@FillZpp"
reviewers:
  - "@Fei-Guo"
  - "@furykerry"
creation-date: 2021-08-09
last-updated: 2021-08-09
status: implementable
---

# ContainerLaunchPriority

Provide a way to help users control the sequence of containers launch in a Pod.

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [ContainerPriority](#containerpriority)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Proposal](#proposal)
    - [API definition](#api-definition)
    - [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    - [Requirements](#requirements)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Implementation History](#implementation-history)

## Motivation

Containers in a same Pod in it might have dependence, which means the application in one container runs depending on another container.
For example:

1. Container A has to start first. Container B can start only if A is already running.
2. Container B has to exit first. Container A can stop only if B has already exited.

Currently, the sequences of containers start and stop are controlled by Kubelet.
Kubernetes used to have a [KEP](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/753-sidecar-containers),
which plans to add a type field for container to identify the priority of start and stop.
However, it has been refused because of [sig-node thought it may bring a huge change to code](https://github.com/kubernetes/enhancements/issues/753#issuecomment-713471597).

Now, OpenKruise tries to provide a way to help users control the sequence of containers start in a Pod.
Unfortunately, the sequence of stop can still only be controlled by the preStop hook in containers, which requires user to configure.

## Proposal

### API definition

Users can set the priority in `env` of each container:

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: main
    # ...
  - name: sidecar
    env:
    - name: KRUISE_CONTAINER_LAUNCH_PRIORITY
      value: "1"
    # ...
```

1. The range of the value is `[-2147483647, 2147483647]`. Defaults to `0` if no such env exists.
2. The container with higher priority will be guaranteed to start before the others with lower priority.
3. The containers with same priority have no limit to their start sequence.

Or you can set the annotation in Pod to declare it needs ordered by containers sequence:

```yaml
apiVersion: v1
kind: Pod
  labels:
    apps.kruise.io/container-launch-priority: Ordered
spec:
  containers:
  - name: main
    # ...
  - name: sidecar
    # ...
```

It means the former container (main) in containers is guaranteed to be started before the later container (sidecar).

*Note that the Pod has no limit to its owner, which means it can be created by Deployment, CloneSet or any other controllers.*

### Implementation Details/Notes/Constraints

When Kruise webhook find `KRUISE_CONTAINER_LAUNCH_PRIORITY` env in a creating Pod, it should inject `KRUISE_CONTAINER_LAUNCH_BARRIER` env into containers.

The value of `KRUISE_CONTAINER_LAUNCH_BARRIER` is set from a ConfigMap named `{pod-name}-barrier` and the key is related to priority of this container.

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: main
    # ...
    env:
    - name: KRUISE_CONTAINER_LAUNCH_BARRIER
      valueFrom:
        configMapKeyRef:
          name: {pod-name}-barrier
          key: "c_0"   # no KRUISE_CONTAINER_LAUNCH_PRIORITY, so defaults to 0
  - name: sidecar
    env:
    - name: KRUISE_CONTAINER_LAUNCH_PRIORITY
      value: "1"
    - name: KRUISE_CONTAINER_LAUNCH_BARRIER
      valueFrom:
        configMapKeyRef:
          name: {pod-name}-barrier
          key: "c_1"   # its KRUISE_CONTAINER_LAUNCH_PRIORITY is 1
    # ...
```

A new controller in Kruise should create an empty ConfigMap for this created Pod, if it finds `KRUISE_CONTAINER_LAUNCH_BARRIER` env in the Pod.
Then the controller adds the different keys into the ConfigMap according to the priorities and containerStatuses of the containers.

For example, the controller will firstly add `c_1` key into ConfigMap and waiting for the `sidecar` container running and ready.
When it confirms this from containerStatuses in Pod status, it will then add `c_0` key into ConfigMap to let Kubelet start `main` container.

### Requirements

ContainerLaunchPriority requires `PodWebhook` to be enabled, which is the default state.

### Risks and Mitigations

Currently, ContainerLaunchPriority only controls the sequence of containers start for Pod creation.
It **can not** restart `main` container again if the `sidecar` container crashes and restarts after running.

## Implementation History

- [ ] 09/08/2021: Proposal submission
