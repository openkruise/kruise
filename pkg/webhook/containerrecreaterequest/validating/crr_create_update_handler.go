package validating

import (
	"context"
	"net/http"

	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"

	admissionv1 "k8s.io/api/admission/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ContainerRecreateRequestHandler handles ContainerRecreateRequest
type ContainerRecreateRequestHandler struct {
	Client client.Client
	// Decoder decodes objects
	Decoder *admission.Decoder
	finders *controllerfinder.ControllerFinder
}

var _ admission.Handler = &ContainerRecreateRequestHandler{}

func (h *ContainerRecreateRequestHandler) validatingContainerRecreateRequestFn(ctx context.Context, req admission.Request) (allowed bool, reason string, err error) {
	allowed = true

	switch req.Operation {
	case admissionv1.Create:
		if utilfeature.DefaultFeatureGate.Enabled(features.PodUnavailableBudgetUpdateGate) {
			allowed, reason, err = h.podUnavailableBudgetValidating(ctx, req)
		}
	}

	return
}

func (h *ContainerRecreateRequestHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	allowed, reason, err := h.validatingContainerRecreateRequestFn(ctx, req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	return admission.ValidationResponse(allowed, reason)
}

var _ inject.Client = &ContainerRecreateRequestHandler{}

// InjectClient injects the client into the ContainerRecreateRequestHandler
func (h *ContainerRecreateRequestHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ admission.DecoderInjector = &ContainerRecreateRequestHandler{}

// InjectDecoder injects the decoder into the ContainerRecreateRequestHandler
func (h *ContainerRecreateRequestHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}
