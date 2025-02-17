/*
Copyright 2023 The Kruise Authors.

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

package validating

import (
	"context"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/pkg/webhook/util/deletionprotection"
)

type IngressHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &IngressHandler{}

// Handle handles admission requests.
func (h *IngressHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.AdmissionRequest.Operation != admissionv1.Delete || req.AdmissionRequest.SubResource != "" {
		return admission.ValidationResponse(true, "")
	}
	if len(req.OldObject.Raw) == 0 {
		klog.InfoS("Skip to validate ingress deletion for no old object, maybe because of Kubernetes version < 1.16", "name", req.Name)
		return admission.ValidationResponse(true, "")
	}

	var metaObj metav1.Object
	switch req.Kind.Version {
	case "v1beta1":
		obj := &networkingv1beta1.Ingress{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		metaObj = obj
	case "v1":
		obj := &networkingv1.Ingress{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		metaObj = obj
	}

	if err := deletionprotection.ValidateIngressDeletion(metaObj); err != nil {
		return admission.Errored(http.StatusForbidden, err)
	}
	return admission.ValidationResponse(true, "")
}
