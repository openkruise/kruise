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
	"github.com/openkruise/kruise/apis/apps/defaults"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"net/http"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ConfigMapSetCreateUpdateHandler handles ConfigMapSet
type ConfigMapSetCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &ConfigMapSetCreateUpdateHandler{}

// Handle handles admission requests.
func (h *ConfigMapSetCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	klog.Infof("configmapset %s/%s action %v", req.Namespace, req.Name, req.AdmissionRequest.Operation)
	obj := &appsv1alpha1.ConfigMapSet{}
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var copy runtime.Object = obj.DeepCopy()
	switch req.AdmissionRequest.Operation {
	case admissionv1.Create, admissionv1.Update:
		klog.Infof("setting default for configmapset %s/%s", obj.Namespace, obj.Name)
		defaults.SetDefaultsConfigMapSet(obj)
	}
	klog.V(4).Infof("configmapset after mutating: %v", util.DumpJSON(obj))
	if reflect.DeepEqual(obj, copy) {
		return admission.Allowed("")
	}
	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
}
