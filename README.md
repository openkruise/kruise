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
|Dec 16th, 2020. Kruise v0.7.0 is **RELEASED**! It provides a new CRD named AdvancedCronJob, promotes AdvancedStatefulSet to v1beta1 and a few features in other controllers, please check the [CHANGELOG](CHANGELOG.md) for details.|
|Oct 1st, 2020. Kruise v0.6.1 is **RELEASED**! It provides various features and bugfix, such as CloneSet lifecycle hook and UnitedDeployment supported CloneSet, please check the [CHANGELOG](CHANGELOG.md) for details.|
|Aug 19th, 2020. Kruise v0.6.0 is **RELEASED**! It updates Kubernetes dependency and switches to new controller runtime framework. It also supports a new controller called Advanced DaemonSet, please check the [CHANGELOG](CHANGELOG.md) for details.|

## Introduction

OpenKruise  (official site: [https://openkruise.io](https://openkruise.io)) is now hosted by the [Cloud Native Computing Foundation](https://cncf.io/) (CNCF) as a Sandbox Level Project.
It consists of several controllers which extend and complement the [Kubernetes core controllers](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) for workload management.

As of now, Kruise offers these workload controllers:

- [CloneSet](https://openkruise.io/en-us/docs/cloneset.html): CloneSet is a workload that mainly focuses on managing stateless applications. It provides a rich set of features for more efficient, deterministic and controlled management, such as in-place update, specified Pod deletion, configurable priority/scatter based update, preUpdate/postUpdate hooks, etc. This [post](https://thenewstack.io/introducing-cloneset-production-grade-kubernetes-deployment-crd/) provides more details about why CloneSet is useful.

- [Advanced StatefulSet](https://openkruise.io/en-us/docs/advanced_statefulset.html): An enhanced version of default [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) with extra functionalities such as `in-place update`, `pause` and `maxUnavailable`.

- [SidecarSet](https://openkruise.io/en-us/docs/sidecarset.html): A controller that injects sidecar containers into the Pod spec based on the Pod selectors. The controller is also responsible for upgrading the sidecar containers.

- [UnitedDeployment](https://openkruise.io/en-us/docs/uniteddeployment.html): This controller manages application Pods spread in multiple fault domains by using multiple workloads.

- [BroadcastJob](https://openkruise.io/en-us/docs/broadcastjob.html): A job that runs Pods to completion across all the nodes in the cluster.

- [Advanced DaemonSet](https://openkruise.io/en-us/docs/advanced_daemonset.html): An enhanced version of default [DaemonSet](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/) with extra upgrade strategies such as partition, node selector, pause and surging.

- [AdvancedCronJob](https://openkruise.io/en-us/docs/advancedcronjob.html): An extended CronJob controller, currently its template supports Job and BroadcastJob.

The project **roadmap** is actively updated in [here](https://github.com/openkruise/kruise/projects).
This [video](https://www.youtube.com/watch?v=elB7reZ6eAQ) demo by [Lachlan Evenson](https://github.com/lachie83) is a good introduction for new users.

## Key Features

- **In-place update**

    In-place update provides an alternative to update container images without deleting and recreating the Pod. It is much faster compared to the recreate update used by the native Deployment/StatefulSet and has almost no side effects on other running containers.

- **Sidecar containers management**

    The Sidecar containers can be simply defined in the SidecarSet custom resource and the controller will inject them into all Pods matched. The implementation is done by using Kubernetes mutating webhooks, similar to what [istio](https://istio.io/latest/docs/setup/additional-setup/sidecar-injection/) does. However, SidecarSet allows you to explicitly manage your own sidecars.

- **Multiple fault domains deployment**

    A global workload can be defined over multiple fault domains, and the Kruise controller will spread a sub workload in each domain. You can manage the domain replicas, sub workload template and update strategies uniformly using the global workload.

## Quick Start

For a Kubernetes cluster with its version higher than v1.13, you can simply install Kruise with helm v3.1.0+:

```bash
# Kubernetes 1.13 and 1.14
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.7.0/kruise-chart.tgz --disable-openapi-validation

# Kubernetes 1.15 and newer versions
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.7.0/kruise-chart.tgz
```

Note that installing this chart directly means it will use the default template values for the kruise-manager.
You may have to set your specific configurations when it is deployed into a production cluster or you want to enable/disable specific controllers.

For more details, see [quick-start](https://openkruise.io/en-us/docs/quick_start.html).

## Documentation

You can view the full documentation from the [OpenKruise website](https://openkruise.io/en-us/docs/what_is_openkruise.html).

We also provide [**tutorials**](./docs/tutorial/README.md) for **ALL** Kruise controllers to demonstrate how to use them.

## Contributing

You are warmly welcome to hack on Kruise. We have prepared a detailed guide [CONTRIBUTING.md](CONTRIBUTING.md).

## Community

Active communication channels:

- Slack: [channel address](https://join.slack.com/t/kruise-workspace/shared_invite/enQtNjU5NzQ0ODcyNjYzLWJlZGJiZjUwNGU5Y2U2ODI3N2JiODI4N2M1OWFlOTgzMDgyOWVkZGRjNzdmZTBjYzgxZmM5MjAyNjhhZTdmMjQ)
- Mailing List: todo
- Dingtalk Group(钉钉讨论群)

<div>
  <img src="docs/img/openkruise-dev-group.JPG" width="280" title="dingtalk">
</div>

## License

Kruise is licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE.md) for the full license text.
