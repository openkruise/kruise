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

	"github.com/openkruise/kruise/apis/apps/defaults"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SidecarSetCreateHandler handles SidecarSet
type SidecarSetCreateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

func setHashSidecarSet(sidecarset *appsv1alpha1.SidecarSet) error {
	if sidecarset.Annotations == nil {
		sidecarset.Annotations = make(map[string]string)
	}

	hash, err := sidecarcontrol.SidecarSetHash(sidecarset)
	if err != nil {
		return err
	}
	sidecarset.Annotations[sidecarcontrol.SidecarSetHashAnnotation] = hash

	hash, err = sidecarcontrol.SidecarSetHashWithoutImage(sidecarset)
	if err != nil {
		return err
	}
	sidecarset.Annotations[sidecarcontrol.SidecarSetHashWithoutImageAnnotation] = hash

	return nil
}

var _ admission.Handler = &SidecarSetCreateHandler{}

// Handle handles admission requests.
func (h *SidecarSetCreateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.SidecarSet{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var copy runtime.Object = obj.DeepCopy()
	switch req.AdmissionRequest.Operation {
	case admissionv1.Create, admissionv1.Update:
		defaults.SetDefaultsSidecarSet(obj)
		if err := setHashSidecarSet(obj); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
	}
	klog.V(4).Infof("sidecarset after mutating: %v", util.DumpJSON(obj))
	if reflect.DeepEqual(obj, copy) {
		return admission.Allowed("")
	}
	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
}

//var _ inject.Client = &SidecarSetCreateHandler{}
//
//// InjectClient injects the client into the SidecarSetCreateHandler
//func (h *SidecarSetCreateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}

var _ admission.DecoderInjector = &SidecarSetCreateHandler{}

// InjectDecoder injects the decoder into the SidecarSetCreateHandler
func (h *SidecarSetCreateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
