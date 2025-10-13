/*
Copyright 2020 The Kruise Authors.

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

package defaults

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	v1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"k8s.io/utils/ptr"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

// SetDefaultsStatefulSet set default values for StatefulSet.
func SetDefaultsStatefulSet(obj *v1beta1.StatefulSet, injectTemplateDefaults bool) {
	if len(obj.Spec.PodManagementPolicy) == 0 {
		obj.Spec.PodManagementPolicy = appsv1.OrderedReadyPodManagement
	}

	if obj.Spec.UpdateStrategy.Type == "" {
		obj.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
	}

	if obj.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType {
		if obj.Spec.UpdateStrategy.RollingUpdate == nil {
			// UpdateStrategy.RollingUpdate will take default values below.
			obj.Spec.UpdateStrategy.RollingUpdate = &v1beta1.RollingUpdateStatefulSetStrategy{}
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.Partition == nil {
			obj.Spec.UpdateStrategy.RollingUpdate.Partition = ptr.To(int32(0))
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable == nil {
			maxUnavailable := intstr.FromInt(1)
			obj.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = &maxUnavailable
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy == "" {
			obj.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy = v1beta1.RecreatePodUpdateStrategyType
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.MinReadySeconds == nil {
			obj.Spec.UpdateStrategy.RollingUpdate.MinReadySeconds = ptr.To(int32(0))
		}
	}

	if utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetAutoDeletePVC) {
		if obj.Spec.PersistentVolumeClaimRetentionPolicy == nil {
			obj.Spec.PersistentVolumeClaimRetentionPolicy = &v1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{}
		}
		if len(obj.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted) == 0 {
			obj.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted = v1beta1.RetainPersistentVolumeClaimRetentionPolicyType
		}
		if len(obj.Spec.PersistentVolumeClaimRetentionPolicy.WhenScaled) == 0 {
			obj.Spec.PersistentVolumeClaimRetentionPolicy.WhenScaled = v1beta1.RetainPersistentVolumeClaimRetentionPolicyType
		}
	}

	if utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetAutoResizePVCGate) {
		if obj.Spec.VolumeClaimUpdateStrategy.Type == "" {
			obj.Spec.VolumeClaimUpdateStrategy.Type = v1beta1.OnPVCDeleteVolumeClaimUpdateStrategyType
		}
	}

	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = ptr.To(int32(1))
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = ptr.To(int32(10))
	}

	if injectTemplateDefaults {
		SetDefaultPodSpec(&obj.Spec.Template.Spec)
		for i := range obj.Spec.VolumeClaimTemplates {
			a := &obj.Spec.VolumeClaimTemplates[i]
			v1.SetDefaults_PersistentVolumeClaim(a)
			v1.SetDefaults_ResourceList(&a.Spec.Resources.Limits)
			v1.SetDefaults_ResourceList(&a.Spec.Resources.Requests)
			v1.SetDefaults_ResourceList(&a.Status.Capacity)
		}
	}
}

// SetDefaultsBroadcastJob set default values for BroadcastJob.
func SetDefaultsBroadcastJob(obj *v1beta1.BroadcastJob, injectTemplateDefaults bool) {
	if injectTemplateDefaults {
		SetDefaultPodSpec(&obj.Spec.Template.Spec)
	}
	if obj.Spec.CompletionPolicy.Type == "" {
		obj.Spec.CompletionPolicy.Type = v1beta1.Always
	}

	if obj.Spec.Parallelism == nil {
		parallelism := int32(1<<31 - 1)
		parallelismIntStr := intstr.FromInt(int(parallelism))
		obj.Spec.Parallelism = &parallelismIntStr
	}

	if obj.Spec.FailurePolicy.Type == "" {
		obj.Spec.FailurePolicy.Type = v1beta1.FailurePolicyTypeFailFast
	}
}

// SetDefaultsAdvancedCronJob set default values for AdvancedCronJob.
func SetDefaultsAdvancedCronJob(obj *v1beta1.AdvancedCronJob, injectTemplateDefaults bool) {
	if obj.Spec.Template.JobTemplate != nil && injectTemplateDefaults {
		SetDefaultPodSpec(&obj.Spec.Template.JobTemplate.Spec.Template.Spec)
	}

	if obj.Spec.Template.BroadcastJobTemplate != nil && injectTemplateDefaults {
		SetDefaultPodSpec(&obj.Spec.Template.BroadcastJobTemplate.Spec.Template.Spec)
	}

	if obj.Spec.Template.ImageListPullJobTemplate != nil && obj.Spec.Template.ImageListPullJobTemplate.Spec.CompletionPolicy.Type == "" {
		obj.Spec.Template.ImageListPullJobTemplate.Spec.CompletionPolicy.Type = v1beta1.Always
	}

	if obj.Spec.ConcurrencyPolicy == "" {
		obj.Spec.ConcurrencyPolicy = v1beta1.AllowConcurrent
	}
	if obj.Spec.Paused == nil {
		obj.Spec.Paused = new(bool)
	}

	if obj.Spec.SuccessfulJobsHistoryLimit == nil {
		obj.Spec.SuccessfulJobsHistoryLimit = new(int32)
		*obj.Spec.SuccessfulJobsHistoryLimit = 3
	}
	if obj.Spec.FailedJobsHistoryLimit == nil {
		obj.Spec.FailedJobsHistoryLimit = new(int32)
		*obj.Spec.FailedJobsHistoryLimit = 1
	}
}

// SetDefaultsImagePullJobV1beta1 sets default values for v1beta1 ImagePullJob.
func SetDefaultsImagePullJobV1beta1(obj *v1beta1.ImagePullJob, addProtection bool) {
	if obj.Spec.CompletionPolicy.Type == "" {
		obj.Spec.CompletionPolicy.Type = v1beta1.Always
	}
	if obj.Spec.PullPolicy == nil {
		obj.Spec.PullPolicy = &v1beta1.PullPolicy{}
	}
	if obj.Spec.PullPolicy.TimeoutSeconds == nil {
		obj.Spec.PullPolicy.TimeoutSeconds = ptr.To(int32(600))
	}
	if obj.Spec.PullPolicy.BackoffLimit == nil {
		obj.Spec.PullPolicy.BackoffLimit = ptr.To(int32(3))
	}
	if obj.Spec.ImagePullPolicy == "" {
		obj.Spec.ImagePullPolicy = v1beta1.PullIfNotPresent
	}
}

// SetDefaultsImageListPullJobV1beta1 sets default values for v1beta1 ImageListPullJob.
func SetDefaultsImageListPullJobV1beta1(obj *v1beta1.ImageListPullJob) {
	if obj.Spec.CompletionPolicy.Type == "" {
		obj.Spec.CompletionPolicy.Type = v1beta1.Always
	}
	if obj.Spec.PullPolicy == nil {
		obj.Spec.PullPolicy = &v1beta1.PullPolicy{}
	}
	if obj.Spec.PullPolicy.TimeoutSeconds == nil {
		obj.Spec.PullPolicy.TimeoutSeconds = ptr.To(int32(600))
	}
	if obj.Spec.PullPolicy.BackoffLimit == nil {
		obj.Spec.PullPolicy.BackoffLimit = ptr.To(int32(3))
	}
}

// SetDefaultsNodeImageV1beta1 sets default values for v1beta1 NodeImage.
func SetDefaultsNodeImageV1beta1(obj *v1beta1.NodeImage) {
	now := metav1.Now()
	for name, imageSpec := range obj.Spec.Images {
		for i := range imageSpec.Tags {
			tagSpec := &imageSpec.Tags[i]
			if tagSpec.CreatedAt == nil {
				tagSpec.CreatedAt = &now
			}
			if tagSpec.PullPolicy == nil {
				tagSpec.PullPolicy = &v1beta1.ImageTagPullPolicy{}
			}
			SetDefaultsImageTagPullPolicyV1beta1(tagSpec.PullPolicy)
		}
		obj.Spec.Images[name] = imageSpec
	}
}

func SetDefaultsImageTagPullPolicyV1beta1(obj *v1beta1.ImageTagPullPolicy) {
	if obj.TimeoutSeconds == nil {
		obj.TimeoutSeconds = ptr.To(int32(600))
	}
	if obj.BackoffLimit == nil {
		obj.BackoffLimit = ptr.To(int32(3))
	}
}
