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

package imagepulljob

import (
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilpointer "k8s.io/utils/pointer"
)

func TestTargetFromSource(t *testing.T) {
	cases := []struct {
		name    string
		getPara func() (*v1.Secret, referenceSet)
		expect  *v1.Secret
	}{
		{
			name: "test1, normal1",
			getPara: func() (*v1.Secret, referenceSet) {
				s1 := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-foo",
						Name:      "foo",
						Annotations: map[string]string{
							"anno1": "value1",
						},
						Labels: map[string]string{
							"labels1": "value2",
						},
						UID: types.UID("db8acf1c-be68-46a2-9a40-a36c65eedd84"),
					},
					Type: v1.SecretTypeOpaque,
					Data: map[string][]byte{
						"data": []byte("foo"),
					},
				}
				ref := map[types.NamespacedName]struct{}{
					{Namespace: "ns-foo", Name: "name1"}: {},
				}
				return s1, ref
			},
			expect: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"anno1":                   "value1",
						SourceSecretKeyAnno:       "ns-foo/foo",
						TargetOwnerReferencesAnno: "ns-foo/name1",
					},
					Labels: map[string]string{
						"labels1":               "value2",
						SourceSecretUIDLabelKey: "db8acf1c-be68-46a2-9a40-a36c65eedd84",
					},
				},
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					"data": []byte("foo"),
				},
			},
		},
		{
			name: "test1, normal2",
			getPara: func() (*v1.Secret, referenceSet) {
				s1 := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns-foo",
						Name:      "foo",
						UID:       types.UID("db8acf1c-be68-46a2-9a40-a36c65eedd84"),
					},
					Type: v1.SecretTypeOpaque,
					Data: map[string][]byte{
						"data": []byte("foo"),
					},
				}
				ref := map[types.NamespacedName]struct{}{
					{Namespace: "ns-foo", Name: "name1"}: {},
				}
				return s1, ref
			},
			expect: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						SourceSecretKeyAnno:       "ns-foo/foo",
						TargetOwnerReferencesAnno: "ns-foo/name1",
					},
					Labels: map[string]string{
						SourceSecretUIDLabelKey: "db8acf1c-be68-46a2-9a40-a36c65eedd84",
					},
				},
				Type: v1.SecretTypeOpaque,
				Data: map[string][]byte{
					"data": []byte("foo"),
				},
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			obj := targetFromSource(cs.getPara())
			obj.Namespace = ""
			obj.GenerateName = ""
			if !reflect.DeepEqual(obj, cs.expect) {
				t.Fatalf("expect(%s), but get(%s)", util.DumpJSON(cs.expect), util.DumpJSON(obj))
			}
		})
	}
}

func TestGetActiveDeadlineSecondsForNever(t *testing.T) {
	cases := []struct {
		name        string
		getImageJob func() *appsv1alpha1.ImagePullJob
		expected    int64
	}{
		{
			name: "not set timeout",
			getImageJob: func() *appsv1alpha1.ImagePullJob {
				return &appsv1alpha1.ImagePullJob{}
			},
			expected: 1800,
		},
		{
			name: "timeout < 1800",
			getImageJob: func() *appsv1alpha1.ImagePullJob {
				return &appsv1alpha1.ImagePullJob{
					Spec: appsv1alpha1.ImagePullJobSpec{
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							PullPolicy: &appsv1alpha1.PullPolicy{
								TimeoutSeconds: utilpointer.Int32Ptr(1799),
							},
						},
					},
				}
			},
			expected: 1800,
		},
		{
			name: "timeout > 1800",
			getImageJob: func() *appsv1alpha1.ImagePullJob {
				return &appsv1alpha1.ImagePullJob{
					Spec: appsv1alpha1.ImagePullJobSpec{
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							PullPolicy: &appsv1alpha1.PullPolicy{
								TimeoutSeconds: utilpointer.Int32Ptr(7200),
							},
						},
					},
				}
			},
			expected: 7200,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			ret := getActiveDeadlineSecondsForNever(cs.getImageJob())
			if *ret != cs.expected {
				t.Fatalf("expect(%d), but get(%d)", cs.expected, *ret)
			}
		})
	}
}
