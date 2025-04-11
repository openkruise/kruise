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

package uniteddeployment

import (
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/uniteddeployment/adapter"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var fakeScheme = runtime.NewScheme()

func init() {
	_ = appsv1alpha1.AddToScheme(fakeScheme)
	_ = clientgoscheme.AddToScheme(fakeScheme)
}

func FuzzParseSubsetReplicas(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		udReplicasInt, err := cf.GetInt()
		if err != nil {
			return
		}
		udReplicas := int32(udReplicasInt)

		subsetReplicas, err := fuzzutils.GenerateSubsetReplicas(cf)
		if err != nil {
			return
		}

		_, _ = ParseSubsetReplicas(udReplicas, subsetReplicas)
	})
}

func FuzzApplySubsetTemplate(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		ud := &appsv1alpha1.UnitedDeployment{}
		if err := cf.GenerateStruct(ud); err != nil {
			return
		}

		if err := fuzzutils.GenerateUnitedDeploymentSubset(cf, ud); err != nil {
			return
		}

		var subAdapter adapter.Adapter
		choice, err := cf.GetInt()
		if err != nil {
			return
		}
		switch choice % 4 {
		case 0:
			subAdapter = initCloneSet(cf, ud)
		case 1:
			subAdapter = initStatefulSet(cf, ud)
		case 2:
			subAdapter = initDeployment(cf, ud)
		case 3:
			subAdapter = initAdvancedStatefulSet(cf, ud)
		}

		revision, err := cf.GetString()
		if err != nil {
			return
		}
		replicas, err := cf.GetInt()
		if err != nil {
			return
		}
		partition, err := cf.GetInt()
		if err != nil {
			return
		}

		_ = subAdapter.ApplySubsetTemplate(
			ud,
			ud.Spec.Topology.Subsets[0].Name, // Use first subset
			revision,
			int32(replicas),
			int32(partition),
			subAdapter.NewResourceObject(),
		)
	})
}

func handleTemplate[T any](
	structured bool,
	cf *fuzz.ConsumeFuzzer,
	template **T,
	newTemplate func() *T,
	fillTemplate func(t *T, ud *appsv1alpha1.UnitedDeployment),
	ud *appsv1alpha1.UnitedDeployment,
) {
	if structured {
		if *template == nil {
			*template = newTemplate()
		}
		fillTemplate(*template, ud)
	} else {
		temp := newTemplate()
		if err := cf.GenerateStruct(temp); err == nil {
			*template = temp
		}
	}
}

func initTemplateMetadata(cf *fuzz.ConsumeFuzzer, meta *metav1.ObjectMeta, ud *appsv1alpha1.UnitedDeployment) {
	labels := make(map[string]string)
	if err := cf.FuzzMap(&labels); err != nil {
		return
	}
	annotations := make(map[string]string)
	if err := cf.FuzzMap(&annotations); err != nil {
		return
	}
	matchLabels := make(map[string]string)
	if err := cf.FuzzMap(&matchLabels); err != nil {
		return
	}
	meta.Labels = labels
	meta.Annotations = annotations
	ud.Spec.Selector.MatchLabels = matchLabels
}

func initCloneSet(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment) adapter.Adapter {
	structured, err := cf.GetBool()
	if err != nil {
		structured = false
	}
	handleTemplate[appsv1alpha1.CloneSetTemplateSpec](
		structured,
		cf,
		&ud.Spec.Template.CloneSetTemplate,
		func() *appsv1alpha1.CloneSetTemplateSpec { return &appsv1alpha1.CloneSetTemplateSpec{} },
		func(t *appsv1alpha1.CloneSetTemplateSpec, ud *appsv1alpha1.UnitedDeployment) {
			initTemplateMetadata(cf, &t.ObjectMeta, ud)
		},
		ud,
	)
	return &adapter.CloneSetAdapter{Scheme: fakeScheme}
}

func initDeployment(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment) adapter.Adapter {
	structured, err := cf.GetBool()
	if err != nil {
		structured = false
	}
	handleTemplate[appsv1alpha1.DeploymentTemplateSpec](
		structured,
		cf,
		&ud.Spec.Template.DeploymentTemplate,
		func() *appsv1alpha1.DeploymentTemplateSpec { return &appsv1alpha1.DeploymentTemplateSpec{} },
		func(t *appsv1alpha1.DeploymentTemplateSpec, ud *appsv1alpha1.UnitedDeployment) {
			initTemplateMetadata(cf, &t.ObjectMeta, ud)
		},
		ud,
	)
	return &adapter.DeploymentAdapter{Scheme: fakeScheme}
}

func initAdvancedStatefulSet(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment) adapter.Adapter {
	structured, err := cf.GetBool()
	if err != nil {
		structured = false
	}
	handleTemplate[appsv1alpha1.AdvancedStatefulSetTemplateSpec](
		structured,
		cf,
		&ud.Spec.Template.AdvancedStatefulSetTemplate,
		func() *appsv1alpha1.AdvancedStatefulSetTemplateSpec {
			return &appsv1alpha1.AdvancedStatefulSetTemplateSpec{}
		},
		func(t *appsv1alpha1.AdvancedStatefulSetTemplateSpec, ud *appsv1alpha1.UnitedDeployment) {
			if t.Spec.UpdateStrategy.Type == "" {
				t.Spec.UpdateStrategy.Type = v1.RollingUpdateStatefulSetStrategyType
			}
			initTemplateMetadata(cf, &t.ObjectMeta, ud)
		},
		ud,
	)
	return &adapter.AdvancedStatefulSetAdapter{Scheme: fakeScheme}
}

func initStatefulSet(cf *fuzz.ConsumeFuzzer, ud *appsv1alpha1.UnitedDeployment) adapter.Adapter {
	structured, err := cf.GetBool()
	if err != nil {
		structured = false
	}
	handleTemplate[appsv1alpha1.StatefulSetTemplateSpec](
		structured,
		cf,
		&ud.Spec.Template.StatefulSetTemplate,
		func() *appsv1alpha1.StatefulSetTemplateSpec { return &appsv1alpha1.StatefulSetTemplateSpec{} },
		func(t *appsv1alpha1.StatefulSetTemplateSpec, ud *appsv1alpha1.UnitedDeployment) {
			if t.Spec.UpdateStrategy.Type == "" {
				t.Spec.UpdateStrategy.Type = v1.RollingUpdateStatefulSetStrategyType
			}
			initTemplateMetadata(cf, &t.ObjectMeta, ud)
		},
		ud,
	)
	return &adapter.StatefulSetAdapter{Scheme: fakeScheme}
}
