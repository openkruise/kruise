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
	"github.com/openkruise/kruise/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	v1 "k8s.io/kubernetes/pkg/apis/core/v1"
	utilpointer "k8s.io/utils/pointer"
)

// SetDefaults_SidecarSet set default values for SidecarSet.
func SetDefaultsSidecarSet(obj *v1alpha1.SidecarSet) {
	setSidecarSetUpdateStratety(&obj.Spec.UpdateStrategy)

	for i := range obj.Spec.InitContainers {
		setSidecarDefaultContainer(&obj.Spec.InitContainers[i])
	}

	for i := range obj.Spec.Containers {
		setDefaultSidecarContainer(&obj.Spec.Containers[i])
	}

	//default setting volumes
	SetDefaultPodVolumes(obj.Spec.Volumes)
}

func setDefaultSidecarContainer(sidecarContainer *v1alpha1.SidecarContainer) {
	if sidecarContainer.PodInjectPolicy == "" {
		sidecarContainer.PodInjectPolicy = v1alpha1.BeforeAppContainerType
	}
	if sidecarContainer.UpgradeStrategy.UpgradeType == "" {
		sidecarContainer.UpgradeStrategy.UpgradeType = v1alpha1.SidecarContainerColdUpgrade
	}
	if sidecarContainer.ShareVolumePolicy.Type == "" {
		sidecarContainer.ShareVolumePolicy.Type = v1alpha1.ShareVolumePolicyDisabled
	}

	setSidecarDefaultContainer(sidecarContainer)
}

func setSidecarSetUpdateStratety(strategy *v1alpha1.SidecarSetUpdateStrategy) {
	if strategy.Type == "" {
		strategy.Type = v1alpha1.RollingUpdateSidecarSetStrategyType
	}
	if strategy.MaxUnavailable == nil {
		maxUnavailable := intstr.FromInt(1)
		strategy.MaxUnavailable = &maxUnavailable
	}
	if strategy.Partition == nil {
		partition := intstr.FromInt(0)
		strategy.Partition = &partition
	}
}

func setSidecarDefaultContainer(sidecarContainer *v1alpha1.SidecarContainer) {
	container := &sidecarContainer.Container
	v1.SetDefaults_Container(container)
	for i := range container.Ports {
		p := &container.Ports[i]
		v1.SetDefaults_ContainerPort(p)
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
		if container.LivenessProbe.Handler.HTTPGet != nil {
			v1.SetDefaults_HTTPGetAction(container.LivenessProbe.Handler.HTTPGet)
		}
	}
	if container.ReadinessProbe != nil {
		v1.SetDefaults_Probe(container.ReadinessProbe)
		if container.ReadinessProbe.Handler.HTTPGet != nil {
			v1.SetDefaults_HTTPGetAction(container.ReadinessProbe.Handler.HTTPGet)
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

// SetDefaults_AdvancedCronJob set default values for BroadcastJob.
func SetDefaultsAdvancedCronJob(obj *v1alpha1.AdvancedCronJob, injectTemplateDefaults bool) {
	if obj.Spec.Template.JobTemplate != nil && injectTemplateDefaults {
		SetDefaultPodSpec(&obj.Spec.Template.JobTemplate.Spec.Template.Spec)
	}

	if obj.Spec.Template.BroadcastJobTemplate != nil && injectTemplateDefaults {
		SetDefaultPodSpec(&obj.Spec.Template.BroadcastJobTemplate.Spec.Template.Spec)
	}

	if obj.Spec.ConcurrencyPolicy == "" {
		obj.Spec.ConcurrencyPolicy = v1alpha1.AllowConcurrent
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

// SetDefaults_BroadcastJob set default values for BroadcastJob.
func SetDefaultsBroadcastJob(obj *v1alpha1.BroadcastJob, injectTemplateDefaults bool) {
	if injectTemplateDefaults {
		SetDefaultPodSpec(&obj.Spec.Template.Spec)
	}
	if obj.Spec.CompletionPolicy.Type == "" {
		obj.Spec.CompletionPolicy.Type = v1alpha1.Always
	}

	if obj.Spec.Parallelism == nil {
		parallelism := int32(1<<31 - 1)
		parallelismIntStr := intstr.FromInt(int(parallelism))
		obj.Spec.Parallelism = &parallelismIntStr
	}

	if obj.Spec.FailurePolicy.Type == "" {
		obj.Spec.FailurePolicy.Type = v1alpha1.FailurePolicyTypeFailFast
	}
}

// SetDefaults_UnitedDeployment set default values for UnitedDeployment.
func SetDefaultsUnitedDeployment(obj *v1alpha1.UnitedDeployment, injectTemplateDefaults bool) {
	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = utilpointer.Int32Ptr(1)
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = utilpointer.Int32Ptr(10)
	}

	if len(obj.Spec.UpdateStrategy.Type) == 0 {
		obj.Spec.UpdateStrategy.Type = v1alpha1.ManualUpdateStrategyType
	}

	if obj.Spec.UpdateStrategy.Type == v1alpha1.ManualUpdateStrategyType && obj.Spec.UpdateStrategy.ManualUpdate == nil {
		obj.Spec.UpdateStrategy.ManualUpdate = &v1alpha1.ManualUpdate{}
	}

	if obj.Spec.Template.StatefulSetTemplate != nil {
		if injectTemplateDefaults {
			SetDefaultPodSpec(&obj.Spec.Template.StatefulSetTemplate.Spec.Template.Spec)
			for i := range obj.Spec.Template.StatefulSetTemplate.Spec.VolumeClaimTemplates {
				a := &obj.Spec.Template.StatefulSetTemplate.Spec.VolumeClaimTemplates[i]
				v1.SetDefaults_PersistentVolumeClaim(a)
				v1.SetDefaults_ResourceList(&a.Spec.Resources.Limits)
				v1.SetDefaults_ResourceList(&a.Spec.Resources.Requests)
				v1.SetDefaults_ResourceList(&a.Status.Capacity)
			}
		}
	}
}

// SetDefaults_CloneSet set default values for CloneSet.
func SetDefaultsCloneSet(obj *v1alpha1.CloneSet, injectTemplateDefaults bool) {
	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = utilpointer.Int32Ptr(1)
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = utilpointer.Int32Ptr(10)
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

	switch obj.Spec.UpdateStrategy.Type {
	case "":
		obj.Spec.UpdateStrategy.Type = v1alpha1.RecreateCloneSetUpdateStrategyType
	case v1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType, v1alpha1.InPlaceOnlyCloneSetUpdateStrategyType:
		if obj.Spec.UpdateStrategy.InPlaceUpdateStrategy == nil {
			obj.Spec.UpdateStrategy.InPlaceUpdateStrategy = &appspub.InPlaceUpdateStrategy{}
		}
	}

	if obj.Spec.UpdateStrategy.Partition == nil {
		partition := intstr.FromInt(0)
		obj.Spec.UpdateStrategy.Partition = &partition
	}
	if obj.Spec.UpdateStrategy.MaxUnavailable == nil {
		maxUnavailable := intstr.FromString(v1alpha1.DefaultCloneSetMaxUnavailable)
		obj.Spec.UpdateStrategy.MaxUnavailable = &maxUnavailable
	}
	if obj.Spec.UpdateStrategy.MaxSurge == nil {
		maxSurge := intstr.FromInt(0)
		obj.Spec.UpdateStrategy.MaxSurge = &maxSurge
	}
}

// SetDefaults_DaemonSet set default values for DaemonSet.
func SetDefaultsDaemonSet(obj *v1alpha1.DaemonSet) {
	if obj.Spec.BurstReplicas == nil {
		BurstReplicas := intstr.FromInt(250)
		obj.Spec.BurstReplicas = &BurstReplicas
	}

	if obj.Spec.UpdateStrategy.Type == "" {
		obj.Spec.UpdateStrategy.Type = v1alpha1.RollingUpdateDaemonSetStrategyType

		// UpdateStrategy.RollingUpdate will take default values below.
		obj.Spec.UpdateStrategy.RollingUpdate = &v1alpha1.RollingUpdateDaemonSet{}
	}

	if obj.Spec.UpdateStrategy.Type == v1alpha1.RollingUpdateDaemonSetStrategyType {
		if obj.Spec.UpdateStrategy.RollingUpdate == nil {
			obj.Spec.UpdateStrategy.RollingUpdate = &v1alpha1.RollingUpdateDaemonSet{}
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.Partition == nil {
			obj.Spec.UpdateStrategy.RollingUpdate.Partition = new(int32)
			*obj.Spec.UpdateStrategy.RollingUpdate.Partition = 0
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable == nil {
			maxUnavailable := intstr.FromInt(1)
			obj.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = &maxUnavailable
		}

		if obj.Spec.UpdateStrategy.RollingUpdate.Type == "" {
			obj.Spec.UpdateStrategy.RollingUpdate.Type = v1alpha1.StandardRollingUpdateType
		}
		// Only when RollingUpdate Type is SurgingRollingUpdateType, it need to initialize the MaxSurge.
		if obj.Spec.UpdateStrategy.RollingUpdate.Type == v1alpha1.SurgingRollingUpdateType {
			if obj.Spec.UpdateStrategy.RollingUpdate.MaxSurge == nil {
				MaxSurge := intstr.FromInt(1)
				obj.Spec.UpdateStrategy.RollingUpdate.MaxSurge = &MaxSurge
			}
		}
	}

	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = new(int32)
		*obj.Spec.RevisionHistoryLimit = 10
	}
}

// SetDefaultPod sets default pod
func SetDefaultPod(in *corev1.Pod) {
	SetDefaultPodSpec(&in.Spec)
	if in.Spec.EnableServiceLinks == nil {
		enableServiceLinks := corev1.DefaultEnableServiceLinks
		in.Spec.EnableServiceLinks = &enableServiceLinks
	}
}

// SetDefaults_NodeImage set default values for NodeImage.
func SetDefaultsNodeImage(obj *v1alpha1.NodeImage) {
	now := metav1.Now()
	for name, imageSpec := range obj.Spec.Images {
		for i := range imageSpec.Tags {
			tagSpec := &imageSpec.Tags[i]
			if tagSpec.CreatedAt == nil {
				tagSpec.CreatedAt = &now
			}
			if tagSpec.PullPolicy == nil {
				tagSpec.PullPolicy = &v1alpha1.ImageTagPullPolicy{}
			}
			SetDefaultsImageTagPullPolicy(tagSpec.PullPolicy)
		}
		obj.Spec.Images[name] = imageSpec
	}
}

func SetDefaultsImageTagPullPolicy(obj *v1alpha1.ImageTagPullPolicy) {
	if obj.TimeoutSeconds == nil {
		obj.TimeoutSeconds = utilpointer.Int32Ptr(600)
	}
	if obj.BackoffLimit == nil {
		obj.BackoffLimit = utilpointer.Int32Ptr(3)
	}
}

// SetDefaults_ImagePullJob set default values for ImagePullJob.
func SetDefaultsImagePullJob(obj *v1alpha1.ImagePullJob) {
	if obj.Spec.CompletionPolicy.Type == "" {
		obj.Spec.CompletionPolicy.Type = v1alpha1.Always
	}
	if obj.Spec.PullPolicy == nil {
		obj.Spec.PullPolicy = &v1alpha1.PullPolicy{}
	}
	if obj.Spec.PullPolicy.TimeoutSeconds == nil {
		obj.Spec.PullPolicy.TimeoutSeconds = utilpointer.Int32Ptr(600)
	}
	if obj.Spec.PullPolicy.BackoffLimit == nil {
		obj.Spec.PullPolicy.BackoffLimit = utilpointer.Int32Ptr(3)
	}
}
