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

package pubcontrol

import (
	"fmt"
	"testing"

	"github.com/openkruise/kruise/apis/apps/pub"
	corev1 "k8s.io/api/core/v1"
)

func TestIsPodUnavailableChanged(t *testing.T) {
	cases := []struct {
		name      string
		getOldPod func() *corev1.Pod
		getNewPod func() *corev1.Pod
		expect    bool
	}{
		{
			name: "only annotations change",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Annotations["add"] = "annotations"
				return demo
			},
			expect: false,
		},
		{
			name: "add unavailable label",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Labels[fmt.Sprintf("%sdata", pub.PubUnavailablePodLabelPrefix)] = "true"
				return demo
			},
			expect: true,
		},
		{
			name: "image changed",
			getOldPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			getNewPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Spec.Containers[0].Image = "nginx:v2"
				return demo
			},
			expect: true,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			control := commonControl{}
			is := control.IsPodUnavailableChanged(cs.getOldPod(), cs.getNewPod())
			if cs.expect != is {
				t.Fatalf("IsPodUnavailableChanged failed")
			}
		})
	}
}

func TestIsPodReady(t *testing.T) {
	cases := []struct {
		name   string
		getPod func() *corev1.Pod
		expect bool
	}{
		{
			name: "pod ready",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				return demo
			},
			expect: true,
		},
		{
			name: "pod not ready",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Status.Conditions[0].Status = corev1.ConditionFalse
				return demo
			},
			expect: false,
		},
		{
			name: "pod contains unavailable label",
			getPod: func() *corev1.Pod {
				demo := podDemo.DeepCopy()
				demo.Labels[fmt.Sprintf("%sdata", pub.PubUnavailablePodLabelPrefix)] = "true"
				return demo
			},
			expect: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			control := commonControl{}
			is := control.IsPodReady(cs.getPod())
			if cs.expect != is {
				t.Fatalf("IsPodReady failed")
			}
		})
	}
}
