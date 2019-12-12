# Run a UnitedDeployment to manage multiple domains

This tutorial walks you through an example to manage a group of pods in multiple domains.

## Prepare nodes for this example

Let's say there are three nodes, each of which has an unique label to identify its domain, as following:

```bash
$ kubectl get node --show-labels
NAME     STATUS   ROLES    AGE    VERSION   LABELS
node-a   Ready    <none>   107d   v1.12.2   beta.kubernetes.io/arch=amd64,...,node=zone-a
node-b   Ready    <none>   107d   v1.12.2   beta.kubernetes.io/arch=amd64,...,node=zone-b
node-c   Ready    <none>   107d   v1.12.2   beta.kubernetes.io/arch=amd64,...,node=zone-c
```

## Manage multiple domains

 Run below command to apply the UnitedDeployment:

```bash
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/uniteddeployment.yaml
```

 Then check the UnitedDeployment.

```bash
$ kubectl get ud
NAME                      DESIRED   CURRENT   UPDATED   READY   AGE
demo-guestbook-kruise     10        10        10        10      3m

$ kubectl get sts
NAME                                     DESIRED   CURRENT   AGE
demo-guestbook-kruise-subset-a-c9x8n     2         2         3m25s
demo-guestbook-kruise-subset-b-9c8dg     5         5         3m25s
demo-guestbook-kruise-subset-c-bxpd9     3         3         3m25s

$ kubectl get pod
NAME                                     READY   STATUS    RESTARTS   AGE
demo-guestbook-kruise-subset-a-c9x8n-0   1/1     Running   0          3m40s
demo-guestbook-kruise-subset-a-c9x8n-1   1/1     Running   0          3m33s
demo-guestbook-kruise-subset-b-9c8dg-0   1/1     Running   0          3m40s
demo-guestbook-kruise-subset-b-9c8dg-1   1/1     Running   0          3m34s
demo-guestbook-kruise-subset-b-9c8dg-2   1/1     Running   0          3m27s
demo-guestbook-kruise-subset-b-9c8dg-3   1/1     Running   0          3m21s
demo-guestbook-kruise-subset-b-9c8dg-4   1/1     Running   0          3m15s
demo-guestbook-kruise-subset-c-bxpd9-0   1/1     Running   0          3m40s
demo-guestbook-kruise-subset-c-bxpd9-1   1/1     Running   0          3m35s
demo-guestbook-kruise-subset-c-bxpd9-2   1/1     Running   0          3m29s
```

 There should be 3 StatefulSets created which represent three subsets. Depending on the subset name prefix format `<UnitedDeployment-name>-<subset-name>-`,
 it is easy to identify which subset a StatefulSet represents.
 For example, the StatefulSet `demo-guestbook-kruise-subset-a-c9x8n` is a subset named `subset-a` under the UnitedDeployment `demo-guestbook-kruise`.
 The desired replicas of each StatefulSet should match the value indicated in `spec.topology`.

```yaml
......
  topology:
    subsets:
      - name: subset-a
        replicas: 2
        nodeSelectorTerm:
          matchExpressions:
            - key: node
              operator: In
              values:
                - zone-a
      - name: subset-b
        replicas: 50%
        nodeSelectorTerm:
          matchExpressions:
            - key: node
              operator: In
              values:
                - zone-b
      - name: subset-c
        nodeSelectorTerm:
          matchExpressions:
            - key: node
              operator: In
              values:
                - zone-c
```

 As the yaml showing, the `subset-a` should provision 2 pods. The subset `subset-b` is supposed to provision half of `spec.replicas` pods, which is 5 pods.
 And the rest pods, which should be 3 pods, should belong to subset `subset-c`.

 From the UnitedDeployment status, the general detail is also showed.

```yaml
......
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
......
```

### Manage the subset distributed topology

 It is possible to re-allocate the pods by editing each subset's replicas in `spec.topology`.
 If we change the `topology` to the following version, controller will trigger a pod re-allocation.

```yaml
......
  topology:
    subsets:
    - name: subset-a
      nodeSelectorTerm:
        matchExpressions:
        - key: node
          operator: In
          values:
          - zone-a
      replicas: 1
    - name: subset-b
      nodeSelectorTerm:
        matchExpressions:
        - key: node
          operator: In
          values:
          - zone-b
      replicas: 40%
    - name: subset-c
      nodeSelectorTerm:
        matchExpressions:
        - key: node
          operator: In
          values:
          - zone-c
```

 Then check the StatefulSet and Pods. Controller has adjusted the pods distribution to match the `topology`.

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

 If we then leave each of the subset replicas empty in `topology`, these pods will be re-allocated evenly.

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

 UnitedDeployment provide `Manual` update strategy to control subsets update progress,
 by indicating the partition of each subset in `updateStrategy.manualUpdate.partitions`.

 Let's say the UnitedDeployment needs to update the pod's environment variable like following.
 In order to control the update progress, the partitions need to be provided.

```yaml
......
* updateStrategy:
*   type: Manual
*   manualUpdate:
*     partitions:
*       subset-a: 1
*       subset-b: 2
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
*             env:
*             - name: version
*               value: v2
              ports:
              - containerPort: 3000
                name: http-server
......
```

 After minutes, part of pods should be updated and look like this:

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

 With `--show-labels` appended, the revision every pod belongs to is showed.
 The label `apps.kruise.io/controller-revision-hash` attaches the revision on the pod.

 Depending on the UnitedDeployment status below, it is able to know the UnitedDeployment is during a update from `currentRevision` to `updatedRevision`, and the target revision is `demo-guestbook-kruise-64494c46ff`.
 Furthermore, there are 2, 1, 4 pods in each subset have been updated.
 A empty partition of a subset means `partition=0`, so all pods of subset-c have been selected to update.

```yaml
......
status:
......
* currentRevision: demo-guestbook-kruise-55f9dbcb4b
  ......
  subsetReplicas:
    subset-a: 3
    subset-b: 3
    subset-c: 4
  updateStatus:
    currentPartitions:
      subset-a: 1
      subset-b: 2
      subset-c: 0
*   updatedRevision: demo-guestbook-kruise-64494c46ff
  updatedReadyReplicas: 7
  updatedReplicas: 7
```

## Support subsets

 UnitedDeployment is supposed to support multiple kinds of workload as its subset.
 Currently, only StatefulSet is included.

### StatefulSet

 In order to support StatefulSet well, UnitedDeployment controller considers some special cases.

#### Delete Stuck pod

 To make sure a `RollingUpdate` progress will not be blocked by this [issue](https://github.com/kubernetes/kubernetes/issues/67250),
 UnitedDeployment controller will help to delete the stuck pods.

#### support OnDelete update strategy

 `OnDelete` update strategy is allowed to indicate in `template.statefulSetTemplate`.
 However, the pods also need to be deleted manually to keep consistent with the behavior of StatefulSet controller.