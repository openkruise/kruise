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
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// BroadcastJobCreateUpdateHandler handles BroadcastJob
type BroadcastJobCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

func (h *BroadcastJobCreateUpdateHandler) mutatingBroadcastJobFn(ctx context.Context, obj *appsv1alpha1.BroadcastJob) error {
	appsv1alpha1.SetDefaultsBroadcastJob(obj)
	return nil
}

var _ admission.Handler = &BroadcastJobCreateUpdateHandler{}

// Handle handles admission requests.
func (h *BroadcastJobCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.BroadcastJob{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var copy runtime.Object = obj.DeepCopy()
	err = h.mutatingBroadcastJobFn(ctx, obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if reflect.DeepEqual(obj, copy) {
		return admission.Allowed("")
	}

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
}

//var _ inject.Client = &BroadcastJobCreateUpdateHandler{}
//
//// InjectClient injects the client into the BroadcastJobCreateUpdateHandler
//func (h *BroadcastJobCreateUpdateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}

var _ admission.DecoderInjector = &BroadcastJobCreateUpdateHandler{}

// InjectDecoder injects the decoder into the BroadcastJobCreateUpdateHandler
func (h *BroadcastJobCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
