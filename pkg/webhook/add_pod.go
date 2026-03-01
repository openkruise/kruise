/*
Copyright 2020 The Kruise Authors.

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

package webhook

import (
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/webhook/pod/mutating"
	"github.com/openkruise/kruise/pkg/webhook/pod/validating"
)

func init() {
	addHandlersWithGate(mutating.HandlerGetterMap, func() (enabled bool) {
		if !utilfeature.DefaultFeatureGate.Enabled(features.PodWebhook) {
			return false
		}

		// Currently, if SidecarSet is not installed, we can also disable pod webhook.
		if !utildiscovery.DiscoverObject(&appsv1alpha1.SidecarSet{}) {
			return false
		}

		return true
	})

	addHandlersWithGate(validating.HandlerGetterMap, func() (enabled bool) {
		if !utilfeature.DefaultFeatureGate.Enabled(features.PodWebhook) {
			return false
		}

		// Currently, if PodUnavailableBudget is not installed, we can also disable pod webhook.
		if !utildiscovery.DiscoverObject(&policyv1alpha1.PodUnavailableBudget{}) {
			return false
		}

		return true
	})
}
