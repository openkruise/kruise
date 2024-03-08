---
title: CloneSet
authors:
- "@hantmac"
  reviewers:
- "@Fei-Guo"
- "@furykerry"
- "@FillZpp"
  creation-date: 2024-03-10
  last-updated: 2024-03-10
  status: implementable
---

# Support progressDeadlineSeconds in CloneSet
Table of Contents
=================

- [Support progressDeadlineSeconds in CloneSet](#Support progressDeadlineSeconds in CloneSet)
- [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Proposal](#proposal)
    - [1. add .spec.progressDeadlineSeconds field](#1add-.spec.progressDeadlineSeconds-field)
    - [2. The behavior of progressDeadlineSeconds](#2the-behavior-of-progressDeadlineSeconds)
    - [3. handle the logic](#2handle-the-logic)

## Motivation

`.spec.progressDeadlineSeconds` is an optional field in Deployment that specifies the number of seconds one wants to wait for their Deployment to progress before the system reports back that the Deployment has failed progressing.
Once the deadline has been exceeded, the Deployment controller adds a DeploymentCondition with the following attributes to the Deployment's `.status.conditions`:
```
type: Progressing
status: "False"
reason: ProgressDeadlineExceeded
```

This is useful for users to control the progress of the deployment.
So we should add support for `progressDeadlineSeconds` in CloneSet as well.

## Proposal
Firstly, add the `progressDeadlineSeconds` field to the CloneSetSpec.
Then add the handle logic in cloneSet controller to handle the `progressDeadlineSeconds` field.

### 1. add .spec.progressDeadlineSeconds field
The default value of `progressDeadlineSeconds` is 600 seconds according to the [official document](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#progress-deadline-seconds).
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
However, the behavior of `progressDeadlineSeconds` in CloneSet might differ from its behavior in Deployment due to the support of partition in CloneSet. If a CloneSet is paused due to partition, it's debatable whether the paused time should be included in the progress deadline.
Here are two possible interpretations of `progressDeadlineSeconds` in the context of CloneSet:
1. `progressDeadlineSeconds` could be redefined as the time taken for the CloneSet to reach completion or the "paused" state due to partition. In this case, the time during which the CloneSet is paused would NOT be included in the progress deadline.
2. Secondly, `progressDeadlineSeconds` could only be supported if the partition is not set. This means that if a partition is set, the `progressDeadlineSeconds` field would not be applicable or has no effect.

After the discussion of the community, we choose the first interpretation.So we should re-set the  progressDeadlineSeconds when the CloneSet reach completion OR "the paused state".

### 3. handle the logic
In cloneset controller, we should add the logic to handle the `progressDeadlineSeconds` field. We firstly add a timer to check the progress of the CloneSet. 
If the progress exceeds the `progressDeadlineSeconds`, we should add a CloneSetCondition to the CloneSet's `.status.conditions`: 
```go
// add a timer to check the progress of the CloneSet
if cloneSet.Spec.ProgressDeadlineSeconds != nil {
    // handle the logic
	starttime := time.Now()
	...
    if time.Now().After(starttime.Add(time.Duration(*cloneSet.Spec.ProgressDeadlineSeconds) * time.Second)) {
        newStatus.Conditions = append(newStatus.Conditions, appsv1alpha1.CloneSetCondition{
            Type:   appsv1alpha1.CloneSetProgressing,
            Status: corev1.ConditionFalse,
            Reason: appsv1alpha1.CloneSetProgressDeadlineExceeded,
        })
    }
}
```

When the CloneSet reaches the "paused" state, we should reset the timer to avoid the progress deadline being exceeded.
And we check the progress of the CloneSet in the `syncCloneSetStatus` function. If the progress exceeds the `progressDeadlineSeconds`, we should add a CloneSetCondition to the CloneSet's `.status.conditions`:

```go
const (
    CloneSetProgressDeadlineExceeded CloneSetConditionReason = "ProgressDeadlineExceeded"
    CloneSetConditionTypeProgressing CloneSetConditionType = "Progressing"
)
```

```go
func (c *CloneSetController) syncCloneSetStatus(cloneSet *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus) error {
    ...
    if cloneSet.Spec.ProgressDeadlineSeconds != nil {
        // handle the logic
        if time.Now().After(starttime.Add(time.Duration(*cloneSet.Spec.ProgressDeadlineSeconds) * time.Second)) {
            newStatus.Conditions = append(newStatus.Conditions, appsv1alpha1.CloneSetCondition{
                Type:   appsv1alpha1.CloneSetProgressing,
                Status: corev1.ConditionFalse,
                Reason: appsv1alpha1.CloneSetProgressDeadlineExceeded,
            })
        }
    }
    ...
}
```

When the CloneSet reaches the "paused" state, we should reset the timer to avoid the progress deadline being exceeded.
```go
func (c *CloneSetController) syncCloneSetStatus(cloneSet *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus) error {
    ...
	// reset the starttime when the CloneSet reaches the "paused" state or complete state
	if cloneSet.Status.UpdatedReadyReplicas == cloneSet.Status.Replicas ||  replicas - updatedReplicas = partition {
        starttime = time.Now()
    }
	
	if cloneSet.Spec.Paused {
		        starttime = time.Now()
	}
    ...
	}

    ...
}
```

And we can save the starttime in the `LastUpdateTime` in the CloneSet's `.status.conditions`:
```
status:
  conditions:
  - lastTransitionTime: "2021-11-26T20:52:12Z"
    lastUpdateTime: "2021-11-26T20:52:12Z"
    message: CloneSet has minimum availability.
    reason: MinimumReplicasAvailable
    status: "True"
    type: Available
  - lastTransitionTime: "2021-11-26T20:52:12Z"
    lastUpdateTime: "2021-11-26T20:52:12Z"
    message: 'progress deadline exceeded'
    reason: ProgressDeadlineExceeded
    status: "False"
    type: Progressing
```

## Implementation History

- [ ] 06/07/2024: Proposal submission


