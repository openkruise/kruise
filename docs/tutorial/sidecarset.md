# Inject Sidecar Container with SidecarSet
This tutorial walks you through an example to automatically inject a sidecar container with sidecarset.

## Install Guestbook sidecarset

The sidecarset controller is a webhook controller and will watch pod creation and automatically inject a sidecar guestbook container into the matched pods

`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-sidecar.yaml`

Below is how the sidecarset looks like:
```yaml
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
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-sts-for-sidecar-demo.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-service-for-sidecar-demo.yaml
```

Check the guestbook are started. `statefulset.apps.kruise.io` or shortname `sts.apps.kruise.io` is the resource kind. 
`app.kruise.io` postfix needs to be appended due to naming collision with Kubernetes native `statefulset` kind.
 Verify that all pods are READY.
```
kubectl get sts.apps.kruise.io

NAME                     DESIRED   CURRENT   UPDATED   READY   AGE
guestbook-with-sidecar   20        20        20        20      6m
```

Describe one Guestbook pod
`kubectl describe pod guestbook-with-sidecar-0`

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

## View the Sidecar Guestbook

You can now view the Sidecar Guestbook on browser.

* **Local Host:**
    If you are running Kubernetes locally, to view the sidecar guestbook, navigate to `http://localhost:4000`. 

* **Remote Host:**
    To view the sidecar guestbook on a remote host, locate the external IP of the load balancer in the **IP** column of the `kubectl get services` output.
    For example, run 
```
$ kubectl get svc

NAME                        TYPE           CLUSTER-IP     EXTERNAL-IP     PORT(S)                         AGE
guestbook-with-sidecar      LoadBalancer   172.21.2.187   47.101.74.131   3000:31459/TCP,4000:32099/TCP   35m
kubernetes                  ClusterIP      172.21.0.1     <none>          443/TCP                         104m
redis-master                ClusterIP      172.21.10.81   <none>          6379/TCP                        86m
redis-slave                 ClusterIP      172.21.5.58    <none>          6379/TCP                        86m
```

`47.101.74.131` is the external IP. 


Visit `http://47.101.74.131:4000` for the sidecar guestbook.
![Guestbook](./v1/guestbook-sidecar.jpg)

The main guestbook is running on port `3000`, i.e. `http://localhost:4000`