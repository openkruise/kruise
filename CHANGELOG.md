# Change Log

## v0.10.0

### New feature: PodUnavailableBudget

Kubernetes offers [Pod Disruption Budget (PDB)](https://kubernetes.io/docs/tasks/run-application/configure-pdb/) to help you run highly available applications even when you introduce frequent voluntary disruptions.
PDB limits the number of Pods of a replicated application that are down simultaneously from voluntary disruptions.
However, it can only constrain the voluntary disruption triggered by the Eviction API.
For example, when you run kubectl drain, the tool tries to evict all of the Pods on the Node you're taking out of service.

PodUnavailableBudget can achieve the effect of preventing ALL application disruption or SLA degradation, including pod eviction, deletion, inplace-update, ...

[doc](https://openkruise.io/docs/user-manuals/podunavailablebudget)

### New feature: WorkloadSpread

WorkloadSpread supports to constrain the spread of stateless workload, which empowers single workload the abilities for multi-domain and elastic deployment.

It can be used with those stateless workloads, such as CloneSet, Deployment, ReplicaSet and even Job.

[doc](https://openkruise.io/docs/user-manuals/workloadspread)

### CloneSet

- Scale-down supports topology spread constraints. [doc](https://openkruise.io/docs/user-manuals/cloneset#deletion-by-spread-constraints)
- Fix in-place update pods in Updated state.

### SidecarSet

- Add imagePullSecrets field to support pull secrets for the sidecar images. [doc](https://openkruise.io/docs/user-manuals/sidecarset#imagepullsecrets)
- Add injectionStrategy.paused to stop injection temporarily. [doc](https://openkruise.io/docs/user-manuals/sidecarset#injection-pause)

### Advanced StatefulSet

- Support image pre-download for in-place update, which can accelerate the progress of applications upgrade. [doc](https://openkruise.io/docs/user-manuals/advancedstatefulset#pre-download-image-for-in-place-update)
- Support scaling with rate limit. [doc](https://openkruise.io/docs/user-manuals/advancedstatefulset#scaling-with-rate-limiting)

### Advanced DaemonSet

- Fix rolling update stuck caused by deleting terminating pods.

### Other

- Bump to Kubernetes dependency to 1.18
- Add pod informer for kruise-daemon
- More `kubectl ... -o wide` fields for kruise resources

## v0.9.0

### New feature: ContainerRecreateRequest

[[doc](https://openkruise.io/docs/user-manuals/containerrecreaterequest)]

ContainerRecreateRequest provides a way to let users **restart/recreate** one or more containers in an existing Pod.

### New feature: Deletion Protection

[[doc](https://openkruise.io/docs/user-manuals/deletionprotection)]

This feature provides a safety policy which could help users protect Kubernetes resources and
applications' availability from the cascading deletion mechanism.

### CloneSet

- Support `pod-deletion-cost` to let users set the priority of pods deletion. [[doc](https://openkruise.io/docs/user-manuals/cloneset#pod-deletion-cost)]
- Support image pre-download for in-place update, which can accelerate the progress of applications upgrade. [[doc](https://openkruise.io/docs/user-manuals/cloneset#pre-download-image-for-in-place-update)]
- Add `CloneSetShortHash` feature-gate, which solves the length limit of CloneSet name. [[doc](https://openkruise.io/docs/user-manuals/cloneset#short-hash-label)]
- Make `maxUnavailable` and `maxSurge` effective for specified deletion. [[doc](https://openkruise.io/docs/user-manuals/cloneset#maxunavailable)]
- Support efficient update and rollback using `partition`. [[doc](https://openkruise.io/docs/user-manuals/cloneset#rollback-by-partition)]

### SidecarSet

- Support sidecar container **hot upgrade**. [[doc](https://openkruise.io/docs/user-manuals/sidecarset#hot-upgrade-sidecar)]

### ImagePullJob

- Add `podSelector` to pull image on nodes of the specific pods.

## v0.8.1

### Kruise-daemon

- Optimize cri-runtime for kruise-daemon

### BroadcastJob

- Fix broadcastjob expectation observed when node assigned by scheduler

## v0.8.0

### Breaking changes

1. The flags for kruise-manager must start with `--` instead of `-`. If you install Kruise with helm chart, ignore this.
2. SidecarSet has been refactored. Make sure there is no SidecarSet being upgrading when you upgrade Kruise,
   and read [the latest doc for SidecarSet](https://openkruise.io/docs/user-manuals/sidecarset).
3. A new component named `kruise-daemon` comes in. It is deployed in kruise-system using DaemonSet, defaults on every Node.

Now Kruise includes two components:

- **kruise-controller-manager**: contains multiple controllers and webhooks, deployed using Deployment.
- **kruise-daemon**: contains bypass features like image pre-download and container restart in the future, deployed using DaemonSet.

### New CRDs: NodeImage and ImagePullJob

[Official doc](https://openkruise.io/docs/user-manuals/imagepulljob)

Kruise will create a NodeImage for each Node, and its `spec` contains the images that should be downloaded on this Node.

Also, users can create an ImagePullJob CR to declare an image should be downloaded on which nodes.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: ImagePullJob
metadata:
  name: test-imagepulljob
spec:
  image: nginx:latest
  completionPolicy:
    type: Always
  parallelism: 10
  pullPolicy:
    backoffLimit: 3
    timeoutSeconds: 300
  selector:
    matchLabels:
      node-label: xxx
```

### SidecarSet

[Official doc](https://openkruise.io/docs/user-manuals/sidecarset)

- Refactor the controller and webhook for SidecarSet:
  - For `spec`:
    - Add `namespace`: indicates this SidecarSet will only inject for Pods in this namespace.
    - For `spec.containers`:
      - Add `podInjectPolicy`: indicates this sidecar container should be injected in the front or end of `containers` list.
      - Add `upgradeStrategy`: indicates the upgrade strategy of this sidecar container (currently it only supports `ColdUpgrade`)
      - Add `shareVolumePolicy`: indicates whether to share other containers' VolumeMounts in the Pod.
      - Add `transferEnv`: can transfer the names of env shared from other containers.
    - For `spec.updateStrategy`:
      - Add `type`: contains `NotUpdate` or `RollingUpdate`.
      - Add `selector`: indicates only update Pods that matched this selector.
      - Add `partition`: indicates the desired number of Pods in old revisions.
      - Add `scatterStrategy`: defines the scatter rules to make pods been scattered during updating.

### CloneSet

- Add `currentRevision` field in status.
- Optimize CloneSet scale sequence.
- Fix condition for pod lifecycle state from Updated to Normal.
- Change annotations `inplace-update-state` => `apps.kruise.io/inplace-update-state`, `inplace-update-grace` => `apps.kruise.io/inplace-update-grace`.
- Fix `maxSurge` calculation when partition > replicas.

### UnitedDeployment

- Support Deployment as template in UnitedDeployment.

### Advanced StatefulSet

- Support lifecycle hook for in-place update and pre-delete.

### BroadcastJob

- Add PodFitsResources predicates.
- Add `--assign-bcj-pods-by-scheduler` flag to control whether to use scheduler to assign BroadcastJob's Pods.

### Others

- Add feature-gate to replace the CUSTOM_RESOURCE_ENABLE env.
- Add GetScale/UpdateScale into clientsets for scalable resources.
- Support multi-platform build in Makefile.
- Set different user-agent for controllers.

## v0.7.0

### Breaking changes

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

Mainly focuses on managing stateless applications. ([Concept for CloneSet](https://openkruise.io/docs/user-manuals/cloneset))

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

- Support [priority update](https://openkruise.io/docs/user-manuals/advancedstatefulset#priority-strategy), which allows users to configure the sequence for Pods updating.

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

- Three blog posts are added in Kruise [website](http://openkruise.io), titled:
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
