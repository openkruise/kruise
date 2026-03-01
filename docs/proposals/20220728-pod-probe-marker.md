---
title: PodProbeMarker
authors:
- "@zmberg"
reviewers:
- "@furykerry"
- "@FillZpp"
creation-date: 2022-08-09
last-updated: 2021-09-26
status: implementable
---

# Pod Probe Marker

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Title](#title)
- [Table of Contents](#table-of-contents)
- [Motivation](#motivation)
- [Proposal](#proposal)
  - [API Definition](#api-definition)
  - [Implementation](#implementation)
  - [Relationship With Startup Probe](#relationship-with-startup-probe)

## Motivation
Kubernetes provides two probes by default for Pod lifecycle management:
- **Readiness Probe** is used to determine whether the business container is ready to respond to requests, and if it fails, the Pod will be removed from the Service Endpoints.
- **Liveness Probe** is used to determine the health status of the container, and if it fails, Kebelet will restart the Container.

**So K8S on the provision of Probe capabilities are limited to specific semantics and behavior. In addition, there are some business applications that have the need to customize the semantics and behavior of the Probe**, such as:
- GameServer defines Idle Probe to determine whether there is a game match for the current Pod, if there is no match, from cost optimization considerations can be given priority to offline this Pod
- Operator defines Master-Slave Probe to determine the role of the current Pod (master or slave), and upgrade the Slave node in priority

## Proposal
OpenKruise provides the ability to customize Container Probe, Kruise Daemon executes customize Probe scripts and returns the results to the Pod yaml.

### API Definition
```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: PodProbeMarker
metadata:
  name: game-server-probe
  namespace: ns
spec:
  selector:
    matchLabels:
      app: game-server
  probes:
  - name: Idle
    containerName: game-server
    probe:
      exec: /home/game/idle.sh
      initialDelaySeconds: 10
      timeoutSeconds: 3
      periodSeconds: 10
      successThreshold: 1
      failureThreshold: 3
    markerPolicy:
    - state: Succeeded
      labels:
        gameserver-idle: 'true'
      annotations:
        controller.kubernetes.io/pod-deletion-cost: '-10'
      - state: Failed
      labels:
        gameserver-idle: 'false'
      annotations:
        controller.kubernetes.io/pod-deletion-cost: '10'
    podConditionType: game.io/idle

# for kruise daemon
apiVersion: apps.kruise.io/v1alpha1
kind: NodePodProbe
metadata:
  name: node-name
spec:
  podContainerProbes:
  - podName: gameserver-0
    containerProbes:
    - name: game-server-probe#Idle
      containerName: gameserver
      probes:
        exec: /home/game/idle.sh
        initialDelaySeconds: 10
        timeoutSeconds: 3
        periodSeconds: 10
        successThreshold: 1
        failureThreshold: 3

status:
  podProbeResults:
  - podName: gameserver-0
    containerProbes:
    - name: game-server-probe#Idle
      status: 'Failed'


// probe result
apiVersion: v1
kind: Pod
metadata:
  name: gameserver-0
  labels:
    gameserver-idle: 'false'
  annotations:
    controller.kubernetes.io/pod-deletion-cost: '10'
spec:
  ...
status:
  conditions:
  - type: game.io/idle
    status: 'False'
```

### Implementation
- **pod-probe-controller**: Responsible for automatically generating and cleaning NodePodProbe resources (one per Node) based on PodProbeMarker
and writing the results back to Pod yaml based on NodePodProbe Status
- **kruise-daemon**: Responsible for executing the probe (EXEC, HTTP) and returning the results to NodePodProbe Status

### Relationship With Startup Probe
StartupProbe indicates that the Pod has successfully initialized. If specified, no other probes are executed until this completes successfully.
So Kruise PodProbeMarker will be executed until StartupProbe succeed.


