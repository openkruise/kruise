/*
Copyright 2022 The Kruise Authors.

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

package configuration

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openkruise/kruise/pkg/util"
)

func TestGetKruiseConfiguration(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))

	testCases := []struct {
		name          string
		existingObjs  []client.Object
		expectData    map[string]string
		expectErr     bool
		expectNilData bool
	}{
		{
			name: "ConfigMap found",
			existingObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: util.GetKruiseNamespace(),
						Name:      KruiseConfigurationName,
					},
					Data: map[string]string{"foo": "bar"},
				},
			},
			expectData: map[string]string{"foo": "bar"},
			expectErr:  false,
		},
		{
			name:          "ConfigMap not found",
			existingObjs:  []client.Object{},
			expectData:    map[string]string{},
			expectErr:     false,
			expectNilData: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.existingObjs...).Build()
			data, err := getKruiseConfiguration(fakeClient)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tc.expectNilData {
				assert.Nil(t, data)
			} else {
				assert.Equal(t, tc.expectData, data)
			}
		})
	}
}

func TestGetSidecarSetPatchMetadataWhiteList(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))

	validWhitelist := &SidecarSetPatchMetadataWhiteList{
		Rules: []SidecarSetPatchMetadataWhiteRule{
			{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}}},
		},
	}
	validWhitelistJSON, _ := json.Marshal(validWhitelist)

	testCases := []struct {
		name         string
		existingObjs []client.Object
		expectErr    bool
		expectNil    bool
		expectResult *SidecarSetPatchMetadataWhiteList
	}{
		{
			name: "Success: ConfigMap and key exist",
			existingObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: util.GetKruiseNamespace(), Name: KruiseConfigurationName},
					Data:       map[string]string{SidecarSetPatchPodMetadataWhiteListKey: string(validWhitelistJSON)},
				},
			},
			expectErr:    false,
			expectNil:    false,
			expectResult: validWhitelist,
		},
		{
			name:      "Success: ConfigMap not found",
			expectErr: false,
			expectNil: true,
		},
		{
			name: "Success: Key not found in ConfigMap",
			existingObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: util.GetKruiseNamespace(), Name: KruiseConfigurationName},
					Data:       map[string]string{"other-key": "other-value"},
				},
			},
			expectErr: false,
			expectNil: true,
		},
		{
			name: "Error: Invalid JSON",
			existingObjs: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: util.GetKruiseNamespace(), Name: KruiseConfigurationName},
					Data:       map[string]string{SidecarSetPatchPodMetadataWhiteListKey: `{"invalid-json`},
				},
			},
			expectErr: true,
			expectNil: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.existingObjs...).Build()
			result, err := GetSidecarSetPatchMetadataWhiteList(fakeClient)

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tc.expectNil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tc.expectResult, result)
			}
		})
	}
}

func TestGetPPSWatchCustomWorkloadWhiteList(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))

	validWhitelist := &CustomWorkloadWhiteList{
		Workloads: []schema.GroupVersionKind{{Group: "apps.kruise.io", Version: "v1alpha1", Kind: "CloneSet"}},
	}
	validWhitelistJSON, _ := json.Marshal(validWhitelist)

	t.Run("Success: key exists", func(t *testing.T) {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: util.GetKruiseNamespace(), Name: KruiseConfigurationName},
			Data:       map[string]string{PPSWatchCustomWorkloadWhiteList: string(validWhitelistJSON)},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()

		result, err := GetPPSWatchCustomWorkloadWhiteList(fakeClient)
		assert.NoError(t, err)
		assert.Equal(t, validWhitelist, result)
	})

	t.Run("Success: key does not exist", func(t *testing.T) {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: util.GetKruiseNamespace(), Name: KruiseConfigurationName},
			Data:       map[string]string{},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()

		result, err := GetPPSWatchCustomWorkloadWhiteList(fakeClient)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Workloads)
	})

	t.Run("Error: invalid json", func(t *testing.T) {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: util.GetKruiseNamespace(), Name: KruiseConfigurationName},
			Data:       map[string]string{PPSWatchCustomWorkloadWhiteList: `{"invalid`},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
		_, err := GetPPSWatchCustomWorkloadWhiteList(fakeClient)
		assert.Error(t, err)
	})
}

func TestGetWSWatchCustomWorkloadWhiteList(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(scheme))

	validWhitelist := WSCustomWorkloadWhiteList{
		Workloads: []CustomWorkload{
			{
				GroupVersionKind: schema.GroupVersionKind{Group: "apps.kruise.io", Version: "v1alpha1", Kind: "CloneSet"},
				ReplicasPath:     "spec.replicas",
			},
		},
	}
	validWhitelistJSON, _ := json.Marshal(validWhitelist)

	t.Run("Success: key exists", func(t *testing.T) {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: util.GetKruiseNamespace(), Name: KruiseConfigurationName},
			Data:       map[string]string{WSWatchCustomWorkloadWhiteList: string(validWhitelistJSON)},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()

		result, err := GetWSWatchCustomWorkloadWhiteList(fakeClient)
		assert.NoError(t, err)
		assert.Equal(t, validWhitelist, result)
	})

	t.Run("Success: configmap not found", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		result, err := GetWSWatchCustomWorkloadWhiteList(fakeClient)
		assert.NoError(t, err)
		assert.Empty(t, result.Workloads)
	})

	t.Run("Error: invalid json", func(t *testing.T) {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: util.GetKruiseNamespace(), Name: KruiseConfigurationName},
			Data:       map[string]string{WSWatchCustomWorkloadWhiteList: `{"invalid`},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
		_, err := GetWSWatchCustomWorkloadWhiteList(fakeClient)
		assert.Error(t, err)
	})
}
