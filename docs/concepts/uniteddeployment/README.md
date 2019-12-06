# UnitedDeployment

  This controller provides a new way to manage pods in multi-domain
  by uniting multiple workloads.

  Different domains in one Kubernetes cluster are represented by multiple groups of
  nodes with different labels. UnitedDeployment is proposed to provision one workload
  for each group of this kind of nodes with corresponding `NodeSelector`, so that
  the pods created by these workloads will be scheduled to target domain.

  The workloads united by UnitedDeployment is called `subset`.
  Subset should at least provide the capacity for managing `replicas` of pods.
  Currently only `StatefulSet` is considered.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: UnitedDeployment
metadata:
  name: sample
spec:
  replicas: 6
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: sample
  template:
    statefulSetTemplate:
      metadata:
        labels:
          app: sample
      spec:
        template:
          metadata:
            labels:
              app: sample
          spec:
            containers:
            - image: nginx:alpine
              name: nginx
  topology:
    subsets:
    - name: subset-a
      nodeSelector:
        nodeSelectorTerms:
        - matchExpressions:
          - key: node
            operator: In
            values:
            - zone-a
      replicas: 1
    - name: subset-b
      nodeSelector:
        nodeSelectorTerms:
        - matchExpressions:
          - key: node
            operator: In
            values:
            - zone-b
      replicas: 50%
    - name: subset-c
      nodeSelector:
        nodeSelectorTerms:
        - matchExpressions:
          - key: node
            operator: In
            values:
            - zone-c
  updateStrategy:
    manualUpdate:
      partitions:
        subset-a: 0
        subset-b: 0
        subset-c: 0
    type: Manual
...
```

## Pod Distribution Management

  This controller provides `spec.topology` to describe the pod distribution detail.

```go
// Topology defines the spread detail of each subset under UnitedDeployment.
// A UnitedDeployment manages multiple homogeneous workloads which are called subset.
// Each of subsets under the UnitedDeployment is described in Topology.
type Topology struct {
	// Contains the details of each subset. Each element in this array represents one subset
	// which will be provisioned and managed by UnitedDeployment.
	// +optional
	Subsets []Subset `json:"subsets,omitempty"`
}

// Subset defines the detail of a subset.
type Subset struct {
	// Indicates subset name as a DNS_LABEL, which will be used to generate
	// subset workload name prefix in the format '<deployment-name>-<subset-name>-'.
	// Name should be unique between all of the subsets under one UnitedDeployment.
	Name string `json:"name"`

	// Indicates the node selector to form the subset. Depending on the node selector,
	// pods provisioned could be distributed across multiple groups of nodes.
	// A subset's nodeSelector is not allowed to be updated.
	// +optional
	NodeSelector corev1.NodeSelector `json:"nodeSelector,omitempty"`

	// Indicates the number of the pod to be created under this subset. Replicas could also be
	// percentage like '10%', which means 10% of UnitedDeployment replicas of pods will be distributed
	// under this subset. If nil, the number of replicas in this subset is determined by controller.
	// Controller will try to keep all the subsets with nil replicas have average pods.
	// +optional
	Replicas *intstr.IntOrString `json:"replicas,omitempty"`
}
```

  `topology` provides the key information to provision subsets.

  `topology.subsets` contains multiple `subset`s, each of which represents one subset.
  A subset added to or deleted from this array will be created or removed immediately.
  Each subset is created based on the description of UnitedDeployment `spec.template`.
  Currently, UnitedDeployment provides a field `spec.template.StatefulSetTemplate` to describe the StatefulSet
  as its subset workload.

  `subset` carries the necessary topology information to create a subset workload.
  Each subset under `topology` must have a unique name.
  A subset workload is created with the name prefix in format of '\<UnitedDeployment-name\>-\<Subset-name\>-'.
  Each subset will also be configured with the `nodeSelector`. Take StatefulSet for example.
  When provisioning a subset with type of StatefulSet, the `nodeSelector` will be added
  to the StatefulSet's `podTemplate`, in order for the StatefulSet to create pods with the expected node affinity.

  By default policy, UnitedDeployment's `spec.replicas` pods are evenly distributed to every subset.
  There are two scenarios the controller does not follow this policy:

  The first one is to customize the distribution policy by indicating `subset.replicas`.
  A valid `subset.replicas` could be integer to represent a real replicas of pods or
  string in format of percentage like '40%' to represent a fixed proportion of pods.
  Once a `subset.replicas` is given, the controller is going to reconcile to make sure each subset having the expected replicas.
  The subsets with empty `subset.replicas` will share out the left replicas evenly.

  The other scenario is that the indicated subset replicas policy becomes invalid like the UnitedDeployment's `spec.replicas` is decremented to below the sum of all `subset.replicas`.
  Until then, the indicated `subset.replicas` is ineffective, the controller is going to automatically scale each subset's replicas
  to match the UnitedDeployment's replicas. The controller will try its best to make this operation smoothly.

## Pod Update Management

  When `spec.template` is updated, a upgrade progress will be triggered.
  New template will be patch to each subset workload. It will then continue to trigger subset controller to update their pods.
  Furthermore, if subset workload supports `partition`, like StatefulSet, UnitedDeployment is also able to
  provide `Manual` update strategy.

```go
// UnitedDeploymentUpdateStrategy defines the update performance
// when template of UnitedDeployment is changed.
type UnitedDeploymentUpdateStrategy struct {
	// Type of UnitedDeployment update strategy.
	// Default is Manual.
	// +optional
	Type UpdateStrategyType `json:"type,omitempty"`
	// Includes all of the parameters a Manual update strategy needs.
	// +optional
	ManualUpdate *ManualUpdate `json:"manualUpdate,omitempty"`
}

// ManualUpdate is a update strategy which allows users to control the update progress
// by providing the partition of each subset.
type ManualUpdate struct {
	// Indicates number of subset partition.
	// +optional
	Partitions map[string]int32 `json:"partitions,omitempty"`
}
```

  `Manual` update strategy allows users to control the update progress by indicating the partition of each subset.
  The controller will pass the partition to each subset. For example, if the subset workload is StatefulSet with RollingUpdate
  strategy, the partition will be assigned to `spec.updateStrategy.rollingUpdate.partition`.
  The subset controller, currently it is StatefulSet controller, will continue to control its update progress with considering this partition.