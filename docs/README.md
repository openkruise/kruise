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

- [CloneSet](https://openkruise.io/en-us/docs/cloneset.html): CloneSet is a workload that mainly focuses on managing stateless applications. It provides a rich set of features for more efficient, deterministic and controlled management, such as in-place update, specified Pod deletion, configurable priority/scatter based update, preUpdate/postUpdate hooks, etc. This [post](https://thenewstack.io/introducing-cloneset-production-grade-kubernetes-deployment-crd/) provides more details about why CloneSet is useful.
- [Advanced StatefulSet](https://openkruise.io/en-us/docs/advanced_statefulset.html): An enhanced version of default [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) with extra functionalities such as `in-place update`, `pause` and `maxUnavailable`.
- [SidecarSet](https://openkruise.io/en-us/docs/sidecarset.html): A controller that injects sidecar containers into the Pod spec based on the Pod selectors. The controller is also responsible for upgrading the sidecar containers.
- [UnitedDeployment](https://openkruise.io/en-us/docs/uniteddeployment.html): This controller manages application Pods spread in multiple fault domains by using multiple workloads.
- [BroadcastJob](https://openkruise.io/en-us/docs/broadcastjob.html): A job that runs Pods to completion across all the nodes in the cluster.
- [Advanced DaemonSet](https://openkruise.io/en-us/docs/advanced_daemonset.html): An enhanced version of default DaemonSet with extra functionalities such as partition, node selector, pause and surging.

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
