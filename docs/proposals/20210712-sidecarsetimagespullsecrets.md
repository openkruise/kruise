---
title: SidecarSetImagePullSecrets

authors:
- "@veophi"
  
reviewers:
- "@Fei-Guo"
- "@furykerry"
- "@FillZpp"
  
creation-date: 2021-07-12
  
last-updated: 2021-07-12
  
status: implementable

---
# SidecarSetImagePullSecrets

- Provide a way to pull images using `Secret` from private repositories for sidecarSet.

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

* [SidecarSetImagePullSecrets](#sidecarsetimagepullsecrets)
    * [Motivation](#motivation)
    * [Proposal](#proposal)
        * [Scheme #1: Create Secret By SidecarSet](#scheme-1-create-secret-by-sidecarset)
            * [Advantages](#advantages)
            * [Problems](#problems)
        * [Scheme #2: Create Secret By User](#scheme-2-create-secret-by-user)
            * [Advantages](#advantages-1)
            * [Problems](#problems-1)
    * [Implementation History](#implementation-history)

Created by [gh-md-toc](https://github.com/ekalinin/github-markdown-toc)

## Motivation

In k8s, secret is an easy and safe way to store and manage sensitive information, such as passwords, OAuth tokens, and ssh keys.
One of the most common-used features of it is to pull images from private repositories. However, sidecarSet does not support this feature so far.

## Proposal

When a sidecarSet is injected into the matched `Namespace.Pod`, the references of the secrets it needs also will be injected into the pod. 
In this situation, we have to consider the following two questions:
1. Which `Secret` we should use? I mean for the required `Namespace.Secret`, how to decide the `Namespace` when using **cluster-level sidecarSet**?
2. Does the `Namespace` have the `Secret`?

Considering the two questions above, we provide two schemes to discuss:
(I prefer the second.)

### Scheme #1: Create Secret By SidecarSet
**Main idea:**
  - Which `Secret` we should use?  **We Create the Secret for ALL namespaces!**
  - Does the `Namespace` have the `Secret`? **We Create the Secret for ALL namespaces!**

This is a rough but effective way to solve these two problems. 

First, we add the secret fields in `sidecarset_types.go`, it looks like this:
```go
type SidecarSetSpec struct {
	...
	SecretFields []SecretField
}

type SecretField struct {
	Data string  // secret data
	Name string  // secret name
	...
}
```
When applying a sidecarSet, we should do the following actions:

- Act 1: Generate secret jsons based on `SecretFields` to create secrets for scoped namespaces (for ALL if it's cluster-level).
- Act 2: Directly inject secrets into the matched `NS.POD` when instantiating a sidecarSet.
- Act 3: When changing `SecretFields` or creating new `Namespace`, we should manage the secrets, including delete old secrets and creating new secrets.

**Note**: If two different secrets are named with the same name by different users, it will be very dangerous. 
Thus, we had better bind the secrets to the sidecarSet when creating.

#### Advantage
1. End users don't need to know the concept of `Namespace`.

#### Problem
1. There may be multiple secrets that store the same content.
2. The `Secret` is hard to reuse.
3. It may be inconsistent with the design of `Secret` in k8s official community. 

### Scheme #2: Create Secret By User
**Main idea**: User create `Secrets`, we copy `Secrets` when using them.

In the `sidecarSet.yaml`, the user should give `Namespace` of the `Secret`:
```yaml
# sidecarset.yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  name: test-sidecarset
  namespace: scs-ns
spec:
  selector:
    ...
  updateStrategy:
    ...
  containers:
    - name: sidecar1
      image: minchou/centos:6.7
     ...
  imagePullSecrets:
    - name: test-secret
      - naamespace: test-secret-namespace
```
**Note**: 
 - if the secret's namespace isn't given, we will enable the ground rule to find the secrets in **default namespace** or **metadata.Namespace**.

When applying a sidecarSet, we should do the following actions:

- Act 1: Copy the Secret and rename the copy with `name` and `namespace` jointly, e.g., `NS1.Secret1` --> `NS2.NS1-Secret1` when coping `NS1.Secret1` to `NS2`. (We will always use the renamed secret)

- Act 2: When sidecarSet is injected into `NS.POD`, check if the secret exists in `NS`. if not, copy it by *Act 1*.

- Act 3: When original secret is deleted, scan all namespaces to delete its copies.

#### Advantage
1. `Secrets` can be reused.

#### Problem
1. The users must know the `Namespaces` of `Secrets`.
2. We have to check `Secrets` at every injection of sidecarSet.
3. In order to reuse secrets, we must agree on a proprietary mapping mechanism to bridge the original `Secrets` with their `renamed copies` for all sidecarSets.

## Implementation History
- [ ] 12/07/2021: Proposal submission
