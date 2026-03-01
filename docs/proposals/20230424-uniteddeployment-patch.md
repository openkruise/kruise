---
title: UnitedDeployment
authors:
  - "@chengleqi"
reviewers:
  - "@Fei-Guo"
  - "@furykerry"
  - "@FillZpp"
creation-date: 2023-04-24
last-updated: 2023-04-24
status: implementable
---

# UnitedDeployment Patch

## Motivation

We need to configure the subsets governed by UnitedDeployment differently to meet various business requirements, such as setting different environment variables or labels for different subsets. Therefore, we need to patch the template to enable different configurations.
[Issue 811](https://github.com/openkruise/kruise/issues/811)

## Proposal

Patch( [runtime.RawExtension](https://github.com/kubernetes/apimachinery/blob/03ac7a9ade429d715a1a46ceaa3724c18ebae54f/pkg/runtime/types.go#L94) ) is the specified strategic patch body of this subset, it will be patched to Workload template `(StatefulSet|AdvancedStatefulSet|CloneSet|Deployment)` .

## Samples

```yaml
# patch metadata:
patch:
  metadata:
    labels:
      xxx-specific-label: xxx
```

```yaml
# patch container resources:
patch:
  spec:
    containers:
    - name: main
      resources:
        limits:
          cpu: "2"
          memory: 800Mi
```

```yaml
# patch container environments:
patch:
  spec:
    containers:
    - name: main
      env:
      - name: K8S_CONTAINER_NAME
        value: main
```

## Implement

```go
if subSetConfig.Patch.Raw != nil {
		cloneBytes, _ := json.Marshal(set)
		modified, err := strategicpatch.StrategicMergePatch(cloneBytes, subSetConfig.Patch.Raw, &v1beta1.StatefulSet{})
		if err != nil {
			klog.Errorf("failed to merge patch raw %s", subSetConfig.Patch.Raw)
			return err
		}
		patchedSet := &v1beta1.StatefulSet{}
		if err = json.Unmarshal(modified, patchedSet); err != nil {
			klog.Errorf("failed to unmarshal %s to AdvancedStatefulSet", modified)
			return err
		}
		// ... check labels
		patchedSet.DeepCopyInto(set)
		klog.V(2).Infof("AdvancedStatefulSet [%s/%s] was patched successfully: %s", set.Namespace, set.GenerateName, subSetConfig.Patch.Raw)
	}
```

1. Add the above code snippet to the following files: `advanced_statefulset_adapter.go`,`cloneset_adapter.go`,`deployment_adapter.go`,`statefulset_adapter.go`.
2. Validate the patch and the workload. Should not broke the fields already set in the template.