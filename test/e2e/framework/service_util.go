/*
Copyright 2019 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.

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

package framework

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateServiceSpec creates service spec
func CreateServiceSpec(serviceName, externalName string, isHeadless bool, selector map[string]string) *v1.Service {
	headlessService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceName,
		},
		Spec: v1.ServiceSpec{
			Selector: selector,
		},
	}
	if externalName != "" {
		headlessService.Spec.Type = v1.ServiceTypeExternalName
		headlessService.Spec.ExternalName = externalName
	} else {
		headlessService.Spec.Ports = []v1.ServicePort{
			{Port: 80, Name: "http", Protocol: v1.ProtocolTCP},
		}
	}
	if isHeadless {
		headlessService.Spec.ClusterIP = "None"
	}
	return headlessService
}
