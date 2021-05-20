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
|May 20th, 2021. Kruise v0.9.0 is **RELEASED**! It provides great features such as ContainerRecreate and DeletionProtection, please check the [CHANGELOG](CHANGELOG.md) for details.|
|Mar 4th, 2021. Kruise v0.8.0 is **RELEASED**! It provides refactoring SidecarSet, Deployment hosted by UnitedDeployment, and a new kruise-daemon component which supports image pre-download, please check the [CHANGELOG](CHANGELOG.md) for details.|
|Dec 16th, 2020. Kruise v0.7.0 is **RELEASED**! It provides a new CRD named AdvancedCronJob, promotes AdvancedStatefulSet to v1beta1 and a few features in other controllers, please check the [CHANGELOG](CHANGELOG.md) for details.|

## Introduction

OpenKruise  (official site: [https://openkruise.io](https://openkruise.io)) is now hosted by the [Cloud Native Computing Foundation](https://cncf.io/) (CNCF) as a Sandbox Level Project.
It consists of several controllers which extend and complement the [Kubernetes core controllers](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) for workload and application management.

As of now, Kruise mainly offers these controllers:

- [CloneSet](https://openkruise.io/en-us/docs/cloneset.html): CloneSet is a workload that mainly focuses on managing stateless applications. It provides a rich set of features for more efficient, deterministic and controlled management, such as in-place update, specified Pod deletion, configurable priority/scatter based update, preUpdate/postUpdate hooks, etc. This [post](https://thenewstack.io/introducing-cloneset-production-grade-kubernetes-deployment-crd/) provides more details about why CloneSet is useful.

- [Advanced StatefulSet](https://openkruise.io/en-us/docs/advanced_statefulset.html): An enhanced version of default [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) with extra functionalities such as `in-place update`, `pause` and `maxUnavailable`.

- [SidecarSet](https://openkruise.io/en-us/docs/sidecarset.html): A controller that injects sidecar containers into the Pod spec based on the Pod selectors. The controller is also responsible for upgrading the sidecar containers.

- [Advanced DaemonSet](https://openkruise.io/en-us/docs/advanced_daemonset.html): An enhanced version of default [DaemonSet](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/) with extra upgrade strategies such as partition, node selector, pause and surging.

- [UnitedDeployment](https://openkruise.io/en-us/docs/uniteddeployment.html): This controller manages application Pods spread in multiple fault domains by using multiple workloads.

- [BroadcastJob](https://openkruise.io/en-us/docs/broadcastjob.html): A job that runs Pods to completion across all the nodes in the cluster.

- [AdvancedCronJob](https://openkruise.io/en-us/docs/advancedcronjob.html): An extended CronJob controller, currently its template supports Job and BroadcastJob.

- [ImagePullJob](https://openkruise.io/en-us/docs/imagepulljob.html): Help users download images on any nodes they want.

- [ContainerRecreateRequest](https://openkruise.io/en-us/docs/containerrecreaterequest.html):  Provides a way to let users restart/recreate one or more containers in an existing Pod.

- [Deletion Protection](https://openkruise.io/en-us/docs/deletion_protection.html): Provides a safety policy which could help users protect Kubernetes resources and applications' availability from the cascading deletion mechanism.

## Key Features

- **In-place update**

    In-place update provides an alternative to update container images without deleting and recreating the Pod. It is much faster compared to the recreate update used by the native Deployment/StatefulSet and has almost no side effects on other running containers.

- **Sidecar containers management**

    The Sidecar containers can be simply defined in the SidecarSet custom resource and the controller will inject them into all Pods matched. The implementation is done by using Kubernetes mutating webhooks, similar to what [istio](https://istio.io/latest/docs/setup/additional-setup/sidecar-injection/) does. However, SidecarSet allows you to explicitly manage your own sidecars.

- **Multiple fault domains deployment**

    A global workload can be defined over multiple fault domains, and the Kruise controller will spread a sub workload in each domain. You can manage the domain replicas, sub workload template and update strategies uniformly using the global workload.

- **Image pre-download**

  Help users download images on any nodes they want.

- **Container recreate/restart**

  Help users restart/recreate one or more containers in an existing Pod.

- **...**

## Quick Start

For a Kubernetes cluster with its version higher than v1.13, you can simply install Kruise with helm v3.1.0+:

```bash
# Kubernetes 1.13 and 1.14
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.9.0/kruise-chart.tgz --disable-openapi-validation

# Kubernetes 1.15 and newer versions
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.9.0/kruise-chart.tgz
```

Note that installing this chart directly means it will use the default template values for the kruise-manager.
You may have to set your specific configurations when it is deployed into a production cluster or you want to configure feature-gates.

For more details, see [installation doc](https://openkruise.io/en-us/docs/installation.html).

## Documentation

You can view the full documentation from the [OpenKruise website](https://openkruise.io/en-us/docs/what_is_openkruise.html).

We also provide [**tutorials**](./docs/tutorial/README.md) for **ALL** Kruise controllers to demonstrate how to use them.

## Users

Registration: [Who is using Kruise](https://github.com/openkruise/kruise/issues/289)

- Alibaba Group, Ant Group, DouyuTV, Sto, Boss直聘
- hangyinxiaofei, vanyitech, Dmall, Bringg, 佐疆科技
- Lyft, Ctrip, 享住智慧, VIPKID, zhangmen
- xiaohongshu, bixin, 永辉科技中心, 跟谁学, 哈啰出行
- Spectro Cloud, ihomefnt, Arkane Systems, Deepexi, 火花思维
- OPPO, Suning.cn

## Contributing

You are warmly welcome to hack on Kruise. We have prepared a detailed guide [CONTRIBUTING.md](CONTRIBUTING.md).

## Community

Active communication channels:

- Slack: [Channel in Kubernetes Slack](https://kubernetes.slack.com/channels/openkruise)
- Mailing List: todo
- Dingtalk Group(钉钉讨论群)

<div>
  <img src="docs/img/openkruise-dev-group.JPG" width="280" title="dingtalk">
</div>

## License

Kruise is licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE.md) for the full license text.
