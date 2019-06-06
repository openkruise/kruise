# OpenKruise/Kruise

Kruise is an operation suite which extends 
[Kubernetes controllers](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/)
for better workload management.

* For more information about Kruise, please visit [kruise.io](https://kruise.io).

Currently, Kruise offers three Kubernetes workload controllers. 
- [Advanced StatefulSet](./astatefulset/README.md): An enhanced version of default [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) with extra functionalities such as `inplace-update`.
- [BroadcastJob](./broadcastJob/README.md): A job that runs pods to completion across all the nodes in the cluster.
- [SidecarSet](./sidecarSet/README.md): A controller that injects sidecar container into the pod spec based on selectors.

Their detailed descriptions can be found 
in [documents](./docs/README.md).

# Getting start

## Install with YAML files

#### Install CRDs
```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_broadcastjob.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_sidecarset.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_statefulset.yaml
```
Note that ALL three CRDs need to be installed for kruise-controller to run properly.

#### Install kruise-controller
`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/manager/all_in_one.yaml`

Or run from the repo root directory:

`kustomize build config/default | kubectl apply -f -`

To Install Kustomize, check kustomize [website](https://github.com/kubernetes-sigs/kustomize).

Note that use Kustomize 1.0.11. Version 2.0.3 and less has compatibility issues with kube-builder

# Examples

## Advanced StatefulSet

## Broadcast Job
Run a BroadcastJob that each Pod computes pi, with `ttlSecondsAfterFinished` set to 30. The job
will be deleted in 30 seconds after the job is finished.

```
apiVersion: apps.kruise.io/v1alpha1
kind: BroadcastJob
metadata:
  name: broadcastjob-ttl
spec:
  template:
    spec:
      containers:
        - name: pi
          image: perl
          command: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
      restartPolicy: Never
  completionPolicy:
    type: Always
    ttlSecondsAfterFinished: 30
```
## SidecarSet

The yaml file below describes a SidecarSet that contains a sidecar container named `sidecar1`

```
# sidecarset.yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  name: test-sidecarset
spec:
  selector: # select the pods to be injected with sidecar containers
    matchLabels:
      app: nginx
  containers:
  - name: sidecar1
    image: centos:7
    command: ["sleep", "999d"] # do nothing at all 
```

# Contribute

There's a `Makefile` in the root folder which describes the options to build and install. Here are some common ones:

Build the controller manager binary

`make manager`

Run the tests

`make test`

Build the docker image, by default the image name is `kruiseio/kruise-controller:v1`

`export IMG=<your_image_name> && make docker-build`

Push the image

`export IMG=<your_image_name> && make docker-push`
or just
`docker push <your_image_name>`

Generate manifests e.g. CRD, RBAC etc.

`make manifests` 

