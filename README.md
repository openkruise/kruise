# Kruise

[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openkruise/kruise)](https://goreportcard.com/report/github.com/openkruise/kruise)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/2908/badge)](https://bestpractices.coreinfrastructure.org/en/projects/2908)
[![Build Status](https://travis-ci.org/openkruise/kruise.svg?branch=master)](https://travis-ci.org/openkruise/kruise)
[![CircleCI](https://circleci.com/gh/openkruise/kruise.svg?style=svg)](https://circleci.com/gh/openkruise/kruise)
[![codecov](https://codecov.io/gh/openkruise/kruise/branch/master/graph/badge.svg)](https://codecov.io/gh/openkruise/kruise)
[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-v2.0%20adopted-ff69b4.svg)](./CODE_OF_CONDUCT.md)

English | [简体中文](./README-zh_CN.md)

## Introduction

OpenKruise  (official site: [https://openkruise.io](https://openkruise.io)) is now hosted by the [Cloud Native Computing Foundation](https://cncf.io/) (CNCF) as a Sandbox Level Project.
It consists of several controllers which extend and complement the [Kubernetes core controllers](https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/) for workload and application management.

## Key Features

- **Typical Workloads**

  Typical Workloads can help you manage applications of stateless, stateful and daemon.

  They all support not only the basic features which are similar to the original Workloads in Kubernetes, but also more advanced abilities like **in-place update**, **configurable scale/upgrade strategies**, **parallel operations**.

  - [**CloneSet** for stateless applications](https://openkruise.io/docs/user-manuals/cloneset/)
  - [**Advanced StatefulSet** for stateful applications](https://openkruise.io/docs/user-manuals/advancedstatefulset)
  - [**Advanced DaemonSet** for daemon applications](https://openkruise.io/docs/user-manuals/advanceddaemonset)

- **Job Workloads**

  - [**BroadcastJob** for deploying jobs over specific nodes](https://openkruise.io/docs/user-manuals/broadcastjob)
  - [**AdvancedCronJob** for creating Job or BroadcastJob periodically](https://openkruise.io/docs/user-manuals/advancedcronjob)

- **Sidecar container Management**

  The Sidecar containers can be simply defined in the **SidecarSet** custom resource and Kruise will inject them into all Pods matched.

  The implementation is done by using Kubernetes mutating webhooks, similar to what [istio](https://istio.io/latest/docs/setup/additional-setup/sidecar-injection/) does.
  However, it allows you to explicitly manage your own sidecars.

  - [**SidecarSet** for defining and upgrading your own sidecars](https://openkruise.io/docs/user-manuals/sidecarset)

- **Multi-domain Management**

  This can help you manage applications over nodes with multiple domains,
  such as different node pools, available zones, architectures(x86 & arm) or node types(kubelet & virtual kubelet).

  Here we provide two different ways:

  - [**WorkloadSpread** for bypass distributing pods in workloads](https://openkruise.io/docs/user-manuals/workloadspread)
  - [**UnitedDeployment**, a new workload to manage multiple sub-workloads](https://openkruise.io/docs/user-manuals/uniteddeployment)

- **Enhanced Operations**

  - [Restart containers in a running pod](https://openkruise.io/docs/user-manuals/containerrecreaterequest)
  - [Download images on specific nodes](https://openkruise.io/docs/user-manuals/imagepulljob)

- **Application Protection**

  - [Protect Kubernetes resources and applications' availability from the cascading deletion](https://openkruise.io/docs/user-manuals/deletionprotection)
  - [**PodUnavailableBudget** for achieving the effect of preventing application disruption or SLA degradation](https://openkruise.io/docs/user-manuals/podunavailablebudget)

## Quick Start

You can view the full documentation from the [OpenKruise website](https://openkruise.io/docs/).

- Install or upgrade Kruise with [the stable version](https://openkruise.io/docs/installation).
- Install or upgrade Kruise with [the latest version including alpha/beta/rc](https://openkruise.io/docs/next/installation).

## Users

Registration: [Who is using Kruise](https://github.com/openkruise/kruise/issues/289)

- Alibaba Group, Ant Group, DouyuTV, Sto, Boss直聘
- hangyinxiaofei, vanyitech, Dmall, Bringg, 佐疆科技
- Lyft, Ctrip, 享住智慧, VIPKID, zhangmen
- xiaohongshu, bixin, 永辉科技中心, 跟谁学, 哈啰出行
- Spectro Cloud, ihomefnt, Arkane Systems, Deepexi, 火花思维
- OPPO, Suning.cn, joyy, Mobvista, 深圳凤凰木网络有限公司
- xiaomi, Netease, MeiTuan Finance, Shopee, Esign
- Wholee

## Contributing

You are warmly welcome to hack on Kruise. We have prepared a detailed guide [CONTRIBUTING.md](CONTRIBUTING.md).

## Community

Active communication channels:

- Slack: [OpenKruise channel](https://kubernetes.slack.com/channels/openkruise) (*English*)
- DingTalk：Search Group ID `23330762` (*Chinese*)
- Bi-weekly Community Meeting (APAC, *Chinese*):
  - Thursday 19:00 GMT+8 (Asia/Shanghai), [Calendar](https://calendar.google.com/calendar/u/2?cid=MjdtbDZucXA2bjVpNTFyYTNpazV2dW8ybHNAZ3JvdXAuY2FsZW5kYXIuZ29vZ2xlLmNvbQ)
  - [Meeting Link(zoom)](https://us02web.zoom.us/j/87059136652?pwd=NlI4UThFWXVRZkxIU0dtR1NINncrQT09)
  - [Notes and agenda](https://shimo.im/docs/gXqmeQOYBehZ4vqo)
- Bi-weekly Community Meeting (*English*): TODO

## License

Kruise is licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE.md) for the full license text.
