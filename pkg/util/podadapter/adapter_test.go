/*
Copyright 2023 The Kruise Authors.

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

package podadapter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type testPatch struct {
	data      []byte
	patchType types.PatchType
	err       error
}

func (p *testPatch) Type() types.PatchType {
	return p.patchType
}

func (p *testPatch) Data(obj ctrlclient.Object) ([]byte, error) {
	return p.data, p.err
}

// Helper function to create a test Pod
func newTestPod(name, namespace string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "test-container",
					Image: "nginx:latest",
				},
			},
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}
}

func TestAdapterRuntimeClient(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	testPod := newTestPod("test-pod", "default")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(testPod).Build()
	adapter := &AdapterRuntimeClient{Client: cl}

	t.Run("GetPod", func(t *testing.T) {
		pod, err := adapter.GetPod("default", "test-pod")
		assert.NoError(t, err)
		assert.Equal(t, "test-pod", pod.Name)
	})

	t.Run("UpdatePod", func(t *testing.T) {
		updatedPod, err := adapter.UpdatePod(testPod)
		assert.NoError(t, err)
		assert.Equal(t, "test-pod", updatedPod.Name)
	})

	t.Run("UpdatePodStatus", func(t *testing.T) {
		err := adapter.UpdatePodStatus(testPod)
		assert.NoError(t, err)
	})
}

func TestAdapterTypedClient_PatchPod(t *testing.T) {
	testPod := newTestPod("test-pod", "default")
	clientset := fakeclientset.NewSimpleClientset(testPod)
	adapter := &AdapterTypedClient{Client: clientset}

	t.Run("successful patch", func(t *testing.T) {
		patch := &testPatch{
			data:      []byte(`{"metadata":{"labels":{"test":"value"}}}`),
			patchType: types.StrategicMergePatchType,
		}

		result, err := adapter.PatchPod(testPod, patch)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "test-pod", result.Name)
	})

	t.Run("patch data error", func(t *testing.T) {
		patch := &testPatch{
			err: assert.AnError,
		}

		result, err := adapter.PatchPod(testPod, patch)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, assert.AnError, err)
	})
}

func TestAdapterTypedClient_PatchPodResource(t *testing.T) {
	testPod := newTestPod("test-pod", "default")
	clientset := fakeclientset.NewSimpleClientset(testPod)
	adapter := &AdapterTypedClient{Client: clientset}

	t.Run("successful resource patch", func(t *testing.T) {
		patch := &testPatch{
			data:      []byte(`{"spec":{"containers":[{"name":"test-container","resources":{"requests":{"cpu":"200m"}}}]}}`),
			patchType: types.StrategicMergePatchType,
		}

		result, err := adapter.PatchPodResource(testPod, patch)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "test-pod", result.Name)
	})

	t.Run("patch data error", func(t *testing.T) {
		patch := &testPatch{
			err: assert.AnError,
		}

		result, err := adapter.PatchPodResource(testPod, patch)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, assert.AnError, err)
	})
}

func TestAdapterInformer_UpdatePod(t *testing.T) {
	testPod := newTestPod("test-pod", "default")
	clientset := fakeclientset.NewSimpleClientset()

	informerFactory := informers.NewSharedInformerFactory(clientset, time.Minute)
	podInformer := informerFactory.Core().V1().Pods()
	adapter := &AdapterInformer{PodInformer: podInformer}

	result, err := adapter.UpdatePod(testPod)
	assert.NoError(t, err)
	assert.Equal(t, testPod, result)
}

func TestAdapterInformer_UpdatePodStatus(t *testing.T) {
	testPod := newTestPod("test-pod", "default")
	testPod.Status.Phase = v1.PodRunning

	clientset := fakeclientset.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(clientset, time.Minute)
	podInformer := informerFactory.Core().V1().Pods()
	adapter := &AdapterInformer{PodInformer: podInformer}

	err := adapter.UpdatePodStatus(testPod)
	assert.NoError(t, err)
}
