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
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

// StatefulSetCreateUpdateHandler handles StatefulSet
type StatefulSetCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &StatefulSetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *StatefulSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1beta1.StatefulSet{}
	var objv1alpha1 *appsv1alpha1.StatefulSet

	switch req.AdmissionRequest.Resource.Version {
	case appsv1beta1.GroupVersion.Version:
		if err := h.Decoder.Decode(req, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	case appsv1alpha1.GroupVersion.Version:
		objv1alpha1 = &appsv1alpha1.StatefulSet{}
		if err := h.Decoder.Decode(req, objv1alpha1); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if err := objv1alpha1.ConvertTo(obj); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to convert v1alpha1->v1beta1: %v", err))
		}
	}
	var copy runtime.Object = obj.DeepCopy()

	injectTemplateDefaults := false
	if !utilfeature.DefaultFeatureGate.Enabled(features.TemplateNoDefaults) {
		if req.AdmissionRequest.Operation == admissionv1.Update {
			oldObj := &appsv1beta1.StatefulSet{}
			var oldObjv1alpha1 *appsv1alpha1.StatefulSet
			switch req.AdmissionRequest.Resource.Version {
			case appsv1beta1.GroupVersion.Version:
				if err := h.Decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
					return admission.Errored(http.StatusBadRequest, err)
				}
			case appsv1alpha1.GroupVersion.Version:
				oldObjv1alpha1 = &appsv1alpha1.StatefulSet{}
				if err := h.Decoder.DecodeRaw(req.OldObject, oldObjv1alpha1); err != nil {
					return admission.Errored(http.StatusBadRequest, err)
				}
				if err := oldObjv1alpha1.ConvertTo(oldObj); err != nil {
					return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to convert v1alpha1->v1beta1: %v", err))
				}
			}
			if !reflect.DeepEqual(obj.Spec.Template, oldObj.Spec.Template) {
				injectTemplateDefaults = true
			}
		} else {
			injectTemplateDefaults = true
		}
	}
	defaults.SetDefaultsStatefulSet(obj, injectTemplateDefaults)
	obj.Status = appsv1beta1.StatefulSetStatus{}

	var err error
	var marshalled []byte
	if objv1alpha1 != nil {
		if err := objv1alpha1.ConvertFrom(obj); err != nil {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("failed to convert v1beta1->v1alpha1: %v", err))
		}
		marshalled, err = json.Marshal(objv1alpha1)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
	} else {
		marshalled, err = json.Marshal(obj)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
	}
	if reflect.DeepEqual(obj, copy) {
		return admission.Allowed("")
	}
	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		klog.V(5).InfoS("Admit StatefulSet patches", "namespace", obj.Namespace, "name", obj.Name, "patches", util.DumpJSON(resp.Patches))
	}
	return resp
}

//var _ inject.Client = &StatefulSetCreateUpdateHandler{}
//
//// InjectClient injects the client into the StatefulSetCreateUpdateHandler
//func (h *StatefulSetCreateUpdateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}
