# OpenKruise/Kruise

[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openkruise/kruise)](https://goreportcard.com/report/github.com/openkruise/kruise)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2908/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2908)
[![Build Status](https://travis-ci.org/openkruise/kruise.svg?branch=master)](https://travis-ci.org/openkruise/kruise)
[![CircleCI](https://circleci.com/gh/openkruise/kruise.svg?style=svg)](https://circleci.com/gh/openkruise/kruise)
[![codecov](https://codecov.io/gh/openkruise/kruise/branch/master/graph/badge.svg)](https://codecov.io/gh/openkruise/kruise)
[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-v2.0%20adopted-ff69b4.svg)](./CODE_OF_CONDUCT.md)

English | [简体中文](./README-zh_CN.md)

|![notification](docs/img/bell-outline-badge.svg) What is NEW!|
|------------------|
|May 19th, 2020. Kruise v0.5.0 is **RELEASED**! It supports `maxSurge` for CloneSet and fixes bugs for StatefulSet/SidecarSet, please check the [CHANGELOG](CHANGELOG.md) for details.|
|Mar 20th, 2020. Kruise v0.4.1 is **RELEASED**! It provides **graceful in-place update** for Advanced StatefulSet and CloneSet, please check the [CHANGELOG](CHANGELOG.md) for details.|
|Nov 24th, 2019. A blog about new UnitedDeployment controller is posted in Kruise Blog ([link](http://openkruise.io/en-us/blog/blog3.html)).|

## Introduction

Kruise is the core of the OpenKruise project. It is a set of controllers which extends and complements [Kubernetes core controllers](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) on workload management.

Today, Kruise offers five workload controllers:

- [CloneSet](./docs/concepts/cloneset/README.md): CloneSet is a workload that mainly focuses on managing stateless applications. It provides full features for more efficient, deterministic and controlled deployment, such as inplace update, specified pod deletion, configurable priority/scatter update, preUpdate/postUpdate hooks.

- [Advanced StatefulSet](./docs/concepts/astatefulset/README.md): An enhanced version of default [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) with extra functionalities such as `inplace-update`, `pause` and `MaxUnavailable`.

- [SidecarSet](./docs/concepts/sidecarSet/README.md): A controller that injects sidecar containers into the Pod spec based on selectors and also is able to upgrade the sidecar containers.

- [UnitedDeployment](./docs/concepts/uniteddeployment/README.md): This controller manages application pods spread in multiple fault domains by using multiple workloads.

- [BroadcastJob](./docs/concepts/broadcastJob/README.md): A job that runs Pods to completion across all the nodes in the cluster.

The project **roadmap** is actively updated in [here](https://github.com/openkruise/kruise/projects).
This [video](https://www.youtube.com/watch?v=elB7reZ6eAQ) demo by [Lachlan Evenson](https://github.com/lachie83) is great for new users.

## Getting started

### Check before installation

Kruise requires APIServer to enable features such as `MutatingAdmissionWebhook` and `ValidatingAdmissionWebhook`.
If your Kubernetes version is lower than 1.12, you should check your cluster qualification
before installing Kruise by running one of the following commands locally.
The script assumes a read/write permission to /tmp and the local `Kubectl` is configured to access the target cluster.

```bash
sh -c "$(curl -fsSL https://raw.githubusercontent.com/openkruise/kruise/master/scripts/check_for_installation.sh)"
```

### Install with helm charts

It is recommended that you should install Kruise with helm v3, which is a simple command-line tool and you can get it from [here](https://github.com/helm/helm/releases).

```
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.5.0/kruise-chart.tgz
```

Note that installing this chart directly means it will use the default template values for kruise-manager.
You may have to set your specific configurations when it is deployed into a production cluster or you want to enable specific controllers.

The official kruise-controller-manager image is hosted under [docker hub](https://hub.docker.com/r/openkruise/kruise-manager).

### Optional: Enable specific controllers

If you only need some of the Kruise controllers and want to disable others, you can use either one of the two options or both:

1. Only install the CRDs you need.

2. Set env `CUSTOM_RESOURCE_ENABLE` in kruise-manager container by changing kruise-controller-manager statefulset template. The value is a list of resource names that you want to enable. For example, `CUSTOM_RESOURCE_ENABLE=StatefulSet,SidecarSet` means only AdvancedStatefulSet and SidecarSet controllers/webhooks are enabled, all other controllers/webhooks are disabled. This option can also be applied by using helm chart:

```
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.5.0/kruise-chart.tgz --set manager.custom_resource_enable="StatefulSet\,SidecarSet"
```

## Usage

Please see detailed [documents](./docs/README.md) which include examples, about Kruise controllers.
We also provider [**tutorials**](./docs/tutorial/README.md) to demonstrate how to use Kruise controllers.

## Uninstall

Note that this will lead to all resources created by Kruise, including webhook configurations, services, namespace, CRDs, CR instances and Pods managed by Kruise controller, to be deleted!
Please do this **ONLY** when you fully understand the consequence.

To uninstall kruise if it is installed with helm charts:

```bash
helm uninstall kruise
```

## Contributing

You are warmly welcomed to hack on Kruise. We have prepared a detailed guide [CONTRIBUTING.md](CONTRIBUTING.md).

## Community

Active communication channels:

- Slack: [channel address](https://join.slack.com/t/kruise-workspace/shared_invite/enQtNjU5NzQ0ODcyNjYzLWJlZGJiZjUwNGU5Y2U2ODI3N2JiODI4N2M1OWFlOTgzMDgyOWVkZGRjNzdmZTBjYzgxZmM5MjAyNjhhZTdmMjQ)
- Mailing List: todo
- Dingtalk Group(钉钉讨论群)

<div align="center">
  <img src="docs/img/openkruise-dev-group.JPG" width="250" title="dingtalk">
</div>

## License

Kruise is licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE.md) for the full license text.