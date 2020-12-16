# Change Log

## v0.7.0

Since v0.7.0:

1. OpenKruise requires Kubernetes 1.13+ because of CRD conversion.
  Note that for Kubernetes 1.13 and 1.14, users must enable `CustomResourceWebhookConversion` feature-gate in kube-apiserver before install or upgrade Kruise.
2. OpenKruise official image supports multi-arch, by default including linux/amd64, linux/arm64, and linux/arm platforms.

### A NEW workload controller - AdvancedCronJob
Thanks for @rishi-anand contributing!

An enhanced version of CronJob, it supports multiple kind in a template:

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: AdvancedCronJob
spec:
  template:

    # Option 1: use jobTemplate, which is equivalent to original CronJob
    jobTemplate:
      # ...

    # Option 2: use broadcastJobTemplate, which will create a BroadcastJob object when cron schedule triggers
    broadcastJobTemplate:
      # ...

    # Options 3(future): ...
```

### CloneSet

- **Partition support intOrStr format**
- Warning log for expectation timeout
- Remove ownerRef when pod's labels not matched CloneSet's selector
- Allow updating revisionHistoryLimit in validation
- Fix resourceVersionExpectation race condition
- Fix overwrite gracePeriod update
- Fix webhook checking podsToDelete

### StatefulSet

- **Promote Advanced StatefulSet to v1beta1**
  - A conversion webhook will help users to transfer existing and new `v1alpha1` advanced statefulsets to `v1beta1` automatically
  - Even all advanced statefulsets have been converted to `v1beta1`, users can still get them through `v1alpha1` client and api
- **Support reserveOrdinal for Advanced StatefulSet**

### DaemonSet

- Add validation webhook for DaemonSet
- Fix pending pods created by controller

### BroadcastJob

- Optimize the way to calculate parallelism
- Check ownerReference for filtered pods
- Add pod label validation
- Add ScaleExpectation for BroadcastJob

### Others

- Initializing capabilities if allowPrivileged is true
- Support secret cert for webhook with vip
- Add rate limiter config
- Fix in-place rollback when spec image no latest tag

## v0.6.1

### CloneSet

#### Features

- Support lifecycle hooks for pre-delete and in-place update

#### Bugs

- Fix map concurrent write
- Fix current revision during rollback
- Fix update expectation for pod deletion

### SidecarSet

#### Features

- Support initContainers definition and injection

### UnitedDeployment

#### Features

- Support to define CloneSet as UnitedDeployment's subset

### StatefulSet

#### Features

- Support minReadySeconds strategy

### Others

- Add webhook controller to optimize certs and configurations generation
- Add pprof server and flag
- Optimize discovery logic in custom resource gate

## v0.6.0

### Project

- Update dependencies: k8s v1.13 -> v1.16, controller-runtime v0.1.10 -> v0.5.7
- Support multiple active webhooks
- Fix CRDs using openkruise/controller-tools

### A NEW workload controller - Advanced DaemonSet

An enhanced version of default DaemonSet with extra functionalities such as:

- **inplace update** and **surging update**
- **node selector for update**
- **partial update**

### CloneSet

#### Features

- Not create excessive pods when updating with maxSurge
- Round down maxUnavaliable when maxSurge > 0

#### Bugs

- Skip recreate when inplace update failed
- Fix scale panic when replicas < partition
- Fix CloneSet blocked by terminating PVC

---

## v0.5.0

### CloneSet

#### Features

- Support `maxSurge` strategy which could work well with `maxUnavailable` and `partition`
- Add CloneSet core interface to support multiple implementations

#### Bugs

- Fix in-place update for metadata in template

### StatefulSet

#### Bugs

- Make sure `maxUnavailable` should not be less than 1
- Fix in-place update for metadata in template

### SidecarSet

#### Bugs

- Merge volumes during injecting sidecars into Pod

### Installation

- Expose `CUSTOM_RESOURCE_ENABLE` env by chart set option

---

## v0.4.1

### CloneSet

#### Features

- Add `labelSelector` to optimize scale subresource for HPA
- Add `minReadySeconds`, `availableReplicas` fields for CloneSet
- Add `gracePeriodSeconds` for graceful in-place update

### Advanced StatefulSet

#### Features

- Support label selector in scale for HPA
- Add `gracePeriodSeconds` for graceful in-place update

#### Bugs

- Fix StatefulSet default update sequence
- Fix ControllerRevision adoption

### Installation

- Fix `check_for_installation.sh` script for k8s 1.11 to 1.13

---

## v0.4.0**

### A NEW workload controller - CloneSet

Mainly focuses on managing stateless applications. ([Concept for CloneSet](./docs/concepts/cloneset/README.md))

It provides full features for more efficient, deterministic and controlled deployment, such as:

- **inplace update**
- **specified pod deletion**
- **configurable priority/scatter update**
- **preUpdate/postUpdate hooks**

### Features

- UnitedDeployment supports both StatefulSet and AdvancedStatefulSet.
- UnitedDeployment supports toleration config in subset.

### Bugs

- Fix statefulset inplace update fields in pod metadata such as labels/annotations.

---

## v0.3.1**

### Installation

- Simplify installation with helm charts, one simple command to install kruise charts, instead of downloading and executing scripts.

### Advanced StatefulSet

#### Features

- Support [priority update](./docs/concepts/astatefulset/README.md#priority-unordered-rolling-update-strategy), which allows users to configure the sequence for Pods updating.

#### Bugs

- Fix maxUnavailable calculation, which should not be less than 1.

### Broadcast Job

#### Bugs

- Fix BroadcastJob cleaning up after TTL.

---

## v0.3.0**

### Installation

- Provide a script to check if the K8s cluster has enabled MutatingAdmissionWebhook and ValidatingAdmissionWebhook admission plugins before installing Kruise.
- Users can now install specific controllers if they only need some of the Kruise CRD/controllers.

### Kruise-Controller-Manager

#### Bugs

- Fix a jsonpatch bug by updating the vendor code.

### Advanced StatefulSet

#### Features

- Add condition report in `status` to indicate the scaling or rollout results.

### A NEW workload controller - UnitedDeployment

#### Features

- Define a set of APIs for UnitedDeployment workload which manages multiple workloads spread over multiple domains in one cluster.
- Create one workload for each `Subset` in `Topology`.
- Manage Pod replica distribution across subset workloads.
- Rollout all subset workloads by specifying a new workload template.
- Manually manage the rollout of subset workloads by specifying the `Partition` of each workload.

### Documents

- Three blog posts are added in Kruise [website](http://openkruise.io/en-us/blog/index.html), titled:
  1. Kruise Controller Classification Guidance.
  2. Learning Concurrent Reconciling.
  3. UnitedDeploymemt - Supporting Multi-domain Workload Management.
- New documents are added for UnitedDeployment, including a [tutorial](./docs/tutorial/uniteddeployment.md).
- Revise main README.md.

---

## v0.2.0**

### Installation

- Provide a script to generate helm charts for Kruise. User can specify the release version.
- Automatically install kubebuilder if it does not exist in the machine.
- Add Kruise uninstall script.

### Kruise-Controller-Manager

#### Bugs

- Fix a potential controller crash problem when APIServer disables MutatingAdmissionWebhook and ValidatingAdmissionWebhook admission plugins.

### Broadcast Job

#### Features

- Change the type of `Parallelism` field in BroadcastJob from `*int32` to `intOrString`.
- Support `Pause` in BroadcastJob.
- Add `FailurePolicy` in BroadcastJob, supporting `Continue`, `FastFailed`, and `Pause` polices.
- Add `Phase` in BroadcastJob `status`.

### Sidecar Set

#### Features

- Allow parallelly upgrading SidecarSet Pods by specifying `MaxUnavailable`.
- Support sidecar volumes so that user can specify volume mount in sidecar containers.

---

## v0.1.0**

### Kruise-Controller-Manager

#### Features

- Support to run kruise-controller-manager locally
- Allow selectively install required CRDs for kruise controllers

#### Bugs

- Remove `sideEffects` in kruise-manager all-in-one YAML file to avoid start failure

### Advanced Statefulset

#### Features

- Add MaxUnavailable rolling upgrade strategy
- Add In-Place pod update strategy
- Add paused functionality during rolling upgrade

### Broadcast Job

#### Features

- Add BroadcastJob that runs pods on all nodes to completion
- Add `Never` termination policy to have job running after it finishes all pods
- Add `ttlSecondsAfterFinished` to delete the job after it finishes in x seconds.

#### Bugs

- Make broadcastjob honor node unschedulable condition

### Sidecar Set

#### Features

- Add SidecarSet that automatically injects sidecar container into selected pods
- Support sidecar update functionality for SidecarSet
