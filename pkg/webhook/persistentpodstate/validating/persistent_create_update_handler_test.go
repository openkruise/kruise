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

package validating

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/configuration"
)

func TestPersistentPodStateCreateUpdateHandler_Handle(t *testing.T) {
	testScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(testScheme))
	utilruntime.Must(appsv1beta1.AddToScheme(testScheme))

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: util.GetKruiseNamespace(),
			Name:      configuration.KruiseConfigurationName,
		},
		Data: map[string]string{},
	}
	handler := &PersistentPodStateCreateUpdateHandler{
		Client:  fake.NewClientBuilder().WithScheme(testScheme).WithObjects(configMap).Build(),
		Decoder: admission.NewDecoder(testScheme),
	}

	newBeta := func(name, stsName string) *appsv1beta1.PersistentPodState {
		return &appsv1beta1.PersistentPodState{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: appsv1beta1.PersistentPodStateSpec{
				TargetReference: appsv1beta1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       stsName,
				},
				RequiredPersistentTopology: &appsv1beta1.NodeTopologyTerm{
					Keys: []string{"kubernetes.io/hostname"},
				},
			},
		}
	}
	newAlpha := func(name, stsName string) *appsv1alpha1.PersistentPodState {
		return &appsv1alpha1.PersistentPodState{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: appsv1alpha1.PersistentPodStateSpec{
				TargetReference: appsv1alpha1.TargetReference{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       stsName,
				},
				RequiredPersistentTopology: &appsv1alpha1.NodeTopologyTerm{
					NodeTopologyKeys: []string{"kubernetes.io/hostname"},
				},
			},
		}
	}

	cases := []struct {
		name        string
		version     string
		operation   admissionv1.Operation
		buildObj    func() runtime.Object
		buildOldObj func() runtime.Object
		wantAllowed bool
	}{
		{
			name:        "v1beta1 create with keys",
			version:     appsv1beta1.GroupVersion.Version,
			operation:   admissionv1.Create,
			buildObj:    func() runtime.Object { return newBeta("pps-beta", "sts") },
			wantAllowed: true,
		},
		{
			name:        "v1alpha1 create with nodeTopologyKeys",
			version:     appsv1alpha1.GroupVersion.Version,
			operation:   admissionv1.Create,
			buildObj:    func() runtime.Object { return newAlpha("pps-alpha", "sts") },
			wantAllowed: true,
		},
		{
			name:        "v1beta1 update with unchanged targetRef is allowed",
			version:     appsv1beta1.GroupVersion.Version,
			operation:   admissionv1.Update,
			buildObj:    func() runtime.Object { return newBeta("pps-beta", "sts") },
			buildOldObj: func() runtime.Object { return newBeta("pps-beta", "sts") },
			wantAllowed: true,
		},
		{
			name:        "v1beta1 update changing targetRef is rejected (immutable)",
			version:     appsv1beta1.GroupVersion.Version,
			operation:   admissionv1.Update,
			buildObj:    func() runtime.Object { return newBeta("pps-beta", "sts-changed") },
			buildOldObj: func() runtime.Object { return newBeta("pps-beta", "sts") },
			wantAllowed: false,
		},
		{
			name:        "v1alpha1 update changing targetRef is rejected (immutable)",
			version:     appsv1alpha1.GroupVersion.Version,
			operation:   admissionv1.Update,
			buildObj:    func() runtime.Object { return newAlpha("pps-alpha", "sts-changed") },
			buildOldObj: func() runtime.Object { return newAlpha("pps-alpha", "sts") },
			wantAllowed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.buildObj())
			require.NoError(t, err)

			ar := admissionv1.AdmissionRequest{
				Operation: tc.operation,
				Resource: metav1.GroupVersionResource{
					Group:    "apps.kruise.io",
					Version:  tc.version,
					Resource: "persistentpodstates",
				},
				Object: runtime.RawExtension{Raw: raw},
			}
			if tc.buildOldObj != nil {
				oldRaw, err := json.Marshal(tc.buildOldObj())
				require.NoError(t, err)
				ar.OldObject = runtime.RawExtension{Raw: oldRaw}
			}

			resp := handler.Handle(context.Background(), admission.Request{AdmissionRequest: ar})
			var resultMsg string
			if resp.Result != nil {
				resultMsg = resp.Result.Message
			}
			require.Equal(t, tc.wantAllowed, resp.Allowed, "response: %s", resultMsg)
		})
	}
}
