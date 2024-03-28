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
    - [2. handle the logic](#2handle-the-logic)


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

### 2. handle the logic
In cloneset controller, we should add the logic to handle the `progressDeadlineSeconds` field. 
```go
func (c *CloneSetController) syncCloneSetStatus(cloneSet *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus) error {
    ...
    if cloneSet.Spec.ProgressDeadlineSeconds != nil {
        // handle the logic
    }
    ...
}
```
And we check the progress of the CloneSet in the `syncCloneSetStatus` function. If the progress exceeds the `progressDeadlineSeconds`, we should add a CloneSetCondition to the CloneSet's `.status.conditions`:
```go
func (c *CloneSetController) syncCloneSetStatus(cloneSet *appsv1alpha1.CloneSet, newStatus *appsv1alpha1.CloneSetStatus) error {
    ...
    if cloneSet.Spec.ProgressDeadlineSeconds != nil {
        // handle the logic
        if time.Now().After(cloneSet.Status.UpdatedTime.Add(time.Duration(*cloneSet.Spec.ProgressDeadlineSeconds) * time.Second)) {
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


