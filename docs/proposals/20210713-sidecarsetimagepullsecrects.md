---
title: SidecarSet ImagePullSecrets

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
# SidecarSet ImagePullSecrets
- Provide a way to pull images using `Secret` from private repositories for `SidecarSet`.

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
In k8s, `Secret` is an easy and safe way to store and manage sensitive information, such as passwords, OAuth tokens, and ssh keys.
One of the most common-used features of it is to pull images from private repositories. However, `SidecarSet` does not support this feature so far.

## Proposal
**Main idea**: In this design, we separate the logic of `Secret Distribution` and `SidecarSet`. In `SidecarSet` part, we only consider injecting their `imagePullSecrets` feilds into Pod.

###API Definition
We add `imagePullSecrets` field in `apis/apps/v1alpha1/sidecarset_types.go`:
```go
type SidecarSetSpec struct {
	...
	// List of the names of secrets required by pulling sidecar container images
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}
```

### User Story
```yaml
# sidecarset.yaml
apiVersion: apps.kruise.io/v1alpha1
kind: SidecarSet
metadata:
  name: my-sidecarset
spec:
  selector:
    ...
  updateStrategy:
    ...
  containers:
    - name: my-sidecar
      image: minchou/centos:6.7
     ...
  imagePullSecrets:
    - name: my-secret
```

### Implementation Plan
We will merge the `imagePullSecrets` both in `Pod` and `SidecarSet` in `pkg/webhook/pod/sidercarset.go` file:
```go
func (h *PodCreateHandler) sidecarsetMutatingPod(ctx context.Context, req admission.Request, pod *corev1.Pod) error {
	...
	// inject imagePullSecrets 
	pod.Spec.ImagePullSecrets = mergeSidecarSecrets(pod.Spec.ImagePullSecrets, sidecarSecrets)
	...
}

func mergeSidecarSecrets(secretsInPod, secretsInSidecar []corev1.LocalObjectReference) (allSecrets []corev1.LocalObjectReference) {
	secretFilter := make(map[string]bool)
	for _, podSecret := range secretsInPod {
		if _, ok := secretFilter[podSecret.Name]; !ok {
			secretFilter[podSecret.Name] = true
			allSecrets = append(allSecrets, podSecret)
		}
	}
	for _, sidecarSecret := range secretsInSidecar {
		if _, ok := secretFilter[sidecarSecret.Name]; !ok {
			secretFilter[sidecarSecret.Name] = true
			allSecrets = append(allSecrets, sidecarSecret)
		}
	}
	return allSecrets
}
```

## Implementation History
- [ ] 13/07/2021: Proposal submission

