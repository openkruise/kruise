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

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

var (
	defaultMaxImagesPerNode = 256
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
	if !utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) {
		return admission.Errored(http.StatusForbidden, fmt.Errorf("feature-gate %s is not enabled", features.KruiseDaemon))
	}

	if err := validate(obj); err != nil {
		klog.ErrorS(err, "Error validate NodeImage", "name", obj.Name)
		return admission.Errored(http.StatusBadRequest, err)
	}

	return admission.ValidationResponse(true, "allowed")
}

func validate(obj *appsv1alpha1.NodeImage) error {
	if len(obj.Spec.Images) > defaultMaxImagesPerNode {
		return fmt.Errorf("spec images length %v can not be more than %v", len(obj.Spec.Images), defaultMaxImagesPerNode)
	}

	for name, imageSpec := range obj.Spec.Images {
		if len(name) <= 0 {
			return fmt.Errorf("image name can not be empty")
		}
		existingTags := sets.NewString()
		for _, tagSpec := range imageSpec.Tags {
			if len(tagSpec.Tag) <= 0 {
				return fmt.Errorf("tag for image %s can not be empty", name)
			}
			if existingTags.Has(tagSpec.Tag) {
				return fmt.Errorf("duplicated tag %s for image %s", tagSpec.Tag, name)
			}
			existingTags.Insert(tagSpec.Tag)

			fullName := fmt.Sprintf("%s/%s", name, tagSpec.Tag)
			if tagSpec.CreatedAt == nil {
				return fmt.Errorf("createAt field for %s can not be empty", fullName)
			}
			if tagSpec.Version < 0 {
				return fmt.Errorf("version for %s can not be less than 0", fullName)
			}
			if tagSpec.PullPolicy == nil {
				return fmt.Errorf("pullPolicy field for %s must set", fullName)
			}
			if tagSpec.PullPolicy.TTLSecondsAfterFinished == nil {
				return fmt.Errorf("pullPolicy.ttlSecondsAfterFinished for %s must set", fullName)
			}

		}
	}

	return nil
}
