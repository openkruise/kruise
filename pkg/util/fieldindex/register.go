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
	"sync"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

const (
	IndexNameForPodNodeName = "spec.nodeName"
	IndexNameForOwnerRefUID = "ownerRefUID"
	IndexNameForController  = ".metadata.controller"
	IndexNameForIsActive    = "isActive"
)

var (
	registerOnce sync.Once
	apiGVStr     = appsv1alpha1.GroupVersion.String()
)

var ownerIndexFunc = func(obj runtime.Object) []string {
	metaObj, ok := obj.(metav1.Object)
	if !ok {
		return []string{}
	}
	var owners []string
	for _, ref := range metaObj.GetOwnerReferences() {
		owners = append(owners, string(ref.UID))
	}
	return owners
}

func RegisterFieldIndexes(c cache.Cache) error {
	var err error
	registerOnce.Do(func() {

		// pod ownerReference
		if err = c.IndexField(&v1.Pod{}, IndexNameForOwnerRefUID, ownerIndexFunc); err != nil {
			return
		}
		// pvc ownerReference
		if err = c.IndexField(&v1.PersistentVolumeClaim{}, IndexNameForOwnerRefUID, ownerIndexFunc); err != nil {
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
	})
	return err
}

func indexPodNodeName(c cache.Cache) error {
	return c.IndexField(&v1.Pod{}, IndexNameForPodNodeName, func(obj runtime.Object) []string {
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
	return c.IndexField(&batchv1.Job{}, IndexNameForController, func(rawObj runtime.Object) []string {
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
	return c.IndexField(&appsv1alpha1.BroadcastJob{}, IndexNameForController, func(rawObj runtime.Object) []string {
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
	return c.IndexField(&appsv1alpha1.ImagePullJob{}, IndexNameForIsActive, func(rawObj runtime.Object) []string {
		obj := rawObj.(*appsv1alpha1.ImagePullJob)
		isActive := "false"
		if obj.DeletionTimestamp == nil && obj.Status.CompletionTime == nil {
			isActive = "true"
		}
		return []string{isActive}
	})
}
