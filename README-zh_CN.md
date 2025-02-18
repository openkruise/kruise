# Kruise

[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openkruise/kruise)](https://goreportcard.com/report/github.com/openkruise/kruise)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2908/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2908)
[![Build Status](https://travis-ci.org/openkruise/kruise.svg?branch=master)](https://travis-ci.org/openkruise/kruise)
[![CircleCI](https://circleci.com/gh/openkruise/kruise.svg?style=svg)](https://circleci.com/gh/openkruise/kruise)
[![codecov](https://codecov.io/gh/openkruise/kruise/branch/master/graph/badge.svg)](https://codecov.io/gh/openkruise/kruise)
[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-v2.0%20adopted-ff69b4.svg)](./CODE_OF_CONDUCT.md)

[English](./README.md) | 简体中文

## 介绍

OpenKruise (官网: [https://openkruise.io](https://openkruise.io)) 是CNCF([Cloud Native Computing Foundation](https://cncf.io/)) 的孵化项目。
它提供一套在 [Kubernetes核心控制器](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) 之外的扩展工作负载、应用管理能力。

## 核心能力

- **高级工作负载**

  通用工作负载能帮助你管理 stateless(无状态)、stateful(有状态)、daemon 类型和作业类的应用。

  它们不仅支持类似于 Kubernetes 原生 Workloads 的基础功能，还提供了如 **原地升级**、**可配置的扩缩容/发布策略**、**并发操作** 等。

  - [**CloneSet** - 无状态应用](https://openkruise.io/zh/docs/user-manuals/cloneset/)
  - [**Advanced StatefulSet** - 有状态应用](https://openkruise.io/zh/docs/user-manuals/advancedstatefulset)
  - [**Advanced DaemonSet** - daemon 类型应用](https://openkruise.io/zh/docs/user-manuals/advanceddaemonset)
  - [**BroadcastJob** - 部署任务到一批特定节点上](https://openkruise.io/zh/docs/user-manuals/broadcastjob)
  - [**AdvancedCronJob** - 周期性地创建 Job 或 BroadcastJob](https://openkruise.io/zh/docs/user-manuals/advancedcronjob)

- **Sidecar 容器管理**

  Kruise通过**SidecarSet**简化了Sidecar的注入， 并提供了sidecar原地升级的能力。另外， Kruise提供了增强的sidecar启动、退出的控制

  - [**SidecarSet** - 定义和升级你的 sidecar 容器](https://openkruise.io/zh/docs/user-manuals/sidecarset)
  - [**Container Launch Priority** 控制sidecar启动顺序](https://openkruise.io/zh/docs/user-manuals/containerlaunchpriority)
  - [**Sidecar Job Terminator** 当 Job 类 Pod 主容器退出后，Terminator Sidecar容器](https://openkruise.io/zh/docs/user-manuals/jobsidecarterminator)

- **多区域管理**

  它可以帮助你在一个 Kubernetes 集群中的多个区域上部署应用，比如 不同的 node 资源池、可用区、机型架构（x86 & arm）、节点类型（kubelet & virtual kubelet）等。

  这里我们提供两种不同的方式：

  - [**WorkloadSpread** - 旁路地分发 workload 创建的 pods](https://openkruise.io/zh/docs/user-manuals/workloadspread)
  - [**UnitedDeployment** - 一个新的 workload 来管理多个下属的 workloads](https://openkruise.io/zh/docs/user-manuals/uniteddeployment)

- **增强运维能力**

  - [原地重启 pod 中的容器](https://openkruise.io/zh/docs/user-manuals/containerrecreaterequest)
  - [指定的一批节点上拉取镜像](https://openkruise.io/zh/docs/user-manuals/imagepulljob)
  - [**ResourceDistribution** 支持 Secret、Configmaps 资源跨 Namespace 分发](https://openkruise.io/zh/docs/user-manuals/resourcedistribution)
  - [**PersistentPodState** 保持Pod的一些状态，比如："固定IP调度"](https://openkruise.io/zh/docs/user-manuals/persistentpodstate)
  - [**PodProbeMarker** 提供自定义Probe探测的能力](https://openkruise.io/zh/docs/user-manuals/podprobemarker)

- **应用安全防护**

  - [保护 Kubernetes 资源及应用 pods 不被级联删除](https://openkruise.io/zh/docs/user-manuals/deletionprotection)
  - [**PodUnavailableBudget** - 覆盖更多的 Voluntary Disruption 场景，提供应用更加强大的防护能力](https://openkruise.io/zh/docs/user-manuals/podunavailablebudget)

## 快速开始

你可以在 [OpenKruise website](https://openkruise.io/zh/docs/) 查看到完整的文档集。

- 安装/升级 Kruise [稳定版本](https://openkruise.io/docs/installation)
- 安装/升级 Kruise [最新版本（包括 alpha/beta/rc）](https://openkruise.io/docs/next/installation)

### 在阿里云上快速体验

- 3分钟内在阿里云上创建 Kruise 体验环境:

  <a href="https://acs.console.aliyun.com/quick-deploy?repo=openkruise/charts&branch=master&paths=%5B%22versions/kruise/1.7.3%22%5D" target="_blank">
    <img src="https://img.alicdn.com/imgextra/i1/O1CN01aiPSuA1Wiz7wkgF5u_!!6000000002823-55-tps-399-70.svg" width="200" alt="Deploy on Alibaba Cloud">
  </a>

## 用户

登记: [如果贵司正在使用 Kruise 请留言](https://github.com/openkruise/kruise/issues/289)

- 阿里巴巴集团, 蚂蚁集团, 斗鱼TV, 申通, Boss直聘
- 杭银消费, 万翼科技, 多点, Bringg, 佐疆科技
- Lyft, 携程, 享住智慧, VIPKID, 掌门1对1
- 小红书, 比心, 永辉科技中心, 跟谁学, 哈啰出行
- Spectro Cloud, 艾佳生活, Arkane Systems, 滴普科技, 火花思维
- OPPO, 苏宁, 欢聚时代, 汇量科技, 深圳凤凰木网络有限公司
- 小米, 网易, 美团金融, 虾皮购物, e签宝
- LinkedIn, 雪球, 兴盛优选, Wholee, LilithGames, Baidu
- Bilibili, 冠赢互娱, MeiTuan, 同城

## 贡献

我们非常欢迎每一位社区同学共同参与 Kruise 的建设，你可以从 [CONTRIBUTING.md](CONTRIBUTING.md) 手册开始。

## 社区

活跃的社区途径：

- Slack: [OpenKruise channel](https://kubernetes.slack.com/channels/openkruise) (*English*)
- 钉钉：搜索群ID `23330762` (*Chinese*)
- 微信：添加用户 `openkruise` 并让机器人拉你入群 (*Chinese*)
- 社区双周会 (APAC, *Chinese*):
  - 周四 19:30 GMT+8 (Asia/Shanghai)
  - 进入会议(钉钉): 搜索群ID `23330762`
  - [会议纪要](https://shimo.im/docs/gXqmeQOYBehZ4vqo)
- Bi-weekly Community Meeting (*English*): TODO
  - [进入会议(zoom)](https://us02web.zoom.us/j/87059136652?pwd=NlI4UThFWXVRZkxIU0dtR1NINncrQT09)

## 安全

汇报安全漏洞请通过邮箱kubernetes-security@service.aliyun.com, 更多安全细节并参见[SECURITY.md](SECURITY.md)

## License

Kruise is licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE.md) for the full license text.
