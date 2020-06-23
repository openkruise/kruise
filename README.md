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

Kruise is the core of the OpenKruise (official site: [https://openkruise.io](https://openkruise.io)) project. It is a set of controllers which extends and complements [Kubernetes core controllers](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) on workload management.

Today, Kruise offers five workload controllers:

- [CloneSet](https://openkruise.io/en-us/docs/cloneset.html): CloneSet is a workload that mainly focuses on managing stateless applications. It provides full features for more efficient, deterministic and controlled deployment, such as inplace update, specified pod deletion, configurable priority/scatter update, preUpdate/postUpdate hooks.

- [Advanced StatefulSet](https://openkruise.io/en-us/docs/advanced_statefulset.html): An enhanced version of default [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) with extra functionalities such as `inplace-update`, `pause` and `MaxUnavailable`.

- [SidecarSet](https://openkruise.io/en-us/docs/sidecarset.html): A controller that injects sidecar containers into the Pod spec based on selectors and also is able to upgrade the sidecar containers.

- [UnitedDeployment](https://openkruise.io/en-us/docs/uniteddeployment.html): This controller manages application pods spread in multiple fault domains by using multiple workloads.

- [BroadcastJob](https://openkruise.io/en-us/docs/broadcastjob.html): A job that runs Pods to completion across all the nodes in the cluster.

The project **roadmap** is actively updated in [here](https://github.com/openkruise/kruise/projects).
This [video](https://www.youtube.com/watch?v=elB7reZ6eAQ) demo by [Lachlan Evenson](https://github.com/lachie83) is great for new users.

## Key features

- **In-place update**

    In-place update is a way to update container images without deleting and creating Pod. It is quite faster than re-create update used by original Deployment/StatefulSet and less side-effect on other containers.

- **Sidecar management**

    Define sidecar containers in one CR and OpenKruise will inject them into all Pods matched. It's just like istio, but you can manage your own sidecars.

- **Multiple fault domains deployment**

    Define a global workload over multiple fault domains, and OpenKruise will spread a sub workload for each domain. You can manage replicas, template and update strategies for workloads in different fault domains.

- **...**

## Quick Start

It is super easy to get started with OpenKruise.

For a Kubernetes cluster with its version higher than v1.12+, you can simply install Kruise with helm v3:

```
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.5.0/kruise-chart.tgz
```

Note that installing this chart directly means it will use the default template values for kruise-manager.
You may have to set your specific configurations when it is deployed into a production cluster or you want to enable specific controllers.

For more details, see [quick-start](https://openkruise.io/en-us/docs/quick_start.html).

## Documentation

You can view the full documentation from the [OpenKruise website](https://openkruise.io/en-us/docs/what_is_openkruise.html).

We also provider [**tutorials**](./docs/tutorial/README.md) to demonstrate how to use Kruise controllers.

## Contributing

You are warmly welcomed to hack on Kruise. We have prepared a detailed guide [CONTRIBUTING.md](CONTRIBUTING.md).

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