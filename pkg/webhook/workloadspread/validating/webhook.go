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
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openkruise/kruise/pkg/webhook/types"
)

// +kubebuilder:webhook:path=/validate-apps-kruise-io-v1alpha1-workloadspread,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=apps.kruise.io,resources=workloadspreads,verbs=create;update,versions=v1alpha1,name=vworkloadspread.kb.io

var (
	// HandlerGetterMap contains admission webhook handlers
	HandlerGetterMap = map[string]types.HandlerGetter{
		"validate-apps-kruise-io-v1alpha1-workloadspread": func(mgr manager.Manager) admission.Handler {
			return &WorkloadSpreadCreateUpdateHandler{
				Client:  mgr.GetClient(),
				Decoder: admission.NewDecoder(mgr.GetScheme()),
			}
		},
	}
)
