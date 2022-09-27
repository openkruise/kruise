---
title: SidecarSetPatchPodMetadata
authors:
- "@zmberg"
reviewers:
- "@furykerry"
- "@FillZpp"
creation-date: 2021-06-15
last-updated: 2021-08-10
status: implementable
---

# SidecarSet Patch Pod Metadata

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Title](#title)
- [Table of Contents](#table-of-contents)
- [Motivation](#motivation)
- [Proposal](#proposal)
- [API Definition](#api-definition)
- [Permission Control](#permission-control)

## Motivation
Some sidecar containers may require special configurations that take effect via annotations/labels. Thus, sidecarSet should support inject or in-place update these configurations.

**User Story:**
- log-agent sidecar container inject pod annotation[oom-score]='{"log-agent": 1}' to set sidecar oom score.

## Proposal
SidecarSet support patch pod annotations/labels, as follows:

### Api Definition
```yaml
// SidecarSetSpec defines the desired state of SidecarSet
type SidecarSetSpec struct {
    // SidecarSet support to inject & in-place update metadata in pod.
    PatchPodMetadata []SidecarSetPatchPodMetadata `json:"patchPodMetadata,omitempty"`
}

type SidecarSetPatchPodMetadata struct {
    // annotations
    Annotations map[string]string `json:"annotations,omitempty"`

    // labels map[string]string `json:"labels,omitempty"`
    // patch pod metadata policy, Default is "Retain"
    PatchPolicy SidecarSetPatchPolicyType `json:"patchPolicy,omitempty"`
}

type SidecarSetPatchPolicyType string

var (
    // SidecarSetRetainPatchPolicy indicates if PatchPodFields conflicts with Pod,
    // will ignore PatchPodFields, and retain the corresponding fields of pods.
    // SidecarSet webhook cannot allow the conflict of PatchPodFields between SidecarSets under this policy type.
    // Note: Retain is only supported for injection, and the Metadata will not be updated when upgrading the Sidecar Container in-place.
    SidecarSetRetainPatchPolicy SidecarSetPatchPolicyType = "Retain"

    // SidecarSetOverwritePatchPolicy indicates if PatchPodFields conflicts with Pod,
    // SidecarSet will apply PatchPodFields to overwrite the corresponding fields of pods.
    // SidecarSet webhook cannot allow the conflict of PatchPodFields between SidecarSets under this policy type.
    // Overwrite support to inject and in-place metadata.
    SidecarSetOverwritePatchPolicy SidecarSetPatchPolicyType = "Overwrite"

    // SidecarSetMergePatchJsonPatchPolicy indicate that sidecarSet use application/merge-patch+json to patch annotation value,
    // for example, A patch annotation[oom-score] = '{"log-agent": 1}' and B patch annotation[oom-score] = '{"envoy": 2}'
    // result pod annotation[oom-score] = '{"log-agent": 1, "envoy": 2}'
    // MergePatchJson support to inject and in-place metadata.
    SidecarSetMergePatchJsonPatchPolicy SidecarSetPatchPolicyType = "MergePatchJson"
)
```

### Permission Control
SidecarSet should not modify any configuration outside the sidecar container from permission perspective. Metadata, as an important configuration of Pod, should not be modified by sidecarSet by default.

Objectively, sidecar does need to have some annotations or labels injected into the pod as well. In order to meet the needs of sidecar and security considerations.
if sidecarSet needs to modify the metadata, it needs to be whitelisted in kruise configmap which is maintained by the system administrator.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
    name: kruise-configuration
    namespace: kruise-system
data:
  "SidecarSet_PatchPodMetadata_WhiteList": |
    type SidecarSetPatchMetadataWhiteList struct {
        Rules []SidecarSetPatchMetadataWhiteRule `json:"rules"`
    }

    type SidecarSetPatchMetadataWhiteRule struct {
        // selector sidecarSet against labels
        // If selector is nil, assume that the rules should apply for every sidecarSets
        Selector *metav1.LabelSelector `json:"selector,omitempty"`
        // must be regular expressions
        AllowedAnnotationKeyExprs []string `json:"allowedAnnotationKeyExprs"`
    }
```

for example:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
    name: kruise-configuration
    namespace: kube-system
data:
    "SidecarSet_PatchPodMetadata_WhiteList": |
        {
            "rules": [
                {
                    // No selector is configured to take effect for all SidecarSets
                    "allowedAnnotationKeyExprs": [
                        "^oom-score$",
                    ]
                },
                {
                    // selector sidecarSet based on labels
                    "selector":  {
                        "matchLabels": {
                            "sidecar": "sidecar-demo"
                        }
                    }
                    "allowedAnnotationKeyExprs": [
                        "key.*",
                    ]
                },
            ]
        }
```

### feature-gates
"SidecarSet_PatchPodMetadata_WhiteList" is mainly for security reasons. If the user's business cluster scenario is relatively simple, the whitelist method may not be used.
```shell
$ helm install kruise https://... --set featureGates="SidecarSetPatchPodMetadataDefaultsAllowed=true"
```
**Note:** The above feature-gate will not verify any sidecarSet patch key, so users need to ensure security.

