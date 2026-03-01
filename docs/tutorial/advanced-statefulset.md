# Install Guestbook Application

This tutorial walks you through an example to install a guestbook application using advanced statefulset.
The guestbook app used is from this [repo](https://github.com/IBM/guestbook/tree/master/v1).

## Installing the Guestbook application using Helm

To install the chart with release name (application name) of `demo-v1`, replica of `20`:

```bash
helm install demo-v1 apphub/guestbook-kruise --set replicaCount=20,image.repository=openkruise/guestbook
```

The Chart is located in [this repo](https://github.com/cloudnativeapp/workshop/tree/master/kubecon2019china/charts/guestbook-kruise).

Now the guestbook-kruise app has been installed!

If you don't use helm, you need to install with YAML files as below.

## Install the Guestbook application with YAML files

Below installs a redis cluster with 1 master 2 replicas

```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-master-deployment.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-master-service.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-slave-deployment.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-slave-service.yaml
```

Below creates a guestbook application using advanced statefulset.

```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-statefulset.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-service.yaml
```

Several things to note in the `guestbook-statefulset.yaml`

```yaml
* apiVersion: apps.kruise.io/v1alpha1  # the kruise group version
  kind: StatefulSet
  ...
    spec:
*     readinessGates:
*        # A new condition must be added to ensure the pod remain at NotReady state while the in-place update is happening
*        - conditionType: InPlaceUpdateReady
      containers:
      - name: guestbook-kruise
        image: openkruise/guestbook:v1
        ports:
        - name: http-server
          containerPort: 3000
*    podManagementPolicy: Parallel  # allow parallel updates, works together with maxUnavailable
*    updateStrategy:
*       type: RollingUpdate
*       rollingUpdate:
*         # Do in-place update if possible, currently only image update is supported for in-place update
*         podUpdatePolicy: InPlaceIfPossible
*         # Allow parallel updates with max number of unavailable instances equals to 3
*         maxUnavailable: 3
```

Now the app has been installed.

## Verify Guestbook Started

Check the guestbook are started. `statefulset.apps.kruise.io` or shortname `sts.apps.kruise.io` is the resource kind.
`app.kruise.io` postfix needs to be appended due to naming collision with Kubernetes native `statefulset` kind.
 Verify that all pods are READY.

```
kubectl get sts.apps.kruise.io

NAME           DESIRED   CURRENT   UPDATED   READY   AGE
demo-v1-guestbook-kruise   20        20        20        20      6m
```

## View the Guestbook

You can now view the Guestbook on browser.

* **Local Host:**
    If you are running Kubernetes locally, to view the guestbook, navigate to `http://localhost:3000` for the guestbook

* **Remote Host:**
    To view the guestbook on a remote host, locate the external IP of the application in the **IP** column of the `kubectl get services` output.
    For example, run

```
kubectl get svc

NAME           TYPE           CLUSTER-IP     EXTERNAL-IP     PORT(S)                         AGE
guestbook      LoadBalancer   172.21.2.187   47.101.74.131   3000:31459/TCP,4000:32099/TCP   35m
```

`47.101.74.131` is the external IP.
Visit `http://47.101.74.131:3000` for the guestbook UI.
![Guestbook](./v1/guestbook.jpg)

## Inplace-update guestbook to the new image

First, check the running pods.

```
kubectl get pod -L controller-revision-hash -o wide | grep guestbook
NAME                                        READY   STATUS    RESTARTS   AGE     IP             NODE            NOMINATED NODE   CONTROLLER-REVISION-HASH
demo-v1-guestbook-kruise-0                  1/1     Running   0          35s     172.29.1.21    192.168.1.113   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-1                  1/1     Running   0          35s     172.29.0.148   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-10                 1/1     Running   0          33s     172.29.1.23    192.168.1.113   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-11                 1/1     Running   0          33s     172.29.0.151   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-12                 1/1     Running   0          32s     172.29.0.152   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-13                 1/1     Running   0          32s     172.29.0.153   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-14                 1/1     Running   0          32s     172.29.0.27    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-15                 1/1     Running   0          31s     172.29.0.28    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-16                 1/1     Running   0          31s     172.29.1.24    192.168.1.113   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-17                 1/1     Running   0          30s     172.29.0.29    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-18                 1/1     Running   0          30s     172.29.0.154   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-19                 1/1     Running   0          30s     172.29.1.25    192.168.1.113   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-2                  1/1     Running   0          35s     172.29.0.22    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-3                  1/1     Running   0          35s     172.29.0.149   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-4                  1/1     Running   0          35s     172.29.0.23    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-5                  1/1     Running   0          35s     172.29.1.22    192.168.1.113   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-6                  1/1     Running   0          35s     172.29.0.24    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-7                  1/1     Running   0          34s     172.29.0.150   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-8                  1/1     Running   0          34s     172.29.0.25    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-9                  1/1     Running   0          34s     172.29.0.26    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
```

Run this command to update the statefulset to use the new image.

```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-patch-to-v2.yaml
```

What this command does is that it changes the image version to `v2` and changes `partition` to `15`.
This will update pods with ordinal number >= 15 (i.e. 15 - 19)to image version `v2`. The rest pods (0 ~ 14) will remain at version `v1`.
The YAML diff details are shown below:

```yaml
spec:
    ...
      containers:
      - name: guestbook-kruise
-       image: openkruise/guestbook:v1
+       image: openkruise/guestbook:v2
  podManagementPolicy: Parallel  # allow parallel updates, works together with maxUnavailable
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      # Do in-place update if possible, currently only image update is supported for in-place update
      podUpdatePolicy: InPlaceIfPossible
      # Allow parallel updates with max number of unavailable instances equals to 2
      maxUnavailable: 3
+     partition: 15
```

Check the statefulset, find the statefulset has 5 pods updated

```
kubectl get sts.apps.kruise.io

NAME                       DESIRED   CURRENT   UPDATED   READY   AGE
demo-v1-guestbook-kruise   20        20        5         20      18h
```

Check the pods again. `demo-v1-guestbook-kruise-15` to `demo-v1-guestbook-kruise-19` are updated with `RESTARTS` showing `1`,
IPs remain the same, `CONTROLLER-REVISION-HASH` are updated from `demo-v1-guestbook-kruise-7c947b5f94` to `demo-v1-guestbook-kruise-576bd76785`

```
kubectl get pod -L controller-revision-hash -o wide | grep guestbook

NAME                                        READY   STATUS    RESTARTS   AGE     IP             NODE            NOMINATED NODE   CONTROLLER-REVISION-HASH
demo-v1-guestbook-kruise-0                  1/1     Running   0          3m22s   172.29.1.21    192.168.1.113   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-1                  1/1     Running   0          3m22s   172.29.0.148   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-10                 1/1     Running   0          3m20s   172.29.1.23    192.168.1.113   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-11                 1/1     Running   0          3m20s   172.29.0.151   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-12                 1/1     Running   0          3m19s   172.29.0.152   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-13                 1/1     Running   0          3m19s   172.29.0.153   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-14                 1/1     Running   0          3m19s   172.29.0.27    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-15                 1/1     Running   1          3m18s   172.29.0.28    192.168.1.114   <none>           demo-v1-guestbook-kruise-576bd76785
demo-v1-guestbook-kruise-16                 1/1     Running   1          3m18s   172.29.1.24    192.168.1.113   <none>           demo-v1-guestbook-kruise-576bd76785
demo-v1-guestbook-kruise-17                 1/1     Running   1          3m17s   172.29.0.29    192.168.1.114   <none>           demo-v1-guestbook-kruise-576bd76785
demo-v1-guestbook-kruise-18                 1/1     Running   1          3m17s   172.29.0.154   192.168.1.112   <none>           demo-v1-guestbook-kruise-576bd76785
demo-v1-guestbook-kruise-19                 1/1     Running   1          3m17s   172.29.1.25    192.168.1.113   <none>           demo-v1-guestbook-kruise-576bd76785
demo-v1-guestbook-kruise-2                  1/1     Running   0          3m22s   172.29.0.22    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-3                  1/1     Running   0          3m22s   172.29.0.149   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-4                  1/1     Running   0          3m22s   172.29.0.23    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-5                  1/1     Running   0          3m22s   172.29.1.22    192.168.1.113   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-6                  1/1     Running   0          3m22s   172.29.0.24    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-7                  1/1     Running   0          3m21s   172.29.0.150   192.168.1.112   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-8                  1/1     Running   0          3m21s   172.29.0.25    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
demo-v1-guestbook-kruise-9                  1/1     Running   0          3m21s   172.29.0.26    192.168.1.114   <none>           demo-v1-guestbook-kruise-7c947b5f94
```

Now upgrade all the pods, run

```
kubectl edit sts.apps.kruise.io demo-v1-guestbook-kruise
```

and update `partition` to `0`, all pods will be updated to v2 this time, and all pods' IP remain `unchanged`. You should also find
that all 20 pods are updated fairly fast because the `maxUnavailable` feature allows parallel updates instead of sequential update.

```
kubectl get sts.apps.kruise.io
NAME           DESIRED   CURRENT   UPDATED   READY   AGE
demo-v1-guestbook-kruise   20        20        20        20      18h
```

Describe a pod and find that the events show the original container is killed and new container is started. This verifies `in-place` update

```
kubectl describe pod demo-v1-guestbook-kruise-0

...
Events:
  Type    Reason   Age                From                                Message
  ----    ------   ----               ----                                -------
  Normal  Created  10m (x2 over 18h)  kubelet, 192.168.1.107  Created container
  Normal  Started  10m (x2 over 18h)  kubelet, 192.168.1.107  Started container
  Normal  Killing  10m                kubelet, 192.168.1.107  Killing container with id docker://guestbook:Container spec hash changed (4055768332 vs 2933593838).. Container will be killed and recreated.
  Normal  Pulled   10m                kubelet, 192.168.1.107  Container image "openkruise/guestbook:v3" already present on machine
```

The pods should also be in `Ready` state, the `InPlaceUpdateReady` will be set to `False` right before in-place update and to `True` after update is complete

```yaml
Readiness Gates:
  Type                 Status
  InPlaceUpdateReady   True
Conditions:
  Type                 Status
  InPlaceUpdateReady   True  # Should be False right before in-place update and True after update is complete
  Initialized          True
  Ready                True  # Should be True after in-place update is complete
  ContainersReady      True
  PodScheduled         True
```

## Uninstall app

Using helm to uninstall apps is very easy.

First you may want to list your helm apps:

```
helm list
NAME          NAMESPACE  REVISION  UPDATED                               STATUS    CHART
demo-v1       default    1         2019-06-23 13:33:21.278013 +0800 CST  deployed  guestbook-kruise-0.3.0
```

Then uninstall it:

```
helm uninstall demo-v1
```

If you are not using helm, deleting the application using below commands:

```
kubectl delete sts.apps.kruise.io demo-v1-guestbook-kruise
kubectl delete svc guestbook redis-master redis-slave
kubectl delete deploy redis-master redis-slave
```
