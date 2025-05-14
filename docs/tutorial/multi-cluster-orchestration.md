# Using OpenKruise Workloads in Multi-Cluster Orchestration Systems

This document provides best practices and guidelines for using OpenKruise workloads with popular multi-cluster orchestration systems, specifically Karmada and Open Cluster Management (OCM).

## Table of Contents

- [Introduction](#introduction)
- [OpenKruise Workloads Overview](#openkruise-workloads-overview)
- [Karmada Integration](#karmada-integration)
  - [Understanding Karmada Resource Interpreters](#understanding-karmada-resource-interpreters)
  - [CloneSet in Karmada](#cloneset-in-karmada)
  - [Advanced StatefulSet in Karmada](#advanced-statefulset-in-karmada)
  - [Other OpenKruise Workloads](#other-openkruise-workloads)
- [Open Cluster Management (OCM) Integration](#open-cluster-management-ocm-integration)
  - [ManifestWork for OpenKruise Workloads](#manifestwork-for-openkruise-workloads)
  - [Best Practices for OCM Integration](#best-practices-for-ocm-integration)
- [Common Challenges and Solutions](#common-challenges-and-solutions)
- [References](#references)

## Introduction

[OpenKruise](https://openkruise.io) extends Kubernetes with enhanced workload management capabilities. When deploying OpenKruise workloads in multi-cluster environments, Karmada and Open Cluster Management (OCM) are popular orchestration frameworks that can be used.

This guide outlines best practices for integrating OpenKruise workloads like CloneSet, Advanced StatefulSet, and others with these multi-cluster orchestration systems.

## OpenKruise Workloads Overview

Before diving into multi-cluster integration, it's important to understand the key OpenKruise workloads:

- **CloneSet**: Enhanced deployment workload with in-place update capabilities
- **Advanced StatefulSet**: Extended version of Kubernetes StatefulSet with additional features
- **SidecarSet**: For managing sidecar containers across multiple pods
- **UnitedDeployment**: Manages multiple workloads across different domains

## Karmada Integration

[Karmada](https://karmada.io) (Kubernetes Armada) is a multi-cluster orchestration system that enables unified management of multiple Kubernetes clusters.

### Understanding Karmada Resource Interpreters

Karmada uses Resource Interpreters to understand how to handle custom resources from different providers. For OpenKruise workloads, you need to define appropriate interpreters to ensure Karmada can properly manage these resources.

Resource Interpreters help Karmada understand:
- How to retain or reset certain fields during propagation
- How to get replicas and scale workloads
- How to aggregate resource status from member clusters

### CloneSet in Karmada

To use CloneSet with Karmada, you need to create a proper ResourceInterpreter for the CloneSet resource:

```yaml
apiVersion: config.karmada.io/v1alpha1
kind: ResourceInterpreterCustomization
metadata:
  name: openkruise-cloneset-interpreter
spec:
  target:
    apiVersion: apps.kruise.io/v1alpha1
    kind: CloneSet
  customizations:
    replicas:
      jsonPath: ".spec.replicas"
    scale:
      jsonPath: ".spec.replicas"
    statusReflection:
      jsonPath: ".status"
    retention:
      jsonPaths: 
      - ".spec.updateStrategy"
      - ".spec.scaleStrategy"
```

When creating a PropagationPolicy for CloneSet resources, consider:

```yaml
apiVersion: policy.karmada.io/v1alpha1
kind: PropagationPolicy
metadata:
  name: cloneset-propagation
spec:
  resourceSelectors:
    - apiVersion: apps.kruise.io/v1alpha1
      kind: CloneSet
  placement:
    clusterAffinity:
      clusterNames:
        - member1
        - member2
    replicaScheduling:
      replicaSchedulingType: Divided
```

### Advanced StatefulSet in Karmada

For Advanced StatefulSet, create a ResourceInterpreter similar to CloneSet but tailored to StatefulSet specifics:

```yaml
apiVersion: config.karmada.io/v1alpha1
kind: ResourceInterpreterCustomization
metadata:
  name: openkruise-statefulset-interpreter
spec:
  target:
    apiVersion: apps.kruise.io/v1beta1
    kind: StatefulSet
  customizations:
    replicas:
      jsonPath: ".spec.replicas"
    scale:
      jsonPath: ".spec.replicas"
    statusReflection:
      jsonPath: ".status"
    retention:
      jsonPaths: 
      - ".spec.updateStrategy"
      - ".spec.podManagementPolicy"
```

### Other OpenKruise Workloads

For other OpenKruise workloads like SidecarSet and UnitedDeployment, you'll need to create custom interpreters based on their specific fields. For example, UnitedDeployment manages multiple workloads across different domains, so its interpreter should reflect this complexity.

## Open Cluster Management (OCM) Integration

[Open Cluster Management](https://open-cluster-management.io) (OCM) is another framework for managing multiple Kubernetes clusters, using the hub-spoke model.

### ManifestWork for OpenKruise Workloads

OCM uses ManifestWork resources to deploy workloads to managed clusters. Here's how to use ManifestWork with OpenKruise resources:

```yaml
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: example-cloneset
  namespace: cluster1  # This should be the managed cluster's namespace
spec:
  workload:
    manifests:
    - apiVersion: apps.kruise.io/v1alpha1
      kind: CloneSet
      metadata:
        name: example-cloneset
        namespace: default
      spec:
        replicas: 3
        selector:
          matchLabels:
            app: example
        template:
          metadata:
            labels:
              app: example
          spec:
            containers:
            - name: nginx
              image: nginx:latest
```

### Best Practices for OCM Integration

When using OpenKruise workloads with OCM:

1. **Pre-install OpenKruise on managed clusters**: Ensure OpenKruise is installed on all managed clusters before deploying OpenKruise CRDs through ManifestWork.

2. **Use ManifestWork status tracking**: Leverage OCM's ManifestWork status tracking to monitor the deployment status:

```yaml
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: example-cloneset
  namespace: cluster1
spec:
  workload:
    manifests:
    - apiVersion: apps.kruise.io/v1alpha1
      kind: CloneSet
      # ... CloneSet spec ...
  manifestConfigs:
  - resourceIdentifier:
      group: apps.kruise.io
      resource: clonesets
      name: example-cloneset
      namespace: default
    feedbackRules:
    - type: WellKnownStatus
```

3. **Configure ApplicationSet with OCM**: If using ArgoCD alongside OCM, configure ApplicationSet generators to target OCM-managed clusters.

## Common Challenges and Solutions

### Challenge 1: OpenKruise CRD Distribution

**Problem**: OpenKruise CRDs must be installed on all clusters before workloads can be deployed.

**Solution**: Use a ManifestWork or Karmada Policy to ensure OpenKruise operator and CRDs are deployed to all clusters first. Consider using ClusterManagementAddons in OCM.

### Challenge 2: Version Compatibility

**Problem**: Different OpenKruise versions across clusters can cause compatibility issues.

**Solution**: Standardize OpenKruise versions across all clusters or use version-specific manifests.

### Challenge 3: Workload-Specific Features

**Problem**: Some OpenKruise workloads have unique features not easily translated to standard kubernetes concepts.

**Solution**: Create custom resource interpreters in Karmada or use OCM ManifestWork feedback rules to properly handle these unique features.

## References

- [OpenKruise Documentation](https://openkruise.io/docs/)
- [Karmada Resource Interpreter](https://karmada.io/docs/userguide/globalview/customizing-resource-interpreter/)
- [OCM ManifestWork](https://open-cluster-management.io/docs/concepts/work-distribution/manifestwork/)
- [KubeCon Talk on Multi-Cluster Management](https://www.youtube.com/watch?v=gcllTXRkz-E)