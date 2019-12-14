# UnitedDeployment

  This controller provides a new way to manage pods in multi-domain by using multiple workloads.
  A high level description about this workload can be found in this [blog post](http://openkruise.io/en-us/blog/blog3.html).

  Different domains in one Kubernetes cluster are represented by multiple groups of
  nodes identified by labels. UnitedDeployment controller provisions one type of workload
  for each group of with corresponding matching `NodeSelector`, so that
  the pods created by individual workload will be scheduled to the target domain.

  Each workload managed by UnitedDeployment is called a `subset`.
  Each domain should at least provide the capacity to run the `replicas` number of pods.
  Currently only `StatefulSet` is the supported workload. The below sample yaml
  presents a UnitedDeployment which manages three StatefulSet instances in three domains.
  The total number of managed pods is 6.

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

  This controller provides `spec.topology` to describe the pod distribution specification.

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

  `topology.subsets` specifies the desired group of `subset`s.
  A subset added to or removed from this array will be created or deleted by controller during reconcile.
  Each subset workload is created based on the description of UnitedDeployment `spec.template`.
  `subset` provides the necessary topology information to create a subset workload.
  Each subset has a unique name.  A subset workload is created with the name prefix in
  format of '\<UnitedDeployment-name\>-\<Subset-name\>-'. Each subset will also be configured with
  the `nodeSelector`.
  When provisioning a StatefulSet `subset`, the `nodeSelector` will be added
  to the StatefulSet's `podTemplate`, so that the Pods of the StatefulSet will be created with the
  expected node affinity.

  By default, UnitedDeployment's Pods are evenly distributed across all subsets.
  There are two scenarios the controller does not follow this policy:

  The first one is to customize the distribution policy by indicating `subset.replicas`.
  A valid `subset.replicas` could be integer to represent a real replicas of pods or
  string in format of percentage like '40%' to represent a fixed proportion of pods.
  Once a `subset.replicas` is given, the controller is going to reconcile to make sure
  each subset has the expected replicas.
  The subsets with empty `subset.replicas` will divide the remaining replicas evenly.

  The other scenario is that the indicated subset replicas policy becomes invalid.
  For example, the UnitedDeployment's `spec.replicas` is decremented to be less
  than the sum of all `subset.replicas`.
  In this case, the indicated `subset.replicas` is ineffective and the controller
  will automatically scale each subset's replicas to match the total replicas number.
  The controller will try its best to apply this adjustment smoothly.

## Pod Update Management

  When `spec.template` is updated, a upgrade progress will be triggered.
  New template will be patch to each subset workload, which triggers subset controller
  to update their pods.
  Furthermore, if subset workload supports `partition`, like StatefulSet, UnitedDeployment
  is also able to provide `Manual` update strategy.

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

  `Manual` update strategy allows users to control the update progress by indicating
  the `partition` of each subset. The controller will pass the `partition` to each subset.

## Tutorial

- [Run a UnitedDeployment in a multi-domain cluster](../../tutorial/uniteddeployment.md)
