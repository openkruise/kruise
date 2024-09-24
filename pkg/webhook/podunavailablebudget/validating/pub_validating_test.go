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
	"context"
	"testing"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(policyv1alpha1.AddToScheme(scheme))
}

var (
	scheme *runtime.Scheme

	pubDemo = policyv1alpha1.PodUnavailableBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: policyv1alpha1.GroupVersion.String(),
			Kind:       "PodUnavailableBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Name:        "pub-test",
			Annotations: map[string]string{},
		},
		Spec: policyv1alpha1.PodUnavailableBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "pub-controller",
				},
			},
			TargetReference: &policyv1alpha1.TargetReference{
				APIVersion: "apps",
				Kind:       "Deployment",
				Name:       "deployment-test",
			},
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "20%",
			},
			MinAvailable: &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 100,
			},
		},
	}
)

func TestValidatingPub(t *testing.T) {
	cases := []struct {
		name          string
		pub           func() *policyv1alpha1.PodUnavailableBudget
		expectErrList int
	}{
		{
			name: "valid pub, TargetReference and MaxUnavailable",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "valid pub, Selector and MinAvailable",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MaxUnavailable = nil
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "invalid pub, Selector and TargetReference are nil",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "invalid pub, Selector and TargetReference are mutually exclusive",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.MinAvailable = nil
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "invalid pub, MaxUnavailable and MinAvailable are nil",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.MaxUnavailable = nil
				pub.Spec.MinAvailable = nil
				pub.Spec.Selector = nil
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "invalid pub, MaxUnavailable and MinAvailable are mutually exclusive",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "invalid pub feature-gate annotation",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Annotations[policyv1alpha1.PubProtectOperationAnnotation] = "xxxxx"
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "valid pub feature-gate annotation",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Annotations[policyv1alpha1.PubProtectOperationAnnotation] = string(policyv1alpha1.PubEvictOperation + "," + policyv1alpha1.PubDeleteOperation)
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "invalid pub feature-gate annotation",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Annotations[policyv1alpha1.PubProtectTotalReplicasAnnotation] = "%%"
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "valid pub feature-gate annotation",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Annotations[policyv1alpha1.PubProtectTotalReplicasAnnotation] = "1000"
				return pub
			},
			expectErrList: 0,
		},
	}

	decoder := admission.NewDecoder(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	pubHandler := PodUnavailableBudgetCreateUpdateHandler{
		Client:  client,
		Decoder: decoder,
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			errList := pubHandler.validatingPodUnavailableBudgetFn(cs.pub(), nil)
			if len(errList) != cs.expectErrList {
				t.Fatalf("expect errList(%d) but get(%d) error: %s", cs.expectErrList, len(errList), errList.ToAggregate().Error())
			}
		})
	}
}

func TestPubConflictWithOthers(t *testing.T) {
	cases := []struct {
		name          string
		pub           func() *policyv1alpha1.PodUnavailableBudget
		otherPubs     func() []*policyv1alpha1.PodUnavailableBudget
		expectErrList int
	}{
		{
			name: "no conflict with other pubs, and TargetReference",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			otherPubs: func() []*policyv1alpha1.PodUnavailableBudget {
				pub1 := pubDemo.DeepCopy()
				pub1.Name = "pub1"
				pub1.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps",
					Kind:       "Deployment",
					Name:       "deployment-test1",
				}
				pub2 := pubDemo.DeepCopy()
				pub2.Name = "pub2"
				pub2.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps",
					Kind:       "Deployment",
					Name:       "deployment-test2",
				}
				return []*policyv1alpha1.PodUnavailableBudget{pub1, pub2}
			},
			expectErrList: 0,
		},
		{
			name: "invalid conflict with other pubs, and TargetReference",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			otherPubs: func() []*policyv1alpha1.PodUnavailableBudget {
				pub1 := pubDemo.DeepCopy()
				pub1.Name = "pub1"
				pub1.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps",
					Kind:       "Deployment",
					Name:       "deployment-test",
				}
				pub2 := pubDemo.DeepCopy()
				pub2.Name = "pub2"
				pub2.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps",
					Kind:       "Deployment",
					Name:       "deployment-test2",
				}
				return []*policyv1alpha1.PodUnavailableBudget{pub1, pub2}
			},
			expectErrList: 1,
		},
		{
			name: "no conflict with other pubs, and Selector",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			otherPubs: func() []*policyv1alpha1.PodUnavailableBudget {
				pub1 := pubDemo.DeepCopy()
				pub1.Name = "pub1"
				pub1.Spec.TargetReference = nil
				pub1.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "pub1-controller",
					},
				}
				pub2 := pubDemo.DeepCopy()
				pub2.Name = "pub2"
				pub2.Spec.TargetReference = nil
				pub2.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "pub2-controller",
					},
				}
				return []*policyv1alpha1.PodUnavailableBudget{pub1, pub2}
			},
			expectErrList: 0,
		},
		{
			name: "conflict with other pubs, and Selector",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			otherPubs: func() []*policyv1alpha1.PodUnavailableBudget {
				pub1 := pubDemo.DeepCopy()
				pub1.Name = "pub1"
				pub1.Spec.TargetReference = nil
				pub1.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "pub-controller",
					},
				}
				pub2 := pubDemo.DeepCopy()
				pub2.Name = "pub2"
				pub2.Spec.TargetReference = nil
				pub2.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "pub2-controller",
					},
				}
				return []*policyv1alpha1.PodUnavailableBudget{pub1, pub2}
			},
			expectErrList: 1,
		},
		{
			name: "no conflict with other pubs, and Selector, other namespace",
			pub: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			otherPubs: func() []*policyv1alpha1.PodUnavailableBudget {
				pub1 := pubDemo.DeepCopy()
				pub1.Name = "pub1"
				pub1.Namespace = "pub1"
				pub1.Spec.TargetReference = nil
				pub1.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "pub-controller",
					},
				}
				pub2 := pubDemo.DeepCopy()
				pub2.Name = "pub2"
				pub2.Spec.TargetReference = nil
				pub2.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "pub2-controller",
					},
				}
				return []*policyv1alpha1.PodUnavailableBudget{pub1, pub2}
			},
			expectErrList: 0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			decoder := admission.NewDecoder(scheme)
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			for _, pub := range cs.otherPubs() {
				client.Create(context.TODO(), pub)
			}
			pubHandler := PodUnavailableBudgetCreateUpdateHandler{
				Client:  client,
				Decoder: decoder,
			}
			errList := pubHandler.validatingPodUnavailableBudgetFn(cs.pub(), nil)
			if len(errList) != cs.expectErrList {
				t.Fatalf("expect errList(%d) but get(%d) error: %s", cs.expectErrList, len(errList), errList.ToAggregate().Error())
			}
		})
	}
}

func TestValidatingUpdatePub(t *testing.T) {
	cases := []struct {
		name          string
		old           func() *policyv1alpha1.PodUnavailableBudget
		obj           func() *policyv1alpha1.PodUnavailableBudget
		expectErrList int
	}{
		{
			name: "valid pub, targetRef not changed",
			old: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			obj: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "invalid pub, targetRef changed",
			old: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			obj: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Spec.TargetReference = &policyv1alpha1.TargetReference{
					APIVersion: "apps",
					Kind:       "Deployment",
					Name:       "deployment-changed",
				}
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "invalid pub, Selector changed",
			old: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			obj: func() *policyv1alpha1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MinAvailable = nil
				pub.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "pub-changed",
					},
				}
				return pub
			},
			expectErrList: 1,
		},
	}

	decoder := admission.NewDecoder(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	pubHandler := PodUnavailableBudgetCreateUpdateHandler{
		Client:  client,
		Decoder: decoder,
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			errList := pubHandler.validatingPodUnavailableBudgetFn(cs.obj(), cs.old())
			if len(errList) != cs.expectErrList {
				t.Fatalf("expect errList(%d) but get(%d) error: %s", cs.expectErrList, len(errList), errList.ToAggregate().Error())
			}
		})
	}
}
