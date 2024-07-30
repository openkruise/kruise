/*
Copyright 2021 The Kruise Authors.

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

package utilfunction

import (
	"context"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func CreateJobForWorkload(c client.Client, owner metav1.Object, gvk schema.GroupVersionKind, name, image string, labels map[string]string, annotations map[string]string, podSelector metav1.LabelSelector, pullSecrets []string) error {
	var pullTimeoutSeconds int32 = 300
	if str, ok := owner.GetAnnotations()[appsv1alpha1.ImagePreDownloadTimeoutSecondsKey]; ok {
		if i, err := strconv.ParseInt(str, 10, 32); err == nil {
			pullTimeoutSeconds = int32(i)
		}
	}

	parallelism := intstr.FromInt(1)
	if str, ok := owner.GetAnnotations()[appsv1alpha1.ImagePreDownloadParallelismKey]; ok {
		if i, err := strconv.Atoi(str); err == nil {
			parallelism = intstr.FromInt(i)
		}
	}

	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       owner.GetNamespace(),
			Name:            name,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(owner, gvk)},
			Labels:          labels,
		},
		Spec: appsv1alpha1.ImagePullJobSpec{
			Image: image,
			ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
				PullSecrets: pullSecrets,
				PodSelector: &appsv1alpha1.ImagePullJobPodSelector{LabelSelector: podSelector},
				Parallelism: &parallelism,
				PullPolicy:  &appsv1alpha1.PullPolicy{BackoffLimit: utilpointer.Int32Ptr(1), TimeoutSeconds: &pullTimeoutSeconds},
				CompletionPolicy: appsv1alpha1.CompletionPolicy{
					Type:                    appsv1alpha1.Always,
					TTLSecondsAfterFinished: utilpointer.Int32Ptr(600),
				},
				SandboxConfig: &appsv1alpha1.SandboxConfig{
					Annotations: annotations,
					Labels:      labels,
				},
			},
		},
	}

	return c.Create(context.TODO(), job)
}

func DeleteJobsForWorkload(c client.Client, ownerObj metav1.Object) error {
	jobList := &appsv1alpha1.ImagePullJobList{}
	if err := c.List(context.TODO(), jobList, client.InNamespace(ownerObj.GetNamespace())); err != nil {
		return err
	}

	for i := range jobList.Items {
		job := &jobList.Items[i]
		owner := metav1.GetControllerOf(job)
		if owner == nil || owner.UID != ownerObj.GetUID() {
			continue
		}
		klog.InfoS("Deleting ImagePullJob for workload", "jobName", job.Name, "ownerKind", owner.Kind, "ownerNamespace", ownerObj.GetNamespace(), "ownerName", ownerObj.GetName())
		if err := c.Delete(context.TODO(), job); err != nil {
			return err
		}
	}
	return nil
}
