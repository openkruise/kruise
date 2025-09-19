---
title: Add ImageListPullJob Template Support to AdvancedCronJob
authors:
  - "@zhusyang-jlu"
reviewers:
  - "@furykerry"
  - "@qizha"
creation-date: 2025-9-19
last-updated: 2025-9-19
status: implementable
---

# Add ImageListPullJob Template Support to AdvancedCronJob

---

## Table of Contents
- [Summary](#summary)
- [Motivation](#motivation)
- [Goals](#goals)
- [Non-Goals](#non-goals)
- [API Design](#api-design)
- [Core Implementation Logic](#core-implementation-logic)
- [Use Cases](#use-cases)
- [Compatibility and Upgrade Strategy](#compatibility-and-upgrade-strategy)
- [Test Plan](#test-plan)
- [Future Extensions](#future-extensions)
- [References](#references)

---

## Summary
Extend the AdvancedCronJob custom resource to declaratively schedule the periodic instantiation of ImageListPullJob resources, enabling automated image pre-pulling in line with GitOps and cluster lifecycle management practices.

## Motivation

In large-scale Kubernetes environments, Pod startup latency is often constrained by the time required to pull container images, particularly during node scaling events or high-frequency deployment cycles.

Kruise provides the `ImageListPullJob` resource to facilitate batch image pre-pulling across cluster nodes. However, there is currently no native mechanism to **automate and schedule** such operations on a recurring basis.

Users require the ability to:
- Schedule bulk image pre-pulling using Cron expressions.
- Proactively cache critical container images on nodes to reduce Pod initialization time.
- Minimize operational overhead through declarative automation.

`AdvancedCronJob`, an enhanced CronJob implementation from Kruise, supports multiple job templates (e.g., `Job`, `BroadcastJob`). To enable periodic image preheating, we propose extending `AdvancedCronJob` with support for the `ImageListPullJob` template.

---

## Goals

- Introduce a new optional field `imageListPullJobTemplate` in the `spec.jobTemplate` of `AdvancedCronJob`.
- Enable users to create scheduled `ImageListPullJob` instances via `AdvancedCronJob`.
- Reuse existing `ImageListPullJob` controller logic without duplicating functionality.
- Surface the execution status of `ImageListPullJob` instances in the `AdvancedCronJob` status.

---

## Non-Goals

- Do not modify the API or controller logic of the `ImageListPullJob` resource.
- Do not introduce new image sourcing mechanisms (e.g., registry scanning or dynamic image discovery).
- Do not support `ImagePullJob` (single-image) templates; this proposal is limited to `ImageListPullJob`.

---

## API Design

### 1. New Field: `imageListPullJobTemplate`

Add an optional field to the `CronJobTemplate` structure:

```go
type CronJobTemplate struct {
    // ... existing fields ...

    // Specifies the imagelistpulljob to be created when the cron schedule triggers.
    // +optional
    ImageListPullJobTemplate *ImageListPullJobTemplateSpec `json:"imageListPullJobTemplate,omitempty" protobuf:"bytes,3,opt,name=imageListPullJobTemplate"`
}
```
### 2. New Template Type: ImageListPullJobTemplate
Extend the TemplateKind enum:
```go
const (
    JobTemplate              TemplateKind = "Job"
    BroadcastJobTemplate     TemplateKind = "BroadcastJob"
    ImageListPullJobTemplate TemplateKind = "ImageListPullJob"
)
```
### 3. New Struct: ImageListPullJobTemplateSpec
```go
// ImageListPullJobTemplateSpec describes the data an ImageListPullJob should have when created from a template
type ImageListPullJobTemplateSpec struct {
    // Standard object's metadata of the jobs created from this template.
    // +optional
    metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

    // Specification of the desired behavior of the imagelistpulljob.
    // +optional
    Spec ImageListPullJobSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}
```

### 4. Example YAML
```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: AdvancedCronJob
metadata:
  name: nightly-image-prefetch
spec:
  schedule: "0 2 * * *" # Executes every day at 2:00 AM
  jobTemplate:
    imageListPullJobTemplate:
      metadata:
        labels:
          app: image-prefetch
      spec:
        imageList:
          - nginx:1.25
          - redis:7.0
          - myapp:v1.8.0
        pullPolicy: IfNotPresent
        podSelector:
          matchLabels:
            pod-label: xxx
          matchExpressions:
          - key: pod-label
            operator: In
            values:
              - xxx
        parallelism: 5
        completionPolicy:
          type: Always
```
## Core Implementation Logic
### 1. webhook validating
In advancedcronjob_create_update_handler.go：
* add validateImageListPullJobTemplateSpec function:
```go
// Validation rules:
// - The spec.template field must be a valid ImagePullListJob specification.
// - spec.completionPolicy.type, if specified, must equal "Always".
func validateImageListPullJobTemplateSpec(ilpJobSpec *appsv1alpha1.ImageListPullJobTemplateSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if ilpJobSpec.Spec.Selector != nil {
		if ilpJobSpec.Spec.Selector.MatchLabels != nil || ilpJobSpec.Spec.Selector.MatchExpressions != nil {
			if ilpJobSpec.Spec.Selector.Names != nil {
				return append(allErrs, field.Forbidden(fldPath.Child("Selector"), "can not set both names and labelSelector in this spec.selector"))
			}
			if _, err := metav1.LabelSelectorAsSelector(&ilpJobSpec.Spec.Selector.LabelSelector); err != nil {
				return append(allErrs, field.Forbidden(fldPath.Child("Selector"), fmt.Sprintf("invalid selector: %v", err)))
			}
		}
		if ilpJobSpec.Spec.Selector.Names != nil {
			names := sets.NewString(ilpJobSpec.Spec.Selector.Names...)
			if names.Len() != len(ilpJobSpec.Spec.Selector.Names) {
				return append(allErrs, field.Forbidden(fldPath.Child("Selector"), "duplicated name in selector names"))
			}
		}
	}
	if ilpJobSpec.Spec.PodSelector != nil {
		if ilpJobSpec.Spec.Selector != nil {
			return append(allErrs, field.Forbidden(fldPath.Child("PodSelector"), "can not set both selector and podSelector"))
		}
		if _, err := metav1.LabelSelectorAsSelector(&ilpJobSpec.Spec.PodSelector.LabelSelector); err != nil {
			return append(allErrs, field.Forbidden(fldPath.Child("PodSelector"), fmt.Sprintf("invalid podSelector: %v", err)))
		}
	}

	if len(ilpJobSpec.Spec.Images) == 0 {
		return append(allErrs, field.Forbidden(fldPath.Child("Images"), "image can not be empty"))
	}

	for i := 0; i < len(ilpJobSpec.Spec.Images); i++ {
		for j := i + 1; j < len(ilpJobSpec.Spec.Images); j++ {
			if ilpJobSpec.Spec.Images[i] == ilpJobSpec.Spec.Images[j] {
				return append(allErrs, field.Forbidden(fldPath.Child("Images"), "images cannot have duplicate values"))
			}
		}
	}

	if len(ilpJobSpec.Spec.Images) > 255 {
		return append(allErrs, field.Forbidden(fldPath.Child("Images"), "the maximum number of images cannot > 255"))
	}

	for _, image := range ilpJobSpec.Spec.Images {
		if _, err := daemonutil.NormalizeImageRef(image); err != nil {
			return append(allErrs, field.Forbidden(fldPath.Child("Images"), fmt.Sprintf("invalid image %s: %v", image, err)))
		}
	}

	switch ilpJobSpec.Spec.CompletionPolicy.Type {
	case appsv1alpha1.Always:
	// is a no-op here.No need to do parameter dependency verification in this type.
	// In the cronjob scenario, the never value is not required.
	    if *ilpJobSpec.Spec.CompletionPolicy.ActiveDeadlineSeconds > int64(14400) {
	        return append(allErrs, field.Forbidden(fldPath.Child("CompletionPolicy"), fmt.Sprintf("ActiveDeadlineSeconds  must be less than 14400,current: %d", ilpJobSpec.Spec.CompletionPolicy.ActiveDeadlineSeconds)))
	    }
	    if *ilpJobSpec.Spec.CompletionPolicy.TTLSecondsAfterFinished > 86400 {
	        return append(allErrs, field.Forbidden(fldPath.Child("CompletionPolicy"), fmt.Sprintf("TTLSecondsAfterFinished  must be less than 86400,current: %d", ilpJobSpec.Spec.CompletionPolicy.TTLSecondsAfterFinished)))
		}
	default:
		return append(allErrs, field.Forbidden(fldPath.Child("CompletionPolicy"), fmt.Sprintf("completionPolicy only support always,current: %s:", ilpJobSpec.Spec.CompletionPolicy.Type)))
	}

	return allErrs

}
```

### 2. Controller Logic Changes
In advancedcronjob_controller.go:
* Add a new Watch for ImageListPullJob in the add function:
```go
if err = watchImageListPullJob(mgr, c); err != nil {
    klog.ErrorS(err, "Failed to watch ImageListPullJob")
    return err
}
```
* Add a new branch in the Reconcile function to handle ImageListPullJobTemplate:
```go
case appsv1alpha1.ImageListPullJobTemplate:
    return r.reconcileImageListPullJob(ctx, req, advancedCronJob)
```
### 3. reconcileImageListPullJob Function Implementation
* Determining whether a task should be triggered based on the spec.schedule of AdvancedCronJob.
* Constructing an ImageListPullJob object using the ImageListPullJobTemplateSpec.
* Setting the OwnerReference to point to the AdvancedCronJob.
* Creating or updating the ImageListPullJob.
* Updating the status.lastScheduleTime and status.active list of the AdvancedCronJob.

### 4. Status Synchronization (Optional)
Optionally, listen to ImageListPullJob events and aggregate execution results (success/failure) into the status of the AdvancedCronJob.

## Use Cases
### Use Case 1:  Nightly Preheating of Core Images
Pre-pull foundational images (e.g., nginx, redis, sidecar-containers) during off-peak hours to ensure rapid Pod initialization during active workloads.

### Use Case 2: AI/ML Workload Preparation
Pre-pull multiple images for ML inference workloads to optimize the transition from batch jobs on GPU nodes (01:00–06:00) to real-time inference, with flexible schedule adjustments as workloads evolve.

### Use Case 3: Pre-Deployment Image Caching
Trigger image pre-pulling shortly before scheduled deployments or rollouts to minimize image pull latency during rollout phases.

### Use Case 4: Edge Node Optimization
In distributed edge clusters with constrained bandwidth, batch-pull images during maintenance windows to avoid network contention during operational hours.

## Compatibility and Upgrade Strategy
* Backward Compatible: Existing AdvancedCronJob instances remain unaffected; the new field is optional.
* No Data Migration: No migration of existing resources is required.
* Upgrade Path:
   * Upgrade Kruise to a version supporting this feature.
   * Existing controllers will safely ignore the new field.

## Test Plan
* Unit Tests: Validate reconcileImageListPullJob logic.
* End-to-End (E2E) Tests:
  * Create an AdvancedCronJob with imageListPullJobTemplate and verify:
    * ImageListPullJob is created according to schedule.
    * AdvancedCronJob status (e.g., lastScheduleTime, active) is accurately updated.
* Compatibility Tests: Ensure existing AdvancedCronJob resources (without imageListPullJobTemplate) function correctly under the upgraded controller.
* Reliability & Observability:
  * Images within an ImageListPullJob are pulled in parallel.
  * Partial image pull failures do not fail the entire job unless completionPolicy.type = Always.
  * On failure, record status.conditions and the most recent failed image (status.lastFailedImage) for debugging.
  * Emit Kubernetes events to surface errors and aid user troubleshooting.

## Future Extensions
* Support for `ImagePullJobTemplate` (scheduled pulling of individual images).
* Event-driven image preheating (e.g., triggered by Node registration or image update events).
* Priority and resource constraints for image preheating tasks to enable QoS-based scheduling.

## References
* OpenKruise ImageListPullJob Documentation: https://openkruise.io/docs/user-manuals/imagelistpulljob/
* OpenKruise AdvancedCronJob Documentation: https://openkruise.io/docs/user-manuals/advancedcronjob/
