# Advanced StatefulSet

  This controller enhances the rolling update workflow of default [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)
  controller from two aspects: adding [MaxUnavailable rolling update strategy](#maxunavailable-rolling-update-strategy)
  and introducing [In-place Pod Update Strategy](#in-place-pod-update-strategy).
  Note that Advanced StatefulSet extends the same CRD schema of default StatefulSet with newly added fields.
  The CRD kind name is still `StatefulSet`.
  This is done on purpose so that user can easily migrate workload to the Advanced StatefulSet from the
  default StatefulSet. For example, one may simply replace the value of `apiVersion` in the StatefulSet yaml
  file from `apps/v1` to `apps.kruise.io/v1alpha1` after installing Kruise manager.

```yaml
-  apiVersion: apps/v1
+  apiVersion: apps.kruise.io/v1alpha1
   kind: StatefulSet
   metadata:
     name: sample
   spec:
     replicas: 3
     selector:
       matchLabels:
         app: sample
     template:
       metadata:
         labels:
           app: sample
    ...
```

## `MaxUnavailable` Rolling Update Strategy

  This controller adds a `maxUnavailable` capability in the `RollingUpdateStatefulSetStrategy` to allow parallel Pod
  updates with the guarantee that the number of unavailable pods during the update cannot exceed this value.
  It is only allowed to use when the podManagementPolicy is `Parallel`.

  This feature achieves similar update efficiency like Deployment for cases where the order of
  update is not critical to the workload. Without this feature, the native `StatefulSet` controller can only
  update Pods one by one even if the podManagementPolicy is `Parallel`. The API change is described below:

```go
type RollingUpdateStatefulSetStrategy struct {
	// Partition indicates the ordinal at which the StatefulSet should be
	// partitioned.
	// Default value is 0.
	// +optional
	Partition *int32 `json:"partition,omitempty"`
+	// The maximum number of pods that can be unavailable during the update.
+	// Value can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%).
+	// Absolute number is calculated from percentage by rounding down.
+	// Also, maxUnavailable can just be allowed to work with Parallel podManagementPolicy.
+	// Defaults to 1.
+	// +optional
+	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}
```

For example, assuming an Advanced StatefulSet has five replicas named P0 to P4, and the workload can
tolerate losing three replicas temporally. If we want to update the StatefulSet Pod spec from v1 to
v2, we can perform the following steps using the `MaxUnavailable` feature for fast update.

1. Set `MaxUnavailable` to 3 to allow three unavailable Pods maximally.
2. Optionally, Set `Partition` to 4 in case canary update is needed. Partition means all Pods with an ordinal that is
   greater than or equal to the partition will be updated. In this case P4 will be updated even though `MaxUnavailable`
   is 3.
3. After P4 finish update, change `Partition` to 0. The controller will update P1,P2 and P3 concurrently.
   Note that with default StatefulSet, the Pods will be updated sequentially in the order of P3, P2, P1.
4. Once one of P1, P2 and P3 finishes update, P0 will be updated immediately.

## `In-Place` Pod Update Strategy

  This controller adds a `podUpdatePolicy` field in `spec.updateStrategy.rollingUpdate`
  which controls recreate or in-place update for Pods.

  With this feature, a Pod will not be recreated if the container images are the only updated spec in
  the Advanced StatefulSet Pod template.
  Kubelet will handle the image-only update by downloading the new images and restart
  the corresponding containers without destroying the Pod. This feature is particularly useful
  in common container image update cases since all Pod namespace configurations
  (e.g, Pod IP) are preserved after update. In addition, Pods reschedule and reshuffle are avoided
  during the update.

  Note that currently, only container image update is supported for in-place update. Any other Pod
  spec update such as changing the command or container ENV will be rejected by kube-apiserver.

  The API change is described below:

```go
type PodUpdateStrategyType string

const (
	// RecreatePodUpdateStrategyType indicates that we always delete Pod and create new Pod
	// during Pod update, which is the default behavior
	RecreatePodUpdateStrategyType PodUpdateStrategyType = "ReCreate"
+	// InPlaceIfPossiblePodUpdateStrategyType indicates that we try to in-place update Pod instead of
+	// recreating Pod when possible. Currently, only image update of pod spec is allowed. Any other changes to the pod
+	// spec will fall back to ReCreate PodUpdateStrategyType where pod will be recreated.
+	InPlaceIfPossiblePodUpdateStrategyType = "InPlaceIfPossible"
+	// InPlaceOnlyPodUpdateStrategyType indicates that we will in-place update Pod instead of
+	// recreating pod. Currently we only allow image update for pod spec. Any other changes to the pod spec will be
+	// rejected by kube api-server
+	InPlaceOnlyPodUpdateStrategyType = "InPlaceOnly"
)
```

- `ReCreate` is the default strategy of podUpdatePolicy. Controller will recreate Pods when updated.
   This is the same behavior as default StatefulSet.
- `InPlaceIfPossible` strategy implies that the controller will check if current update is eligible
 for in-place update. If so, an in-place update is performed by updating Pod spec directly. Otherwise,
 controller falls back to the original Pod recreation mechanism. The `InPlaceIfPossible` strategy only
 works when `Spec.UpdateStrategy.Type` is set to `RollingUpdate`.
- `InPlaceOnly` strategy implies that the controller will only in-place update Pods. Note that `template.spec`
 is only allowed to update `containers[x].image`, the api-server will return an error if you try to update other fields in
  `template.spec`.

    **More importantly**, a readiness-gate named `InPlaceUpdateReady` must be  added into `template.spec.readinessGates`
    when using `InPlaceIfPossible` or `InPlaceOnly`. The condition `InPlaceUpdateReady` in podStatus will be updated to False before in-place
    update and updated to True after the update is finished. This ensures that pod remain at NotReady state while the in-place
    update is happening.

An example for StatefulSet using in-place update:

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: StatefulSet
metadata:
  name: sample
spec:
  replicas: 3
  serviceName: fake-service
  selector:
    matchLabels:
      app: sample
  template:
    metadata:
      labels:
        app: sample
    spec:
      readinessGates:
         # A new condition that ensures the pod remains at NotReady state while the in-place update is happening
      - conditionType: InPlaceUpdateReady
      containers:
      - name: main
        image: nginx:alpine
  podManagementPolicy: Parallel # allow parallel updates, works together with maxUnavailable
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      # Do in-place update if possible, currently only image update is supported for in-place update
      podUpdatePolicy: InPlaceIfPossible
      # Allow parallel updates with max number of unavailable instances equals to 2
      maxUnavailable: 2
```

## Tutorial

- [Use advanced StatefulSet to install Guestbook app](../../tutorial/advanced-statefulset.md)
