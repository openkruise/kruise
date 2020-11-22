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

package validating

import (
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var valInt32 int32 = 1
var valInt64 int64 = 2

func TestValidateBroadcastJobSpec(t *testing.T) {
	bjSpec1 := &appsv1alpha1.BroadcastJobSpec{
		Parallelism: nil,
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"broadcastjob-name":           "bjtest",
					"broadcastjob-controller-uid": "1234",
				},
			},
			Spec: v1.PodSpec{
				RestartPolicy: v1.RestartPolicyAlways,
			},
		},
		CompletionPolicy: appsv1alpha1.CompletionPolicy{
			Type:                    appsv1alpha1.Never,
			TTLSecondsAfterFinished: &valInt32,
			ActiveDeadlineSeconds:   &valInt64,
		},
		Paused:        false,
		FailurePolicy: appsv1alpha1.FailurePolicy{},
	}
	fieldErrorList := validateBroadcastJobSpec(bjSpec1, field.NewPath("spec"))
	assert.NotNil(t, fieldErrorList, nil)
	assert.Equal(t, fieldErrorList[0].Field, "spec.completionPolicy.ttlSecondsAfterFinished")
	assert.Equal(t, fieldErrorList[1].Field, "spec.completionPolicy.activeDeadlineSeconds")
	assert.Equal(t, fieldErrorList[2].Field, "spec.template.spec.restartPolicy")
	assert.Equal(t, fieldErrorList[3].Field, "spec.template.metadata.labels")
}
