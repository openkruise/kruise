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

package validating

import (
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = appsv1alpha1.AddToScheme(fakeScheme)
}

func FuzzValidateWorkloadSpreadSpec(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		ws := &appsv1alpha1.WorkloadSpread{}
		if err := cf.GenerateStruct(ws); err != nil {
			return
		}

		if err := fuzzutils.GenerateWorkloadSpreadTargetReference(cf, ws); err != nil {
			return
		}
		if err := fuzzutils.GenerateWorkloadSpreadTargetFilter(cf, ws); err != nil {
			return
		}
		if err := fuzzutils.GenerateWorkloadSpreadScheduleStrategy(cf, ws); err != nil {
			return
		}
		if err := fuzzutils.GenerateWorkloadSpreadSubset(cf, ws); err != nil {
			return
		}

		whiteList, err := cf.GetString()
		if err != nil {
			return
		}
		if validWhiteList, err := cf.GetBool(); err == nil && validWhiteList {
			whiteList = "{\"workloads\":[{\"Group\":\"test\",\"Kind\":\"TFJob\"}]}"
			if matched, err := cf.GetBool(); err == nil && matched {
				whiteList = "{\"workloads\":[{\"Group\":\"training.kubedl.io\",\"Kind\":\"TFJob\"}]}"
			}
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(&appsv1alpha1.CloneSet{ObjectMeta: metav1.ObjectMeta{Name: "valid-target", Namespace: "default"}},
				&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "valid-target", Namespace: "default"}},
				&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "valid-target", Namespace: "default"}},
				&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "valid-target", Namespace: "default"}},
				&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "valid-target", Namespace: "default"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kruise-configuration", Namespace: "kruise-system"},
					Data: map[string]string{"WorkloadSpread_Watch_Custom_Workload_WhiteList": whiteList}},
			).Build()

		h := &WorkloadSpreadCreateUpdateHandler{Client: fakeClient}
		_ = validateWorkloadSpreadSpec(h, ws, field.NewPath("spec"))
	})
}

func FuzzValidateWorkloadSpreadConflict(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		ws := &appsv1alpha1.WorkloadSpread{}
		if err := cf.GenerateStruct(ws); err != nil {
			return
		}

		others := make([]appsv1alpha1.WorkloadSpread, 0)
		if numOthers, err := cf.GetInt(); err == nil {
			for i := 0; i < numOthers%5; i++ {
				other := appsv1alpha1.WorkloadSpread{}
				if err := cf.GenerateStruct(&other); err != nil {
					continue
				}

				if sameName, err := cf.GetBool(); sameName && err == nil {
					other.Name = ws.Name
				}

				if ws.Spec.TargetReference != nil {
					if conflict, err := cf.GetBool(); conflict && err == nil {
						other.Spec.TargetReference = &appsv1alpha1.TargetReference{
							APIVersion: ws.Spec.TargetReference.APIVersion,
							Kind:       ws.Spec.TargetReference.Kind,
							Name:       ws.Spec.TargetReference.Name,
						}
					}
				}
				others = append(others, other)
			}
		}

		_ = validateWorkloadSpreadConflict(ws, others, field.NewPath("spec"))
	})
}

func FuzzValidateWorkloadSpreadTargetRefUpdate(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		targetRef := &appsv1alpha1.TargetReference{}
		if err := cf.GenerateStruct(targetRef); err != nil {
			return
		}

		oldTargetRef := &appsv1alpha1.TargetReference{}
		if err := cf.GenerateStruct(oldTargetRef); err != nil {
			return
		}

		if sameGroup, err := cf.GetBool(); sameGroup && err == nil {
			if group, err := cf.GetString(); err == nil {
				targetRef.APIVersion, oldTargetRef.APIVersion = group+"/v1", group+"/v1"
			}
		}

		if sameKind, err := cf.GetBool(); sameKind && err == nil {
			if kind, err := cf.GetString(); err == nil {
				targetRef.Kind, oldTargetRef.Kind = kind, kind
			}
		}

		if sameName, err := cf.GetBool(); sameName && err == nil {
			if name, err := cf.GetString(); err == nil {
				targetRef.Name, oldTargetRef.Name = name, name
			}
		}

		_ = validateWorkloadSpreadTargetRefUpdate(targetRef, oldTargetRef, field.NewPath("spec"))
	})
}
