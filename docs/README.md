# Overview

Kubernetes provides a set of default controllers for workload management,
like StatefulSet, Deployment, DaemonSet for instances. While at the same time, managed applications
express more and more diverse requirements for workload upgrade and deployment, which
in many cases, cannot be satisfied by the default workload controllers.

Kruise attempts to fill such gap by offering a set of controllers as the supplement
to manage new workloads in Kubernetes. The target use cases are representative,
originally collected from the users of Alibaba cloud container services and the
developers of the in-house large scale on-line/off-line container applications.
Most of the use cases can be easily applied to other similar cloud user scenarios.

Currently, Kruise supports the following workloads.

## Workloads

- [Advanced StatefulSet](./concepts/astatefulset/README.md): An enhanced version of default [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) with extra functionalities such as `inplace-update`, `pasue` and `MaxUnavailable`.
- [BroadcastJob](./concepts/broadcastJob/README.md): A job that runs pods to completion across all the nodes in the cluster.
- [SidecarSet](./concepts/sidecarSet/README.md): A controller that injects sidecar containers into the Pod spec based on selectors and also be able to upgrade the sidecar containers.
- [UnitedDeployment](./concepts/uniteddeployment/README.md): This controller manages application pods spread in multiple fault domains by using multiple workloads.
- [CloneSet](./concepts/cloneset/README.md): CloneSet is a workload that mainly focuses on managing stateless applications. It provides full features for more efficient, deterministic and controlled deployment, such as inplace update, specified pod deletion, configurable priority/scatter update, preUpdate/postUpdate hooks.

## Benefits

- In addition to serving new workloads, Kruise also offers extensions to default
  controllers for new capabilities. Kruise owners will be responsible to port
  any change to the default controller from upstream if it has an enhanced
  version inside (e.g., Advanced StatefulSet).

- Kruise provides controllers for representative cloud native applications
  with full Kubernetes API compatibility. Ideally, it can be the first option to
  consider when one wants to extend upstream Kubernetes for workload management.

- Kruise plans to offer more Kubernetes automation solutions in the
  areas of scaling, QoS and operators, etc. Stay tuned!

## Tutorials

Several [Tutorials](./tutorial/README.md) are provided to demonstrate how to use the controllers
