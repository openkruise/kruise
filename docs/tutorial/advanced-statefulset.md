# Install Guestbook Application 
This tutorial walks you through an example to install a guestbook application using advanced statefulset.
The guestbook app used is from this [repo](https://github.com/IBM/guestbook/tree/master/v1).

## Install redis
```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-master-deployment.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-master-service.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-slave-deployment.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-slave-service.yaml
```

## Install Guestbook 

This will create a guestbook application using advanced statefulset.
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
      - name: guestbook
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

Check the guestbook are started. `statefulset.apps.kruise.io` or shortname `sts.apps.kruise.io` is the resource kind. 
`app.kruise.io` postfix needs to be appended due to naming collision with Kubernetes native `statefulset` kind.
 Verify that all pods are READY.
```
kubectl get sts.apps.kruise.io

NAME           DESIRED   CURRENT   UPDATED   READY   AGE
guestbook-v1   20        20        20        20      6m
```

## View the Guestbook

You can now view the Guestbook on browser.

* **Local Host:**
    If you are running Kubernetes locally, to view the guestbook, navigate to `http://localhost:3000` for the guestbook    

* **Remote Host:**
    To view the guestbook on a remote host, locate the external IP of the load balancer in the **IP** column of the `kubectl get services` output.
    For example, run 
```
$ kubectl get svc

NAME           TYPE           CLUSTER-IP     EXTERNAL-IP     PORT(S)                         AGE
guestbook      LoadBalancer   172.21.2.187   47.101.74.131   3000:31459/TCP,4000:32099/TCP   35m
kubernetes     ClusterIP      172.21.0.1     <none>          443/TCP                         104m
redis-master   ClusterIP      172.21.10.81   <none>          6379/TCP                        86m
redis-slave    ClusterIP      172.21.5.58    <none>          6379/TCP                        86m
```

`47.101.74.131` is the external IP. 
Visit `http://47.101.74.131:3000` for the main guestbook.
![Guestbook](./v1/guestbook.jpg)


## Inplace-update guestbook to the new image

First, check the running pods.
```
$ kubectl get pod -L controller-revision-hash -o wide | grep guestbook
NAME                            READY   STATUS    RESTARTS   AGE     IP             NODE                        NOMINATED NODE   CONTROLLER-REVISION-HASH
guestbook-v1-0                  1/1     Running   0          35s     172.29.1.21    cn-shanghai.192.168.1.113   <none>           guestbook-v1-7c947b5f94
guestbook-v1-1                  1/1     Running   0          35s     172.29.0.148   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-10                 1/1     Running   0          33s     172.29.1.23    cn-shanghai.192.168.1.113   <none>           guestbook-v1-7c947b5f94
guestbook-v1-11                 1/1     Running   0          33s     172.29.0.151   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-12                 1/1     Running   0          32s     172.29.0.152   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-13                 1/1     Running   0          32s     172.29.0.153   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-14                 1/1     Running   0          32s     172.29.0.27    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-15                 1/1     Running   0          31s     172.29.0.28    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-16                 1/1     Running   0          31s     172.29.1.24    cn-shanghai.192.168.1.113   <none>           guestbook-v1-7c947b5f94
guestbook-v1-17                 1/1     Running   0          30s     172.29.0.29    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-18                 1/1     Running   0          30s     172.29.0.154   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-19                 1/1     Running   0          30s     172.29.1.25    cn-shanghai.192.168.1.113   <none>           guestbook-v1-7c947b5f94
guestbook-v1-2                  1/1     Running   0          35s     172.29.0.22    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-3                  1/1     Running   0          35s     172.29.0.149   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-4                  1/1     Running   0          35s     172.29.0.23    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-5                  1/1     Running   0          35s     172.29.1.22    cn-shanghai.192.168.1.113   <none>           guestbook-v1-7c947b5f94
guestbook-v1-6                  1/1     Running   0          35s     172.29.0.24    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-7                  1/1     Running   0          34s     172.29.0.150   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-8                  1/1     Running   0          34s     172.29.0.25    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-9                  1/1     Running   0          34s     172.29.0.26    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
```

Run this command to patch the statefulset to use the new image.

`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-statefulset-v2.yaml`

In particular, the difference is that the image version is updated to `v2` and partition is set to `15`, meaning that the pods with 
ordinal larger than or equal to `15` will be updated to v2. The rest pods will remain at `v1`
```yaml
spec:
    ...
      containers:
      - name: guestbook
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
$ kubectl get sts.apps.kruise.io

NAME           DESIRED   CURRENT   UPDATED   READY   AGE
guestbook-v1   20        20        5         20      18h
``` 

Check the pods again. `guestbook-v1-15` to `guestbook-v1-19` are updated with `RESTARTS` showing `1`, 
IPs remain the same, `CONTROLLER-REVISION-HASH` are updated from ` guestbook-v1-7c947b5f94` to `guestbook-v1-576bd76785`

```
$ kubectl get pod -L controller-revision-hash -o wide | grep guestbook

NAME                            READY   STATUS    RESTARTS   AGE     IP             NODE                        NOMINATED NODE   CONTROLLER-REVISION-HASH
guestbook-v1-0                  1/1     Running   0          3m22s   172.29.1.21    cn-shanghai.192.168.1.113   <none>           guestbook-v1-7c947b5f94
guestbook-v1-1                  1/1     Running   0          3m22s   172.29.0.148   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-10                 1/1     Running   0          3m20s   172.29.1.23    cn-shanghai.192.168.1.113   <none>           guestbook-v1-7c947b5f94
guestbook-v1-11                 1/1     Running   0          3m20s   172.29.0.151   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-12                 1/1     Running   0          3m19s   172.29.0.152   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-13                 1/1     Running   0          3m19s   172.29.0.153   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-14                 1/1     Running   0          3m19s   172.29.0.27    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-15                 1/1     Running   1          3m18s   172.29.0.28    cn-shanghai.192.168.1.114   <none>           guestbook-v1-576bd76785
guestbook-v1-16                 1/1     Running   1          3m18s   172.29.1.24    cn-shanghai.192.168.1.113   <none>           guestbook-v1-576bd76785
guestbook-v1-17                 1/1     Running   1          3m17s   172.29.0.29    cn-shanghai.192.168.1.114   <none>           guestbook-v1-576bd76785
guestbook-v1-18                 1/1     Running   1          3m17s   172.29.0.154   cn-shanghai.192.168.1.112   <none>           guestbook-v1-576bd76785
guestbook-v1-19                 1/1     Running   1          3m17s   172.29.1.25    cn-shanghai.192.168.1.113   <none>           guestbook-v1-576bd76785
guestbook-v1-2                  1/1     Running   0          3m22s   172.29.0.22    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-3                  1/1     Running   0          3m22s   172.29.0.149   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-4                  1/1     Running   0          3m22s   172.29.0.23    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-5                  1/1     Running   0          3m22s   172.29.1.22    cn-shanghai.192.168.1.113   <none>           guestbook-v1-7c947b5f94
guestbook-v1-6                  1/1     Running   0          3m22s   172.29.0.24    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-7                  1/1     Running   0          3m21s   172.29.0.150   cn-shanghai.192.168.1.112   <none>           guestbook-v1-7c947b5f94
guestbook-v1-8                  1/1     Running   0          3m21s   172.29.0.25    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
guestbook-v1-9                  1/1     Running   0          3m21s   172.29.0.26    cn-shanghai.192.168.1.114   <none>           guestbook-v1-7c947b5f94
```

Now set `partition` to `0`, all pods will be updated to v2 this time, and all pods' IP remain `unchanged`. You should also find 
that all 20 pods are updated fairly soon because 1) new images are already pre-downloaded, 2) the `maxUnavailable` feature allows parallel updates instead of sequential 

```
$ kubectl get sts.apps.kruise.io
NAME           DESIRED   CURRENT   UPDATED   READY   AGE
guestbook-v1   20        20        20        20      18h
```

Describe a pod and find that the events show the original container is killed and new container is started. This verifies `in-place` update
```
$ kubectl describe pod guestbook-v1-0

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