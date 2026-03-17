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

OpenKruise (official site: https://openkruise.io) is a CNCF (Cloud Native Computing Foundation) incubating project.  
It consists of several controllers that extend and complement Kubernetes core controllers for workload and application management.

---

## Key Features

- **Advanced Workloads**

  Advanced Workloads help manage stateless, stateful, daemon, and batch applications.

  They support not only basic Kubernetes workload features but also advanced capabilities such as:
  - In-place updates
  - Configurable scaling and upgrade strategies
  - Parallel operations

  - [**CloneSet** for stateless applications](https://openkruise.io/docs/user-manuals/cloneset/)
  - [**Advanced StatefulSet** for stateful applications](https://openkruise.io/docs/user-manuals/advancedstatefulset)
  - [**Advanced DaemonSet** for daemon applications](https://openkruise.io/docs/user-manuals/advanceddaemonset)
  - [**BroadcastJob** for distributed jobs](https://openkruise.io/docs/user-manuals/broadcastjob)
  - [**AdvancedCronJob** for scheduled jobs](https://openkruise.io/docs/user-manuals/advancedcronjob)

---

- **Sidecar Container Management**

  Kruise simplifies sidecar injection and enables in-place updates. It also improves startup and termination control.

  - [**SidecarSet** for managing sidecars](https://openkruise.io/docs/user-manuals/sidecarset)
  - [**Container Launch Priority** to control startup order](https://openkruise.io/docs/user-manuals/containerlaunchpriority)
  - [**Sidecar Job Terminator** for cleaning up sidecars](https://openkruise.io/docs/user-manuals/jobsidecarterminator)

---

- **Multi-domain Management**

  Manage applications across multiple domains such as:
  - Node pools
  - Availability zones
  - Architectures (x86 & ARM)
  - Node types (kubelet & virtual kubelet)

  - [**WorkloadSpread** for flexible pod distribution](https://openkruise.io/docs/user-manuals/workloadspread)
  - [**UnitedDeployment** for managing multiple sub-workloads](https://openkruise.io/docs/user-manuals/uniteddeployment)

---

- **Enhanced Operations**

  - [**ContainerRecreateRequest** to restart containers](https://openkruise.io/docs/user-manuals/containerrecreaterequest)
  - [**ImagePullJob** for pre-downloading images](https://openkruise.io/docs/user-manuals/imagepulljob)
  - [**ResourceDistribution** for sharing resources across namespaces](https://openkruise.io/docs/user-manuals/resourcedistribution)
  - [**PersistentPodState** for retaining pod states](https://openkruise.io/docs/user-manuals/persistentpodstate)
  - [**PodProbeMarker** for custom probe handling](https://openkruise.io/docs/user-manuals/podprobemarker)

---

- **Application Protection**

  - Prevent cascading deletion of resources
  - [**PodUnavailableBudget** to maintain application availability](https://openkruise.io/docs/user-manuals/podunavailablebudget)

---

## Quick Start

You can view full documentation at: https://openkruise.io/docs/

### Prerequisites

Before getting started, ensure you have:
- Kubernetes cluster (Minikube or Kind recommended)
- kubectl installed
- Docker installed

### Basic Setup Example

Deploy OpenKruise CRDs:

```bash
kubectl apply -f https://raw.githubusercontent.com/openkruise/kruise/master/config/crd/bases/apps.kruise.io_clonesets.yaml
````

### Installation

* Install stable version: [https://openkruise.io/docs/installation](https://openkruise.io/docs/installation)
* Install latest version: [https://openkruise.io/docs/next/installation](https://openkruise.io/docs/next/installation)

---

## Contributing

We welcome contributions! Please refer to [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

## Community

* Slack: OpenKruise channel (Kubernetes Slack)
* Community meetings and discussions available in docs

---

## License

Kruise is licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE.md) for the full license text.