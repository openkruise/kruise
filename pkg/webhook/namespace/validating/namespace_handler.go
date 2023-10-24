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

package validating

import (
	"context"
	"net/http"

	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/webhook/util/deletionprotection"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type NamespaceHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &NamespaceHandler{}

// Handle handles admission requests.
func (h *NamespaceHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.AdmissionRequest.Operation != admissionv1.Delete || req.AdmissionRequest.SubResource != "" {
		return admission.ValidationResponse(true, "")
	}
	if len(req.OldObject.Raw) == 0 {
		klog.Warningf("Skip to validate namespace %s deletion for no old object, maybe because of Kubernetes version < 1.16", req.Name)
		return admission.ValidationResponse(true, "")
	}
	obj := &v1.Namespace{}
	if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if err := deletionprotection.ValidateNamespaceDeletion(h.Client, obj); err != nil {
		deletionprotection.NamespaceDeletionProtectionMetrics.WithLabelValues(obj.Name, req.UserInfo.Username).Add(1)
		util.LoggerProtectionInfo(util.ProtectionEventDeletionProtection, "Namespace", "", obj.Name, req.UserInfo.Username)
		return admission.Errored(http.StatusForbidden, err)
	}
	return admission.ValidationResponse(true, "")
}

var _ inject.Client = &NamespaceHandler{}

func (h *NamespaceHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &NamespaceHandler{}

func (h *NamespaceHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
