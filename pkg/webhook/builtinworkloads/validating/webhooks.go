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

import "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

// +kubebuilder:webhook:path=/validate-apps-deployment,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=apps,resources=deployments,verbs=delete,versions=v1,name=vbuiltindeployment.kb.io

// +kubebuilder:webhook:path=/validate-apps-replicaset,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=apps,resources=replicasets,verbs=delete,versions=v1,name=vbuiltinreplicaset.kb.io

// +kubebuilder:webhook:path=/validate-apps-statefulset,mutating=false,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups=apps,resources=statefulsets,verbs=delete,versions=v1,name=vbuiltinstatefulset.kb.io

var (
	// HandlerMap contains admission webhook handlers
	HandlerMap = map[string]admission.Handler{
		"validate-apps-deployment":  &WorkloadHandler{},
		"validate-apps-replicaset":  &WorkloadHandler{},
		"validate-apps-statefulset": &WorkloadHandler{},
	}
)
