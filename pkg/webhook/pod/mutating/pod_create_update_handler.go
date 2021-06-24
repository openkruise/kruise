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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// PodCreateHandler handles Pod
type PodCreateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &PodCreateHandler{}

// Handle handles admission requests.
func (h *PodCreateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &corev1.Pod{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var copy runtime.Object = obj.DeepCopy()
	// when pod.namespace is empty, using req.namespace
	if obj.Namespace == "" {
		obj.Namespace = req.Namespace
	}

	injectPodReadinessGate(req, obj)

	err = h.sidecarsetMutatingPod(ctx, req, obj)
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

var _ inject.Client = &PodCreateHandler{}

// InjectClient injects the client into the PodCreateHandler
func (h *PodCreateHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &PodCreateHandler{}

// InjectDecoder injects the decoder into the PodCreateHandler
func (h *PodCreateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
