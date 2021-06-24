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
	"github.com/openkruise/kruise/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// UnitedDeploymentCreateUpdateHandler handles UnitedDeployment
type UnitedDeploymentCreateUpdateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &UnitedDeploymentCreateUpdateHandler{}

// Handle handles admission requests.
func (h *UnitedDeploymentCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.UnitedDeployment{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var copy runtime.Object = obj.DeepCopy()
	appsv1alpha1.SetDefaultsUnitedDeployment(obj)
	obj.Status = appsv1alpha1.UnitedDeploymentStatus{}
	if reflect.DeepEqual(obj, copy) {
		return admission.Allowed("")
	}
	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		klog.V(5).Infof("Admit UnitedDeployment %s/%s patches: %v", obj.Namespace, obj.Name, util.DumpJSON(resp.Patches))
	}
	return resp
}

var _ admission.DecoderInjector = &UnitedDeploymentCreateUpdateHandler{}

// InjectDecoder injects the decoder into the UnitedDeploymentCreateUpdateHandler
func (h *UnitedDeploymentCreateUpdateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
