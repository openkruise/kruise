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

	"github.com/openkruise/kruise/pkg/webhook/util/deletionprotection"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// WorkloadHandler handles built-in workloads, e.g. Deployment, ReplicaSet, StatefulSet
type WorkloadHandler struct {
	// Decoder decodes objects
	Decoder *admission.Decoder
}

func (h *WorkloadHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

// Handle handles admission requests.
func (h *WorkloadHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1beta1.Delete || req.SubResource != "" {
		return admission.ValidationResponse(true, "")
	}
	if len(req.OldObject.Raw) == 0 {
		klog.Warningf("Skip to validate %s %s/%s for no old object, maybe because of Kubernetes version < 1.16", req.Kind.Kind, req.Namespace, req.Name)
		return admission.ValidationResponse(true, "")
	}

	var metaObj metav1.Object
	var replicas *int32
	switch req.Kind.Kind {
	case "Deployment":
		obj := &apps.Deployment{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		metaObj = obj
		replicas = obj.Spec.Replicas
	case "ReplicaSet":
		obj := &apps.ReplicaSet{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		metaObj = obj
		replicas = obj.Spec.Replicas
	case "StatefulSet":
		obj := &apps.StatefulSet{}
		if err := h.Decoder.DecodeRaw(req.AdmissionRequest.OldObject, obj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		metaObj = obj
		replicas = obj.Spec.Replicas
	default:
		klog.Warningf("Skip to validate %s %s/%s for unsupported resource", req.Kind.Kind, req.Namespace, req.Name)
		return admission.ValidationResponse(true, "")
	}

	if err := deletionprotection.ValidateWorkloadDeletion(metaObj, replicas); err != nil {
		return admission.Errored(http.StatusForbidden, err)
	}
	return admission.ValidationResponse(true, "")
}
