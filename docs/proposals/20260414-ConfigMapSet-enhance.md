# 20260414-ConfigMapSet-enhance

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
2. Container selection
3. Reload container injection
4. Update strategy
5. Rollback

## Example Spec

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: ConfigMapSet
metadata:
  name: deploy-cms
  namespace: my-namespace
spec:
  # Select Pod instances by labels
  selector:
    matchLabels:
      app: sample
  # Business configuration to be updated
  data: 
    application.yaml: |
      server:
        port: 6666
      log:
        level: info
    db.conf: |
      url: "mysql://localhost:3306"
  # Business containers that need configuration updates
  containers:
  - name: main
    mountPath: /data/conf1 # used to describe the mount path and read permissions
  - nameFrom:
      fieldRef:
        apiVersion: v1
        fieldPath: metadata.labels['cName']
    mountPath: /data/conf3
  # Container used to update configuration files, injected when the Pod is created
  reloadSidecarConfig:
    # Direct injection by Kubernetes
    type: k8s
    config:
      name: reload-sidecar
      image: openkruise/reload-sidecar:v1.0.0
      restartPolicy: Always
    # Injection by referencing OpenKruise SidecarSet
    type: sidecarset
    config:
      # Specify the container in SidecarSet
      sidecarSetRef:
        name: reload-sidecarSet
        containerName: reload-sidecar
    # Custom container injection
    type: custom
    config:
      # Specify reload-sidecar config through ConfigMap for cross-namespace unified configuration
      configMapRef:
        name: reload-configMap
        namespace: default
  effectPolicy:
    # Both reload-sidecar and business containers are restarted; reload-sidecar restarts first
    type: ReStart
    # Only reload-sidecar is restarted; after restart completes, HTTP/TCP is used to trigger business container reload
    type: PostHook
    postHook:
      httpGet:
        port: 8081
        path: /reload-config
      tcpSocket:
        port: 8081
    # No containers are restarted; reload-sidecar hot-updates the shared configuration and the business container hot-reloads it
    type: HotUpdate
  revisionHistoryLimit: 5
  updateStrategy:
    partition: 10% # used for canary rolling update
    maxUnavailable: 1
    # Group matched Pods by label; Pods in each group roll according to partition and maxUnavailable semantics
    matchLabelKeys:
      - app-group
status:
  currentCustomVersion: v1.0
  currentRevision: e398776e45b31aef
  expectedUpdatedReplicas: 18
  observedGeneration: 8
  readyReplicas: 20
  replicas: 20
  updateCustomVersion: v1.1
  updateRevision: ee70811c764c4bf6
  updatedReadyReplicas: 18
  updatedReplicas: 18
```

## Version Management

For each `ConfigMapSet`, create an associated `ConfigMap` (the RevisionManager ConfigMap, abbreviated below as `RMC`) to persist multiple versions of `ConfigMapSet` data. It is recommended to name revisions by hash. If the hash is the same, it is treated as a no-op and no new revision is generated.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: deploy-cms1-hub
  namespace: infra-demo-uat
data: 
  revisions: |
    en3kp9:
      settings.yaml: |
        value: aaa
    fes34f:
      settings.yaml: |
        value: bbb
```

When a `ConfigMapSet` version is updated, the `ConfigMapSet` controller injects the annotation `configMapSet/cms1/Revision` into Pods that need to be updated, to describe the current version of configuration data.

`revisionHistoryLimit` specifies the maximum number of versions to retain in the RMC.

```yaml
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

In the above example, when `revisionHistoryLimit=5`, after `ConfigMapSet.data` is modified, the webhook checks whether the configuration matches some historical version:
- if yes, move that historical configuration to the front;
- if not, check whether the oldest configuration version is still used by any Pod:
  - if yes, reject the change;
  - if no, delete the oldest version and insert the new configuration at the front.

~~For configuration file storage, the default approach is to use the ConfigMap Hub shown above. For large configuration files exceeding 1 MB, an extension interface can be provided so that users can implement the interface themselves (for example, `Read/Write`) to integrate external storage.~~

## Container Selection

Container selection determines the related business containers.

### Static container injection

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  containers:
  - name: main
    mountPath: /data/conf1 
```

### Dynamic container injection

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  containers:
  - nameFrom: // when binding multiple workloads, labels can be used to mount configuration for different containers
      fieldRef:
        apiVersion: v1
        fieldPath: metadata.labels['cName']
    mountPath: /data/conf3 
```

## Reload Sidecar Container Injection

The Reload sidecar container is injected into the Pod and is used to update configuration files automatically.

### Explicitly declare reload container injection

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  reloadSidecarConfig:
    # Direct injection by Kubernetes
    type: k8s
    config:
      name: reload-sidecar
      image: openkruise/reload-sidecar:v1.0.0
      restartPolicy: Always
```

In this mode, the container is injected directly into the Pod at creation time through a webhook. It also automatically generates the `emptyDir` volume and `InitContainer` volumeMounts, while the business containers are injected with the corresponding `volumeMounts` as well. During injection, the annotation `apps.kruise.io/container-launch-priority: Ordered` is enabled to ensure it starts before the business containers.

### Declare injection by referencing SidecarSet

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  reloadSidecarConfig:
    # Injection by referencing OpenKruise SidecarSet
    type: sidecarset
    config:
      # Specify the container in SidecarSet
      sidecarSetRef:
        name: reload-sidecarSet
        ContainerName: reload-sidecar
```

In this mode, the SidecarSet selector must cover the `ConfigMapSet` selector. When the Pod is created, SidecarSet injection happens first, followed by `ConfigMapSet` injection. SidecarSet is responsible for injecting the container, while `ConfigMapSet` injects the related `volumeMounts` and `Env`. In this mode, the `reload-sidecar` can support horizontal canary updates, and that capability is fully provided by SidecarSet, meeting the requirement of canary updating the reload-sidecar image version.

### Customize reload container injection through ConfigMap

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  reloadSidecarConfig:
   # Custom container injection
    type: custom
    config:
      # Specify reload-sidecar config through ConfigMap for unified cross-namespace configuration
      configMapRef:
        name: reload-configMap
        namespace: default
```

Configure the Reload container through a ConfigMap. With this mode, a unified reload-sidecar configuration can be more conveniently provided for Pods in different namespaces.

#### Example custom ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: reload-configMap
  namespace: default
data: |
   {
     name: reload-sidecar
     image: openkruise/reload-sidecar:v1.0.0
     restartPolicy: Always
     command: ['sh', '-c', 'tail -F /opt/logs.txt']
   }
```

## Configuration Update Strategy

```yaml
apiVersion: xxx/v1alpha1
kind: ConfigMapSet
# ...
spec:
  # ...
  effectPolicy:
    # Both reload-sidecar and business containers are restarted; reload-sidecar restarts first
    type: ReStart
    # Only reload-sidecar is restarted; after restart completes, HTTP/TCP is used to trigger business container reload
    type: PostHook
    postHook:
      httpGet:
        port: 8081
        path: /reload-config
      tcpSocket:
        port: 8081
  updateStrategy:
    partition: 10% # used for canary rolling update
    maxUnavailable: 1
    # Group matched Pods by label; Pods in each group roll according to partition and maxUnavailable semantics
    matchLabelKeys:
      - app-group
```

After `ConfigMapSet.data` is updated, the controller workflow is as follows:

1. Update RMC data
2. Update the Pod annotation: set `apps.kruise.io/configmapset.updateRevision` to the current value
3. Make the configuration take effect according to `effectPolicy`

   1. `ReStart` (default):
      1. Restart `reload-sidecar` first
      2. Wait for `reload-sidecar` to become ready
      3. Check whether the latest configuration has been loaded
      4. Restart the business containers
      5. Wait for the business containers to become ready

   2. `HotUpdate`:
      1. Neither the Reload container nor the business container is restarted. The Reload container watches for ConfigMap file updates and hot-updates the configuration files in `EmptyDir`.
      2. Check whether the latest configuration has been loaded.
      3. Business containers need to implement hot reload of configuration files by themselves.

   3. `PostHook`:
      1. Restart `reload-sidecar` first
      2. Wait until the Reload container has restarted and becomes ready
      3. Check whether the latest configuration has been loaded
      4. Notify the business containers by the method defined in `effectPolicy.postHook` to trigger configuration reload. Both TCP and HTTP are supported.

4. Update `ConfigMapSet` Status

### Rolling strategy

The semantics of `partition` are **the number or percentage of Pods that remain on the old version**, with a default value of `0`. Here, `partition` **does not** represent any order index. The semantics of `maxUnavailable` are **the number or percentage of Pods that can be unavailable at most during rolling update**.

According to the rolling strategy:
- no more than `maxUnavailable` Pods are updated simultaneously in each round;
- the total number of updated Pods does not exceed `replicas - partition`.

When a new Pod is created, the controller obtains the old/new version information for the current group, then decides whether the Pod should use the new version or old version based on `partition` semantics, and injects the corresponding information.

### Pod selection strategy

When the controller selects Pods to update according to `partition`, to ensure version convergence it prioritizes in the following order:

1. non-`currentRevision` > `currentRevision`
2. among non-`currentRevision` versions, smaller revision sequence > larger revision sequence
3. among non-`currentRevision` versions, versions not present in RMC > versions present in RMC

### Startup order

In recreate scenarios, it must be guaranteed that the Reload container starts successfully before the business containers. OpenKruise's `Container Launch Priority` capability is used here to **control the startup order of containers within a Pod**.

### Recreate-only effect scenarios

For updates to non-configuration-file parts of `ConfigMapSet` (that is, anything outside the `data` section), no action is taken regardless of the update strategy. Such changes only take effect when the business redeploys and Pods are recreated.

### Stacked release

If a new update is initiated halfway through an existing update, the above workflow is restarted. The Pod selection strategy prioritizes Pods that are not on `currentRevision`.

### Uninstall flow

Considering Kubernetes Pod self-healing behavior, `ConfigMapSet` cannot be deleted directly. Otherwise, when Pods migrate or are recreated, the lack of ReloadContainer injection may cause anomalies in existing instances. A more graceful approach is to block `ConfigMapSet` deletion through a `finalizer`. At that point, the controller stops injecting into new Pods. After the business side completes one full workload rollout and `ConfigMapSet` no longer affects any Pod, the controller performs cleanup work (such as cleaning up the ConfigMap), then removes the `finalizer` and completes the uninstall.

## Rollback Flow

Simply re-apply `ConfigMapSet`, and the controller will execute the rolling update process described above again, rolling the configuration file back to the original version. Whether rolling back to `currentRevision` or `historyRevision`, rollback can be done by re-applying `ConfigMapSet`.

For rollback to `currentRevision`, existing Pods that have not been updated will not be touched, and the Pod selection strategy will prioritize updating Pods that are not on `currentRevision`. For rollback to `historyRevision`, the process is equivalent to the full release flow described above.

## Scenario Constraints

1. Injecting multiple Reload containers is not supported
2. Multiple `ConfigMapSet`s controlling the same path on the same Pod is not supported
3. Simultaneous modification of configuration and business `Spec` is not supported for now
4. Hot update is not supported for Pods on `virtual-kubelet` nodes