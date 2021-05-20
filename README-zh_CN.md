# OpenKruise/Kruise

[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openkruise/kruise)](https://goreportcard.com/report/github.com/openkruise/kruise)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2908/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2908)
[![Build Status](https://travis-ci.org/openkruise/kruise.svg?branch=master)](https://travis-ci.org/openkruise/kruise)
[![CircleCI](https://circleci.com/gh/openkruise/kruise.svg?style=svg)](https://circleci.com/gh/openkruise/kruise)
[![codecov](https://codecov.io/gh/openkruise/kruise/branch/master/graph/badge.svg)](https://codecov.io/gh/openkruise/kruise)
[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-v2.0%20adopted-ff69b4.svg)](./CODE_OF_CONDUCT.md)

[English](./README.md) | 简体中文

|![notification](docs/img/bell-outline-badge.svg) 最新进展：|
|------------------|
|May 20th, 2021. Kruise v0.9.0 发布! 新增了多种重大功能如 容器重建/重启、删除安全防护等，详情参见 [CHANGELOG](CHANGELOG.md).|
|Mar 4th, 2021. Kruise v0.8.0 发布! 提供了重构版本的 SidecarSet、UnitedDeployment 支持管理 Deployment，以及一个新的 kruise-daemon 组件目前支持镜像预热，详情参见 [CHANGELOG](CHANGELOG.md).|
|Dec 16th, 2020. Kruise v0.7.0 发布! 提供一个新的 AdvancedCronJob CRD、将 Advanced StatefulSet 升级 v1beta1 版本、以及其他控制器一些新增能力，详情参见 [CHANGELOG](CHANGELOG.md).|

## 介绍

OpenKruise (官网: [https://openkruise.io](https://openkruise.io)) 是托管在 [Cloud Native Computing Foundation](https://cncf.io/) (CNCF) 下的 Sandbox 项目。
它提供一套在 [Kubernetes核心控制器](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) 之外的扩展工作负载、应用管理能力。

目前，Kruise 主要提供了以下控制器能力：

- [CloneSet](https://openkruise.io/zh-cn/docs/cloneset.html): 提供了更加高效、确定可控的应用管理和部署能力，支持优雅原地升级、指定删除、发布顺序可配置、并行/灰度发布等丰富的策略，可以满足更多样化的应用场景。

- [Advanced StatefulSet](https://openkruise.io/zh-cn/docs/advanced_statefulset.html): 基于原生 [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) 之上的增强版本，默认行为与原生完全一致，在此之外提供了原地升级、并行发布（最大不可用）、发布暂停等功能。

- [SidecarSet](https://openkruise.io/zh-cn/docs/sidecarset.html): 对 sidecar 容器做统一管理，在满足 selector 条件的 Pod 中注入指定的 sidecar 容器。

- [Advanced DaemonSet](https://openkruise.io/zh-cn/docs/advanced_daemonset.html): 基于原生 DaemonSet 之上的增强版本，默认行为与原生一致，在此之外提供了灰度分批、按 Node label 选择、暂停、热升级等发布策略。

- [UnitedDeployment](https://openkruise.io/zh-cn/docs/uniteddeployment.html): 通过多个 subset workload 将应用部署到多个可用区。

- [BroadcastJob](https://openkruise.io/zh-cn/docs/broadcastjob.html): 配置一个 job，在集群中所有满足条件的 Node 上都跑一个 Pod 任务。

- [AdvancedCronJob](https://openkruise.io/zh-cn/docs/advancedcronjob.html): 一个扩展的 CronJob 控制器，目前 template 模板支持配置使用 Job 或 BroadcastJob。

- [ImagePullJob](https://openkruise.io/zh-cn/docs/imagepulljob.html): 支持用户指定在任意范围的节点上预热镜像。

- [ContainerRecreateRequest](https://openkruise.io/zh-cn/docs/containerrecreaterequest.html):  为用户提供了重建/重启存量 Pod 中一个或多个容器的能力。

- [Deletion Protection](https://openkruise.io/zh-cn/docs/deletion_protection.html): 该功能提供了删除安全策略，用来在 Kubernetes 级联删除的机制下保护用户的资源和应用可用性。

## 核心功能

- **原地升级**

    原地升级是一种可以避免删除、新建 Pod 的升级镜像能力。它比原生 Deployment/StatefulSet 的重建 Pod 升级更快、更高效，并且避免对 Pod 中其他不需要更新的容器造成干扰。

- **Sidecar 管理**

    支持在一个单独的 CR 中定义 sidecar 容器，OpenKruise 能够帮你把这些 Sidecar 容器注入到所有符合条件的 Pod 中。这个过程和 Istio 的注入很相似，但是你可以管理任意你关心的 Sidecar。

- **跨多可用区部署**

    定义一个跨多个可用区的全局 workload，容器，OpenKruise 会帮你在每个可用区创建一个对应的下属 workload。你可以统一管理他们的副本数、版本、甚至针对不同可用区采用不同的发布策略。

- **镜像预热**

    支持用户指定在任意范围的节点上下载镜像。

- **容器重建/重启**

    支持用户重建/重启存量 Pod 中一个或多个容器。

- **...**

## 快速开始

想要快速使用 OpenKruise 非常简单！
对于版本高于 v1.13+ 的 Kubernetes 集群来说，只要使用 helm v3.1.0+ 执行安装即可：

```bash
# Kubernetes 版本 1.13 或 1.14
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.9.0/kruise-chart.tgz --disable-openapi-validation

# Kubernetes 版本大于等于 1.15
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.9.0/kruise-chart.tgz
```

注意直接安装 chart 会使用默认的 template values，你也可以根据你的集群情况指定一些特殊配置，比如修改 resources 限制或者配置 feature-gates。

更多细节可以查看 [安装手册](https://openkruise.io/zh-cn/docs/installation.html)

## 文档

你可以在 [OpenKruise website](https://openkruise.io/zh-cn/docs/what_is_openkruise.html) 查看到完整的文档集。

我们也提供了 [**tutorials**](./docs/tutorial/README.md) 来示范如何使用 Kruise 控制器。

## 用户

登记: [如果贵司正在使用 Kruise 请留言](https://github.com/openkruise/kruise/issues/289)

- 阿里巴巴集团, 蚂蚁集团, 斗鱼TV, 申通, Boss直聘
- 杭银消费, 万翼科技, 多点, Bringg, 佐疆科技
- Lyft, 携程, 享住智慧, VIPKID, 掌门1对1
- 小红书, 比心, 永辉科技中心, 跟谁学, 哈啰出行
- Spectro Cloud, 艾佳生活, Arkane Systems, 滴普科技, 火花思维
- OPPO, 苏宁

## 贡献

我们非常欢迎每一位社区同学共同参与 Kruise 的建设，你可以从 [CONTRIBUTING.md](CONTRIBUTING.md) 手册开始。

## 社区

活跃的社区途径：

- Slack: [Channel in Kubernetes Slack](https://kubernetes.slack.com/channels/openkruise)
- 钉钉讨论群

<div>
  <img src="docs/img/openkruise-dev-group.JPG" width="280" title="dingtalk">
</div>

## License

Kruise is licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE.md) for the full license text.
