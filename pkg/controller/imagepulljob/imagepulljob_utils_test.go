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

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
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
								TimeoutSeconds: ptr.To(int32(1799)),
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
								TimeoutSeconds: ptr.To(int32(7200)),
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

func TestGetTTLSecondsForAlways(t *testing.T) {
	cases := []struct {
		name        string
		getImageJob func() *appsv1alpha1.ImagePullJob
		checkFunc   func(result *int32) bool
	}{
		{
			name: "with TTLSecondsAfterFinished",
			getImageJob: func() *appsv1alpha1.ImagePullJob {
				return &appsv1alpha1.ImagePullJob{
					Spec: appsv1alpha1.ImagePullJobSpec{
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{
								TTLSecondsAfterFinished: ptr.To(int32(3600)),
							},
						},
					},
				}
			},
			checkFunc: func(result *int32) bool {
				return *result >= 3600 && *result <= 3600+600
			},
		},
		{
			name: "with ActiveDeadlineSeconds",
			getImageJob: func() *appsv1alpha1.ImagePullJob {
				return &appsv1alpha1.ImagePullJob{
					Spec: appsv1alpha1.ImagePullJobSpec{
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							CompletionPolicy: appsv1alpha1.CompletionPolicy{
								ActiveDeadlineSeconds: ptr.To(int64(7200)),
							},
						},
					},
				}
			},
			checkFunc: func(result *int32) bool {
				return *result >= 7200 && *result <= 7200+600
			},
		},
		{
			name: "with PullPolicy",
			getImageJob: func() *appsv1alpha1.ImagePullJob {
				return &appsv1alpha1.ImagePullJob{
					Spec: appsv1alpha1.ImagePullJobSpec{
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							PullPolicy: &appsv1alpha1.PullPolicy{
								TimeoutSeconds: ptr.To(int32(300)),
								BackoffLimit:   ptr.To(int32(5)),
							},
						},
					},
				}
			},
			checkFunc: func(result *int32) bool {
				expectedBase := int32(300 * 5)
				return *result >= expectedBase && *result <= expectedBase+600
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			ret := getTTLSecondsForAlways(cs.getImageJob())
			if !cs.checkFunc(ret) {
				t.Fatalf("result %d is not in expected range", *ret)
			}
		})
	}
}

func TestGetTTLSecondsForNever(t *testing.T) {
	result := getTTLSecondsForNever()
	if *result < defaultTTLSecondsForNever-600 || *result > defaultTTLSecondsForNever+600 {
		t.Fatalf("expected result to be in range [%d, %d], but got %d",
			defaultTTLSecondsForNever-600, defaultTTLSecondsForNever+600, *result)
	}
}

func TestGetSecrets(t *testing.T) {
	cases := []struct {
		name        string
		getImageJob func() *appsv1alpha1.ImagePullJob
		expected    []appsv1alpha1.ReferenceObject
	}{
		{
			name: "no secrets",
			getImageJob: func() *appsv1alpha1.ImagePullJob {
				return &appsv1alpha1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							PullSecrets: []string{},
						},
					},
				}
			},
			expected: []appsv1alpha1.ReferenceObject{},
		},
		{
			name: "with secrets",
			getImageJob: func() *appsv1alpha1.ImagePullJob {
				return &appsv1alpha1.ImagePullJob{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
					},
					Spec: appsv1alpha1.ImagePullJobSpec{
						ImagePullJobTemplate: appsv1alpha1.ImagePullJobTemplate{
							PullSecrets: []string{"secret1", "secret2"},
						},
					},
				}
			},
			expected: []appsv1alpha1.ReferenceObject{
				{Namespace: "default", Name: "secret1"},
				{Namespace: "default", Name: "secret2"},
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			result := getSecrets(cs.getImageJob())
			if !reflect.DeepEqual(result, cs.expected) {
				t.Fatalf("expected %v, but got %v", cs.expected, result)
			}
		})
	}
}

func TestContainsObject(t *testing.T) {
	slice := []appsv1alpha1.ReferenceObject{
		{Namespace: "ns1", Name: "name1"},
		{Namespace: "ns2", Name: "name2"},
	}

	cases := []struct {
		name     string
		obj      appsv1alpha1.ReferenceObject
		expected bool
	}{
		{
			name:     "contains object",
			obj:      appsv1alpha1.ReferenceObject{Namespace: "ns1", Name: "name1"},
			expected: true,
		},
		{
			name:     "does not contain object",
			obj:      appsv1alpha1.ReferenceObject{Namespace: "ns3", Name: "name3"},
			expected: false,
		},
		{
			name:     "same name different namespace",
			obj:      appsv1alpha1.ReferenceObject{Namespace: "ns3", Name: "name1"},
			expected: false,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			result := containsObject(slice, cs.obj)
			if result != cs.expected {
				t.Fatalf("expected %v, but got %v", cs.expected, result)
			}
		})
	}
}

func TestFormatStatusMessage(t *testing.T) {
	cases := []struct {
		name     string
		status   *appsv1alpha1.ImagePullJobStatus
		expected string
	}{
		{
			name: "completed job",
			status: &appsv1alpha1.ImagePullJobStatus{
				CompletionTime: &metav1.Time{},
			},
			expected: "job has completed",
		},
		{
			name: "no progress",
			status: &appsv1alpha1.ImagePullJobStatus{
				Desired: 0,
			},
			expected: "job is running, no progress",
		},
		{
			name: "partial progress",
			status: &appsv1alpha1.ImagePullJobStatus{
				Desired:   10,
				Succeeded: 3,
				Failed:    2,
			},
			expected: "job is running, progress 50.0%",
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			result := formatStatusMessage(cs.status)
			if result != cs.expected {
				t.Fatalf("expected %s, but got %s", cs.expected, result)
			}
		})
	}
}

func TestKeyFromRef(t *testing.T) {
	ref := appsv1alpha1.ReferenceObject{
		Namespace: "test-ns",
		Name:      "test-name",
	}

	result := keyFromRef(ref)
	expected := types.NamespacedName{
		Namespace: "test-ns",
		Name:      "test-name",
	}

	if result != expected {
		t.Fatalf("expected %v, but got %v", expected, result)
	}
}

func TestKeyFromObject(t *testing.T) {
	obj := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "test-name",
		},
	}

	result := keyFromObject(obj)
	expected := types.NamespacedName{
		Namespace: "test-ns",
		Name:      "test-name",
	}

	if result != expected {
		t.Fatalf("expected %v, but got %v", expected, result)
	}
}

func TestReferenceSet(t *testing.T) {
	set := makeReferenceSet()

	// Test Insert
	key1 := types.NamespacedName{Namespace: "ns1", Name: "name1"}
	key2 := types.NamespacedName{Namespace: "ns2", Name: "name2"}

	set.Insert(key1)
	set.Insert(key2)

	// Test Contains
	if !set.Contains(key1) {
		t.Fatalf("expected set to contain %v", key1)
	}
	if !set.Contains(key2) {
		t.Fatalf("expected set to contain %v", key2)
	}

	// Test String
	str := set.String()
	if !reflect.DeepEqual(str, "ns1/name1,ns2/name2") && !reflect.DeepEqual(str, "ns2/name2,ns1/name1") {
		t.Fatalf("unexpected string representation: %s", str)
	}

	// Test Delete
	set.Delete(key1)
	if set.Contains(key1) {
		t.Fatalf("expected set to not contain %v after deletion", key1)
	}

	// Test IsEmpty
	if set.IsEmpty() {
		t.Fatalf("expected set to not be empty")
	}

	set.Delete(key2)
	if !set.IsEmpty() {
		t.Fatalf("expected set to be empty")
	}
}

func TestComputeTargetSyncAction(t *testing.T) {
	source := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "source-ns",
			Name:      "source-name",
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}

	job := &appsv1alpha1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "job-ns",
			Name:      "job-name",
		},
	}

	cases := []struct {
		name     string
		target   *v1.Secret
		expected syncAction
	}{
		{
			name:     "no target",
			target:   nil,
			expected: create,
		},
		{
			name: "target with empty UID",
			target: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					UID: "",
				},
			},
			expected: create,
		},
		{
			name: "target needs update - different data",
			target: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					UID: "test-uid",
					Annotations: map[string]string{
						TargetOwnerReferencesAnno: "job-ns/job-name",
					},
				},
				Data: map[string][]byte{
					"key": []byte("different-value"),
				},
			},
			expected: update,
		},
		{
			name: "target needs update - missing job reference",
			target: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					UID: "test-uid",
					Annotations: map[string]string{
						TargetOwnerReferencesAnno: "other-ns/other-name",
					},
				},
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			},
			expected: update,
		},
		{
			name: "no action needed",
			target: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					UID: "test-uid",
					Annotations: map[string]string{
						TargetOwnerReferencesAnno: "job-ns/job-name",
					},
				},
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			},
			expected: noAction,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			result := computeTargetSyncAction(source, cs.target, job)
			if result != cs.expected {
				t.Fatalf("expected %v, but got %v", cs.expected, result)
			}
		})
	}
}
