---
title: CloneSet Support Progress Deadline Seconds
authors:
- "@hantmac"
reviewers:
- "@Fei-Guo"
- "@furykerry"
- "@FillZpp"
creation-date: 2024-03-10
last-updated: 2024-05-15
status: implemented
---

# Support progressDeadlineSeconds in CloneSet

## Table of Contents
- [Support progressDeadlineSeconds in CloneSet](#support-progressdeadlineseconds-in-cloneset)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Proposal](#proposal)
    - [1. Add .spec.progressDeadlineSeconds field](#1-add-specprogressdeadlineseconds-field)
    - [2. The behavior of progressDeadlineSeconds](#2-the-behavior-of-progressdeadlineseconds)
    - [3. Implementation details](#3-implementation-details)
  - [Usage Examples](#usage-examples)
  - [Implementation Status](#implementation-status)

## Motivation

`.spec.progressDeadlineSeconds` is an optional field in Deployment that specifies the number of seconds one wants to wait for their Deployment to progress before the system reports back that the Deployment has failed progressing. Once the deadline has been exceeded, the Deployment controller adds a DeploymentCondition with the following attributes to the Deployment's `.status.conditions`:

```
type: Progressing
status: "False"
reason: ProgressDeadlineExceeded
```

This is useful for users to control the progress of the deployment. So we should add support for progressDeadlineSeconds in CloneSet as well.

## Proposal

Firstly, add the progressDeadlineSeconds field to the CloneSetSpec. Then add the handle logic in cloneSet controller to handle the progressDeadlineSeconds field.

### 1. Add .spec.progressDeadlineSeconds field

The default value of progressDeadlineSeconds is 600 seconds according to the official document.

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
metadata:
  name: cloneset-example
spec:
  replicas: 3
  progressDeadlineSeconds: 600
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
```

### 2. The behavior of progressDeadlineSeconds

However, the behavior of progressDeadlineSeconds in CloneSet differs from its behavior in Deployment due to the support of partition in CloneSet. If a CloneSet is paused due to partition, it's important to determine whether the paused time should be included in the progress deadline.

After discussion with the community, we decided to implement the following behavior:

1. progressDeadlineSeconds is redefined as the time taken for the CloneSet to reach completion or the "paused" state due to partition.
2. The timer resets when the CloneSet reaches completion OR the "paused state".
3. The timer also resets when the CloneSet is explicitly paused using `.spec.paused: true`.

This allows CloneSet to support advanced update strategies with partitions while still providing progress monitoring capabilities.

### 3. Implementation details

The implementation adds the following components:

1. A new field `progressDeadlineSeconds` in CloneSetSpec
2. New condition constants:
   ```go
   // CloneSetConditionTypeProgressing indicates the status of CloneSet progress
   CloneSetConditionTypeProgressing CloneSetConditionType = "Progressing"
   
   // CloneSetProgressDeadlineExceeded indicates the CloneSet progress exceeded the deadline
   CloneSetProgressDeadlineExceeded CloneSetConditionReason = "ProgressDeadlineExceeded"
   ```

3. Logic in the CloneSet controller to:
   - Initialize the Progressing condition for new CloneSets
   - Track progress and detect when the deadline is exceeded
   - Reset the timer when the CloneSet is paused or reaches desired state
   - Add/update the appropriate condition in the CloneSet status

The progress deadline is tracked using the `lastUpdateTime` in the Progressing condition. When a CloneSet update begins, this time is set, and if the CloneSet doesn't reach the desired state within the specified deadline, the condition is updated to indicate failure.

## Usage Examples

**Example 1: Basic Usage**

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
metadata:
  name: nginx-cloneset
spec:
  replicas: 5
  progressDeadlineSeconds: 300  # 5 minutes deadline
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
```

**Example 2: With Partition**

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: CloneSet
metadata:
  name: nginx-cloneset-with-partition
spec:
  replicas: 10
  progressDeadlineSeconds: 600
  updateStrategy:
    partition: 5  # Only update 5 pods
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
```

In this example, the progress deadline will be considered met when 5 pods are updated (due to the partition), and the condition will remain "True" even though only half of the pods are on the new version.

**Checking Progress Status**

You can check the status using:

```bash
kubectl get cloneset nginx-cloneset -o=jsonpath='{.status.conditions[?(@.type=="Progressing")]}' | jq
```

Output for a successful update:
```json
{
  "lastTransitionTime": "2024-05-15T10:32:45Z",
  "lastUpdateTime": "2024-05-15T10:35:12Z",
  "message": "CloneSet is progressing.",
  "status": "True",
  "type": "Progressing"
}
```

Output for a failed update:
```json
{
  "lastTransitionTime": "2024-05-15T10:42:45Z",
  "lastUpdateTime": "2024-05-15T10:42:45Z",
  "message": "CloneSet progress deadline exceeded.",
  "reason": "ProgressDeadlineExceeded",
  "status": "False",
  "type": "Progressing"
}
```

## Implementation Status

- [x] Design approval
- [x] API changes
- [x] Implementation in controller
- [x] Unit tests
- [x] Documentation
- [x] Feature complete