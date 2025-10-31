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
	"fmt"
	"net/http"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis/apps/defaults"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util"
)

// SidecarSetCreateHandler handles SidecarSet
type SidecarSetCreateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &SidecarSetCreateHandler{}

// Handle handles admission requests.
func (h *SidecarSetCreateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		obj := &appsv1beta1.SidecarSet{}
		if err := h.Decoder.Decode(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		var copy runtime.Object = obj.DeepCopy()
		switch req.AdmissionRequest.Operation {
		case admissionv1.Create, admissionv1.Update:
			defaults.SetDefaultsSidecarSetV1beta1(obj)
			if err := defaults.SetHashSidecarSetV1beta1(obj); err != nil {
				return admission.Errored(http.StatusInternalServerError, err)
			}
		}
		klog.V(4).InfoS("sidecarset after mutating", "object", util.DumpJSON(obj))
		if reflect.DeepEqual(obj, copy) {
			return admission.Allowed("")
		}
		marshaled, err := json.Marshal(obj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshaled)
		if len(resp.Patches) > 0 {
			klog.V(5).InfoS("Admit SidecarSet patches", "name", obj.Name, "patches", util.DumpJSON(resp.Patches))
		}
		return resp
	case appsv1alpha1.GroupVersion.Version:
		obj := &appsv1alpha1.SidecarSet{}
		if err := h.Decoder.Decode(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		var copy runtime.Object = obj.DeepCopy()
		switch req.AdmissionRequest.Operation {
		case admissionv1.Create, admissionv1.Update:
			defaults.SetDefaultsSidecarSet(obj)
			if err := defaults.SetHashSidecarSet(obj); err != nil {
				return admission.Errored(http.StatusInternalServerError, err)
			}
		}
		klog.V(4).InfoS("sidecarset after mutating", "object", util.DumpJSON(obj))
		if reflect.DeepEqual(obj, copy) {
			return admission.Allowed("")
		}
		marshaled, err := json.Marshal(obj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
		resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshaled)
		if len(resp.Patches) > 0 {
			klog.V(5).InfoS("Admit SidecarSet patches", "name", obj.Name, "patches", util.DumpJSON(resp.Patches))
		}
		return resp
	}
	return admission.Errored(http.StatusBadRequest, fmt.Errorf("unsupported version: %s", req.AdmissionRequest.Resource.Version))
}

// var _ inject.Client = &SidecarSetCreateHandler{}
//
// // InjectClient injects the client into the SidecarSetCreateHandler
// func (h *SidecarSetCreateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
// }
