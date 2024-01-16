/*
Copyright 2024 The Kruise Authors.

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

package mutating

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util"
	utilcontainerlaunchpriority "github.com/openkruise/kruise/pkg/util/containerlaunchpriority"
)

func TestContainerLaunchPriorityInitialization(t *testing.T) {
	cases := []struct {
		pod                *corev1.Pod
		expectedSkip       bool
		expectedContainers []corev1.Container
	}{
		{
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fake",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "a"},
						{Name: "b"},
					},
				},
			},
			expectedSkip: true,
			expectedContainers: []corev1.Container{
				{Name: "a"},
				{Name: "b"},
			},
		},
		{
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "fake",
					Annotations: map[string]string{appspub.ContainerLaunchPriorityKey: appspub.ContainerLaunchOrdered},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "a"},
						{Name: "b"},
						{Name: "c"},
					},
				},
			},
			expectedSkip: false,
			expectedContainers: []corev1.Container{
				{Name: "a", Env: []corev1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(0, "fake")}},
				{Name: "b", Env: []corev1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-1, "fake")}},
				{Name: "c", Env: []corev1.EnvVar{utilcontainerlaunchpriority.GeneratePriorityEnv(-2, "fake")}},
			},
		},
	}

	h := &PodCreateHandler{}
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Resource:  metav1.GroupVersionResource{Resource: "pods", Version: "v1"},
	}}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			skip, err := h.containerLaunchPriorityInitialization(context.TODO(), req, tc.pod)
			if err != nil {
				t.Fatal(err)
			}
			if skip != tc.expectedSkip {
				t.Fatalf("expected skip %v, got %v", tc.expectedSkip, skip)
			}
			if !reflect.DeepEqual(tc.expectedContainers, tc.pod.Spec.Containers) {
				t.Fatalf("expected containers\n%v\ngot\n%v", util.DumpJSON(tc.expectedContainers), util.DumpJSON(tc.pod.Spec.Containers))
			}
		})
	}
}
