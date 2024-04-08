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
	"sync"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	IndexNameForPodNodeName          = "spec.nodeName"
	IndexNameForOwnerRefUID          = "ownerRefUID"
	IndexNameForController           = ".metadata.controller"
	IndexNameForIsActive             = "isActive"
	IndexNameForSidecarSetNamespace  = "namespace"
	IndexValueSidecarSetClusterScope = "clusterScope"
	LabelMetadataName                = v1.LabelMetadataName
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

		// pod ownerReference
		if err = c.IndexField(context.TODO(), &v1.Pod{}, IndexNameForOwnerRefUID, ownerIndexFunc); err != nil {
			return
		}
		// pvc ownerReference
		if err = c.IndexField(context.TODO(), &v1.PersistentVolumeClaim{}, IndexNameForOwnerRefUID, ownerIndexFunc); err != nil {
			return
		}
		// ImagePullJob ownerReference
		if err = c.IndexField(context.TODO(), &appsv1alpha1.ImagePullJob{}, IndexNameForOwnerRefUID, ownerIndexFunc); err != nil {
			return
		}

		// pod name
		if err = indexPodNodeName(c); err != nil {
			return
		}
		// job owner
		if err = indexJob(c); err != nil {
			return
		}
		// broadcastjob owner
		if utildiscovery.DiscoverObject(&appsv1alpha1.BroadcastJob{}) {
			if err = indexBroadcastCronJob(c); err != nil {
				return
			}
		}
		// imagepulljob active
		if utildiscovery.DiscoverObject(&appsv1alpha1.ImagePullJob{}) {
			if err = indexImagePullJobActive(c); err != nil {
				return
			}
		}
		// sidecar spec namespaces
		if utildiscovery.DiscoverObject(&appsv1alpha1.SidecarSet{}) {
			if err = indexSidecarSet(c); err != nil {
				return
			}
		}
	})
	return err
}

func indexPodNodeName(c cache.Cache) error {
	return c.IndexField(context.TODO(), &v1.Pod{}, IndexNameForPodNodeName, func(obj client.Object) []string {
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
	return c.IndexField(context.TODO(), &batchv1.Job{}, IndexNameForController, func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		job := rawObj.(*batchv1.Job)
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

func indexBroadcastCronJob(c cache.Cache) error {
	return c.IndexField(context.TODO(), &appsv1alpha1.BroadcastJob{}, IndexNameForController, func(rawObj client.Object) []string {
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

func indexImagePullJobActive(c cache.Cache) error {
	return c.IndexField(context.TODO(), &appsv1alpha1.ImagePullJob{}, IndexNameForIsActive, func(rawObj client.Object) []string {
		obj := rawObj.(*appsv1alpha1.ImagePullJob)
		isActive := "false"
		if obj.DeletionTimestamp == nil && obj.Status.CompletionTime == nil {
			isActive = "true"
		}
		return []string{isActive}
	})
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
	return c.IndexField(context.TODO(), &appsv1alpha1.SidecarSet{}, IndexNameForSidecarSetNamespace, func(rawObj client.Object) []string {
		return IndexSidecarSet(rawObj)
	})
}
