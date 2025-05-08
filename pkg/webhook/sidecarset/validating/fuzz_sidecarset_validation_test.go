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

package validating

import (
	"encoding/json"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/configuration"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = appsv1alpha1.AddToScheme(fakeScheme)
	_ = appsv1alpha1.AddToScheme(clientgoscheme.Scheme)
}

func FuzzValidateSidecarSetSpec(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		ss := &appsv1alpha1.SidecarSet{}
		if err := cf.GenerateStruct(ss); err != nil {
			return
		}

		if err := fuzzutils.GenerateSidecarSetSpec(cf, ss,
			fuzzutils.GenerateSidecarSetSelector,
			fuzzutils.GenerateSidecarSetNamespace,
			fuzzutils.GenerateSidecarSetNamespaceSelector,
			fuzzutils.GenerateSidecarSetInitContainer,
			fuzzutils.GenerateSidecarSetContainer,
			fuzzutils.GenerateSidecarSetUpdateStrategy,
			fuzzutils.GenerateSidecarSetInjectionStrategy,
			fuzzutils.GenerateSidecarSetPatchPodMetadata); err != nil {
			return
		}

		h, err := newFakeSidecarSetCreateUpdateHandler(cf, ss)
		if err != nil {
			return
		}

		_ = h.validateSidecarSetSpec(ss, field.NewPath("spec"))
	})
}

func newFakeSidecarSetCreateUpdateHandler(cf *fuzz.ConsumeFuzzer, ss *appsv1alpha1.SidecarSet) (*SidecarSetCreateUpdateHandler, error) {
	name, hash := "", ""
	if ss.Spec.InjectionStrategy.Revision != nil && ss.Spec.InjectionStrategy.Revision.RevisionName != nil {
		name = *ss.Spec.InjectionStrategy.Revision.RevisionName
	}

	if ss.Spec.InjectionStrategy.Revision != nil && ss.Spec.InjectionStrategy.Revision.CustomVersion != nil {
		hash = *ss.Spec.InjectionStrategy.Revision.CustomVersion
	}

	object := &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: webhookutil.GetNamespace(),
			Name:      name,
		},
	}

	objectList := &apps.ControllerRevisionList{
		Items: []apps.ControllerRevision{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: webhookutil.GetNamespace(),
					Name:      "default",
					Labels: map[string]string{
						sidecarcontrol.SidecarSetKindName:         ss.GetName(),
						appsv1alpha1.SidecarSetCustomVersionLabel: hash,
					},
				},
			},
		},
	}

	whiteList := &configuration.SidecarSetPatchMetadataWhiteList{}
	if err := fuzzutils.GenerateSidecarSetWhiteListRule(cf, whiteList); err != nil {
		return nil, err
	}
	whiteListJson, err := json.Marshal(whiteList)
	if err != nil {
		return nil, err
	}

	config := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configuration.KruiseConfigurationName,
			Namespace: util.GetKruiseNamespace(),
		},
		Data: map[string]string{
			configuration.SidecarSetPatchPodMetadataWhiteListKey: string(whiteListJson),
		},
	}

	return &SidecarSetCreateUpdateHandler{
		Client:  fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(object, config).WithLists(objectList).Build(),
		Decoder: admission.NewDecoder(fakeScheme),
	}, err
}
