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

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ImagePullJobCreateUpdateHandler handles ImagePullJob
type ImagePullJobCreateUpdateHandler struct {
	// Decoder decodes objects
	Decoder admission.Decoder
}

var _ admission.Handler = &ImagePullJobCreateUpdateHandler{}

// Handle handles admission requests.
func (h *ImagePullJobCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &appsv1alpha1.ImagePullJob{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if !utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) {
		return admission.Errored(http.StatusForbidden, fmt.Errorf("feature-gate %s is not enabled", features.KruiseDaemon))
	}
	if !utilfeature.DefaultFeatureGate.Enabled(features.ImagePullJobGate) {
		return admission.Errored(http.StatusForbidden, fmt.Errorf("feature-gate %s is not enabled", features.ImagePullJobGate))
	}

	if err := validate(obj); err != nil {
		klog.ErrorS(err, "Error validate ImagePullJob", "namespace", obj.Namespace, "name", obj.Name)
		return admission.Errored(http.StatusBadRequest, err)
	}

	return admission.ValidationResponse(true, "allowed")
}

func validate(obj *appsv1alpha1.ImagePullJob) error {
	if obj.Spec.Selector != nil {
		if obj.Spec.Selector.MatchLabels != nil || obj.Spec.Selector.MatchExpressions != nil {
			if obj.Spec.Selector.Names != nil {
				return fmt.Errorf("can not set both names and labelSelector in this spec.selector")
			}
			if _, err := metav1.LabelSelectorAsSelector(&obj.Spec.Selector.LabelSelector); err != nil {
				return fmt.Errorf("invalid selector: %v", err)
			}
		}
		if obj.Spec.Selector.Names != nil {
			names := sets.NewString(obj.Spec.Selector.Names...)
			if names.Len() != len(obj.Spec.Selector.Names) {
				return fmt.Errorf("duplicated name in selector names")
			}
		}
	}
	if obj.Spec.PodSelector != nil {
		if obj.Spec.Selector != nil {
			return fmt.Errorf("can not set both selector and podSelector")
		}
		if _, err := metav1.LabelSelectorAsSelector(&obj.Spec.PodSelector.LabelSelector); err != nil {
			return fmt.Errorf("invalid podSelector: %v", err)
		}
	}

	if len(obj.Spec.Image) == 0 {
		return fmt.Errorf("image can not be empty")
	}

	if _, err := daemonutil.NormalizeImageRef(obj.Spec.Image); err != nil {
		return fmt.Errorf("invalid image %s: %v", obj.Spec.Image, err)
	}
	if obj.Spec.PullPolicy == nil {
		obj.Spec.PullPolicy = &appsv1alpha1.PullPolicy{}
	}
	if obj.Spec.PullPolicy.TimeoutSeconds == nil {
		obj.Spec.PullPolicy.TimeoutSeconds = utilpointer.Int32Ptr(600)
	}
	switch obj.Spec.CompletionPolicy.Type {
	case appsv1alpha1.Always:
		if obj.Spec.CompletionPolicy.ActiveDeadlineSeconds != nil && int64(*obj.Spec.PullPolicy.TimeoutSeconds) > *obj.Spec.CompletionPolicy.ActiveDeadlineSeconds {
			return fmt.Errorf("completionPolicy.activeDeadlineSeconds must be greater than pullPolicy.timeoutSeconds(default 600)")
		}
	case appsv1alpha1.Never:
		if obj.Spec.CompletionPolicy.ActiveDeadlineSeconds != nil || obj.Spec.CompletionPolicy.TTLSecondsAfterFinished != nil {
			return fmt.Errorf("activeDeadlineSeconds and ttlSecondsAfterFinished can only work with Always CompletionPolicyType")
		}
	default:
		return fmt.Errorf("unknown type of completionPolicy: %s", obj.Spec.CompletionPolicy.Type)
	}

	return nil
}
