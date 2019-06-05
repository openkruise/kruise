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

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

func init() {
	webhookName := "mutating-create-sidecarset"
	if HandlerMap[webhookName] == nil {
		HandlerMap[webhookName] = []admission.Handler{}
	}
	HandlerMap[webhookName] = append(HandlerMap[webhookName], &SidecarSetCreateHandler{})
}

// SidecarSetCreateHandler handles SidecarSet
type SidecarSetCreateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder types.Decoder
}

func (h *SidecarSetCreateHandler) mutatingSidecarSetFn(ctx context.Context, obj *appsv1alpha1.SidecarSet) error {
	setDefaultSidecarSet(obj)
	return nil
}

func setDefaultSidecarSet(sidecarset *appsv1alpha1.SidecarSet) {
	for i := range sidecarset.Spec.Containers {
		setDefaultContainer(&sidecarset.Spec.Containers[i])
	}
}

func setDefaultContainer(container *appsv1alpha1.SidecarContainer) {
	if container.TerminationMessagePolicy == "" {
		container.TerminationMessagePolicy = corev1.TerminationMessageReadFile
	}
	if container.ImagePullPolicy == "" {
		container.ImagePullPolicy = corev1.PullIfNotPresent
	}
}

var _ admission.Handler = &SidecarSetCreateHandler{}

// Handle handles admission requests.
func (h *SidecarSetCreateHandler) Handle(ctx context.Context, req types.Request) types.Response {
	obj := &appsv1alpha1.SidecarSet{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	copy := obj.DeepCopy()

	err = h.mutatingSidecarSetFn(ctx, copy)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return admission.PatchResponse(obj, copy)
}

//var _ inject.Client = &SidecarSetCreateHandler{}
//
//// InjectClient injects the client into the SidecarSetCreateHandler
//func (h *SidecarSetCreateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}

var _ inject.Decoder = &SidecarSetCreateHandler{}

// InjectDecoder injects the decoder into the SidecarSetCreateHandler
func (h *SidecarSetCreateHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}
