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

	"k8s.io/api/admission/v1beta1"
	"k8s.io/klog"

	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	patchutil "github.com/openkruise/kruise/pkg/util/patch"
)

const (
	// SidecarSetHashAnnotation represents the key of a sidecarset hash
	SidecarSetHashAnnotation = "kruise.io/sidecarset-hash"
	// SidecarSetHashWithoutImageAnnotation represents the key of a sidecarset hash without images of sidecar
	SidecarSetHashWithoutImageAnnotation = "kruise.io/sidecarset-hash-without-image"
)

func init() {
	webhookName := "mutating-create-update-sidecarset"
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

func setHashSidecarSet(sidecarset *appsv1alpha1.SidecarSet) error {
	if sidecarset.Annotations == nil {
		sidecarset.Annotations = make(map[string]string)
	}

	hash, err := SidecarSetHash(sidecarset)
	if err != nil {
		return err
	}
	sidecarset.Annotations[SidecarSetHashAnnotation] = hash

	hash, err = SidecarSetHashWithoutImage(sidecarset)
	if err != nil {
		return err
	}
	sidecarset.Annotations[SidecarSetHashWithoutImageAnnotation] = hash

	return nil
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

	switch req.AdmissionRequest.Operation {
	case v1beta1.Create, v1beta1.Update:
		appsv1alpha1.SetDefaults_SidecarSet(copy)
		if err := setHashSidecarSet(copy); err != nil {
			return admission.ErrorResponse(http.StatusInternalServerError, err)
		}
	}
	klog.V(4).Infof("sidecarset after mutating: %v", util.DumpJSON(copy))
	// related issue: https://github.com/kubernetes-sigs/kubebuilder/issues/510
	marshaledSidecarSet, err := json.Marshal(copy)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return patchutil.ResponseFromRaw(req.AdmissionRequest.Object.Raw, marshaledSidecarSet)
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
