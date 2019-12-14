# OpenKruise/Kruise

[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openkruise/kruise)](https://goreportcard.com/report/github.com/openkruise/kruise)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2908/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2908)

|![notification](docs/img/bell-outline-badge.svg) What is NEW!|
|------------------|
|Nov 24th, 2019. A blog about new UnitedDeployment controller is posted in Kruise Blog ([link](http://openkruise.io/en-us/blog/blog3.html)).|

Kruise is the core of the OpenKruise project. It is a set of controllers which extends and complements [Kubernetes core controllers](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) on workload management.

Today, Kruise offers four workload controllers:

- [Advanced StatefulSet](./docs/concepts/astatefulset/README.md): An enhanced version of default [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) with extra functionalities such as `inplace-update`, `pasue` and `MaxUnavailable`.

- [BroadcastJob](./docs/concepts/broadcastJob/README.md): A job that runs Pods to completion across all the nodes in the cluster.

- [SidecarSet](./docs/concepts/sidecarSet/README.md): A controller that injects sidecar containers into the Pod spec based on selectors and also is able to upgrade the sidecar containers.

- [UnitedDeployment](./docs/concepts/uniteddeployment/README.md): This controller manages application pods spread in multiple fault domains by using multiple workloads.

The project **roadmap** is actively updated in [here](https://github.com/openkruise/kruise/projects).
This [video](https://www.youtube.com/watch?v=elB7reZ6eAQ) demo by [Lachlan Evenson](https://github.com/lachie83) is great for new users.

## Getting started

### Check before installation

Kruise requires APIServer to enable features such as `MutatingAdmissionWebhook` admission webhook. You can check your cluster qualification
before installing Kruise by running one of the following commands locally. The script assumes a read/write permission to /tmp and the local
`Kubectl` is configured to access the target cluster.

#### via curl

```bash
sh -c "$(curl -fsSL https://raw.githubusercontent.com/openkruise/kruise/master/scripts/check_for_installation.sh)"
```

#### via wget

```bash
sh -c "$(wget -O- https://raw.githubusercontent.com/openkruise/kruise/master/scripts/check_for_installation.sh)"
```

### Install with helm charts

```
wget https://raw.githubusercontent.com/openkruise/kruise/master/hack/auto_generate_charts.sh
chmod +x auto_generate_charts.sh
./auto_generate_charts.sh
helm install kruise charts/
```

### Install with YAML files

#### Install CRDs

```
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_broadcastjob.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_sidecarset.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_statefulset.yaml
kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/crds/apps_v1alpha1_uniteddeployment.yaml
```

#### Install kruise-controller-manager

`kubectl apply -f https://raw.githubusercontent.com/kruiseio/kruise/master/config/manager/all_in_one.yaml`

The official kruise-controller-manager image is hosted under [docker hub](https://hub.docker.com/r/openkruise/kruise-manager).

### Enable specific controllers

If you only need some of CRDs in Kruise and want to disable others, you can use either one of the two options or both:

1. Only install the CRDs you need.

2. Set env `CUSTOM_RESOURCE_ENABLE` in kruise-manager container by changing kruise-controller-manager statefulset template. The value is a list of resource names that you want to enable. For example, `CUSTOM_RESOURCE_ENABLE=StatefulSet,SidecarSet` means only AdvancedStatefulSet and SidecarSet controllers/webhooks are enabled, all other controllers/webhooks are disabled.

## Usage

Please see detailed [documents](./docs/README.md) which include examples, about Kruise controllers.
We also provider [**tutorials**](./docs/tutorial/README.md) to demonstrate how to use Kruise controllers.

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

- Slack: [channel address](https://join.slack.com/t/kruise-workspace/shared_invite/enQtNjU5NzQ0ODcyNjYzLWJlZGJiZjUwNGU5Y2U2ODI3N2JiODI4N2M1OWFlOTgzMDgyOWVkZGRjNzdmZTBjYzgxZmM5MjAyNjhhZTdmMjQ)
- Mailing List: todo
- Dingtalk Group(钉钉讨论群)

<div align="center">
  <img src="docs/img/openkruise-dev-group.JPG" width="250" title="dingtalk">
</div>

## Copyright

Certain implementation relies on existing code from Kubernetes and the credit goes to original Kubernetes authors.
