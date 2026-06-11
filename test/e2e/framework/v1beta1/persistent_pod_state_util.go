/*
Copyright 2026 The Kruise Authors.

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

// +kubebuilder:skip
package v1beta1

import (
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"

	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	fwv1alpha1 "github.com/openkruise/kruise/test/e2e/framework/v1alpha1"
)

// PersistentPodStateTester reuses v1alpha1 workload helpers; PPS objects are accessed via v1beta1 in e2e tests.
type PersistentPodStateTester struct {
	*fwv1alpha1.PersistentPodStateTester
}

func NewPersistentPodStateTester(c clientset.Interface, kc kruiseclientset.Interface, d dynamic.Interface, a apiextensionsclientset.Interface) *PersistentPodStateTester {
	return &PersistentPodStateTester{
		PersistentPodStateTester: fwv1alpha1.NewPersistentPodStateTester(c, kc, d, a),
	}
}
