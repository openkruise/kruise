---
title: Resource Distribution

authors:
  - "@veophi"

reviewers:
  - "@Fei-Guo"
  - "@furykerry"
  - "@FillZpp"

creation-date: 2021-07-13
last-updated: 2021-07-13
status: implementable
---
# Resource Distribution
- Provide a way to distribute some namespaced resource (e.g., `Secret`, `ConfigMap`) to other namespaces.

## Table of Contents
A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [SidecarSet ImagePullSecrets](#sidecarset-imagepullsecrets)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Proposal](#proposal)
    - [User Story](#user-story)
    - [Implementation Plan](#implementation-plan)
  - [Implementation History](#implementation-history)
  
## Motivation
Sometimes, we want to apply some namespaced resources, such as `Secret` and `ConfigMap`,  to other namespaces, even to the whole cluster.

For example, a `SidecarSet` with `imagePullSecrets` field may be injected into different namespaces, we must make sure the `Secrets` exist in all of these namespaces.
In this situation, we may need a resource distribution mechanism to help us do it more easily.

## Proposal
**Main idea**: Distribute and Update the resources by the user-defined annotation.

Next, we will take `Secret Distribution` as an example to illustrate our proposal.

### User Stories
#### Story 1: distribute to other namespaces
```yaml
# my-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: namespaced-secret
  annotations:
    openkruise.io/sync-to: "namespaces"
    openkruise.io/sync-to/namespaces: "ns-1;ns-2;ns-3"
type: generic
stringData:
  username: admin
  password: t0p-Secret
```
When `kubectl apply -f my-secret.yaml`,  the secret will be created or updated in ns-1, ns-2, and ns-3.
#### Story 2: distribute to the whole cluster
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cluseter-level-secret
  annotations:
    openkruise.io/sync-to: "cluster"
type: generic
stringData:
  username: admin
  password: t0p-Secret
```
When `kubectl apply -f my-secret.yaml`,  the secret will be created or updated in **all namespaces**.

### Implementation Plan

#### Controller Reconcile 

We watch all the events about `Secret`, and sync it when reconciling it.
The main logic looks like this:
```go
package secret

//1.Get the secret instance
instance := &corev1.Secret{}
if err := r.client.Get(context.TODO(), request.NamespacedName, instance); err != nil {
	...
}

target, _ := Annotation["openkruise.io/sync-to"]

//2. Get sync namespaces, return empty slice when target is nil
syncNamespaces := GetSyncNamespace(target)

//3. Get all namespaces
allNamespaces := GetAllNamespace()

//4. Create or update secret for required namespaces
for _, namespace := range allNamespaces {
    secret = NewSecret(namespace, instance)
    
    // 5.Check and see if the namespace needs to sync 
    if IsIn(namespace, syncNamespaces) {
        if IsSecretExisted(secret, namespace) {
            r.Client.Update(ctx.TODO(), secret)
        } else {
            r.Client.Create(ctx.TODO(), secret)
        }
    } else {
        // 6. Delete the copy that don't belong to syncNamespaces  
        if IsSecretExisted(secret, namespace) {
            r.Client.Delete(ctx.TODO(), secret)
        }
    }
}

```

### Risks and Mitigations
Problem: When users delete the original secret, how to delete its copies in other namespaces? 

Solution #1: Users delete the copies by clearing the `Annotation["openkruise.io/sync-to"]`, then delete the original secret. Of course, we must add the note in the document.

Solution #2: When the `delete event` is observed, we will delete all copies of the secret.
However, once the `delete event` is lost, or panic happens after the `delete event`,  the copies of secret may no longer be deleted.

## Implementation History
- [ ] 13/07/2021: Proposal submission

