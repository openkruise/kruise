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
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/webhook/util/deletionprotection"
)

// WorkloadHandler handles built-in workloads, e.g. Deployment, ReplicaSet, StatefulSet
type WorkloadHandler struct {
	// Decoder decodes objects
	Decoder admission.Decoder
}

func (h *WorkloadHandler) InjectDecoder(d admission.Decoder) error {
	h.Decoder = d
	return nil
}

// Handle handles admission requests.
func (h *WorkloadHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Delete || req.SubResource != "" {
		return admission.ValidationResponse(true, "")
	}
	if len(req.OldObject.Raw) == 0 {
		klog.InfoS("Skip to validate for no old object, maybe because of Kubernetes version < 1.16", "kind", req.Kind.Kind, "namespace", req.Namespace, "name", req.Name)
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
		klog.InfoS("Skip to validate for unsupported resource", "kind", req.Kind.Kind, "namespace", req.Namespace, "name", req.Name)
		return admission.ValidationResponse(true, "")
	}

	if err := deletionprotection.ValidateWorkloadDeletion(metaObj, replicas); err != nil {
		deletionprotection.WorkloadDeletionProtectionMetrics.WithLabelValues(fmt.Sprintf("%s_%s_%s", req.Kind.Kind, metaObj.GetNamespace(), metaObj.GetName()), req.UserInfo.Username).Add(1)
		util.LoggerProtectionInfo(util.ProtectionEventDeletionProtection, req.Kind.Kind, metaObj.GetNamespace(), metaObj.GetName(), req.UserInfo.Username)
		return admission.Errored(http.StatusForbidden, err)
	}
	return admission.ValidationResponse(true, "")
}
