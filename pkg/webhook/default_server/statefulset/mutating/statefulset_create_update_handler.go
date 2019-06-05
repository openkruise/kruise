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

package mutating

import (
	"context"
	"net/http"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

func init() {
	webhookName := "mutating-create-update-statefulset"
	if HandlerMap[webhookName] == nil {
		HandlerMap[webhookName] = []admission.Handler{}
	}
	HandlerMap[webhookName] = append(HandlerMap[webhookName], &StatefulSetCreateUpdateHandler{})
}

// StatefulSetCreateUpdateHandler handles StatefulSet
type StatefulSetCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder types.Decoder
}

func (h *StatefulSetCreateUpdateHandler) mutatingStatefulSetFn(ctx context.Context, obj *appsv1alpha1.StatefulSet) error {
	// TODO(user): implement your admission logic
	return nil
}

var _ admission.Handler = &StatefulSetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *StatefulSetCreateUpdateHandler) Handle(ctx context.Context, req types.Request) types.Response {
	obj := &appsv1alpha1.StatefulSet{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	copy := obj.DeepCopy()

	SetObjectDefaults(copy)
	obj.Status = appsv1alpha1.StatefulSetStatus{}

	err = h.mutatingStatefulSetFn(ctx, copy)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	resp := admission.PatchResponse(obj, copy)
	return resp
}

func SetObjectDefaults(in *appsv1alpha1.StatefulSet) {
	setDefaults(in)
	corev1.SetDefaults_PodSpec(&in.Spec.Template.Spec)
	for i := range in.Spec.Template.Spec.Volumes {
		a := &in.Spec.Template.Spec.Volumes[i]
		corev1.SetDefaults_Volume(a)
		if a.VolumeSource.HostPath != nil {
			corev1.SetDefaults_HostPathVolumeSource(a.VolumeSource.HostPath)
		}
		if a.VolumeSource.Secret != nil {
			corev1.SetDefaults_SecretVolumeSource(a.VolumeSource.Secret)
		}
		if a.VolumeSource.ISCSI != nil {
			corev1.SetDefaults_ISCSIVolumeSource(a.VolumeSource.ISCSI)
		}
		if a.VolumeSource.RBD != nil {
			corev1.SetDefaults_RBDVolumeSource(a.VolumeSource.RBD)
		}
		if a.VolumeSource.DownwardAPI != nil {
			corev1.SetDefaults_DownwardAPIVolumeSource(a.VolumeSource.DownwardAPI)
			for j := range a.VolumeSource.DownwardAPI.Items {
				b := &a.VolumeSource.DownwardAPI.Items[j]
				if b.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(b.FieldRef)
				}
			}
		}
		if a.VolumeSource.ConfigMap != nil {
			corev1.SetDefaults_ConfigMapVolumeSource(a.VolumeSource.ConfigMap)
		}
		if a.VolumeSource.AzureDisk != nil {
			corev1.SetDefaults_AzureDiskVolumeSource(a.VolumeSource.AzureDisk)
		}
		if a.VolumeSource.Projected != nil {
			corev1.SetDefaults_ProjectedVolumeSource(a.VolumeSource.Projected)
			for j := range a.VolumeSource.Projected.Sources {
				b := &a.VolumeSource.Projected.Sources[j]
				if b.DownwardAPI != nil {
					for k := range b.DownwardAPI.Items {
						c := &b.DownwardAPI.Items[k]
						if c.FieldRef != nil {
							corev1.SetDefaults_ObjectFieldSelector(c.FieldRef)
						}
					}
				}
				if b.ServiceAccountToken != nil {
					corev1.SetDefaults_ServiceAccountTokenProjection(b.ServiceAccountToken)
				}
			}
		}
		if a.VolumeSource.ScaleIO != nil {
			corev1.SetDefaults_ScaleIOVolumeSource(a.VolumeSource.ScaleIO)
		}
	}
	for i := range in.Spec.Template.Spec.InitContainers {
		a := &in.Spec.Template.Spec.InitContainers[i]
		corev1.SetDefaults_Container(a)
		for j := range a.Ports {
			b := &a.Ports[j]
			corev1.SetDefaults_ContainerPort(b)
		}
		for j := range a.Env {
			b := &a.Env[j]
			if b.ValueFrom != nil {
				if b.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(b.ValueFrom.FieldRef)
				}
			}
		}
		corev1.SetDefaults_ResourceList(&a.Resources.Limits)
		corev1.SetDefaults_ResourceList(&a.Resources.Requests)
		if a.LivenessProbe != nil {
			corev1.SetDefaults_Probe(a.LivenessProbe)
			if a.LivenessProbe.Handler.HTTPGet != nil {
				corev1.SetDefaults_HTTPGetAction(a.LivenessProbe.Handler.HTTPGet)
			}
		}
		if a.ReadinessProbe != nil {
			corev1.SetDefaults_Probe(a.ReadinessProbe)
			if a.ReadinessProbe.Handler.HTTPGet != nil {
				corev1.SetDefaults_HTTPGetAction(a.ReadinessProbe.Handler.HTTPGet)
			}
		}
		if a.Lifecycle != nil {
			if a.Lifecycle.PostStart != nil {
				if a.Lifecycle.PostStart.HTTPGet != nil {
					corev1.SetDefaults_HTTPGetAction(a.Lifecycle.PostStart.HTTPGet)
				}
			}
			if a.Lifecycle.PreStop != nil {
				if a.Lifecycle.PreStop.HTTPGet != nil {
					corev1.SetDefaults_HTTPGetAction(a.Lifecycle.PreStop.HTTPGet)
				}
			}
		}
	}
	for i := range in.Spec.Template.Spec.Containers {
		a := &in.Spec.Template.Spec.Containers[i]
		corev1.SetDefaults_Container(a)
		for j := range a.Ports {
			b := &a.Ports[j]
			corev1.SetDefaults_ContainerPort(b)
		}
		for j := range a.Env {
			b := &a.Env[j]
			if b.ValueFrom != nil {
				if b.ValueFrom.FieldRef != nil {
					corev1.SetDefaults_ObjectFieldSelector(b.ValueFrom.FieldRef)
				}
			}
		}
		corev1.SetDefaults_ResourceList(&a.Resources.Limits)
		corev1.SetDefaults_ResourceList(&a.Resources.Requests)
		if a.LivenessProbe != nil {
			corev1.SetDefaults_Probe(a.LivenessProbe)
			if a.LivenessProbe.Handler.HTTPGet != nil {
				corev1.SetDefaults_HTTPGetAction(a.LivenessProbe.Handler.HTTPGet)
			}
		}
		if a.ReadinessProbe != nil {
			corev1.SetDefaults_Probe(a.ReadinessProbe)
			if a.ReadinessProbe.Handler.HTTPGet != nil {
				corev1.SetDefaults_HTTPGetAction(a.ReadinessProbe.Handler.HTTPGet)
			}
		}
		if a.Lifecycle != nil {
			if a.Lifecycle.PostStart != nil {
				if a.Lifecycle.PostStart.HTTPGet != nil {
					corev1.SetDefaults_HTTPGetAction(a.Lifecycle.PostStart.HTTPGet)
				}
			}
			if a.Lifecycle.PreStop != nil {
				if a.Lifecycle.PreStop.HTTPGet != nil {
					corev1.SetDefaults_HTTPGetAction(a.Lifecycle.PreStop.HTTPGet)
				}
			}
		}
	}
	for i := range in.Spec.VolumeClaimTemplates {
		a := &in.Spec.VolumeClaimTemplates[i]
		corev1.SetDefaults_PersistentVolumeClaim(a)
		corev1.SetDefaults_ResourceList(&a.Spec.Resources.Limits)
		corev1.SetDefaults_ResourceList(&a.Spec.Resources.Requests)
		corev1.SetDefaults_ResourceList(&a.Status.Capacity)
	}
}

func setDefaults(obj *appsv1alpha1.StatefulSet) {
	if len(obj.Spec.PodManagementPolicy) == 0 {
		obj.Spec.PodManagementPolicy = appsv1.OrderedReadyPodManagement
	}

	if obj.Spec.UpdateStrategy.Type == "" {
		obj.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType

		// UpdateStrategy.RollingUpdate will take default values below.
		obj.Spec.UpdateStrategy.RollingUpdate = &appsv1alpha1.RollingUpdateStatefulSetStrategy{}
	}

	if obj.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType {
		if obj.Spec.UpdateStrategy.RollingUpdate == nil {
			obj.Spec.UpdateStrategy.RollingUpdate = &appsv1alpha1.RollingUpdateStatefulSetStrategy{}
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.Partition == nil {
			obj.Spec.UpdateStrategy.RollingUpdate.Partition = new(int32)
			*obj.Spec.UpdateStrategy.RollingUpdate.Partition = 0
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable == nil {
			maxUnavailable := intstr.FromInt(1)
			obj.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = &maxUnavailable
		}
		if obj.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy == "" {
			obj.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy = appsv1alpha1.RecreatePodUpdateStrategyType
		}
	}

	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = new(int32)
		*obj.Spec.Replicas = 1
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = new(int32)
		*obj.Spec.RevisionHistoryLimit = 10
	}
}

//var _ inject.Client = &StatefulSetCreateUpdateHandler{}
//
//// InjectClient injects the client into the StatefulSetCreateUpdateHandler
//func (h *StatefulSetCreateUpdateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}

var _ inject.Decoder = &StatefulSetCreateUpdateHandler{}

// InjectDecoder injects the decoder into the StatefulSetCreateUpdateHandler
func (h *StatefulSetCreateUpdateHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}
