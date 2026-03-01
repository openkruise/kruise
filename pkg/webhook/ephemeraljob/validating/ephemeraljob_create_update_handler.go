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

	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/webhook/util/convertor"
)

// EphemeralJobCreateUpdateHandler handles EphemeralJob
type EphemeralJobCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &EphemeralJobCreateUpdateHandler{}

func NewHandler(mgr manager.Manager) admission.Handler {
	return &EphemeralJobCreateUpdateHandler{Decoder: admission.NewDecoder(mgr.GetScheme())}
}

// Handle handles admission requests.
func (h *EphemeralJobCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.EphemeralJob{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if err := validate(obj); err != nil {
		klog.ErrorS(err, "Error validate EphemeralJob", "name", obj.Name)
		return admission.Errored(http.StatusBadRequest, err)
	}

	return admission.ValidationResponse(true, "allowed")
}

func validate(obj *appsv1alpha1.EphemeralJob) error {
	ecs, err := convertor.ConvertEphemeralContainer(obj.Spec.Template.EphemeralContainers)
	if err != nil {
		return err
	}
	// todo: expose this field (`spec.SecurityContext.HostUsers` in pod spec) in the feature if needed.
	// default hostUsers is true
	hostUsers := true
	// don't validate EphemeralContainer TargetContainerName
	allErrs := validateEphemeralContainers(ecs, field.NewPath("ephemeralContainers"), validation.PodValidationOptions{}, hostUsers)
	return allErrs.ToAggregate()
}
