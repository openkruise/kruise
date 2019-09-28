---
# v0.2.0

## Installation

- Provide a script to generate helm charts for Kruise. User can specify the release version.
- Automatically install kubebuilder if it does not exist in the machine.
- Add Kruise uninstall script. 

## Kruise-Controller-Manager

### Bugs

- Fix a potential controller crash problem when APIServer disables MutatingAdmissionWebhook and ValidatingAdmissionWebhook admission plugins.

## Broadcast Job

### Features

- Change the type of `Parallelism` field in BroadcastJob from `*int32` to `intOrString`.
- Support `Pause` in BroadcastJob.
- Add `FailurePolicy` in BroadcastJob, supporting `Continue`, `FastFailed`, and `Pause` polices.
- Add `Phase` in BroadcastJob `status`.


## Sidecar Set

### Features

- Allow parallely upgrading SidecarSet Pods by specifying `MaxUnavailable`.
- Support sidecar volumes so that user can specify volume mount in sidecar containers.

---

# v0.1.0

## Kruise-Controller-Manager

### Features

- Support to run kruise-controller-manager locally
- Allow selectively install required CRDs for kruise controllers

### Bugs

- Remove `sideEffects` in kruise-manager all-in-one YAML file to avoid start failure

## Advanced Statefulset

### Features

- Add MaxUnavailable rolling upgrade strategy
- Add In-Place pod update strategy
- Add paused functionality during rolling upgrade

## Broadcast Job

### Features

- Add BroadcastJob that runs pods on all nodes to completion
- Add `Never` termination policy to have job running after it finishes all pods
- Add `ttlSecondsAfterFinished` to delete the job after it finishes in x seconds.

### Bugs

- Make broadcastjob honor node unschedulable condition

## Sidecar Set

### Features

- Add SidecarSet that automatically injects sidecar container into selected pods
- Support sidecar update functionality for SidecarSet
