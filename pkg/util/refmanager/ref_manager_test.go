/*
Copyright 2019 The Kruise Authors.

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

package refmanager

import (
	"testing"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	val int32 = 2
)

func Test(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	UID := "uid"
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
			UID:       types.UID(UID),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &val,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "foo",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "foo",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginxImage",
						},
					},
				},
			},
		},
	}

	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod",
				Namespace: "default",
				Labels: map[string]string{
					"app": "foo",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: "nginx",
					},
				},
			},
		},
	}

	getOwner = func(owner metav1.Object, schema *runtime.Scheme, c client.Client) (runtime.Object, error) {
		return sts, nil
	}

	var ownerRefs []metav1.OwnerReference
	updateOwner = func(obj client.Object, c client.Client) (err error) {
		ownerRefs = obj.GetOwnerReferences()
		return nil
	}
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, &appsv1.StatefulSet{})
	m, err := New(nil, sts.Spec.Selector, sts, scheme)
	g.Expect(err).Should(gomega.BeNil())

	mts := make([]metav1.Object, 1)
	for i, pod := range pods {
		mts[i] = &pod
	}
	ps, err := m.ClaimOwnedObjects(mts)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(len(ps)).Should(gomega.BeEquivalentTo(1))

	// remove pod label
	pod := pods[0]
	pod.Labels["app"] = "foo2"

	mts = make([]metav1.Object, 1)
	mts[0] = &pod
	ps, err = m.ClaimOwnedObjects(mts)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(len(ps)).Should(gomega.BeEquivalentTo(0))

	// remove pod label
	pod.OwnerReferences = ownerRefs

	mts = make([]metav1.Object, 1)
	mts[0] = &pod
	ps, err = m.ClaimOwnedObjects(mts)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(len(ps)).Should(gomega.BeEquivalentTo(0))
}
