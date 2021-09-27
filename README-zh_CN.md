# Kruise

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
|Sep 6th, 2021. Kruise v0.10.0 发布! 新增 WorkloadSpread、PodUnavailableBudget 控制器以及一系列新功能, please check the [CHANGELOG](CHANGELOG.md) for details.|
|May 20th, 2021. Kruise v0.9.0 发布! 新增了多种重大功能如 容器重建/重启、删除安全防护等，详情参见 [CHANGELOG](CHANGELOG.md).|
|Mar 4th, 2021. Kruise v0.8.0 发布! 提供了重构版本的 SidecarSet、UnitedDeployment 支持管理 Deployment，以及一个新的 kruise-daemon 组件目前支持镜像预热，详情参见 [CHANGELOG](CHANGELOG.md).|

## 介绍

OpenKruise (官网: [https://openkruise.io](https://openkruise.io)) 是托管在 [Cloud Native Computing Foundation](https://cncf.io/) (CNCF) 下的 Sandbox 项目。
它提供一套在 [Kubernetes核心控制器](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) 之外的扩展工作负载、应用管理能力。

## 核心能力

- **通用工作负载**

  通用工作负载能帮助你管理 stateless(无状态)、stateful(有状态)、daemon 类型的应用。

  它们不仅支持类似于 Kubernetes 原生 Workloads 的基础功能，还提供了如 **原地升级**、**可配置的扩缩容/发布策略**、**并发操作** 等。

  - [**CloneSet** - 无状态应用](https://openkruise.io/zh/docs/user-manuals/cloneset/)
  - [**Advanced StatefulSet** - 有状态应用](https://openkruise.io/zh/docs/user-manuals/advancedstatefulset)
  - [**Advanced DaemonSet** - daemon 类型应用](https://openkruise.io/zh/docs/user-manuals/advanceddaemonset)

- **任务工作负载**

  - [**BroadcastJob** - 部署任务到一批特定节点上](https://openkruise.io/zh/docs/user-manuals/broadcastjob)
  - [**AdvancedCronJob** - 周期性地创建 Job 或 BroadcastJob](https://openkruise.io/zh/docs/user-manuals/advancedcronjob)

- **Sidecar 容器管理**

  Sidecar 容器可以很简单地通过 **SidecarSet** 来定义，然后 Kruise 会将它们注入到所有匹配的 Pod 中。

  它是通过 Kubernetes webhook 机制来实现的，和 [istio](https://istio.io/latest/docs/setup/additional-setup/sidecar-injection/) 的注入实现方式类似，
  但是它允许你指定管理你自己的 sidecar 容器。

  - [**SidecarSet** - 定义和升级你的 sidecar 容器](https://openkruise.io/zh/docs/user-manuals/sidecarset)

- **多区域管理**

  它可以帮助你在一个 Kubernetes 集群中的多个区域上部署应用，比如 不同的 node 资源池、可用区、机型架构（x86 & arm）、节点类型（kubelet & virtual kubelet）等。

  这里我们提供两种不同的方式：

  - [**WorkloadSpread** - 旁路地分发 workload 创建的 pods](https://openkruise.io/zh/docs/user-manuals/workloadspread)
  - [**UnitedDeployment** - 一个新的 workload 来管理多个下属的 workloads](https://openkruise.io/zh/docs/user-manuals/uniteddeployment)

- **增强运维能力**

  - [原地重启 pod 中的容器](https://openkruise.io/zh/docs/user-manuals/containerrecreaterequest)
  - [指定的一批节点上拉取镜像](https://openkruise.io/zh/docs/user-manuals/imagepulljob)

- **应用安全防护**

  - [保护 Kubernetes 资源及应用 pods 不被级联删除](https://openkruise.io/zh/docs/user-manuals/deletionprotection)
  - [**PodUnavailableBudget** - 覆盖更多的 Voluntary Disruption 场景，提供应用更加强大的防护能力](https://openkruise.io/zh/docs/user-manuals/podunavailablebudget)

## 快速开始

我们强烈建议在 **Kubernetes >= 1.16** 以上版本的集群中使用 Kruise，使用 helm v3.1.0+ 执行安装即可：

```bash
helm install kruise https://github.com/openkruise/kruise/releases/download/v0.10.0/kruise-chart.tgz
```

> 注意直接安装 chart 会使用默认的 template values，你也可以根据你的集群情况指定一些特殊配置，比如修改 resources 限制或者配置 feature-gates。

更多的安装/升级细节、或者更老版本的 Kubernetes 集群，可以查看 [这个文档](https://openkruise.io/docs/installation)。

## 文档

你可以在 [OpenKruise website](https://openkruise.io/zh/docs/) 查看到完整的文档集。

## 用户

登记: [如果贵司正在使用 Kruise 请留言](https://github.com/openkruise/kruise/issues/289)

- 阿里巴巴集团, 蚂蚁集团, 斗鱼TV, 申通, Boss直聘
- 杭银消费, 万翼科技, 多点, Bringg, 佐疆科技
- Lyft, 携程, 享住智慧, VIPKID, 掌门1对1
- 小红书, 比心, 永辉科技中心, 跟谁学, 哈啰出行
- Spectro Cloud, 艾佳生活, Arkane Systems, 滴普科技, 火花思维
- OPPO, 苏宁, 欢聚时代, 汇量科技, 深圳凤凰木网络有限公司

## 贡献

我们非常欢迎每一位社区同学共同参与 Kruise 的建设，你可以从 [CONTRIBUTING.md](CONTRIBUTING.md) 手册开始。

## 社区

活跃的社区途径：

- Slack: [OpenKruise channel](https://kubernetes.slack.com/channels/openkruise) (*English*)
- 钉钉：搜索群ID `23330762` (*Chinese*)
- 社区双周会 (APAC, *Chinese*):
  - 周四 19:00 GMT+8 (Asia/Shanghai)
  - [进入会议(zoom)](https://us02web.zoom.us/j/87059136652?pwd=NlI4UThFWXVRZkxIU0dtR1NINncrQT09)
  - [会议纪要](https://shimo.im/docs/gXqmeQOYBehZ4vqo)
- Bi-weekly Community Meeting (*English*): TODO

## License

Kruise is licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE.md) for the full license text.
