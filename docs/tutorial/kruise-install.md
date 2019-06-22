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

## Install with helm

TODO