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

	admissionv1 "k8s.io/api/admission/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/webhook/util/deletionprotection"
)

type CRDHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &CRDHandler{}

// Handle handles admission requests.
func (h *CRDHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.AdmissionRequest.Operation != admissionv1.Delete || req.AdmissionRequest.SubResource != "" {
		return admission.ValidationResponse(true, "")
	}
	if len(req.OldObject.Raw) == 0 {
		klog.InfoS("Skip to validate CRD %s deletion for no old object, maybe because of Kubernetes version < 1.16", "name", req.Name)
		return admission.ValidationResponse(true, "")
	}

	var metaObj metav1.Object
	var gvk schema.GroupVersionKind
	switch req.Kind.Version {
	case "v1beta1":
		crd := &apiextensionsv1beta1.CustomResourceDefinition{}
		if err := h.Decoder.DecodeRaw(req.OldObject, crd); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		metaObj = crd
		for _, v := range crd.Spec.Versions {
			if v.Storage {
				gvk = schema.GroupVersionKind{Group: crd.Spec.Group, Kind: crd.Spec.Names.ListKind, Version: v.Name}
				break
			}
		}
	case "v1":
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := h.Decoder.DecodeRaw(req.OldObject, crd); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		metaObj = crd
		for _, v := range crd.Spec.Versions {
			if v.Storage {
				gvk = schema.GroupVersionKind{Group: crd.Spec.Group, Kind: crd.Spec.Names.ListKind, Version: v.Name}
				break
			}
		}
	default:
		klog.InfoS("Skip to validate CRD deletion for unrecognized version %s", "name", req.Name, "version", req.Kind.Version)
		return admission.ValidationResponse(true, "")
	}

	if err := deletionprotection.ValidateCRDDeletion(h.Client, metaObj, gvk); err != nil {
		deletionprotection.CRDDeletionProtectionMetrics.WithLabelValues(metaObj.GetName(), req.UserInfo.Username).Add(1)
		util.LoggerProtectionInfo(util.ProtectionEventDeletionProtection, "CustomResourceDefinition", "", metaObj.GetName(), req.UserInfo.Username)
		return admission.Errored(http.StatusForbidden, err)
	}
	return admission.ValidationResponse(true, "")
}
