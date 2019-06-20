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
	"github.com/openkruise/kruise/pkg/webhook/default_server/utils"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

func init() {
	webhookName := "mutating-create-update-broadcastjob"
	if HandlerMap[webhookName] == nil {
		HandlerMap[webhookName] = []admission.Handler{}
	}
	HandlerMap[webhookName] = append(HandlerMap[webhookName], &BroadcastJobCreateUpdateHandler{})
}

// BroadcastJobCreateUpdateHandler handles BroadcastJob
type BroadcastJobCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder types.Decoder
}

func (h *BroadcastJobCreateUpdateHandler) mutatingBroadcastJobFn(ctx context.Context, obj *appsv1alpha1.BroadcastJob) error {
	setDefaultBroadcastJob(obj)
	return nil
}

// SetDefaults_BroadcastJob sets any unspecified values to defaults.
func setDefaultBroadcastJob(job *appsv1alpha1.BroadcastJob) {
	utils.SetDefaultPodTemplate(&job.Spec.Template.Spec)
	if job.Spec.CompletionPolicy.Type == "" {
		job.Spec.CompletionPolicy.Type = appsv1alpha1.Always
	}

	if job.Spec.Parallelism == nil {
		parallelism := int32(1<<31 - 1)
		job.Spec.Parallelism = &parallelism
	}
}

var _ admission.Handler = &BroadcastJobCreateUpdateHandler{}

// Handle handles admission requests.
func (h *BroadcastJobCreateUpdateHandler) Handle(ctx context.Context, req types.Request) types.Response {
	obj := &appsv1alpha1.BroadcastJob{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	copy := obj.DeepCopy()

	err = h.mutatingBroadcastJobFn(ctx, copy)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return admission.PatchResponse(obj, copy)
}

//var _ inject.Client = &BroadcastJobCreateUpdateHandler{}
//
//// InjectClient injects the client into the BroadcastJobCreateUpdateHandler
//func (h *BroadcastJobCreateUpdateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}

var _ inject.Decoder = &BroadcastJobCreateUpdateHandler{}

// InjectDecoder injects the decoder into the BroadcastJobCreateUpdateHandler
func (h *BroadcastJobCreateUpdateHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}
