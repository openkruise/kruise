---
title: AdvancedStatefulSetVolumeResize
authors:
  - "@magicsong"
reviewers:
  - "@furykerry"
  - "@zmberg"
creation-date: 2024-10-29
last-updated: 2024-11-06
status: 

---
# ServiceAccount auto injection in SidecarSet
## Table of Contents

- [ServiceAccount auto injection in SidecarSet](#serviceaccount-auto-injection-in-sidecarset)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
    - [Notes/Constraints/Caveats](#notesconstraintscaveats)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Design Details](#design-details)
    - [API Changes](#api-changes)
    - [Implementation Strategy](#implementation-strategy)
  - [Graduation Criteria](#graduation-criteria)
  - [Drawbacks](#drawbacks)
  - [Alternatives](#alternatives)

---

## Summary

This KEP proposes the ability to inject a ServiceAccount into a SidecarSet in Kruise. This feature will allow sidecar containers managed by Kruise's SidecarSet to use specific ServiceAccounts, enabling better access control and security configurations.

## Motivation

Currently, sidecars injected by SidecarSet inherit the ServiceAccount of the main application container. In certain use cases, sidecars may require distinct permissions and should be able to specify their own ServiceAccounts. This proposal aims to provide a solution for injecting ServiceAccounts into SidecarSets, giving users finer control over sidecar permissions.

related issue: https://github.com/openkruise/kruise/issues/1747

### Goals

- Allow users to specify a ServiceAccount for sidecar containers managed by SidecarSet.
- Improve the security and flexibility of permission management for sidecars.

### Non-Goals

- Overhauling the existing ServiceAccount system in Kubernetes.
- Providing role-based or permission-specific configurations beyond the ServiceAccount level.

## Proposal

### User Stories

- **Story 1**: As a user, I want to assign a dedicated ServiceAccount to a logging sidecar injected by SidecarSet to limit its access to certain resources without affecting the main application container.
- **Story 2**: As a developer, I want SidecarSet to inject sidecars with different ServiceAccounts depending on the permissions required by each sidecar. A sidecar container needs to access Kubernetes resources such as ConfigMaps, Secrets, or interact with the API server for other reasons, it would require a ServiceAccount with appropriate permissions. Injecting the ServiceAccount directly through the SidecarSet simplifies this process and makes it more manageable.

### Notes/Constraints/Caveats

This feature must ensure compatibility with existing Kruise ServiceAccount management and avoid conflicts when updating ServiceAccounts.

### Risks and Mitigations

- **Risk**: Potential misconfiguration may lead to sidecars running with unintended permissions.
  - **Mitigation**: Add validation to ensure only permitted ServiceAccounts are specified.

## Design Details

### API Changes

1. Add a new `serviceAccountName` field in the SidecarSet specification, allowing users to define a ServiceAccount specifically for the sidecar containers.
2. Add a new `errorWhenServiceAccountInjectionConflict` field in the SidecarSet specification to control the behavior when multiple SidecarSets inject different ServiceAccounts into the same pod.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  name: sidecarset-sample
spec:
    serviceAccountName: "sidecar-service-account"
    errorWhenServiceAccountInjectionConflict: true
    containers:
    - name: sidecar
        image: sidecar-image
        ...
    - name: main-app
        image: main-app-image
        ...
```

### Implementation Strategy

1. **Validation**:
    - Ensure the `serviceAccountName` field is only processed if the relevant FeatureGate is enabled.
    - Validate that the specified ServiceAccount is not the default ServiceAccount.
    - Check if the service account is present
    - Check for conflicts if multiple SidecarSets attempt to inject ServiceAccounts into the same pod.

2. **Controller Modifications**:
    - Modify the Kruise SidecarSet controller to interpret the new `serviceAccountName` field.
    - Ensure the application container does not set its own ServiceAccount, or it is set to default.
    - Remove the automatically injected ServiceAccount mount by Kubernetes if present.

3. **Conflict Resolution**:
    - Implement logic to handle and resolve conflicts when multiple SidecarSets inject different ServiceAccounts into the same pod.


## Graduation Criteria

- Alpha: Basic functionality with the ability to inject ServiceAccount into sidecar containers.
- Beta: Stability improvements and expanded validation testing.
- Stable: Full user feedback incorporated, and feature ready for general use.

## Drawbacks
- There may be conflicts if multiple SidecarSets require ServiceAccount injection
- May require additional user education on configuring separate ServiceAccounts for sidecars.

## Alternatives

1. Use custom sidecar injection mechanisms to apply specific ServiceAccounts.
2. Manually configure ServiceAccount permissions outside of SidecarSet.

---