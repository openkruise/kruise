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

	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// PodCreateHandler handles Pod
type PodCreateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder

	finders *controllerfinder.ControllerFinder
}

func (h *PodCreateHandler) validatingPodFn(ctx context.Context, req admission.Request) (allowed bool, reason string, err error) {
	allowed = true
	if req.Operation == admissionv1.Delete && len(req.OldObject.Raw) == 0 {
		klog.Warningf("Skip to validate pod %s/%s deletion for no old object, maybe because of Kubernetes version < 1.16", req.Namespace, req.Name)
		return
	}

	switch req.Operation {
	case admissionv1.Update:
		if utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetUpdateGate) {
			allowed, reason, err = h.podUnavailableBudgetValidatingPod(ctx, req)
		}
	case admissionv1.Delete, admissionv1.Create:
		if utilfeature.DefaultFeatureGate.Enabled(features.WorkloadSpread) {
			allowed, reason, err = h.workloadSpreadValidatingPod(ctx, req)
			if !allowed || err != nil {
				return
			}
		}

		if utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetDeleteGate) {
			allowed, reason, err = h.podUnavailableBudgetValidatingPod(ctx, req)
		}
	}

	return
}

var _ admission.Handler = &PodCreateHandler{}

// Handle handles admission requests.
func (h *PodCreateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	allowed, reason, err := h.validatingPodFn(ctx, req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	return admission.ValidationResponse(allowed, reason)
}

var _ inject.Client = &PodCreateHandler{}

// InjectClient injects the client into the PodCreateHandler
func (h *PodCreateHandler) InjectClient(c client.Client) error {
	h.Client = c
	h.finders = controllerfinder.Finder
	return nil
}

var _ admission.DecoderInjector = &PodCreateHandler{}

// InjectDecoder injects the decoder into the PodCreateHandler
func (h *PodCreateHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
