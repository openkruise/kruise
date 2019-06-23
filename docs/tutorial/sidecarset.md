# Inject Sidecar Container with SidecarSet
This tutorial walks you through an example to automatically inject a sidecar container with sidecarset.

## Install Guestbook sidecarset

The sidecarset controller is a webhook controller and will watch pod creation and automatically inject a sidecar guestbook container into the matched pods

`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-sidecar.yaml`

## Take a closer look into guestbook application

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  name: guestbook-sidecar
spec:
  selector: # select the pods to be injected with sidecar containers
    matchLabels:
      # This needs to match the labels of the pods to be injected.
      app.kubernetes.io/name: guestbook-kruise
  containers:
    - name: guestbook-sidecar
      image: openkruise/guestbook:sidecar
      imagePullPolicy: Always
      ports:
        - name: sidecar-server
          containerPort: 4000 # different from main guestbook containerPort which is 3000
``` 


## Installing the application

To install the chart with release name (application name) of `demo-v1`, replica of `20`:

```bash
helm install demo-v1 apphub/guestbook-kruise --set replicaCount=20,image.repository=openkruise/guestbook,image.tag=v2
```
The Chart is located in [this repo](https://github.com/cloudnativeapp/workshop/tree/master/kubecon2019china/charts/guestbook-kruise).


Alternatively, Install the application using YAML files:
```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-sts-for-sidecar-demo.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/docs/tutorial/v1/guestbook-service-for-sidecar-demo.yaml
```

## Check your application
Check the guestbook are started. `statefulset.apps.kruise.io` or shortname `sts.apps.kruise.io` is the resource kind. 
`app.kruise.io` postfix needs to be appended due to naming collision with Kubernetes native `statefulset` kind.
 Verify that all pods are READY.
```
kubectl get sts.apps.kruise.io
NAME                            DESIRED    CURRENT    UPDATED    READY    AGE
demo-v1-guestbook-kruise   20         20         20         20       17s

kubectl get pods
NAME                                   READY   STATUS    RESTARTS   AGE
demo-v1-guestbook-kruise-0        1/1     Running   0          39s
demo-v1-guestbook-kruise-1        1/1     Running   0          39s
...
demo-v1-guestbook-kruise-16       1/1     Running   0          35s
demo-v1-guestbook-kruise-17       1/1     Running   0          34s
demo-v1-guestbook-kruise-18       1/1     Running   0          34s
demo-v1-guestbook-kruise-19       1/1     Running   0          33s
```

Describe one guestbook pod:

```
kubectl describe pod demo-v1-guestbook-kruise-0
```

Check that the sidecar container is injected.

```yaml
    Containers:
      guestbook:
        Container ID:   docker://44f19a140c30de2c5b1a3f63c252c074efbb9c1b5eb7893ee7134461466b35c8
        Image:          openkruise/guestbook:v2
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
    To view the sidecar guestbook on a remote host, locate the external IP of the application in the **IP** column of the `kubectl get services` output.
    For example, run 
```
kubectl get svc

NAME           TYPE           CLUSTER-IP     EXTERNAL-IP     PORT(S)                         AGE
demo-v1-guestbook-kruise      LoadBalancer   172.21.2.187   47.101.74.131   3000:31459/TCP,4000:32099/TCP   35m
```

`47.101.74.131` is the external IP. 


Visit `http://47.101.74.131:4000` for the sidecar guestbook.
![Guestbook](./v1/guestbook-sidecar.jpg)

The main guestbook is running on port `3000`, i.e. `http://localhost:4000`

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
kubectl delete svc demo-v1-guestbook-kruise redis-master redis-slave
kubectl delete deploy redis-master redis-slave
```