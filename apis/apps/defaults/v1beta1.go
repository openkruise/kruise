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
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	v1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"k8s.io/utils/ptr"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
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
		if obj.Spec.Template.ImageListPullJobTemplate.Spec.PullPolicy == nil {
			obj.Spec.Template.ImageListPullJobTemplate.Spec.PullPolicy = &v1beta1.PullPolicy{}
		}
		if obj.Spec.Template.ImageListPullJobTemplate.Spec.PullPolicy.TimeoutSeconds == nil {
			obj.Spec.Template.ImageListPullJobTemplate.Spec.PullPolicy.TimeoutSeconds = ptr.To(int32(600))
		}
		if obj.Spec.Template.ImageListPullJobTemplate.Spec.PullPolicy.BackoffLimit == nil {
			obj.Spec.Template.ImageListPullJobTemplate.Spec.PullPolicy.BackoffLimit = ptr.To(int32(3))
		}
		if obj.Spec.Template.ImageListPullJobTemplate.Spec.ImagePullPolicy == "" {
			obj.Spec.Template.ImageListPullJobTemplate.Spec.ImagePullPolicy = v1beta1.PullIfNotPresent
		}
	}

	if obj.Spec.ConcurrencyPolicy == "" {
		if obj.Spec.Template.ImageListPullJobTemplate != nil {
			// concurrent run imagepulljob is useless
			obj.Spec.ConcurrencyPolicy = v1beta1.ReplaceConcurrent
		} else {
			obj.Spec.ConcurrencyPolicy = v1beta1.AllowConcurrent
		}
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

// SetDefaultsDaemonSetV1beta1 sets default values for v1beta1 DaemonSet.
func SetDefaultsDaemonSetV1beta1(obj *v1beta1.DaemonSet) {
	if obj.Spec.BurstReplicas == nil {
		BurstReplicas := intstr.FromInt(250)
		obj.Spec.BurstReplicas = &BurstReplicas
	}

	if obj.Spec.UpdateStrategy.Type == "" {
		obj.Spec.UpdateStrategy.Type = v1beta1.RollingUpdateDaemonSetStrategyType
	}
	if obj.Spec.UpdateStrategy.Type == v1beta1.RollingUpdateDaemonSetStrategyType {
		if obj.Spec.UpdateStrategy.RollingUpdate == nil {
			obj.Spec.UpdateStrategy.RollingUpdate = &v1beta1.RollingUpdateDaemonSet{}
		}

		// Default to Standard
		if obj.Spec.UpdateStrategy.RollingUpdate.Type == "" {
			obj.Spec.UpdateStrategy.RollingUpdate.Type = v1beta1.StandardRollingUpdateType
		}

		if obj.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable == nil && obj.Spec.UpdateStrategy.RollingUpdate.MaxSurge == nil {
			maxUnavailable := intstr.FromInt(1)
			obj.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = &maxUnavailable
			MaxSurge := intstr.FromInt(0)
			obj.Spec.UpdateStrategy.RollingUpdate.MaxSurge = &MaxSurge
		}
	}

	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = ptr.To(int32(10))
	}
}

func setDefaultContainerV1beta1(sidecarContainer *v1beta1.SidecarContainer) {
	container := &sidecarContainer.Container
	v1.SetDefaults_Container(container)
	for i := range container.Ports {
		p := &container.Ports[i]
		if p.Protocol == "" {
			p.Protocol = "TCP"
		}
	}
	for i := range sidecarContainer.TransferEnv {
		tEnv := &sidecarContainer.TransferEnv[i]
		if tEnv.SourceContainerNameFrom != nil {
			v1.SetDefaults_ObjectFieldSelector(tEnv.SourceContainerNameFrom.FieldRef)
		}
	}
	for i := range container.Env {
		e := &container.Env[i]
		if e.ValueFrom != nil {
			if e.ValueFrom.FieldRef != nil {
				v1.SetDefaults_ObjectFieldSelector(e.ValueFrom.FieldRef)
			}
		}
	}
	v1.SetDefaults_ResourceList(&container.Resources.Limits)
	v1.SetDefaults_ResourceList(&container.Resources.Requests)
	if container.LivenessProbe != nil {
		v1.SetDefaults_Probe(container.LivenessProbe)
		if container.LivenessProbe.ProbeHandler.HTTPGet != nil {
			v1.SetDefaults_HTTPGetAction(container.LivenessProbe.ProbeHandler.HTTPGet)
		}
	}
	if container.ReadinessProbe != nil {
		v1.SetDefaults_Probe(container.ReadinessProbe)
		if container.ReadinessProbe.ProbeHandler.HTTPGet != nil {
			v1.SetDefaults_HTTPGetAction(container.ReadinessProbe.ProbeHandler.HTTPGet)
		}
	}
	if container.Lifecycle != nil {
		if container.Lifecycle.PostStart != nil {
			if container.Lifecycle.PostStart.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(container.Lifecycle.PostStart.HTTPGet)
			}
		}
		if container.Lifecycle.PreStop != nil {
			if container.Lifecycle.PreStop.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(container.Lifecycle.PreStop.HTTPGet)
			}
		}
	}
}

func SetDefaultRevisionHistoryLimitV1beta1(revisionHistoryLimit **int32) {
	if *revisionHistoryLimit == nil {
		*revisionHistoryLimit = ptr.To(int32(10))
	}
}

func SetHashSidecarSetV1beta1(sidecarset *v1beta1.SidecarSet) error {
	if sidecarset.Annotations == nil {
		sidecarset.Annotations = make(map[string]string)
	}

	hash, err := sidecarcontrol.SidecarSetHashV1beta1(sidecarset)
	if err != nil {
		return err
	}
	sidecarset.Annotations[sidecarcontrol.SidecarSetHashAnnotation] = hash

	hash, err = sidecarcontrol.SidecarSetHashWithoutImageV1beta1(sidecarset)
	if err != nil {
		return err
	}
	sidecarset.Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = hash

	return nil
}

// SetDefaultsSidecarSetV1beta1 set default values for SidecarSet v1beta1.
func SetDefaultsSidecarSetV1beta1(obj *v1beta1.SidecarSet) {
	setSidecarSetUpdateStrategyV1beta1(&obj.Spec.UpdateStrategy)

	for i := range obj.Spec.InitContainers {
		setDefaultSidecarContainerV1beta1(&obj.Spec.InitContainers[i], v1beta1.AfterAppContainerType)
	}

	for i := range obj.Spec.Containers {
		setDefaultSidecarContainerV1beta1(&obj.Spec.Containers[i], v1beta1.BeforeAppContainerType)
	}

	// default setting volumes
	SetDefaultPodVolumes(obj.Spec.Volumes)

	// default setting history revision limitation
	SetDefaultRevisionHistoryLimitV1beta1(&obj.Spec.RevisionHistoryLimit)

	// default patchPolicy is 'Retain'
	for i := range obj.Spec.PatchPodMetadata {
		patch := &obj.Spec.PatchPodMetadata[i]
		if patch.PatchPolicy == "" {
			patch.PatchPolicy = v1beta1.SidecarSetRetainPatchPolicy
		}
	}

	// default setting injectRevisionStrategy
	SetDefaultInjectRevisionV1beta1(&obj.Spec.InjectionStrategy)
}

func SetDefaultInjectRevisionV1beta1(strategy *v1beta1.SidecarSetInjectionStrategy) {
	if strategy.Revision != nil && strategy.Revision.Policy == "" {
		strategy.Revision.Policy = v1beta1.AlwaysSidecarSetInjectRevisionPolicy
	}
}

func setDefaultSidecarContainerV1beta1(sidecarContainer *v1beta1.SidecarContainer, injectPolicy v1beta1.PodInjectPolicyType) {
	if sidecarContainer.PodInjectPolicy == "" {
		sidecarContainer.PodInjectPolicy = injectPolicy
	}

	if sidecarContainer.UpgradeStrategy.UpgradeType == "" {
		sidecarContainer.UpgradeStrategy.UpgradeType = v1beta1.SidecarContainerColdUpgrade
	}
	if sidecarContainer.ShareVolumePolicy.Type == "" {
		sidecarContainer.ShareVolumePolicy.Type = v1beta1.ShareVolumePolicyDisabled
	}

	setDefaultContainerV1beta1(sidecarContainer)
}

func setSidecarSetUpdateStrategyV1beta1(strategy *v1beta1.SidecarSetUpdateStrategy) {
	if strategy.Type == "" {
		strategy.Type = v1beta1.RollingUpdateSidecarSetStrategyType
	}
	if strategy.MaxUnavailable == nil {
		maxUnavailable := intstr.FromInt(1)
		strategy.MaxUnavailable = &maxUnavailable
	}
	if strategy.Partition == nil {
		strategy.Partition = &intstr.IntOrString{Type: intstr.Int, IntVal: 0}
	}
}

// SetDefaultsCloneSetV1beta1 sets default values for v1beta1 CloneSet.
func SetDefaultsCloneSetV1beta1(obj *v1beta1.CloneSet, injectTemplateDefaults bool) {
	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = ptr.To(int32(1))
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = ptr.To(int32(10))
	}

	// For v1beta1, set DisablePVCReuse default to true (safer default)
	// This is only applied during Create operations by the webhook
	// Note: v1alpha1 keeps default as false for backward compatibility

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

	switch obj.Spec.UpdateStrategy.Type {
	case "":
		obj.Spec.UpdateStrategy.Type = v1beta1.RecreateCloneSetUpdateStrategyType
	case v1beta1.InPlaceIfPossibleCloneSetUpdateStrategyType, v1beta1.InPlaceOnlyCloneSetUpdateStrategyType:
		if obj.Spec.UpdateStrategy.InPlaceUpdateStrategy == nil {
			obj.Spec.UpdateStrategy.InPlaceUpdateStrategy = &appspub.InPlaceUpdateStrategy{}
		}
	}

	if obj.Spec.UpdateStrategy.Partition == nil {
		partition := intstr.FromInt(0)
		obj.Spec.UpdateStrategy.Partition = &partition
	}
	if obj.Spec.UpdateStrategy.MaxUnavailable == nil {
		maxUnavailable := intstr.FromString(v1beta1.DefaultCloneSetMaxUnavailable)
		obj.Spec.UpdateStrategy.MaxUnavailable = &maxUnavailable
	}
	if obj.Spec.UpdateStrategy.MaxSurge == nil {
		maxSurge := intstr.FromInt(0)
		obj.Spec.UpdateStrategy.MaxSurge = &maxSurge
	}
}
