# Tutorial

This tutorial walks through an example to deploy a redis cluster(1 master, 2 slaves) and a guestbook app and do in-place
update of the guestbook app using Kruise controllers. The guestbook app used is from this [repo](https://github.com/IBM/guestbook/tree/master/v1).
Below steps assume you have an existing kubernetes cluster running properly.

## Install Kruise CRDs
```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_broadcastjob.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_sidecarset.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_statefulset.yaml
```

## Install kruise-manager

`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/manager/all_in_one.yaml`

## Install redis
```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-master-deployment.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-master-service.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-slave-deployment.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/redis-slave-service.yaml
```

## Install Guestbook sidecarset

The sidecarset controller is a webhook controller and will watch pod creation and automatically inject a sidecar guestbook container into the matched pods

`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-sidecar.yaml`

Below is how the sidecarset looks like:
```
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  name: guestbook-sidecar
spec:
  selector: # select the pods to be injected with sidecar containers
    matchLabels:
      app: guestbook
  containers:
    - name: guestbook-sidecar
      image: openkruise/guestbook:sidecar
      imagePullPolicy: Always
      ports:
        - name: sidecar-server
          containerPort: 4000 # different from main guestbook containerPort which is 3000
```

## Install Guestbook 

This will create an Advanced StatefulSet with guestbook containers.
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

Describe one Guestbook pod
`kubectl describe pod guestbook-v1-0`

Find that the sidecar container is injected.

```yaml
    Containers:
      guestbook:
        Container ID:   docker://44f19a140c30de2c5b1a3f63c252c074efbb9c1b5eb7893ee7134461466b35c8
        Image:          openkruise/guestbook:v1
        Image ID:       docker-pullable://openkruise/guestbook@sha256:a5b6e5462982ca795fa9c7ddc378ea5b24a31e5d57eb806095526f7b21384dbd
        Port:           3000/TCP
        Host Port:      0/TCP
        State:          Running
          Started:      Wed, 19 Jun 2019 17:30:29 +0800
        Ready:          True
        Restart Count:  0
        Environment:    <none>
        Mounts:
          /var/run/secrets/kubernetes.io/serviceaccount from default-token-k5qpw (ro)
+     guestbook-sidecar:
+       Container ID:   docker://cbc379ce84624d9801928d5b2f1f2739e24094b440c55d62f7e0892eb31b0719
+       Image:          openkruise/guestbook:sidecar
+       Image ID:       docker-pullable://openkruise/guestbook@sha256:016eddf673cc7afc5da2fa96b5148161b521cff20583fb1d0c3aa44e6ac75272
+       Port:           4000/TCP
+       Host Port:      0/TCP
+       State:          Running
+         Started:      Wed, 19 Jun 2019 17:30:45 +0800
+       Ready:          True
+       Restart Count:  0
+       Environment:
+         IS_INJECTED:  true
+       Mounts:         <none>
```



## View the Guestbook

You can now view the Guestbook on browser.

* **Local Host:**
    If you are running Kubernetes locally, to view the guestbook, navigate to `http://localhost:3000` for the main guestbook
    and `http://localhost:4000` for the sidecar guestbook. 
    

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

Visit `http://47.101.74.131:4000` for the sidecar guestbook.

![Guestbook](./v1/guestbook-sidecar.jpg)


## Run a BroadcastJob to pre download a new image
First check that the nodes do not have images present. Below command should output nothing.
```
kubectl get nodes -o yaml | grep "openkruise/guestbook:v2"
```

Then, run a broadcastjob to download the images.

`kubect apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/broadcastjob.yaml`

Check the broadcastjob is completed. `bj` is short for `broadcastjob`
```
$ kubectl get bj
NAME             DESIRED   ACTIVE   SUCCEEDED   FAILED   AGE
download-image   3         0        3           0        7s
```
Check the pods are completed.
```
$ kubectl get pods
NAME                            READY   STATUS      RESTARTS   AGE
download-image-v99xz            0/1     Completed   0          61s
download-image-xmpkt            0/1     Completed   0          61s
download-image-zc4t4            0/1     Completed   0          61s
```

Now run the same command and check that the images have been downloaded. The testing cluster has 3 nodes. So below command
will output three entries.
```
$ kubectl get nodes -o yaml | grep "openkruise/guestbook:v2"
      - openkruise/guestbook:v2
      - openkruise/guestbook:v2
      - openkruise/guestbook:v2
```


The broadcastjob is configured with `ttlSecondsAfterFinished` to `60`, meaning the job and its associated pods will be deleted 
in `60` seconds after the job is finished.

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