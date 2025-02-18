# Kruise

[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openkruise/kruise)](https://goreportcard.com/report/github.com/openkruise/kruise)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2908/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2908)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/openkruise/kruise/badge)](https://scorecard.dev/viewer/?uri=github.com/openkruise/kruise)
[![CircleCI](https://circleci.com/gh/openkruise/kruise.svg?style=svg)](https://circleci.com/gh/openkruise/kruise)
[![codecov](https://codecov.io/gh/openkruise/kruise/branch/master/graph/badge.svg)](https://codecov.io/gh/openkruise/kruise)
[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-v2.0%20adopted-ff69b4.svg)](./CODE_OF_CONDUCT.md)
[![Gurubase](https://img.shields.io/badge/Gurubase-Ask%20Kruise%20Guru-006BFF)](https://gurubase.io/g/kruise)

English | [简体中文](./README-zh_CN.md)

## Introduction

OpenKruise  (official site: [https://openkruise.io](https://openkruise.io)) is a CNCF([Cloud Native Computing Foundation](https://cncf.io/)) incubating project.
It consists of several controllers which extend and complement the [Kubernetes core controllers](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) for workload and application management.

## Key Features

- **Advance Workloads**

  Advance Workloads can help you manage applications of stateless, stateful, daemon and job.

  They all support not only the basic features which are similar to the original Workloads in Kubernetes, but also more advanced abilities like **in-place update**, **configurable scale/upgrade strategies**, **parallel operations**.

  - [**CloneSet** for stateless applications](https://openkruise.io/docs/user-manuals/cloneset/)
  - [**Advanced StatefulSet** for stateful applications](https://openkruise.io/docs/user-manuals/advancedstatefulset)
  - [**Advanced DaemonSet** for daemon applications](https://openkruise.io/docs/user-manuals/advanceddaemonset)
  - [**BroadcastJob** for deploying jobs over specific nodes](https://openkruise.io/docs/user-manuals/broadcastjob)
  - [**AdvancedCronJob** for creating Job or BroadcastJob periodically](https://openkruise.io/docs/user-manuals/advancedcronjob)

- **Sidecar container Management**

  Kruise simplify sidecar injection and enable sidecar in-place update. Kruise also enhance the sidecar startup and termination control.

  - [**SidecarSet** for defining and upgrading your own sidecars](https://openkruise.io/docs/user-manuals/sidecarset)
  - [**Container Launch Priority** to control the container startup orders](https://openkruise.io/docs/user-manuals/containerlaunchpriority)
  - [**Sidecar Job Terminator** terminates sidecar containers for such job-type Pods when its main containers completed.](https://openkruise.io/docs/user-manuals/jobsidecarterminator)

- **Multi-domain Management**

  This can help you manage applications over nodes with multiple domains,
  such as different node pools, available zones, architectures(x86 & arm) or node types(kubelet & virtual kubelet).

  Here we provide two different ways:

  - [**WorkloadSpread** for bypass distributing pods in workloads](https://openkruise.io/docs/user-manuals/workloadspread)
  - [**UnitedDeployment**, a new workload to manage multiple sub-workloads](https://openkruise.io/docs/user-manuals/uniteddeployment)

- **Enhanced Operations**

  - [**ContainerRecreateRequest** provides a way to let users restart/recreate containers in a running pod](https://openkruise.io/docs/user-manuals/containerrecreaterequest)
  - [**ImagePullJob** pre-download images on specific nodes](https://openkruise.io/docs/user-manuals/imagepulljob)
  - [**ResourceDistribution** support Secret & ConfigMap resource distribution across namespaces](https://openkruise.io/docs/user-manuals/resourcedistribution)
  - [**PersistentPodState** is able to persistent states of the Pod, such as "IP Retention"](https://openkruise.io/docs/user-manuals/persistentpodstate)
  - [**PodProbeMarker** provides the ability to customize the Probe and return the result to the Pod](https://openkruise.io/docs/user-manuals/podprobemarker)

- **Application Protection**

  - [Protect Kubernetes resources and applications' availability from the cascading deletion](https://openkruise.io/docs/user-manuals/deletionprotection)
  - [**PodUnavailableBudget** for achieving the effect of preventing application disruption or SLA degradation](https://openkruise.io/docs/user-manuals/podunavailablebudget)

## Quick Start

You can view the full documentation from the [OpenKruise website](https://openkruise.io/docs/).

- Install or upgrade Kruise with [the stable version](https://openkruise.io/docs/installation).
- Install or upgrade Kruise with [the latest version including alpha/beta/rc](https://openkruise.io/docs/next/installation).

### Get Your Own Demo with Alibaba Cloud

- install Kruise on a Serverless K8S cluster in 3 minutes, try:

  <a href="https://acs.console.aliyun.com/quick-deploy?repo=openkruise/charts&branch=master&paths=%5B%22versions/kruise/1.7.3%22%5D" target="_blank">
    <img src="https://img.alicdn.com/imgextra/i1/O1CN01aiPSuA1Wiz7wkgF5u_!!6000000002823-55-tps-399-70.svg" width="200" alt="Deploy on Alibaba Cloud">
  </a>

## Users

Registration: [Who is using Kruise](https://github.com/openkruise/kruise/issues/289)

- Alibaba Group, Ant Group, DouyuTV, Sto, Boss直聘
- hangyinxiaofei, vanyitech, Dmall, Bringg, 佐疆科技
- Lyft, Ctrip, 享住智慧, VIPKID, zhangmen
- xiaohongshu, bixin, 永辉科技中心, 跟谁学, 哈啰出行
- Spectro Cloud, ihomefnt, Arkane Systems, Deepexi, 火花思维
- OPPO, Suning.cn, joyy, Mobvista, 深圳凤凰木网络有限公司
- xiaomi, Netease, MeiTuan Finance, Shopee, Esign
- LinkedIn, 雪球, 兴盛优选, Wholee, LilithGames, Baidu
- Bilibili, 冠赢互娱, MeiTuan, 同城

## Contributing

You are warmly welcome to hack on Kruise. We have prepared a detailed guide [CONTRIBUTING.md](CONTRIBUTING.md).

## Community

Active communication channels:

- Slack: [OpenKruise channel](https://kubernetes.slack.com/channels/openkruise) (*English*)
- DingTalk：Search GroupID `23330762` (*Chinese*)
- WeChat: Search User `openkruise` and let the robot invite you (*Chinese*)
- Bi-weekly Community Meeting (APAC, *Chinese*):
  - Thursday 19:30 GMT+8 (Asia/Shanghai), [Calendar](https://calendar.google.com/calendar/u/2?cid=MjdtbDZucXA2bjVpNTFyYTNpazV2dW8ybHNAZ3JvdXAuY2FsZW5kYXIuZ29vZ2xlLmNvbQ)
  - Join Meeting(DingTalk): Search GroupID `23330762` (*Chinese*)
  - [Notes and agenda](https://shimo.im/docs/gXqmeQOYBehZ4vqo)
- Bi-weekly Community Meeting (*English*): TODO
  - [Meeting Link(zoom)](https://us02web.zoom.us/j/87059136652?pwd=NlI4UThFWXVRZkxIU0dtR1NINncrQT09)

## Security
Please report vulnerabilities by email to kubernetes-security@service.aliyun.com. Also see our [SECURITY.md](SECURITY.md) file for details.

## License

Kruise is licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE.md) for the full license text.
