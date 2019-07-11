# Install Kruise Controller Manager

Below steps assume you have an existing kubernetes cluster running properly.

## Install with YAML files

### Install Kruise CRDs

```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_broadcastjob.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_sidecarset.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_statefulset.yaml
```

### Install kruise-manager

`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/manager/all_in_one.yaml`

## Verify Kruise-manager is running

Check the kruise-manager pod is running

```
kubectl get pods -n kruise-system

NAME                          READY   STATUS    RESTARTS   AGE
kruise-controller-manager-0   1/1     Running   0          4m11s
```

Check the kruise-manager endpoint is registered in the `kruise-webhook-server-service`

```
kubectl describe svc kruise-webhook-server-service -n kruise-system
Name:              kruise-webhook-server-service
Namespace:         kruise-system
Labels:            <none>
Annotations:       <none>
Selector:          control-plane=controller-manager
Type:              ClusterIP
IP:                172.21.9.220
Port:              <unset>  443/TCP
TargetPort:        9876/TCP
Endpoints:         172.20.1.12:9876  # Verify this entry is not empty
Session Affinity:  None
Events:            <none>
```