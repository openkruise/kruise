---
title: AdvancedStatefulSetVolumeResize
authors:
  - "@Abner-1"
reviewers:
  - "@furykerry"
  - "@zmberg"
creation-date: 2024-06-26
last-updated: 2024-06-26
status: 
---

# Advanced StatefulSet Volume Resize

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

* [Table of Contents](#table-of-contents)
* [Motivation](#motivation)
    * [User Story](#user-story)
    * [Failure Recovery User Story](#failure-recovery-user-story)
    * [Fundamental Issues](#fundamental-issues)
    * [Goal](#goal)
    * [None Goal](#none-goal)
* [Proposal](#proposal)
    * [API Definition](#api-definition)
    * [Adding Webhook Validation](#adding-webhook-validation)
    * [Updating PVC Process](#updating-pvc-process)
    * [Handling In-place PVC Update Failures](#handling-in-place-pvc-update-failures)
    * [Reasons for Not Tracking Historical Versions of VolumeClaimTemplates per KEP-661](#reasons-for-not-tracking-historical-versions-of-volumeclaimtemplates-per-kep-661)
    * [Implementation](#implementation)

## Motivation

We have recently implemented a feature that triggers pod reconstruction when there are changes to
the `VolumeClaimTemplates` in `CloneSet`. This leads to the reconciliation of Persistent Volume Claims (PVCs) and the
implementation of reconfiguration. However, this approach may be too aggressive for stateful applications. Therefore, it
is essential to refine the update strategy for `AdvancedStatefulSet` in response to changes in `VolumeClaimTemplates`.

### User Story

The current behavior of `AdvancedStatefulSet` is to disregard changes in `VolumeClaimTemplates`, reconciling only for
the creation of new pods. Users may encounter situations such as:

1. **[H]** In cases where StorageClasses support expansion, users can directly edit the PVC's storage capacity to
   increase it (decreasing is not supported).
2. For StorageClasses that do not support expansion, users must ensure that the contents of existing PVCs are no longer
   needed before manually or automatically deleting the PVC and associated pods. New PVCs and pods will then be
   reconciled with the latest configuration. (This scenario requires refinement as it is theoretically necessary.)
    - In some use cases, disk fragmentation may occur over time, prompting users to recreate resources to enhance
      performance. (Would deleting and recreating the StatefulSet be beneficial here?)
3. For scenarios requiring a change in StorageClass, the process is similar to scenario 2:
    - Examples include transitioning from SSD to ESSD or migrating to cloud storage.

### Failure Recovery User Story

In scenario 1, unexpected issues may arise, such as a failure in modifying `VolumeClaimTemplates` for a specific pod.
Users may expect the following recovery options:

1. Complete rollback to the previous configuration, for instance, when both the image and volume specifications are
   altered, and the new image is defective, users desire a full rollback (not covered by KEP-661).
2. **[H]** Partial rollback to the previous configuration, for example, when both the image and volume specifications
   are changed, but users wish to revert to the old image while retaining the new volume specifications (addressed by
   KEP-661).
3. Configuration should not be rolled back, but the issue with the failed pod should be resolved:
    - When reconfiguration is expected but the storage quota on a specific node is insufficient. After consultation with
      the administrator, the quota is increased.
    - When reconfiguration is anticipated but the underlying resources on a specific node are inadequate. The pod should
      be relocated to a node with sufficient resources.
4. Reconfigure the settings again, such as adjusting volume specifications from 10G to 100G, but due to storage quota
   constraints, change to 10G to 20G. (Assuming an update of 10 instances fails at the 5th instance) The first 4 pods
   need to revert from 100G to 20G, and the next 6 need to adjust from 10G to 20G (as per KEP-1790).

### Fundamental Issues

1. How can we identify situations where patching pvc is impractical? (KEP-661)
2. Can we develop a mechanism to automatically and safely delete PVCs, thereby streamlining the PVC deletion process? (
   KEP-4650 focuses more on delegating error handling to higher-level users or platforms)
    - If such a mechanism exists, how does it differ from manual PVC deletion?
        - `delete PVC` is an API interface that requires specific permissions.
        - This mechanism could involve a component that identifies and marks PVC or pod storage states, acting as a
          real-time monitoring tool.
        - The `StatefulSet` controller could then make actual deletion decisions based on concurrency and progress
          tuning.
3. What kind of PVC migration mechanism do users truly require? A flexible approach as proposed in KEP-4650 or a more
   standardized yet broadly applicable solution?

### Goal

1. **Automated Expansion**: Enable automated expansion of `VolumeClaimTemplates` specifications, provided the
   StorageClass supports capacity expansion.
2. **Completion and Error Awareness**: Ensure users can know whether the PVC expansion is complete or if any errors have
   occurred.
3. **Unobstructed Recovery Attempts**: Do not obstruct users from attempting recovery in abnormal situations.
4. **Recovery Expectation in Certain Clusters**: In clusters where the `RecoverVolumeExpansionFailure` feature gate is
   enabled, allow users to achieve recovery expectation 4.
5. **Better Handling for Non-Supportive StorageClasses**: When the StorageClass does not support capacity expansion and
   the user has ensured that existing disk data does not need to be preserved, allow automatic deletion and recreation
   of PVCs by adjusting `VolumeClaimTemplates` configuration and explicitly specifying the update method.
    - Since users are aware of this, they can theoretically wait for manual deletion and reconstruction (OnDelete).
    - Q: How to distinguish between reconstructions due to updates and those due to evictions in abnormal scenarios? A:
      Refer to CloneSet handling in delete pod

### None Goal

1. **Do Not Implement KEP-1790**.
2. **No Version Management**: Do not implement version management and tracking for volume claims.

## Proposal

### API Definition

1. **Introduction of `volumeClaimUpdateStrategy` in StatefulSet `spec`**. The possible values include:
   - `OnDelete`: The default value. PVCs are updated only when the old PVC is deleted.
   - `InPlace`: Updates the PVC in place, encompassing the behavior of `OnDelete`.
   - `ReCreate`: This may integrate the current behavior of PVCs in `CloneSet`.

2. **Introduction of `volumeClaimSyncStrategy` in StatefulSet `spec.updateStrategy.rollingUpdate`**. The possible values include:
   - `Async`: The default value. Maintains the current behavior and is only configurable when `volumeClaimUpdateStrategy` is set to `OnDelete`.
   - `LockStep`: PVCs are updated first, followed by the Pods. Further details are provided below.

3. **Introduction of `volumeClaimTemplates` in StatefulSet `status`**:
   - **`status.volumeClaimTemplates[x].templateName`**: Displays the name of the `spec.volumeClaimTemplates` PVC (template) being tracked. This name must be unique within `volumeClaimTemplates`, facilitating easier troubleshooting for users.
      - **Note**: IndexId is not used because it's less intuitive, and since `volumeClaimTemplates` allows for deletion and reduction, tracking with IndexId could be complex.
   - **`status.volumeClaimTemplates[x].compatibleReplicas`**: Indicates the number of replicas for the current `volumeClaimTemplate` that have been compatible or updated.
   - **`status.volumeClaimTemplates[x].compatibleReadyReplicas`**: Indicates the number of replicas for the current `volumeClaimTemplate` that have been compatible or successfully updated.
   - **`status.volumeClaimTemplates[x].finishedReconciliationGeneration`**: Shows the generation number when the PVC reconciliation was completed. For example, for the two `volumeClaimTemplates` mentioned:
      - The first one, because not all replicas are ready, remains at generation 2.
      - The second one, with all replicas ready, is synchronized with the current generation of the `StatefulSet`.

```yaml
spec:
  volumeClaimUpdateStrategy: OnDelete
    # OnDelete：Default value. PVCs are updated only when the old PVC is deleted.
    # InPlace：Updates the PVC in place. Also includes the behavior of OnDelete.
    # ReCreate (May integrate the current behavior of PVCs in CloneSet)

  updateStrategy:
    rollingUpdate:
      volumeClaimSyncStrategy: Async
        # Async：Default value. Maintains current behavior. Only configurable when volumeClaimUpdateStrategy is set to OnDelete.
      # LockStep：PVCs are updated first, followed by Pods. More details provided below.

status:
  availableReplicas: 3
  collisionCount: 0
  currentReplicas: 3
  currentRevision: ex1-54c5bd476c
  observedGeneration: 3
  readyReplicas: 3
  replicas: 3
  updateRevision: ex1-54c5bd476c
  updatedReplicas: 3
  volumeClaimTemplates: # new field
    - finishedReconciliationGeneration: 2
      updatedReadyReplicas: 1  # resize 状态成功的副本数
      updatedReplicas: 2        # 下发 resize 的副本数
      templateName: vol1
      # 当 updatedReadyReplicas == spec.replicas时，
      # 调整 finishedReconciliationGeneration 为 sts 的 generation
    - finishedReconciliationGeneration: 3
      updatedReadyReplicas: 3
      updatedReplicas: 3
      templateName: vol2
```

> Q: Why do we need to introduction these two fields in spec?
>
>A: The modification of VolumeClaimTemplates in Advanced StatefulSet has always been overlooked. If we completely follow
> KEP-661, it could reduce API flexibility and make some current user scenarios unusable. To achieve forward
> compatibility, there are two approaches: "adding these two fields to increase the flexibility of VolumeClaimTemplates
> tuning" and "using a feature gate for global management of webhook interception and tuning steps".
>
> Tentatively, the approach of "adding these two fields to increase the flexibility of VolumeClaimTemplates tuning" is
> selected.

### Adding Webhook Validation

Webhook validation can be implemented using the `allowVolumeExpansion` field in the `StorageClass` to ascertain whether PVC expansion is supported. However, this field may not always reflect the true capabilities of the CSI.

1. **Update Strategy set to InPlace**:
    - If the associated `StorageClass` permits expansion and the `VolumeClaimTemplates` are unchanged or only increase in size, the update is permitted. In other cases, it is denied.
        - **Question**: What happens if we switch to `OnDelete`, alter the `VolumeClaimTemplates`, and then revert to `InPlace`?
        - **Answer**: The rolling update process may become stuck, initiating an abnormal update sequence.

2. **Update Strategy set to OnDelete**:
    - Updates are directly permitted without additional checks.

### Updating PVC Process

1. **Monitor PVC Changes**:
    - Changes to PVCs are monitored using annotations instead of owner references, which are already utilized for automatic PVC deletion. This method records associated `StatefulSet` objects.

2. **Update Status**:
    - Introduce validation and version management for PVCs in the update status to ensure accurate tracking.

3. **Update PVC before Pod in Rolling Update**:
    - During an expansion, the PVC should be updated first.
    - Implement strategies to manage rollback scenarios for expansions:
        - If KEP-1790 is not enabled, prevent the process from getting stuck.
        - If KEP-1790 is enabled, allow for resizing to proceed.

### Handling In-place PVC Update Failures

In theory, after an `InPlace + LockStep` failure, user intervention is typically necessary, often involving the creation of a new PVC and Pod.

The ideal response would be to revert to the `OnDelete + LockStep` process. For example, using a three-replica scenario where `PVC2` fails to update in place:

1. Delete `Pod2` and simultaneously apply a label to `PVC2`.
2. Detect the label on `PVC2` to prevent the creation of a new `Pod2`.
3. Await `PVC2` compatibility and label removal (which can be automated once compatibility is confirmed).
    - **User intervention scenarios**:
        - If the data on `PVC2` is expendable: Delete `PVC2`.
        - If backup is required: create a job mounting `PVC2` to perform necessary operations. After completion, delete `PVC2`.
4. If `Pod0` is deleted in the meantime, it should be recreated without being hindered by `PVC0` incompatibility.
5. Once compatibility is restored, recreate `Pod2`. After `Pod2` is ready, proceed to update the subsequent replicas.

### Reasons for Not Tracking Historical Versions of `VolumeClaimTemplates` per KEP-661

Currently, `AdvancedStatefulSet/CloneSet` do not track historical `VolumeClaimTemplates` in controller revisions, focusing on current values. This approach is maintained for the following reasons:

1. **Impact on Controller Revision**:
    - Incorporating `VolumeClaimTemplates` in controller revisions would trigger version changes in `AdvancedStatefulSet` even with mere modifications to `VolumeClaimTemplates`. This could significantly disrupt existing controller processes, necessitating extensive modifications and posing high risks.

2. **Handling Rollbacks via Upper Layer**:
    - Rollback operations can typically be managed by reapplying configurations from the upper layer. In most scenarios, reverting PVC configurations (or the lack of urgency for rollbacks) is deemed sufficient.
        - The demand for expanding PVCs is more pressing. Future evolution can be considered based on necessity.

3. **Historical Version Tracking and PVC Deletion**:
    - With historical version tracking, an un-updated PVC would revert to an older version (instead of the latest) upon deletion.
        - **Data Integrity**: The restoration of PVC data is still not guaranteed. If a user deletes a PVC, is the intention to reinstate an older version configuration? The distinction appears negligible.

In the absence of specific user scenarios, the core advantage of reverting to historical version configurations over the latest ones is unclear, particularly in cases of data loss for persistent storage.

### Implementation
Main modification is in the `rollingUpdateStatefulsetPods` Function.
![asts-resize-pvc](../img/asts-resize-pvc.png)

