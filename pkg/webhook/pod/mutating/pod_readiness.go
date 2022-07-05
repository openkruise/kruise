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

package mutating

import (
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func injectPodReadinessGate(req admission.Request, pod *v1.Pod) (skip bool) {
	if req.Operation != admissionv1.Create {
		return true
	}
	if !util.IsPodOwnedByKruise(pod) && !utilfeature.DefaultFeatureGate.Enabled(features.KruisePodReadinessGate) {
		return true
	}
	util.InjectReadinessGateToPod(pod, appspub.KruisePodReadyConditionType)
	return false
}
