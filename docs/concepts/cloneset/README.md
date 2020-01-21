# CloneSet

This controller provides features for more efficient, deterministic and controlled deployment.

It mainly focuses on managing applications with orderless instances, and can be considered as
an enhanced version of `Deployment`.

A basic CloneSet:

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
metadata:
  labels:
    app: sample
  name: sample
spec:
  replicas: 5
  selector:
    matchLabels:
      app: sample
  template:
    metadata:
      labels:
        app: sample
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
```

## Scale features

### Create Pods and PVCs

`CloneSet` allows user to define PVC templates in `CloneSetSpec`, which can support PVCs per Pod.
This feature comes from `StatefulSet`, which is unable to do with `Deployment`.

If you do not configure `volumeClaimTemplates`, `CloneSet` will only create Pods without PVCs.

Note that:

- Each Pod and PVC created by `CloneSet` has an owner reference to this `CloneSet`. So when a `CloneSet` has been deleted, its Pods and PVCs will be cascading deleted.
- Each Pod and PVC created by `CloneSet` has a **instance-id** as suffix of its name and this id also marked in `metadata.labels["apps.kruise.io/cloneset-instance-id"]`.

- A Pod and all PVCs related to it have the same **instance-id**.
- When a Pod has been deleted by `CloneSet`, all PVCs related to it will be deleted together.
- When a Pod has been deleted manually such as `kubectl delete pod xxx`, all PVCs related to it will remain, and `CloneSet` will create a new Pod with the same **instance-id** and reuse the PVCs.
- When a Pod has been **recreate updated**, all PVCs related to it will be deleted together.
- When a Pod has been **inplace update**, all PVCs related to it still remain on it.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
metadata:
  labels:
    app: sample
  name: sample-data
spec:
  replicas: 5
  selector:
    matchLabels:
      app: sample
  template:
    metadata:
      labels:
        app: sample
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        volumeMounts:
        - name: data-vol
          mountPath: /usr/share/nginx/html
  volumeClaimTemplates:
    - metadata:
        name: data-vol
      spec:
        accessModes: [ "ReadWriteOnce" ]
        resources:
          requests:
            storage: 20Gi
```

### Specified Pod deletion

Sometimes we have to reduce `replicas` and hope workload delete the specified Pods.

There is no way to do this using `StatefulSet` or `Deployment`, because `StatefulSet` always delete Pod
by order and `Deployment`/`ReplicaSet` only delete Pod by its own sorted sequence.

We provide this feature in `CloneSet`, for it allows user to specify Pod names when modify `replicas`.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
spec:
  # ...
  replicas: 4
  scaleStrategy:
    podsToDelete:
    - sample-9m4hp
```

When reducing `replicas` from `5` to `4`, we also write the Pod name that we want to delete into `podsToDelete`, which
is a list of Pod name that users expect to delete.
Also, controller will clear those not-existing Pod names in `podsToDelete` automatically.

Note that:

- If you just add Pod name into `podsToDelete` and do not modify `replicas`, controller will delete this Pod, and then create a new Pod because it find the number of Pods is less than `replicas`.
- If you just reduce `replicas` without specified `podsToDelete`, controller will choose Pods to delete in the order such that not-ready < ready, unscheduled < scheduled, and pending < running.

## Update features

### In-place update

`CloneSet` provides three update types just like `AdvancedStatefulSet`, defaults to `ReCreate`.

- `ReCreate`: When Pods updating, controller will delete old Pods and PVCs and create new ones.
- `InPlaceIfPossible`: When Pods updating, controller will try to in-place update Pod instead of recreating them if possible. Currently, only modification of `spec.template.spec.containers[x].image` will cause in-place update.
- `InPlaceOnly`: When Pods updating, controller will in-place update Pod instead of recreating them. With `InPlaceOnly` type, Kruise will forbid users to modify any other fields in `spec.template` except `spec.template.spec.containers[x].image`.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
spec:
  # ...
  updateStrategy:
    type: InPlaceIfPossible  # ReCreate, InPlaceIfPossible, InPlaceOnly
```

When Pods in-place updating, controller will only update `spec.containers[x].image` in existing Pods.

Then Kubelet will find this change and handle it by stopping the old containers in Pod whose image has been changed, downloading
new images and creating new containers with them.
During this process, other unchanged containers in Pod including the sandbox container will remain running.

### Template and revision

`spec.template` defines the latest pod template in this `CloneSet`.
Controller will calculate a revision hash for each version of `spec.template` when it has been initialized or modified.

For example, firstly we create the sample `CloneSet`, controller will calculate the latest `controller-revision-hash=sample-744d4796cc`.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
metadata:
  # ...
  generation: 1
spec:
  replicas: 5
  template:
    metadata:
      labels:
        app: sample
    spec:
      containers:
      - image: nginx:alpine
        imagePullPolicy: Always
        name: nginx
  # ...
status:
  observedGeneration: 1
  readyReplicas: 5
  replicas: 5
  updateRevision: sample-d4d4fb5bd
  updatedReadyReplicas: 5
  updatedReplicas: 5
  # ...
```

- `status.replicas`: Number of pods
- `status.readyReplicas`: Number of **ready** pods

- `status.updateRevision`: Latest revision hash of this CloneSet
- `status.updatedReplicas`: Number of pods with the latest revision
- `status.updatedReadyReplicas`: Number of **ready** pods with the latest revision

- `status.observedGeneration`: Each time we modify `spec`, kube-apiserver will increase `metadata.generation` number. `status.observedGeneration` is the latest `metadata.generation` number that controller has noticed.

### Partition

Partition is the desired number of pods in old revisions, defaults to `0`.
When `partition` is set during pods updating, it means `(replicas - partition)` number of pods will be updated.

For example, we update image to nginx:mainline for the sample `CloneSet` and set `partition=3`, which means we want to keep three Pods not to update.

After a while:

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
metadata:
  # ...
  generation: 2
spec:
  replicas: 5
  template:
    metadata:
      labels:
        app: sample
    spec:
      containers:
      - image: nginx:mainline
        imagePullPolicy: Always
        name: nginx
  updateStrategy:
    partition: 3
  # ...
status:
  observedGeneration: 2
  readyReplicas: 5
  replicas: 5
  updateRevision: sample-56dfb978d4
  updatedReadyReplicas: 2
  updatedReplicas: 2
```

After modified template, `status.updateRevision` has been updated to `sample-56dfb978d4`.
Since we set `partition=3`, controller only update two Pods to the latest revision.

```bash
$ kubectl get pod -L controller-revision-hash
NAME           READY   STATUS    RESTARTS   AGE     CONTROLLER-REVISION-HASH
sample-chvnr   1/1     Running   0          6m46s   sample-d4d4fb5bd
sample-j6c4s   1/1     Running   0          6m46s   sample-d4d4fb5bd
sample-jnjdp   1/1     Running   0          10s     sample-56dfb978d4
sample-ns85c   1/1     Running   0          6m46s   sample-d4d4fb5bd
sample-qqglp   1/1     Running   0          18s     sample-56dfb978d4
```

### MaxUnavailable

MaxUnavailable is the maximum number of pods that can be unavailable during the update.
Value can be an absolute number (ex: 5) or a percentage of desired pods (ex: 10%).
Default value is `20%`.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
spec:
  # ...
  updateStrategy:
    maxUnavailable: 50%
```

### Update sequence

When controller choose Pods to update, it has these default sort logic:

1. **unscheduled** before than **scheduled**
2. **PodPending** before than **PodUnknown**, **PodUnknown** before than **PodRunning**
3. **not-ready** before than **ready**

What's more, `CloneSet` also provides `priority` and `scatter` strategies to let users specify the sequence of Pods update.

#### Sequence with `priority`

`priority` strategy defines rules for calculating the priority of updating pods.
All pods to be updated, will pass through these priority terms and get a sum of weights.

There are two type of `priority` strategy: `weight` and `order`.

- `weight`: Priority will be calculated by the sum of terms weight that matches selector.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
spec:
  # ...
  updateStrategy:
    priorityStrategy:
      weightPriority:
      - weight: 50
        matchSelector:
          matchLabels:
            test-key: foo
      - weight: 30
        matchSelector:
          matchLabels:
            test-key: bar
```

- `order`: Priority will be sorted by the value of orderKey.

Values of this key, will be sorted by GetInt(val). GetInt method will find the last int in value, such as getting 5 in value '5', getting 10 in value 'sts-10'.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
spec:
  # ...
  updateStrategy:
    priorityStrategy:
      orderPriority:
        - orderedKey: some-label-key
```

#### Sequence with `scatter`

`scatter` strategy defines rules to make pods been scattered when update.
This will avoid pods with the same key-value to be updated in one batch.

For a `CloneSet` with `replica=10`, if we put `foo=bar` in 3 Pod label and specify this scatter rule, these 3 Pods will
be updated in the order *1*, *6*, *10*.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
spec:
  # ...
  updateStrategy:
    scatterStrategy:
    - key: foo
      value: bar
```

Note that:

- Pods will be scattered after priority sort. So, although priority strategy and scatter strategy can be applied together, we suggest to use either one of them.
- If scatterStrategy is used, we suggest to just use one term. Otherwise, the update order can be hard to understand.

### Paused update

`paused` indicates that Pods updating is paused, controller will not update Pods but just keep replicas.

So if you want to stop immediately during `CloneSet` updating Pods, you can set `paused=true`.
This can also be a way to achieve the `OnDelete` update strategy in `StatefulSet`.

### PreUpdate and PostUpdate

`PreUpdate` and `PostUpdate` can allow users to do something before and after Pod update.

This feature is still in RoadMap and will be provided in next release version.

## Tutorial

- [Deploy Guestbook using CloneSet](../../tutorial/cloneset.md)
