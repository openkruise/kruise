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

## Features

You can view the full documentation from the [official website](https://openkruise.io/docs/).

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
