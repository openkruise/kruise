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
	"encoding/json"
	"net/http"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	patchutil "github.com/openkruise/kruise/pkg/util/patch"
	"github.com/openkruise/kruise/pkg/webhook/default_server/utils"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog"
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

	SetObjectDefaults(obj)
	obj.Status = appsv1alpha1.StatefulSetStatus{}

	err = h.mutatingStatefulSetFn(ctx, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	marshaledPod, err := json.Marshal(obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	resp := patchutil.ResponseFromRaw(req.AdmissionRequest.Object.Raw, marshaledPod)
	if len(resp.Patches) > 0 {
		klog.V(5).Infof("Admit StatefulSet %s/%s patches: %v", obj.Namespace, obj.Name, util.DumpJSON(resp.Patches))
	}
	return resp
}

// SetObjectDefaults sets object by default
func SetObjectDefaults(in *appsv1alpha1.StatefulSet) {
	setDefaults(in)
	utils.SetDefaultPodTemplate(&in.Spec.Template.Spec)
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
