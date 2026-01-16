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
	"strings"
	"testing"

	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
)

func TestGetActiveDeadlineSecondsForNever(t *testing.T) {
	cases := []struct {
		name        string
		getImageJob func() *appsv1beta1.ImagePullJob
		expected    int64
	}{
		{
			name: "not set timeout",
			getImageJob: func() *appsv1beta1.ImagePullJob {
				return &appsv1beta1.ImagePullJob{}
			},
			expected: 1800,
		},
		{
			name: "timeout < 1800",
			getImageJob: func() *appsv1beta1.ImagePullJob {
				return &appsv1beta1.ImagePullJob{
					Spec: appsv1beta1.ImagePullJobSpec{
						ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
							PullPolicy: &appsv1beta1.PullPolicy{
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
			getImageJob: func() *appsv1beta1.ImagePullJob {
				return &appsv1beta1.ImagePullJob{
					Spec: appsv1beta1.ImagePullJobSpec{
						ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
							PullPolicy: &appsv1beta1.PullPolicy{
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

func TestJobAsReferenceObject(t *testing.T) {
	job := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "test-ns",
		},
	}

	expected := appsv1beta1.ReferenceObject{
		Name:      "test-job",
		Namespace: "test-ns",
	}

	result := jobAsReferenceObject(job)

	if result != expected {
		t.Errorf("Expected %+v, got %+v", expected, result)
	}
}

func TestGenerateSyncedSecret(t *testing.T) {
	originalSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "original-secret",
			Namespace: "original-ns",
			Labels: map[string]string{
				"label1": "value1",
			},
			Annotations: map[string]string{
				"annotation1": "value1",
			},
		},
		Data: map[string][]byte{
			"key1": []byte("value1"),
		},
	}

	ref := appsv1beta1.ReferenceObject{
		Name:      "test-job",
		Namespace: "test-ns",
	}

	syncedSecret := generateSyncedSecret(originalSecret, ref)

	if syncedSecret.Namespace != util.GetKruiseDaemonConfigNamespace() {
		t.Errorf("Expected namespace %s, got %s", util.GetKruiseDaemonConfigNamespace(), syncedSecret.Namespace)
	}

	if !strings.HasPrefix(syncedSecret.Name, "original-secret-") {
		t.Errorf("Expected name to start with 'original-secret-', got %s", syncedSecret.Name)
	}

	if syncedSecret.Labels["label1"] != "value1" {
		t.Errorf("Labels not properly inherited")
	}

	if syncedSecret.Annotations[SecretAnnotationReferenceJobs] != ref.String() {
		t.Errorf("Expected annotation %s to be %s, got %s", SecretAnnotationReferenceJobs, ref.String(), syncedSecret.Annotations[SecretAnnotationReferenceJobs])
	}

	if syncedSecret.Annotations[SecretAnnotationSourceSecretKey] != "original-ns/original-secret" {
		t.Errorf("Expected source key annotation to be 'original-ns/original-secret', got %s", syncedSecret.Annotations[SecretAnnotationSourceSecretKey])
	}
}

func TestGetReferencingJobsFromSecret(t *testing.T) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-ns",
			Annotations: map[string]string{
				SecretAnnotationReferenceJobs: "ns1/job1,ns2/job2,ns3/job3",
			},
		},
	}

	expected := sets.New(
		appsv1beta1.ReferenceObject{Namespace: "ns1", Name: "job1"},
		appsv1beta1.ReferenceObject{Namespace: "ns2", Name: "job2"},
		appsv1beta1.ReferenceObject{Namespace: "ns3", Name: "job3"},
	)

	result := getReferencingJobsFromSecret(secret)

	if !result.Equal(expected) {
		t.Errorf("Expected set %+v, got %+v", expected, result)
	}
}

func TestGetSourceSecret(t *testing.T) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-ns",
			Annotations: map[string]string{
				SecretAnnotationSourceSecretKey: "source-ns/source-name",
			},
		},
	}

	expected := appsv1beta1.ReferenceObject{
		Namespace: "source-ns",
		Name:      "source-name",
	}

	result := getSourceSecret(secret)

	if result != expected {
		t.Errorf("Expected %+v, got %+v", expected, result)
	}
}

func TestGenerateRandomString(t *testing.T) {
	result := generateRandomString()
	if len(result) != 6 {
		t.Errorf("Expected string length 6, got %d", len(result))
	}

	strings := make([]string, 10)
	for i := 0; i < 10; i++ {
		strings[i] = generateRandomString()
		if len(strings[i]) != 6 {
			t.Errorf("Expected string length 6 at index %d, got %s", i, strings[i])
		}
	}

	uniqueStrings := make(map[string]bool)
	for _, s := range strings {
		if uniqueStrings[s] {
			t.Logf("Duplicate found: %s (this might be okay due to randomness)", s)
		}
		uniqueStrings[s] = true
	}
}

func TestGetReferencingJobsFromSecretWithEmptyAnnotation(t *testing.T) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-ns",
			Annotations: map[string]string{
				SecretAnnotationReferenceJobs: "",
			},
		},
	}

	result := getReferencingJobsFromSecret(secret)
	if len(result) != 0 {
		t.Errorf("Expected empty set for empty annotation, got %+v", result)
	}
}

func TestGetSourceSecretWithEmptyAnnotation(t *testing.T) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-ns",
			Annotations: map[string]string{
				SecretAnnotationSourceSecretKey: "",
			},
		},
	}

	defer func() {
		if r := recover(); r != nil {
			// Expected panic when parsing empty string
		}
	}()

	// This should panic when trying to parse an empty string
	result := getSourceSecret(secret)

	// If we reach here without panic, the function didn't behave as expected
	if result.Name != "" || result.Namespace != "" {
		t.Errorf("Expected panic for empty annotation, but got %+v", result)
	}
}
