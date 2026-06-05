# ConfigMapSet Design

# Background

In scenarios involving large-scale data/model loading, in-place upgrade has become one of our fundamental capabilities to improve iteration efficiency. At present, `CloneSet` is the primary mechanism we use.

In our business scenarios, model application updates mainly involve data version updates, which are currently controlled through environment variables.

When a data version is incompatible with the code, the issue may need to be fixed by updating the image. During this process, the data version is expected to remain unchanged so that:
- the validation progress of the data version can be preserved, and
- different data versions can continue serving traffic.

We expect to have a controller similar to `EnvSet` / `ConfigMapSet` that can manage multiple versions of Env/configuration files alongside `CloneSet`, so that image releases and configuration releases can be decoupled.

[https://github.com/openkruise/kruise/issues/1894](https://github.com/openkruise/kruise/issues/1894)

# Core Capabilities

1. Version management
2. ConfigMap management
3. Container selection
4. Update strategy
5. Execution order

## Example Spec

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
metadata:
  name: deploy-cms1
  namespace: infra-demo-uat
spec:
  selector:
    matchLabels:
      app: sample
  data: 
    settings.yaml: |
      value: aaa
  containers:
  - name: main
    mountPath: /data/conf1 // used to describe the mount path and read permissions
  - name: sidecar1
    mountPath: /data/conf2
  - nameFrom: // provides a path for compatibility with dynamic injection scenarios (e.g. SidecarSet)
      fieldRef:
        apiVersion: v1
        fieldPath: metadata.labels['cName']
    mountPath: /data/conf3
  revisionHistoryLimit: 5
  // injectUpdateOrder: true describes the coordination mechanism when working together with a workload controller
  updateStrategy:  
    partition: 10% // used for version management
    restartInjectedContainers: true
    maxUnavailable: 1
  injectStrategy:
    pause: true
status:
  observedGeneration: 2
  currentRevision: en3kp9
  updateRevision: fes34f
  matchedPods: 3
  updatedPods: 1
  readyPods: 3
  updatedReadyPods: 1
  lastContainersTimestamp: "2025-02-14T17:51:48Z"
```

## Version Management

For each `ConfigMapSet`, create a unique `ConfigMap` (the RevisionManager ConfigMap, abbreviated below as `RMC`) to persist multiple versions of `ConfigMapSet` data in strict order. If the hash of a new version is identical to a historical version, the ordering may change.

```plaintext
apiVersion: v1
kind: ConfigMap
metadata:
  name: deploy-cms1-hub
  namespace: infra-demo-uat
spec:
  data: 
    revisions: |
      en3kp9:
        settings.yaml: |
          value: aaa
      fes34f:
        settings.yaml: |
          value: bbb
```

When a `ConfigMapSet` version is updated, `ConfigMapController` injects the annotation `configMapSet/cms1/Revision` into Pods that need to be updated, to describe the current version of configuration data.

`revisionHistoryLimit` specifies the maximum number of versions to keep in the RMC.

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
metadata:
  name: deploy-cms1
  namespace: infra-demo-uat
spec:
  # ...
  revisionHistoryLimit: 5
  # ...
```

## Configuration Management

Use an admission webhook to inject an `emptyDir` declaration into managed Pods, and inject `volumeMounts` into containers according to the rules described in `spec`.

Use an admission webhook to inject a special sidecar (referred to below as `reload-sidecar`) into managed Pods. The `reload-sidecar` mounts the RMC and receives the configuration version described by the Pod annotation `configMapSet/cms1/Revision` through `envFromMetaData`.

The `reload-sidecar` is responsible for parsing RMC data and maintaining the required configuration version in the `emptyDir`.

## Container Selection

Container selection determines:
- which containers are managed, and
- which containers are restarted during dynamic updates.

### Static container injection

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec
  # ...
  containers:
  - name: main
    mountPath: /data/conf1 
```

### Dynamic container injection

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  containers:
    - nameFrom: // provides a path for compatibility with dynamic injection scenarios (e.g. SidecarSet)
      fieldRef:
        apiVersion: v1
        fieldPath: metadata.labels['cName']
      mountPath: /data/conf4
```

## Update Strategy

After the annotation `configMapSet/cms1/Revision` is successfully updated for a Pod that needs updating, Kruise Daemon is responsible for restarting the `reload-sidecar` so that it loads the latest version.

`updateStrategy` describes the concrete rollout behavior.

The semantics of `partition` are **the number or percentage of Pods that should remain on the old version**, with a default value of `0`. Here, `partition` **does not** represent any order index.

PS: support for increasing `partition` is needed to cover AZ rollback scenarios.

When the controller selects Pods to update based on `partition`, in order to ensure convergence in version count, it prioritizes updates in the following order:

1. non-`currentRevision` > `currentRevision`
2. among non-`currentRevision` versions, smaller revision sequence > larger revision sequence
3. among non-`currentRevision` versions, versions not present in RMC > versions present in RMC

### Injection only (default strategy)

After `reload-sidecar` finishes restarting, Kruise Daemon updates the Pod status and triggers subsequent controller behavior.

When `injectUpdateOrder` is enabled, after the controller selects the Pods that need restarting, it adds the label `configMapSet/cms1/updateOrder=1` to the Pod, providing an asynchronous update marker for the control plane / Kruise users. After the controller observes that the containers injected with the configuration have restarted, it removes the label.

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
metadata:
  name: deploy-cms1
  namespace: infra-demo-uat
spec:
  # ...
  injectUpdateOrder: true
  # ...
```

The corresponding workload should cooperate with the platform control plane / Kruise users, for example by injecting `priorityStrategy`:

```plaintext
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
spec:
  # ...
  updateStrategy:
    priorityStrategy:
      orderPriority:
        - orderedKey: configMapSet/cms1/updateOrder
```

### Restart injected configuration containers

After `restartInjectedContainers` is enabled, once `reload-sidecar` finishes restarting, Kruise Daemon restarts the containers that were injected with configuration according to the latest `ConfigMapSet`, and only then updates the Pod status.

`maxUnavailable` describes the maximum number of unavailable Pods when `restartInjectedContainers` is enabled.

```plaintext
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
metadata:
  name: deploy-cms1
  namespace: infra-demo-uat
spec:
  # ...
  injectUpdateOrder: true
  updateStrategy:  
    partition: 10% // used for version management
    restartInjectedContainers: true
    maxUnavailable: 1
```

## Startup Order

During concurrent update/startup, `reload-sidecar` must always restart/start before the containers that consume the injected configuration.

a. For K8S versions < 1.28, use OpenKruise Container Launch Priority

b. For K8S versions >= 1.28, use Kubernetes `initContainer` with `restartPolicy=Always`

(?) When `restartInjectedContainers` is triggered, containers need to be restarted according to their startup order.

## Cold Start

When the mount path changes, a cold start is required, which means the change only takes effect after the Pod is recreated. However, `ConfigMapSetController` itself does not trigger Pod migration.

# Implementation

## Spec Definition

```plaintext
// ConfigMapSetSpec defines the desired state
type ConfigMapSetSpec struct {
  Selector           *metav1.LabelSelector      `json:"selector"`
  Data               map[string]string          `json:"data"`
  Containers         []ContainerInjectSpec      `json:"containers"`
  RevisionHistoryLimit *int32                   `json:"revisionHistoryLimit,omitempty"`
  InjectUpdateOrder  bool                       `json:"injectUpdateOrder,omitempty"`
  UpdateStrategy     ConfigMapSetUpdateStrategy `json:"updateStrategy,omitempty"`
}

type ContainerInjectSpec struct {
  Name      string           `json:"name,omitempty"`
  NameFrom  *ValueFromSource `json:"nameFrom,omitempty"`
  MountPath string           `json:"mountPath"`
}

type ValueFromSource struct {
  FieldRef corev1.ObjectFieldSelector `json:"fieldRef"`
}

type ConfigMapSetUpdateStrategy struct {
  Partition                 *string             `json:"partition,omitempty"`
  RestartInjectedContainers bool                `json:"restartInjectedContainers,omitempty"`
  MaxUnavailable            *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// ConfigMapSetStatus defines the observed state
type ConfigMapSetStatus struct {
  ObservedGeneration      int64        `json:"observedGeneration,omitempty"`
  CurrentRevision         string       `json:"currentRevision,omitempty"`
  UpdateRevision          string       `json:"updateRevision,omitempty"`
  MatchedPods             int32        `json:"matchedPods"`
  UpdatedPods             int32        `json:"updatedPods"`
  ReadyPods               int32        `json:"readyPods"`
  UpdatedReadyPods        int32        `json:"updatedReadyPods"`
  LastContainersTimestamp *metav1.Time `json:"lastContainersTimestamp"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ConfigMapSet struct {
  metav1.TypeMeta   `json:",inline"`
  metav1.ObjectMeta `json:"metadata,omitempty"`

  Spec   ConfigMapSetSpec   `json:"spec,omitempty"`
  Status ConfigMapSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ConfigMapSetList struct {
  metav1.TypeMeta `json:",inline"`
  metav1.ListMeta `json:"metadata,omitempty"`
  Items           []ConfigMapSet `json:"items"`
}
```

## Pod Status Flags

Use Pod annotations to describe `ConfigMapSet` version state:

**apps.kruise.io/configMapSet/:configMapSet-name/revision**: the desired revision. Patched by the controller when updating the Pod.

**apps.kruise.io/configMapSet/:configMapSet-name/revisionTimestamp**: the last update time of the desired revision. Updated together with the controller patch.

**apps.kruise.io/configMapSet/:configMapSet-name/currentRevision**: the current revision. Patched by Kruise Daemon after `reload-sidecar` finishes restarting. If `restartInjectedContainers` is enabled, it must wait until all injected configuration containers have restarted.

**apps.kruise.io/configMapSet/:configMapSet-name/currentRevisionTimestamp**: the last update time of `currentRevision`. Patched together by Kruise Daemon when updating the Pod after restart completion.

## Core Controller Logic

### Reconcile

```plaintext
// core reconcile logic
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
  // get ConfigMapSet instance

  // handle deletion logic

  // handle cold-start state

  // version management

  // pod sync

  // pod version status calculation

  // update status
}
```

#### Deletion logic

Delete `ConfigMapSet`, but do not clean up or recreate Pods.

#### Cold-start state changes

**status.lastContainersTimestamp**: the last update time of `containers`, used for cold-start decisions

**status.lastContainersHash**:

When `spec.containers` changes, the new and old versions need to be hashed. If the hash changes, update `lastContainersTimestamp` when updating Pods, then reload the current `ConfigMapSet` object.

#### Version management

```plaintext
// version management logic
func (r *Reonciler) manageRevisions(ctx context.Context, cms *apsv1alpha1.ConfigMapSet) (string, error) {
  // calculate the hash of the current spec

  // get or create/update the RevisionManager ConfigMap
}
```

#### Pod synchronization

```plaintext
// pod sync logic
func (r *econciler) syncPods(ctx context.Context, cms *ppsv1alpha1.ConfigMapSet) error {
  // get related Pods
  
  // cold-start judgment
  
  // select Pods to update according to the update strategy

  // update selected Pods
}
```

##### Cold start

Currently, when `spec.containers` changes, Pod recreation is not triggered. The new `containers` spec only takes effect for newly created Pods. Since mount-related injection cannot take effect dynamically, existing Pods (Pods whose creation time is earlier than `status.lastContainersTimestamp`) will not participate in `ConfigMapSet` version changes.

##### Pod selection strategy

When the controller selects Pods based on `partition`, after filtering out already updated Pods, to ensure version convergence it prioritizes according to annotation flags in the following order:

1. non-`currentRevision` > `currentRevision`
2. among non-`currentRevision` versions, versions not present in RMC > versions present in RMC
3. among non-`currentRevision` versions, smaller revision sequence > larger revision sequence

##### Pod update

Update Pod annotation **apps.kruise.io/configMapSet/:configMapSet-name/revision**

#### Pod status update

Calculate the Pod's CMS-related state based on Pod container status and annotations, then write it back to annotations.

#### Status update

```plaintext
// status update logic
func (r Reconciler) updateStatus(ctx context.Context, cms appsv1alpha1.ConfigMapSet, newRevision string) error {
  // get related Pod list

  // calculate status metrics

  // update ConfigMapSet status
}
```

**status.currentRevision**: current revision hash of `ConfigMapSet`

**status.updateRevision**: target revision hash of `ConfigMapSet`

**status.matchedPods**: number of related Pods

**status.updatedPods**: number of Pods that have completed update

**status.readyPods**: number of healthy Pods

**status.updatedReadyPods**: number of healthy Pods that have completed update

When Pod annotation

`configMapSet/cms1/currentRevision=configMapSet/cms1/revision`

the Pod is considered fully updated.

## Webhook Injection Logic

```plaintext
// +kubebuilder:webhook:path=/mutate-v1alpha1-configmapset,mutating=true,failurePolicy=fail,groups=apps.kruise.io,resources=configmapsets,verbs=create;update,versions=v1alpha1,name=mconfigmapset.kb.io

// implement volume and sidecar injection
func (h *ConfigMapSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
  // 1. determine whether emptyDir volume is needed and add it

  // 2. determine whether reload-sidecar container is needed and add it

  // 3. determine whether volumeMounts are needed for target containers and add them

  // 4. set startup order according to K8S version

  // 5. inject revision annotation according to partition
}
```

During concurrent update/startup, `reload-sidecar` must always restart/start before the containers that consume injected configuration.

a. For K8S versions < 1.28, use OpenKruise Container Launch Priority

b. For K8S versions >= 1.28, use Kubernetes `initContainer` with `restartPolicy=Always`

## Daemon Reconcile Logic

### Restart containers

```plaintext
// pkg/daemon/containermeta/container_meta_controller.go
func (c *Controller) manageContainerMetaSet(pod *v1.Pod, kubePodStatus *kubeletcontainer.PodStatus, oldMetaSet *appspub.RuntimeContainerMetaSet, criRuntime criapi.RuntimeService) *appspub.RuntimeContainerMetaSet {
// ...
// watch Pod annotation changes and trigger reload-sidecar restart

// restart injectedContainers
// ...
}

func eventFilter(oldPod, newPod *v1.Pod) bool {
// ...

// add filtering logic for ConfigMapSet injection

// ...
  return
}
```

When the container restart time is earlier than annotation `revisionTimestamp`, and `revision != updateRevision`, the container is considered to require restart.

#### Restart injectedContainers

It is necessary to load `ConfigMapSet` and parse `spec.containers` to determine which containers need to be restarted.

Note that asynchronous requests may occur after `spec.containers` has already changed. Therefore, it is necessary to judge based on `status.lastContainersTimestamp`: if the Pod creation time is earlier than `lastContainersTimestamp`, no further restart action will be performed.

### Update Pod state flags

```plaintext
// pkg/daemon/containerrecreate/crr_daemon_controller.go
func (c *Controller) manage(crr *appsv1alpha1.ContainerRecreateRequest) error {
// ...
// update Pod status annotations
// ...
}
```

When `revisionTimestamp` in the annotation is earlier than the restart time of all containers that need restarting, and the Pod is in `Ready` state, the configuration update is considered complete. Then update `currentRevision` to `revision` and set `currentRevisionTimestamp` to the current time.

# Collaboration with the Platform Control Plane

## In-place upgrade scenarios

### Regular/image release only

Forward path: update `ConfigMapSet` and specify `customVersion`, set `updateStrategy.upgradeType=ColdUpgrade`, and roll out the distribution of the corresponding `customVersion` according to the rolling strategy.

Rollback: roll back the distribution of the corresponding `customVersion` according to the rollback strategy, and finally update `ConfigMapSet.Spec` back to the old version.

![Flowchart 1](https://github.com/user-attachments/assets/8166de4f-c538-42e7-9769-4765b9178636)

### Configuration release only

#### Forward

Update `ConfigMapSet` and specify `customVersion`, set `updateStrategy.upgradeType=HotUpgrade` and `maxUnavailable`, thereby triggering container restart; then roll out the distribution of the corresponding `customVersion` according to the rolling strategy.

#### Rollback

Set `updateStrategy.upgradeType=HotUpgrade` and `maxUnavailable`, thereby triggering container restart; then roll back the distribution of the corresponding `customVersion` according to the rollback strategy, and finally update `ConfigMapSet.Spec` back to the old version.

![Flowchart 2](https://github.com/user-attachments/assets/53390e50-acc1-4887-a472-a13a1451aa6d)

### Regular/image release + configuration release (?)

#### Forward

Update `ConfigMapSet` and specify a new `customVersion`, set `updateStrategy.upgradeType=ColdUpgrade`, and roll out the distribution of the corresponding `customVersion` according to the rolling strategy; update `ConfigMapSet` first, then update the workload.

#### Rollback

Roll back the distribution of the corresponding `customVersion` according to the rollback strategy, and finally update `ConfigMapSet.Spec` back to the old version; update `ConfigMapSet` first, then update the workload.

![Flowchart 3](https://github.com/user-attachments/assets/f94f56f6-5a7b-4f84-acf2-bd65f572a11b)