# v0.3.0

## Installation

- Provide a script to check if the K8s cluster has enabled MutatingAdmissionWebhook and ValidatingAdmissionWebhook admission plugins before installing Kruise.
- Users can now install specific controllers if they only need some of the Kruise CRD/controllers.

## Kruise-Controller-Manager

### Bugs

- Fix a jsonpatch bug by updating the vendor code.

## Advanced StatefulSet

### Features

- Add condition report in `status` to indicate the scaling or rollout results.

## A NEW workload controller - UnitedDeployment

### Features

- Define a set of APIs for UnitedDeployment workload which manages multiple workloads spread over multiple domains in one cluster.
- Create one workload for each `Subset` in `Topology`.
- Manage Pod replica distribution across subset workloads.
- Rollout all subset workloads by specifying a new workload template.
- Manually manage the rollout of subset workloads by specifying the `Partition` of each workload.

## Documents

- Three blog posts are added in Kruise [website](http://openkruise.io/en-us/blog/index.html), titled:
    1. Kruise Controller Classification Guidance.
    2. Learning Concurrent Reconciling.
    3. UnitedDeploymemt - Supporting Multi-domain Workload Management.
- New documents are added for UnitedDeployment, including a [tutorial](./docs/tutorial/uniteddeployment.md).
- Revise main README.md.

---

**v0.2.0**

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

- Allow parallelly upgrading SidecarSet Pods by specifying `MaxUnavailable`.
- Support sidecar volumes so that user can specify volume mount in sidecar containers.

---

**v0.1.0**

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
