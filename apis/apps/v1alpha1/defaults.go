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

package v1alpha1

import (
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	v1 "k8s.io/kubernetes/pkg/apis/core/v1"
	utilpointer "k8s.io/utils/pointer"
)

// SetDefaults_SidecarSet set default values for SidecarSet.
func SetDefaultsSidecarSet(obj *SidecarSet) {
	setSidecarSetUpdateStratety(&obj.Spec.Strategy)

	for i := range obj.Spec.InitContainers {
		setSidecarDefaultContainer(&obj.Spec.InitContainers[i])
	}

	for i := range obj.Spec.Containers {
		setSidecarDefaultContainer(&obj.Spec.Containers[i])
	}
}

func setSidecarSetUpdateStratety(strategy *SidecarSetUpdateStrategy) {
	if strategy.RollingUpdate == nil {
		rollingUpdate := RollingUpdateSidecarSet{}
		strategy.RollingUpdate = &rollingUpdate
	}
	if strategy.RollingUpdate.MaxUnavailable == nil {
		maxUnavailable := intstr.FromInt(1)
		strategy.RollingUpdate.MaxUnavailable = &maxUnavailable
	}
}

func setSidecarDefaultContainer(sidecarContainer *SidecarContainer) {
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
func SetDefaultsAdvancedCronJob(obj *AdvancedCronJob) {
	if obj.Spec.Template.JobTemplate != nil {
		SetDefaultPodSpec(&obj.Spec.Template.JobTemplate.Spec.Template.Spec)
	}

	if obj.Spec.Template.BroadcastJobTemplate != nil {
		SetDefaultPodSpec(&obj.Spec.Template.BroadcastJobTemplate.Spec.Template.Spec)
	}

	if obj.Spec.ConcurrencyPolicy == "" {
		obj.Spec.ConcurrencyPolicy = AllowConcurrent
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
func SetDefaultsBroadcastJob(obj *BroadcastJob) {
	SetDefaultPodSpec(&obj.Spec.Template.Spec)
	if obj.Spec.CompletionPolicy.Type == "" {
		obj.Spec.CompletionPolicy.Type = Always
	}

	if obj.Spec.Parallelism == nil {
		parallelism := int32(1<<31 - 1)
		parallelismIntStr := intstr.FromInt(int(parallelism))
		obj.Spec.Parallelism = &parallelismIntStr
	}

	if obj.Spec.FailurePolicy.Type == "" {
		obj.Spec.FailurePolicy.Type = FailurePolicyTypeFailFast
	}
}

// SetDefaults_StatefulSet set default values for StatefulSet.
func SetDefaultsStatefulSet(obj *StatefulSet) {
	if len(obj.Spec.PodManagementPolicy) == 0 {
		obj.Spec.PodManagementPolicy = appsv1.OrderedReadyPodManagement
	}

	if obj.Spec.UpdateStrategy.Type == "" {
		obj.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType

		// UpdateStrategy.RollingUpdate will take default values below.
		obj.Spec.UpdateStrategy.RollingUpdate = &RollingUpdateStatefulSetStrategy{}
	}

	if obj.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType {
		if obj.Spec.UpdateStrategy.RollingUpdate == nil {
			obj.Spec.UpdateStrategy.RollingUpdate = &RollingUpdateStatefulSetStrategy{}
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.Partition == nil {
			obj.Spec.UpdateStrategy.RollingUpdate.Partition = utilpointer.Int32Ptr(0)
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable == nil {
			maxUnavailable := intstr.FromInt(1)
			obj.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = &maxUnavailable
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy == "" {
			obj.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy = RecreatePodUpdateStrategyType
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.MinReadySeconds == nil {
			obj.Spec.UpdateStrategy.RollingUpdate.MinReadySeconds = utilpointer.Int32Ptr(0)
		}
	}

	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = utilpointer.Int32Ptr(1)
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = utilpointer.Int32Ptr(10)
	}

	SetDefaultPodSpec(&obj.Spec.Template.Spec)
	for i := range obj.Spec.VolumeClaimTemplates {
		a := &obj.Spec.VolumeClaimTemplates[i]
		v1.SetDefaults_PersistentVolumeClaim(a)
		v1.SetDefaults_ResourceList(&a.Spec.Resources.Limits)
		v1.SetDefaults_ResourceList(&a.Spec.Resources.Requests)
		v1.SetDefaults_ResourceList(&a.Status.Capacity)
	}
}

// SetDefaults_UnitedDeployment set default values for UnitedDeployment.
func SetDefaultsUnitedDeployment(obj *UnitedDeployment) {
	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = utilpointer.Int32Ptr(1)
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = utilpointer.Int32Ptr(10)
	}

	if len(obj.Spec.UpdateStrategy.Type) == 0 {
		obj.Spec.UpdateStrategy.Type = ManualUpdateStrategyType
	}

	if obj.Spec.UpdateStrategy.Type == ManualUpdateStrategyType && obj.Spec.UpdateStrategy.ManualUpdate == nil {
		obj.Spec.UpdateStrategy.ManualUpdate = &ManualUpdate{}
	}

	if obj.Spec.Template.StatefulSetTemplate != nil {
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

// SetDefaults_CloneSet set default values for CloneSet.
func SetDefaultsCloneSet(obj *CloneSet) {
	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = utilpointer.Int32Ptr(1)
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = utilpointer.Int32Ptr(10)
	}

	SetDefaultPodSpec(&obj.Spec.Template.Spec)
	for i := range obj.Spec.VolumeClaimTemplates {
		a := &obj.Spec.VolumeClaimTemplates[i]
		v1.SetDefaults_PersistentVolumeClaim(a)
		v1.SetDefaults_ResourceList(&a.Spec.Resources.Limits)
		v1.SetDefaults_ResourceList(&a.Spec.Resources.Requests)
		v1.SetDefaults_ResourceList(&a.Status.Capacity)
	}

	switch obj.Spec.UpdateStrategy.Type {
	case "":
		obj.Spec.UpdateStrategy.Type = RecreateCloneSetUpdateStrategyType
	case InPlaceIfPossibleCloneSetUpdateStrategyType, InPlaceOnlyCloneSetUpdateStrategyType:
		if obj.Spec.UpdateStrategy.InPlaceUpdateStrategy == nil {
			obj.Spec.UpdateStrategy.InPlaceUpdateStrategy = &appspub.InPlaceUpdateStrategy{}
		}
	}

	if obj.Spec.UpdateStrategy.Partition == nil {
		partition := intstr.FromInt(0)
		obj.Spec.UpdateStrategy.Partition = &partition
	}
	if obj.Spec.UpdateStrategy.MaxUnavailable == nil {
		maxUnavailable := intstr.FromString(DefaultCloneSetMaxUnavailable)
		obj.Spec.UpdateStrategy.MaxUnavailable = &maxUnavailable
	}
	if obj.Spec.UpdateStrategy.MaxSurge == nil {
		maxSurge := intstr.FromInt(0)
		obj.Spec.UpdateStrategy.MaxSurge = &maxSurge
	}
}

// SetDefaults_DaemonSet set default values for DaemonSet.
func SetDefaultsDaemonSet(obj *DaemonSet) {
	if obj.Spec.BurstReplicas == nil {
		BurstReplicas := intstr.FromInt(250)
		obj.Spec.BurstReplicas = &BurstReplicas
	}

	if obj.Spec.UpdateStrategy.Type == "" {
		obj.Spec.UpdateStrategy.Type = RollingUpdateDaemonSetStrategyType

		// UpdateStrategy.RollingUpdate will take default values below.
		obj.Spec.UpdateStrategy.RollingUpdate = &RollingUpdateDaemonSet{}
	}

	if obj.Spec.UpdateStrategy.Type == RollingUpdateDaemonSetStrategyType {
		if obj.Spec.UpdateStrategy.RollingUpdate == nil {
			obj.Spec.UpdateStrategy.RollingUpdate = &RollingUpdateDaemonSet{}
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
			obj.Spec.UpdateStrategy.RollingUpdate.Type = StandardRollingUpdateType
		}
		// Only when RollingUpdate Type is SurgingRollingUpdateType, it need to initialize the MaxSurge.
		if obj.Spec.UpdateStrategy.RollingUpdate.Type == SurgingRollingUpdateType {
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

// SetDefaultPodSpec sets default pod spec
func SetDefaultPodSpec(in *corev1.PodSpec) {
	v1.SetDefaults_PodSpec(in)
	for i := range in.Volumes {
		a := &in.Volumes[i]
		v1.SetDefaults_Volume(a)
		if a.VolumeSource.HostPath != nil {
			v1.SetDefaults_HostPathVolumeSource(a.VolumeSource.HostPath)
		}
		if a.VolumeSource.Secret != nil {
			v1.SetDefaults_SecretVolumeSource(a.VolumeSource.Secret)
		}
		if a.VolumeSource.ISCSI != nil {
			v1.SetDefaults_ISCSIVolumeSource(a.VolumeSource.ISCSI)
		}
		if a.VolumeSource.RBD != nil {
			v1.SetDefaults_RBDVolumeSource(a.VolumeSource.RBD)
		}
		if a.VolumeSource.DownwardAPI != nil {
			v1.SetDefaults_DownwardAPIVolumeSource(a.VolumeSource.DownwardAPI)
			for j := range a.VolumeSource.DownwardAPI.Items {
				b := &a.VolumeSource.DownwardAPI.Items[j]
				if b.FieldRef != nil {
					v1.SetDefaults_ObjectFieldSelector(b.FieldRef)
				}
			}
		}
		if a.VolumeSource.ConfigMap != nil {
			v1.SetDefaults_ConfigMapVolumeSource(a.VolumeSource.ConfigMap)
		}
		if a.VolumeSource.AzureDisk != nil {
			v1.SetDefaults_AzureDiskVolumeSource(a.VolumeSource.AzureDisk)
		}
		if a.VolumeSource.Projected != nil {
			v1.SetDefaults_ProjectedVolumeSource(a.VolumeSource.Projected)
			for j := range a.VolumeSource.Projected.Sources {
				b := &a.VolumeSource.Projected.Sources[j]
				if b.DownwardAPI != nil {
					for k := range b.DownwardAPI.Items {
						c := &b.DownwardAPI.Items[k]
						if c.FieldRef != nil {
							v1.SetDefaults_ObjectFieldSelector(c.FieldRef)
						}
					}
				}
				if b.ServiceAccountToken != nil {
					v1.SetDefaults_ServiceAccountTokenProjection(b.ServiceAccountToken)
				}
			}
		}
		if a.VolumeSource.ScaleIO != nil {
			v1.SetDefaults_ScaleIOVolumeSource(a.VolumeSource.ScaleIO)
		}
	}
	for i := range in.InitContainers {
		a := &in.InitContainers[i]
		v1.SetDefaults_Container(a)
		for j := range a.Ports {
			b := &a.Ports[j]
			v1.SetDefaults_ContainerPort(b)
		}
		for j := range a.Env {
			b := &a.Env[j]
			if b.ValueFrom != nil {
				if b.ValueFrom.FieldRef != nil {
					v1.SetDefaults_ObjectFieldSelector(b.ValueFrom.FieldRef)
				}
			}
		}
		v1.SetDefaults_ResourceList(&a.Resources.Limits)
		v1.SetDefaults_ResourceList(&a.Resources.Requests)
		if a.LivenessProbe != nil {
			v1.SetDefaults_Probe(a.LivenessProbe)
			if a.LivenessProbe.Handler.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(a.LivenessProbe.Handler.HTTPGet)
			}
		}
		if a.ReadinessProbe != nil {
			v1.SetDefaults_Probe(a.ReadinessProbe)
			if a.ReadinessProbe.Handler.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(a.ReadinessProbe.Handler.HTTPGet)
			}
		}
		if a.Lifecycle != nil {
			if a.Lifecycle.PostStart != nil {
				if a.Lifecycle.PostStart.HTTPGet != nil {
					v1.SetDefaults_HTTPGetAction(a.Lifecycle.PostStart.HTTPGet)
				}
			}
			if a.Lifecycle.PreStop != nil {
				if a.Lifecycle.PreStop.HTTPGet != nil {
					v1.SetDefaults_HTTPGetAction(a.Lifecycle.PreStop.HTTPGet)
				}
			}
		}
	}
	for i := range in.Containers {
		a := &in.Containers[i]
		// For in-place update, we set default imagePullPolicy to Always
		if a.ImagePullPolicy == "" {
			a.ImagePullPolicy = corev1.PullAlways
		}
		v1.SetDefaults_Container(a)
		for j := range a.Ports {
			b := &a.Ports[j]
			v1.SetDefaults_ContainerPort(b)
		}
		for j := range a.Env {
			b := &a.Env[j]
			if b.ValueFrom != nil {
				if b.ValueFrom.FieldRef != nil {
					v1.SetDefaults_ObjectFieldSelector(b.ValueFrom.FieldRef)
				}
			}
		}
		v1.SetDefaults_ResourceList(&a.Resources.Limits)
		v1.SetDefaults_ResourceList(&a.Resources.Requests)
		if a.LivenessProbe != nil {
			v1.SetDefaults_Probe(a.LivenessProbe)
			if a.LivenessProbe.Handler.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(a.LivenessProbe.Handler.HTTPGet)
			}
		}
		if a.ReadinessProbe != nil {
			v1.SetDefaults_Probe(a.ReadinessProbe)
			if a.ReadinessProbe.Handler.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(a.ReadinessProbe.Handler.HTTPGet)
			}
		}
		if a.Lifecycle != nil {
			if a.Lifecycle.PostStart != nil {
				if a.Lifecycle.PostStart.HTTPGet != nil {
					v1.SetDefaults_HTTPGetAction(a.Lifecycle.PostStart.HTTPGet)
				}
			}
			if a.Lifecycle.PreStop != nil {
				if a.Lifecycle.PreStop.HTTPGet != nil {
					v1.SetDefaults_HTTPGetAction(a.Lifecycle.PreStop.HTTPGet)
				}
			}
		}
	}
}
