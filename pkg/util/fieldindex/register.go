/*
Copyright 2019 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fieldindex

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

const (
	IndexNameForPodNodeName          = "spec.nodeName"
	IndexNameForOwnerRefUID          = "ownerRefUID"
	IndexNameForController           = ".metadata.controller"
	IndexNameForIsActive             = "isActive"
	IndexNameForSidecarSetNamespace  = "namespace"
	IndexValueSidecarSetClusterScope = "clusterScope"
	LabelMetadataName                = v1.LabelMetadataName

	indexRegistrationRetryInterval = time.Second
	indexRegistrationTimeout       = 30 * time.Second
)

var (
	registerOnce sync.Once
	apiGVStr     = appsv1alpha1.GroupVersion.String()
)

var ownerIndexFunc = func(obj client.Object) []string {
	var owners []string
	for _, ref := range obj.GetOwnerReferences() {
		owners = append(owners, string(ref.UID))
	}
	return owners
}

func RegisterFieldIndexes(c cache.Cache) error {
	var err error
	registerOnce.Do(func() {
		err = registerFieldIndexes(c)
	})
	return err
}

func registerFieldIndexes(c cache.Cache) (err error) {
	// Built-in resources are always expected to be available. Fail fast if any
	// of these indexes cannot be registered.
	if err = registerFieldIndex(c, &v1.Pod{}, IndexNameForOwnerRefUID, ownerIndexFunc); err != nil {
		return
	}
	if err = registerFieldIndex(c, &v1.PersistentVolumeClaim{}, IndexNameForOwnerRefUID, ownerIndexFunc); err != nil {
		return
	}
	if err = indexPodNodeName(c); err != nil {
		return
	}
	if err = indexJob(c); err != nil {
		return
	}

	// CRDs can be established slightly later than the controller starts, so
	// retry their index registration on discovery NoMatch errors.
	if err = registerCRDFieldIndex(c, &appsv1alpha1.ImagePullJob{}, IndexNameForOwnerRefUID, ownerIndexFunc); err != nil {
		return
	}
	if err = registerCRDFieldIndex(c, &appsv1beta1.ImagePullJob{}, IndexNameForOwnerRefUID, ownerIndexFunc); err != nil {
		return
	}
	if err = indexBroadcastCronJob(c); err != nil {
		return
	}
	if err = indexBroadcastCronJobV1Beta1(c); err != nil {
		return
	}
	if err = indexImagePullJobActive(c); err != nil {
		return
	}
	if err = indexImagePullJobActiveV1Beta1(c); err != nil {
		return
	}
	if err = indexImageListPullJobV1Beta1(c); err != nil {
		return
	}
	if err = indexSidecarSet(c); err != nil {
		return
	}
	if err = indexSidecarSetV1Beta1(c); err != nil {
		return
	}
	return
}

func registerFieldIndex(c cache.Cache, obj client.Object, field string, extractValue client.IndexerFunc) error {
	if err := c.IndexField(context.TODO(), obj, field, extractValue); err != nil {
		return fmt.Errorf("failed to register field index %q for %T: %w", field, obj, err)
	}
	return nil
}

func registerCRDFieldIndex(c cache.Cache, obj client.Object, field string, extractValue client.IndexerFunc) error {
	return registerCRDFieldIndexWithRetry(c, obj, field, extractValue, indexRegistrationRetryInterval, indexRegistrationTimeout)
}

func registerCRDFieldIndexWithRetry(c cache.Cache, obj client.Object, field string, extractValue client.IndexerFunc, retryInterval, timeout time.Duration) error {
	var lastErr error
	retried := false
	startTime := time.Now()
	err := wait.PollUntilContextTimeout(context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
		lastErr = c.IndexField(ctx, obj, field, extractValue)
		if lastErr == nil {
			if retried {
				klog.InfoS("Registered field index after waiting for API resource", "object", fmt.Sprintf("%T", obj), "field", field, "cost", time.Since(startTime))
			}
			return true, nil
		}
		if !meta.IsNoMatchError(lastErr) {
			return false, lastErr
		}
		if !retried {
			retried = true
			klog.InfoS("Waiting for API resource before registering field index", "object", fmt.Sprintf("%T", obj), "field", field, "err", lastErr)
		}
		return false, nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && lastErr != nil {
			return fmt.Errorf("timed out after %s registering field index %q for %T: %w", timeout, field, obj, lastErr)
		}
		return fmt.Errorf("failed to register field index %q for %T: %w", field, obj, err)
	}
	return nil
}

func indexPodNodeName(c cache.Cache) error {
	return registerFieldIndex(c, &v1.Pod{}, IndexNameForPodNodeName, func(obj client.Object) []string {
		pod, ok := obj.(*v1.Pod)
		if !ok {
			return []string{}
		}
		if len(pod.Spec.NodeName) == 0 {
			return []string{}
		}
		return []string{pod.Spec.NodeName}
	})
}

func indexJob(c cache.Cache) error {
	return registerFieldIndex(c, &batchv1.Job{}, IndexNameForController, func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}

		// ...make sure it's a AdvancedCronJob (v1alpha1 or v1beta1)...
		if (owner.APIVersion != apiGVStr && owner.APIVersion != appsv1beta1.SchemeGroupVersion.String()) ||
			(owner.Kind != appsv1alpha1.AdvancedCronJobKind && owner.Kind != appsv1beta1.AdvancedCronJobKind) {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	})
}

func indexBroadcastCronJob(c cache.Cache) error {
	return registerCRDFieldIndex(c, &appsv1alpha1.BroadcastJob{}, IndexNameForController, func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		job := rawObj.(*appsv1alpha1.BroadcastJob)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}

		// ...make sure it's a AdvancedCronJob...
		if owner.APIVersion != apiGVStr || owner.Kind != appsv1alpha1.AdvancedCronJobKind {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	})
}

func indexBroadcastCronJobV1Beta1(c cache.Cache) error {
	return registerCRDFieldIndex(c, &appsv1beta1.BroadcastJob{}, IndexNameForController, func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		job := rawObj.(*appsv1beta1.BroadcastJob)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}

		// ...make sure it's a AdvancedCronJob...
		if owner.APIVersion != appsv1beta1.SchemeGroupVersion.String() || owner.Kind != appsv1beta1.AdvancedCronJobKind {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	})
}

func indexImageListPullJobV1Beta1(c cache.Cache) error {
	return registerCRDFieldIndex(c, &appsv1beta1.ImageListPullJob{}, IndexNameForController, func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		job := rawObj.(*appsv1beta1.ImageListPullJob)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}

		// ...make sure it's a v1beta1 AdvancedCronJob...
		if owner.APIVersion != appsv1beta1.SchemeGroupVersion.String() || owner.Kind != appsv1beta1.AdvancedCronJobKind {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	})
}

func indexImagePullJobActive(c cache.Cache) error {
	return registerCRDFieldIndex(c, &appsv1alpha1.ImagePullJob{}, IndexNameForIsActive, func(rawObj client.Object) []string {
		obj := rawObj.(*appsv1alpha1.ImagePullJob)
		isActive := "false"
		if obj.DeletionTimestamp == nil && obj.Status.CompletionTime == nil {
			isActive = "true"
		}
		return []string{isActive}
	})
}

func indexImagePullJobActiveV1Beta1(c cache.Cache) error {
	return registerCRDFieldIndex(c, &appsv1beta1.ImagePullJob{}, IndexNameForIsActive, func(rawObj client.Object) []string {
		return IndexImagePullJob(rawObj)
	})
}

func IndexImagePullJob(rawObj client.Object) []string {
	obj := rawObj.(*appsv1beta1.ImagePullJob)
	isActive := "false"
	if obj.DeletionTimestamp == nil && obj.Status.CompletionTime == nil {
		isActive = "true"
	}
	return []string{isActive}
}

func IndexSidecarSet(rawObj client.Object) []string {
	obj := rawObj.(*appsv1alpha1.SidecarSet)
	if obj == nil {
		return nil
	}
	if obj.Spec.Namespace != "" {
		return []string{obj.Spec.Namespace}
	}
	if obj.Spec.NamespaceSelector != nil {
		if obj.Spec.NamespaceSelector.MatchLabels != nil {
			if v, ok := obj.Spec.NamespaceSelector.MatchLabels[LabelMetadataName]; ok {
				return []string{v}
			}
		}
		for _, item := range obj.Spec.NamespaceSelector.MatchExpressions {
			if item.Key == LabelMetadataName && item.Operator == metav1.LabelSelectorOpIn {
				return item.Values
			}
		}
	}
	return []string{IndexValueSidecarSetClusterScope}
}

func indexSidecarSet(c cache.Cache) error {
	return registerCRDFieldIndex(c, &appsv1alpha1.SidecarSet{}, IndexNameForSidecarSetNamespace, func(rawObj client.Object) []string {
		return IndexSidecarSet(rawObj)
	})
}

func IndexSidecarSetV1Beta1(rawObj client.Object) []string {
	obj := rawObj.(*appsv1beta1.SidecarSet)
	if obj == nil {
		return nil
	}
	// v1beta1 uses NamespaceSelector
	if obj.Spec.NamespaceSelector != nil {
		if obj.Spec.NamespaceSelector.MatchLabels != nil {
			if v, ok := obj.Spec.NamespaceSelector.MatchLabels[LabelMetadataName]; ok {
				return []string{v}
			}
		}
		for _, item := range obj.Spec.NamespaceSelector.MatchExpressions {
			if item.Key == LabelMetadataName && item.Operator == metav1.LabelSelectorOpIn {
				return item.Values
			}
		}
	}
	return []string{IndexValueSidecarSetClusterScope}
}

func indexSidecarSetV1Beta1(c cache.Cache) error {
	return registerCRDFieldIndex(c, &appsv1beta1.SidecarSet{}, IndexNameForSidecarSetNamespace, func(rawObj client.Object) []string {
		return IndexSidecarSetV1Beta1(rawObj)
	})
}
