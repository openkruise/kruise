/*
Copyright 2021 The Kruise Authors.

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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/apis/apps/defaults"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilimagejob "github.com/openkruise/kruise/pkg/util/imagejob"
)

// NodeImageCreateUpdateHandler handles NodeImage
type NodeImageCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &NodeImageCreateUpdateHandler{}

// Handle handles admission requests.
func (h *NodeImageCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.NodeImage{}
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var copy runtime.Object = obj.DeepCopy()
	// Set defaults
	defaults.SetDefaultsNodeImage(obj)

	// Sort image tags
	for name, imageSpec := range obj.Spec.Images {
		utilimagejob.SortSpecImageTags(&imageSpec)
		obj.Spec.Images[name] = imageSpec
	}
	for name, imageStatus := range obj.Status.ImageStatuses {
		utilimagejob.SortStatusImageTags(&imageStatus)
		obj.Status.ImageStatuses[name] = imageStatus
	}
	if reflect.DeepEqual(obj, copy) {
		return admission.Allowed("")
	}
	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		klog.V(5).InfoS("Admit NodeImage patches", "name", obj.Name, "patches", util.DumpJSON(resp.Patches))
	}

	return resp
}
