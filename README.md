# OpenKruise/Kruise

[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openkruise/kruise)](https://goreportcard.com/report/github.com/openkruise/kruise)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2908/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2908)

|![notification](docs/img/bell-outline-badge.svg)Community Meeting|
|------------------|
|The Kruise Project holds bi-weekly community calls. To join us and watch previous meeting notes and recordings, please see [meeting schedule](https://github.com/openkruise/project/blob/master/MEETING_SCHEDULE.md).|

Kruise is the core of the OpenKruise project. It is a set of controllers which extends and complements [Kubernetes core controllers](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) on workload management.

Today, Kruise offers three workload controllers:

- [Advanced StatefulSet](./docs/concepts/astatefulset/README.md): An enhanced version of default [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) with extra functionalities such as `inplace-update`, `pasue` and `MaxUnavailable`.

- [BroadcastJob](./docs/concepts/broadcastJob/README.md): A job that runs Pods to completion across all the nodes in the cluster.

- [SidecarSet](./docs/concepts/sidecarSet/README.md): A controller that injects sidecar containers into the Pod spec based on selectors and also is able to upgrade the sidecar containers.

Please see [documents](./docs/README.md) for more technical information.

The project **roadmap** is actively updated in [here](https://github.com/openkruise/kruise/projects).

Several [tutorials](./docs/tutorial/README.md) are provided to demonstrate how to use the workload controllers.
A [video](https://www.youtube.com/watch?v=elB7reZ6eAQ) by [Lachlan Evenson](https://github.com/lachie83) is also provided for demonstrating the controllers.

## Getting started

### Install with YAML files

#### Install CRDs

```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_broadcastjob.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_sidecarset.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_statefulset.yaml
```

#### Install kruise-controller-manager

`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/manager/all_in_one.yaml`

Or run from the repo root directory:

`kustomize build config/default | kubectl apply -f -`

To Install Kustomize, check kustomize [website](https://github.com/kubernetes-sigs/kustomize).

Note that use Kustomize 1.0.11. Version 2.0.3 has compatibility issues with kube-builder

The official kruise-controller-manager image is hosted under [docker hub](https://hub.docker.com/r/openkruise/kruise-manager).

## Usage examples

### Advanced StatefulSet

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: StatefulSet
metadata:
  name: sample
spec:
  replicas: 3
  serviceName: fake-service
  selector:
    matchLabels:
      app: sample
  template:
    metadata:
      labels:
        app: sample
    spec:
      readinessGates:
        # A new condition must be added to ensure the pod remain at NotReady state while the in-place update is happening
      - conditionType: InPlaceUpdateReady
      containers:
      - name: main
        image: nginx:alpine
  podManagementPolicy: Parallel  # allow parallel updates, works together with maxUnavailable
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      # Do in-place update if possible, currently only image update is supported for in-place update
      podUpdatePolicy: InPlaceIfPossible
      # Allow parallel updates with max number of unavailable instances equals to 2
      maxUnavailable: 2
```

### Broadcast Job

Run a BroadcastJob that each Pod computes pi, with `ttlSecondsAfterFinished` set to 30. The job
will be deleted in 30 seconds after the job is finished.

```yaml
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

### SidecarSet

The yaml file below describes a SidecarSet that contains a sidecar container named `sidecar1`

```yaml
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

## Developer Guide

There's a `Makefile` in the root folder which describes the options to build and install. Here are some common ones:

Build the controller manager binary

`make manager`

Run the tests

`make test`

Build the docker image, by default the image name is `openkruise/kruise-manager:v1alpha1`

`export IMG=<your_image_name> && make docker-build`

Push the image

`export IMG=<your_image_name> && make docker-push`
or just
`docker push <your_image_name>`

Generate manifests e.g. CRD, RBAC YAML files etc.

`make manifests`

To develop/debug kruise controller manager locally, please check the [debug guide](./docs/debug/README.md).

## Uninstall

To uninstall kruise from a Kubernetes cluster:

```bash
export KUBECONFIG=PATH_TO_CONFIG
sh -c "$(curl -fsSL https://raw.githubusercontent.com/kruiseio/kruise/master/scripts/uninstall.sh)"
```

Note that this will lead to all resources created by Kruise, including webhook configurations, services, namespace, CRDs, CR instances and Pods managed by Kruise controller, to be deleted! 
Please do this **ONLY** when you fully understand the consequence.

## Community

If you have any questions or want to contribute, you are welcome to communicate most things via GitHub issues or pull requests.

Other active communication channels:

- Slack: [channel address](https://join.slack.com/t/kruise-workspace/shared_invite/enQtNjU5NzQ0ODcyNjYzLWMzZDI5NTM3ZjM1MGY2Mjg1NzU4ZjBjMDJmNjZmZTEwYTZkMzk4ZTAzNmY5NTczODhkZDU2NzVhM2I2MzNmODc)
- Mailing List: todo
- Dingtalk Group(钉钉讨论群)

<div align="center">
  <img src="docs/img/openkruise-dev-group.JPG" width="250" title="dingtalk">
</div>

## Copyright

Certain implementation relies on existing code from Kubernetes and the credit goes to original Kubernetes authors.
