/*
Copyright 2025 The Kruise Authors.

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

package fuzz

import (
	fuzz "github.com/AdaLogics/go-fuzz-headers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

var (
	StructuredResources = []struct {
		Name string
		Data []byte
	}{
		{
			Name: "Secret",
			Data: []byte(`{
				"apiVersion": "v1",
				"data": {
					"test": "MWYyZDFlMmU2N2Rm"
				},
				"kind": "Secret",
				"metadata": {
					"name": "test-secret-2"
				},
				"type": "Opaque"
			}`),
		},
		{
			Name: "ConfigMap",
			Data: []byte(`{
				"apiVersion": "v1",
				"data": {
					"game.properties": "enemy.types=aliens,monsters\nplayer.maximum-lives=5\n",
					"player_initial_lives": "3",
					"ui_properties_file_name": "user-interface.properties",
					"user-interface.properties": "color.good=purple\ncolor.bad=yellow\nallow.textmode=true\n"
				},
				"kind": "ConfigMap",
				"metadata": {
					"name": "game-demo"
				}
			}`),
		},
		{
			Name: "Pod",
			Data: []byte(`{
				"apiVersion": "v1",
				"kind": "Pod",
				"metadata": {
					"name": "test-pod-1"
				},
				"spec": {
					"containers": [
						{
							"image": "nginx:1.14.2",
							"name": "test-container"
						}
					]
				}
			}`),
		},
	}
)

func GenerateResourceDistributionResource(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.ResourceDistribution) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		raw := runtime.RawExtension{}
		if err := cf.GenerateStruct(&raw); err != nil {
			return err
		}
		ud.Spec.Resource = raw
		return nil
	}

	choice, err := cf.GetInt()
	if err != nil {
		return err
	}
	ud.Spec.Resource = runtime.RawExtension{
		Raw: StructuredResources[choice%len(StructuredResources)].Data,
	}
	return nil
}

func GenerateResourceDistributionTargets(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.ResourceDistribution) error {
	isStructured, err := cf.GetBool()
	if err != nil {
		return err
	}

	if !isStructured {
		targets := appsv1alpha1.ResourceDistributionTargets{}
		if err := cf.GenerateStruct(&targets); err != nil {
			return err
		}
		ud.Spec.Targets = targets
		return nil
	}

	targets := appsv1alpha1.ResourceDistributionTargets{}
	includedNamespacesSlice := make([]appsv1alpha1.ResourceDistributionNamespace, 0)
	if err := cf.CreateSlice(&includedNamespacesSlice); err != nil {
		return err
	}

	excludedNamespacesSlice := make([]appsv1alpha1.ResourceDistributionNamespace, 0)
	if err := cf.CreateSlice(&excludedNamespacesSlice); err != nil {
		return err
	}

	targets.IncludedNamespaces.List = includedNamespacesSlice
	targets.ExcludedNamespaces.List = excludedNamespacesSlice

	for i := range targets.IncludedNamespaces.List {
		if valid, err := cf.GetBool(); valid && err == nil {
			targets.IncludedNamespaces.List[i].Name = GenerateValidNamespaceName()
		} else {
			targets.IncludedNamespaces.List[i].Name = GenerateInvalidNamespaceName()
		}
	}

	for i := range targets.ExcludedNamespaces.List {
		if valid, err := cf.GetBool(); valid && err == nil {
			targets.ExcludedNamespaces.List[i].Name = GenerateValidNamespaceName()
		} else {
			targets.ExcludedNamespaces.List[i].Name = GenerateInvalidNamespaceName()
		}
	}

	selector := metav1.LabelSelector{}
	if err := GenerateLabelSelector(cf, &selector); err != nil {
		return err
	}
	targets.NamespaceLabelSelector = selector
	ud.Spec.Targets = targets
	return nil
}

func GenerateResourceObject(cf *fuzz.ConsumeFuzzer) (runtime.Object, error) {
	isStructured, err := cf.GetBool()
	if err != nil {
		return nil, err
	}

	resource := &unstructured.Unstructured{}
	if !isStructured {
		object := make(map[string]interface{})
		if err = cf.GenerateStruct(&object); err != nil {
			return resource, err
		}
		resource.Object = object
		return resource, nil
	}

	name, err := cf.GetString()
	if err != nil {
		return resource, err
	}

	choice, err := cf.GetInt()
	if err != nil {
		return resource, err
	}

	switch choice % 3 {
	case 0:
		resource.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Pod"))
		resource.SetName(name)
	case 1:
		resource.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
		resource.SetName(name)
	case 2:
		resource.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
		resource.SetName(name)
	}
	return resource, nil
}
