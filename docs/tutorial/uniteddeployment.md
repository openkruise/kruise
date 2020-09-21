# Run a UnitedDeployment in a multi-domain cluster

This tutorial walks you through an example to manage a group of pods spread across multiple domains.

## Prepare nodes

Let's say there are three nodes, each of which has a unique label to identify its domain. For example,

```bash
$ kubectl get node --show-labels
NAME     STATUS   ROLES    AGE    VERSION   LABELS
node-a   Ready    <none>   107d   v1.12.2   beta.kubernetes.io/arch=amd64,...,az=zone-a
node-b   Ready    <none>   107d   v1.12.2   beta.kubernetes.io/arch=amd64,...,az=zone-b
node-c   Ready    <none>   107d   v1.12.2   beta.kubernetes.io/arch=amd64,...,az=zone-c
```

## Manage multiple workloads

Run below command to create a UnitedDeployment:

```bash
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/uniteddeployment.yaml
```

Then check the UnitedDeployment:

```bash
$ kubectl get ud
NAME                      DESIRED   CURRENT   UPDATED   READY   AGE
demo-guestbook-kruise     10        10        10        10      3m

$ kubectl get sts
NAME                                     DESIRED   CURRENT   AGE
demo-guestbook-kruise-subset-a-c9x8n     2         2         3m25s
demo-guestbook-kruise-subset-b-9c8dg     5         5         3m25s
demo-guestbook-kruise-subset-c-bxpd9     3         3         3m25s

$ kubectl get pod -o wide
NAME                                     READY   STATUS    RESTARTS   AGE     NODE
demo-guestbook-kruise-subset-a-c9x8n-0   1/1     Running   0          3m40s   node-a
demo-guestbook-kruise-subset-a-c9x8n-1   1/1     Running   0          3m33s   node-a
demo-guestbook-kruise-subset-b-9c8dg-0   1/1     Running   0          3m40s   node-b
demo-guestbook-kruise-subset-b-9c8dg-1   1/1     Running   0          3m34s   node-b
demo-guestbook-kruise-subset-b-9c8dg-2   1/1     Running   0          3m27s   node-b
demo-guestbook-kruise-subset-b-9c8dg-3   1/1     Running   0          3m21s   node-b
demo-guestbook-kruise-subset-b-9c8dg-4   1/1     Running   0          3m15s   node-b
demo-guestbook-kruise-subset-c-bxpd9-0   1/1     Running   0          3m40s   node-c
demo-guestbook-kruise-subset-c-bxpd9-1   1/1     Running   0          3m35s   node-c
demo-guestbook-kruise-subset-c-bxpd9-2   1/1     Running   0          3m29s   node-c
```

There should be 3 StatefulSets and 10 pods created. The pods are spread in three nodes following
the node affinity specified in the `subset`. According to the subset name prefix format `<UnitedDeployment-name>-<subset-name>-`,
it is easy to identify which subset a StatefulSet represents.
For example, the StatefulSet `demo-guestbook-kruise-subset-a-c9x8n` represents a subset named `subset-a` under the UnitedDeployment `demo-guestbook-kruise`.
The desired replicas of each StatefulSet should match the values specified in `spec.topology`.

```yaml
......
  topology:
    subsets:
      - name: subset-a
        replicas: 2
        nodeSelectorTerm:
          matchExpressions:
            - key: az
              operator: In
              values:
                - zone-a
      - name: subset-b
        replicas: 50%
        nodeSelectorTerm:
          matchExpressions:
            - key: az
              operator: In
              values:
                - zone-b
      - name: subset-c
        nodeSelectorTerm:
          matchExpressions:
            - key: az
              operator: In
              values:
                - zone-c
```

As shown above, `subset-a` should provision 2 pods, `subset-b` should provision half of `spec.replicas` pods, which is 5,
and the remaining pods, which is 3, should belong to `subset-c`.
From the UnitedDeployment status, we can clearly see the provision results.

```yaml
...
status:
  conditions:
  - lastTransitionTime: "2019-12-12T16:19:06Z"
    status: "True"
    type: SubsetProvisioned
  - lastTransitionTime: "2019-12-12T16:19:06Z"
    status: "True"
    type: SubsetUpdated
  currentRevision: demo-guestbook-kruise-55f9dbcb4b
  observedGeneration: 1
  readyReplicas: 10
  replicas: 10
  subsetReplicas:
    subset-a: 2
    subset-b: 5
    subset-c: 3
  updateStatus:
    currentPartitions:
      subset-a: 0
      subset-b: 0
      subset-c: 0
    updatedRevision: demo-guestbook-kruise-55f9dbcb4b
  updatedReadyReplicas: 10
  updatedReplicas: 10
...
```

### Manage the subset replica distribution

It is possible to re-distribute the pods by editing each subset's replicas in `spec.topology`.
If we change the `topology` to the following, the controller will trigger a pod re-distribution.

```yaml
...
  topology:
    subsets:
    - name: subset-a
      nodeSelectorTerm:
        matchExpressions:
        - key: az
          operator: In
          values:
          - zone-a
      replicas: 1
    - name: subset-b
      nodeSelectorTerm:
        matchExpressions:
        - key: az
          operator: In
          values:
          - zone-b
      replicas: 40%
    - name: subset-c
      nodeSelectorTerm:
        matchExpressions:
        - key: az
          operator: In
          values:
          - zone-c
```

After a minute, we can find that controller has adjusted the pod distribution to match the above `topology`.

```bash
$ kubectl get sts
NAME                                     DESIRED   CURRENT   AGE
demo-guestbook-kruise-subset-a-c9x8n     1         1         4m40s
demo-guestbook-kruise-subset-b-9c8dg     4         5         4m40s
demo-guestbook-kruise-subset-c-bxpd9     5         5         4m40s

$ kubectl get pod
NAME                                     READY   STATUS    RESTARTS   AGE
demo-guestbook-kruise-subset-a-c9x8n-0   1/1     Running   0          4m58s
demo-guestbook-kruise-subset-b-9c8dg-0   1/1     Running   0          4m58s
demo-guestbook-kruise-subset-b-9c8dg-1   1/1     Running   0          4m52s
demo-guestbook-kruise-subset-b-9c8dg-2   1/1     Running   0          4m45s
demo-guestbook-kruise-subset-b-9c8dg-3   1/1     Running   0          4m39s
demo-guestbook-kruise-subset-c-bxpd9-0   1/1     Running   0          4m58s
demo-guestbook-kruise-subset-c-bxpd9-1   1/1     Running   0          4m53s
demo-guestbook-kruise-subset-c-bxpd9-2   1/1     Running   0          4m47s
demo-guestbook-kruise-subset-c-bxpd9-3   1/1     Running   0          31s
demo-guestbook-kruise-subset-c-bxpd9-4   1/1     Running   0          23s
```

By default, if we don't set subset replicas at all, pods will be distributed evenly.

```bash
$ kubectl -n kruise get pod
NAME                                     READY   STATUS    RESTARTS   AGE
demo-guestbook-kruise-subset-a-c9x8n-0   1/1     Running   0          6m1s
demo-guestbook-kruise-subset-a-c9x8n-1   1/1     Running   0          35s
demo-guestbook-kruise-subset-a-c9x8n-2   1/1     Running   0          27s
demo-guestbook-kruise-subset-b-9c8dg-0   1/1     Running   0          6m1s
demo-guestbook-kruise-subset-b-9c8dg-1   1/1     Running   0          5m55s
demo-guestbook-kruise-subset-b-9c8dg-2   1/1     Running   0          5m48s
demo-guestbook-kruise-subset-c-bxpd9-0   1/1     Running   0          6m1s
demo-guestbook-kruise-subset-c-bxpd9-1   1/1     Running   0          5m56s
demo-guestbook-kruise-subset-c-bxpd9-2   1/1     Running   0          5m50s
demo-guestbook-kruise-subset-c-bxpd9-3   1/1     Running   0          94s
```

### Manage the subset upgrade progress

UnitedDeployment provides `Manual` update strategy to control the subsets update progress
by setting the partition of each subset in `updateStrategy.manualUpdate.partitions`.

Let's say we want to update pod's environment variable in a UnitedDeployment and
we provide the following partitions.

```yaml
...
+ updateStrategy:
+   type: Manual
+   manualUpdate:
+     partitions:
+       subset-a: 1
+       subset-b: 2
  template:
    statefulSetTemplate:
      metadata:
        labels:
          app.kubernetes.io/instance: demo
          app.kubernetes.io/name: guestbook-kruise
      spec:
        template:
          metadata:
            labels:
              app.kubernetes.io/instance: demo
              app.kubernetes.io/name: guestbook-kruise
          spec:
            containers:
            - image: openkruise/guestbook:v1
              imagePullPolicy: Always
              name: guestbook-kruise
+             env:
+             - name: version
+               value: v2
              ports:
              - containerPort: 3000
                name: http-server
...
```

After a minute, part of pods should be updated.

```bash
$ kubectl get pod --show-labels
NAME                                     READY   STATUS    RESTARTS   AGE     LABELS
demo-guestbook-kruise-subset-a-c9x8n-0   1/1     Running   0          5m50s   ...,apps.kruise.io/controller-revision-hash=demo-guestbook-kruise-55f9dbcb4b,...
demo-guestbook-kruise-subset-a-c9x8n-1   1/1     Running   0          2m50s   ...,apps.kruise.io/controller-revision-hash=demo-guestbook-kruise-64494c46ff,...
demo-guestbook-kruise-subset-a-c9x8n-2   1/1     Running   0          3m      ...,apps.kruise.io/controller-revision-hash=demo-guestbook-kruise-64494c46ff,...
demo-guestbook-kruise-subset-b-9c8dg-0   1/1     Running   0          5m42s   ...,apps.kruise.io/controller-revision-hash=demo-guestbook-kruise-55f9dbcb4b,...
demo-guestbook-kruise-subset-b-9c8dg-1   1/1     Running   0          5m55s   ...,apps.kruise.io/controller-revision-hash=demo-guestbook-kruise-55f9dbcb4b,...
demo-guestbook-kruise-subset-b-9c8dg-2   1/1     Running   0          3m2s    ...,apps.kruise.io/controller-revision-hash=demo-guestbook-kruise-64494c46ff,...
demo-guestbook-kruise-subset-c-bxpd9-0   1/1     Running   0          2m5s    ...,apps.kruise.io/controller-revision-hash=demo-guestbook-kruise-64494c46ff,...
demo-guestbook-kruise-subset-c-bxpd9-1   1/1     Running   0          2m25s   ...,apps.kruise.io/controller-revision-hash=demo-guestbook-kruise-64494c46ff,...
demo-guestbook-kruise-subset-c-bxpd9-2   1/1     Running   0          2m45s   ...,apps.kruise.io/controller-revision-hash=demo-guestbook-kruise-64494c46ff,...
demo-guestbook-kruise-subset-c-bxpd9-3   1/1     Running   0          3m3s    ...,apps.kruise.io/controller-revision-hash=demo-guestbook-kruise-64494c46ff,...
```

With `--show-labels` appended, the revision of every pod is shown with the
label key `apps.kruise.io/controller-revision-hash`.

From the UnitedDeployment status below, we can see that the UnitedDeployment is
undergoing an update and the target revision is `demo-guestbook-kruise-64494c46ff`.
Furthermore, there are 2, 1, 4 pods in each subset that have been updated.
An empty partition of a subset means `partition` is 0, all pods of `subset-c` will be updated.

```yaml
...
status:
...
+ currentRevision: demo-guestbook-kruise-55f9dbcb4b
  ...
  subsetReplicas:
    subset-a: 3
    subset-b: 3
    subset-c: 4
  updateStatus:
    currentPartitions:
      subset-a: 1
      subset-b: 2
      subset-c: 0
+ updatedRevision: demo-guestbook-kruise-64494c46ff
  updatedReadyReplicas: 7
  updatedReplicas: 7
```

### Manage the subset type

UnitedDeployment now supports three types of subset which are `StatefulSet`、`AdvancedStatefulSet`、`CloneSet`.
It is allowed to change subset type from one to another at runtime. Take the above UnitedDeployment as an example.

Create a new UnitedDeployment with the same definition.

```bash
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/uniteddeployment.yaml
```

We get the three subsets of StatefulSet as expected.

```bash
$ kubectl get sts
NAME                                     DESIRED   CURRENT   AGE
demo-guestbook-kruise-subset-a-jtz9t     2         2         82s
demo-guestbook-kruise-subset-b-bhjm6     5         5         82s
demo-guestbook-kruise-subset-c-bj7f2     3         3         82s
```

Then we update the subset type from StatefulSet to AdvancedStatefulSet
by using the `advancedStatefulSetTemplate` to describe the subset template.
By the way, let's enable the in-place update of the subset pod.
(and use `cloneSetTemplate` if you need CloneSet)

```yaml
...
spec:
  replicas: 10
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app.kubernetes.io/instance: demo
      app.kubernetes.io/name: guestbook-kruise
  template:
-   statefulSetTemplate:
+   advancedStatefulSetTemplate:
      metadata:
        labels:
          app.kubernetes.io/instance: demo
          app.kubernetes.io/name: guestbook-kruise
      spec:
+       updateStrategy:
+         type: RollingUpdate
+         rollingUpdate:
+           podUpdatePolicy: InPlaceIfPossible
+       selector:
+         matchLabels:
+           app.kubernetes.io/instance: demo
+           app.kubernetes.io/name: guestbook-kruise
        template:
          metadata:
            labels:
              app.kubernetes.io/instance: demo
              app.kubernetes.io/name: guestbook-kruise
          spec:
            containers:
            - image: openkruise/guestbook:v1
              imagePullPolicy: Always
              name: guestbook-kruise
              ports:
              - containerPort: 3000
                name: http-server
...
```

Then three StatefulSets are all deleted. Instead, three AdvancedStatefulSets are created with in-place update feature enabled.

```bash
$ kubectl get sts
No resources found.

$ kubectl get statefulset.apps.kruise.io
NAME                                   DESIRED   CURRENT   UPDATED   READY   AGE
demo-guestbook-kruise-subset-a-2r8dm   2         2         2         2       1m
demo-guestbook-kruise-subset-b-tj2n6   5         5         5         5       1m
demo-guestbook-kruise-subset-c-qflc9   3         3         3         3       1m

$ kubectl get statefulset.apps.kruise.io -o custom-columns=NAME:.metadata.name,POD-UPDATE-POLICY:.spec.updateStrategy.rollingUpdate.podUpdatePolicy
NAME                                   POD-UPDATE-POLICY
demo-guestbook-kruise-subset-a-2r8dm   InPlaceIfPossible
demo-guestbook-kruise-subset-b-tj2n6   InPlaceIfPossible
demo-guestbook-kruise-subset-c-qflc9   InPlaceIfPossible
```

Now it is possible to in-place update each pod of these subsets by updating the UnitedDeployment.
More details of AdvancedStatefulSet are provided in this [tutorial](./advanced-statefulset.md).

Please note that the process of switching subset type is not graceful, so the service may be unavailable during it.

## Subset workload support

UnitedDeployment might support multiple kinds of workload as its subset. Currently, only StatefulSet is supported.
To better support StatefulSet, UnitedDeployment controller makes a few enhancements:

### Delete stuck pods

To make sure a `RollingUpdate` progress will not be blocked by this [issue](https://github.com/kubernetes/kubernetes/issues/67250),
UnitedDeployment controller helps to delete the stuck pods.

### Support OnDelete update strategy

`OnDelete` update strategy is allowed in `template.statefulSetTemplate`.
However, the pods need to be deleted manually to keep consistent with the behavior of StatefulSet controller.
