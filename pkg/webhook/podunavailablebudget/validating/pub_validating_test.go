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
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(policyv1alpha1.AddToScheme(scheme))
	utilruntime.Must(policyv1beta1.AddToScheme(scheme))
}

var (
	scheme *runtime.Scheme

	pubDemo = policyv1beta1.PodUnavailableBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: policyv1beta1.GroupVersion.String(),
			Kind:       "PodUnavailableBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Name:        "pub-test",
			Annotations: map[string]string{},
		},
		Spec: policyv1beta1.PodUnavailableBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "pub-controller",
				},
			},
			TargetReference: &policyv1beta1.TargetReference{
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
		pub           func() *policyv1beta1.PodUnavailableBudget
		expectErrList int
	}{
		{
			name: "valid pub, TargetReference and MaxUnavailable",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "valid pub, Selector and MinAvailable",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MaxUnavailable = nil
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "invalid pub, Selector and TargetReference are nil",
			pub: func() *policyv1beta1.PodUnavailableBudget {
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
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.MinAvailable = nil
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "invalid pub, MaxUnavailable and MinAvailable are nil",
			pub: func() *policyv1beta1.PodUnavailableBudget {
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
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "deprecated operation annotation with invalid value is silently accepted in v1beta1",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Annotations[policyv1beta1.PubProtectOperationAnnotation] = "xxxxx"
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "deprecated operation annotation with valid value is silently accepted in v1beta1",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Annotations[policyv1beta1.PubProtectOperationAnnotation] = "EVICT,DELETE,UPDATE,RESIZE"
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "valid pub empty deprecated operation annotation",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Annotations[policyv1beta1.PubProtectOperationAnnotation] = ""
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "invalid pub feature-gate annotation",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Annotations[policyv1beta1.PubProtectTotalReplicasAnnotation] = "%%"
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "valid pub feature-gate annotation",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Annotations[policyv1beta1.PubProtectTotalReplicasAnnotation] = "1000"
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "valid pub ignoredPodSelector",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Spec.IgnoredPodSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"kruise.io/force-deletable": "true"},
				}
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "invalid pub empty ignoredPodSelector",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Spec.IgnoredPodSelector = &metav1.LabelSelector{}
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
			errList := pubHandler.validatingPodUnavailableBudgetFnV1beta1(cs.pub(), nil)
			if len(errList) != cs.expectErrList {
				t.Fatalf("expect errList(%d) but get(%d) error: %v", cs.expectErrList, len(errList), errList.ToAggregate())
			}
		})
	}
}

func TestPubConflictWithOthers(t *testing.T) {
	cases := []struct {
		name          string
		pub           func() *policyv1beta1.PodUnavailableBudget
		otherPubs     func() []*policyv1beta1.PodUnavailableBudget
		expectErrList int
	}{
		{
			name: "no conflict with other pubs, and TargetReference",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			otherPubs: func() []*policyv1beta1.PodUnavailableBudget {
				pub1 := pubDemo.DeepCopy()
				pub1.Name = "pub1"
				pub1.Spec.TargetReference = &policyv1beta1.TargetReference{APIVersion: "apps", Kind: "Deployment", Name: "deployment-test1"}
				pub2 := pubDemo.DeepCopy()
				pub2.Name = "pub2"
				pub2.Spec.TargetReference = &policyv1beta1.TargetReference{APIVersion: "apps", Kind: "Deployment", Name: "deployment-test2"}
				return []*policyv1beta1.PodUnavailableBudget{pub1, pub2}
			},
			expectErrList: 0,
		},
		{
			name: "invalid conflict with other pubs, and TargetReference",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			otherPubs: func() []*policyv1beta1.PodUnavailableBudget {
				pub1 := pubDemo.DeepCopy()
				pub1.Name = "pub1"
				pub1.Spec.TargetReference = &policyv1beta1.TargetReference{APIVersion: "apps", Kind: "Deployment", Name: "deployment-test"}
				pub2 := pubDemo.DeepCopy()
				pub2.Name = "pub2"
				pub2.Spec.TargetReference = &policyv1beta1.TargetReference{APIVersion: "apps", Kind: "Deployment", Name: "deployment-test2"}
				return []*policyv1beta1.PodUnavailableBudget{pub1, pub2}
			},
			expectErrList: 1,
		},
		{
			name: "no conflict with other pubs, and Selector",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			otherPubs: func() []*policyv1beta1.PodUnavailableBudget {
				pub1 := pubDemo.DeepCopy()
				pub1.Name = "pub1"
				pub1.Spec.TargetReference = nil
				pub1.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pub1-controller"}}
				pub2 := pubDemo.DeepCopy()
				pub2.Name = "pub2"
				pub2.Spec.TargetReference = nil
				pub2.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pub2-controller"}}
				return []*policyv1beta1.PodUnavailableBudget{pub1, pub2}
			},
			expectErrList: 0,
		},
		{
			name: "conflict with other pubs, and Selector",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			otherPubs: func() []*policyv1beta1.PodUnavailableBudget {
				pub1 := pubDemo.DeepCopy()
				pub1.Name = "pub1"
				pub1.Spec.TargetReference = nil
				pub1.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pub-controller"}}
				pub2 := pubDemo.DeepCopy()
				pub2.Name = "pub2"
				pub2.Spec.TargetReference = nil
				pub2.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pub2-controller"}}
				return []*policyv1beta1.PodUnavailableBudget{pub1, pub2}
			},
			expectErrList: 1,
		},
		{
			name: "no conflict with other pubs, and Selector, other namespace",
			pub: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			otherPubs: func() []*policyv1beta1.PodUnavailableBudget {
				pub1 := pubDemo.DeepCopy()
				pub1.Name = "pub1"
				pub1.Namespace = "pub1"
				pub1.Spec.TargetReference = nil
				pub1.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pub-controller"}}
				pub2 := pubDemo.DeepCopy()
				pub2.Name = "pub2"
				pub2.Spec.TargetReference = nil
				pub2.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pub2-controller"}}
				return []*policyv1beta1.PodUnavailableBudget{pub1, pub2}
			},
			expectErrList: 0,
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			decoder := admission.NewDecoder(scheme)
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			for _, pub := range cs.otherPubs() {
				if err := client.Create(context.TODO(), pub); err != nil {
					t.Fatalf("failed to create pub: %v", err)
				}
			}
			pubHandler := PodUnavailableBudgetCreateUpdateHandler{
				Client:  client,
				Decoder: decoder,
			}
			errList := pubHandler.validatingPodUnavailableBudgetFnV1beta1(cs.pub(), nil)
			if len(errList) != cs.expectErrList {
				t.Fatalf("expect errList(%d) but get(%d) error: %v", cs.expectErrList, len(errList), errList.ToAggregate())
			}
		})
	}
}

func TestValidatingUpdatePub(t *testing.T) {
	cases := []struct {
		name          string
		old           func() *policyv1beta1.PodUnavailableBudget
		obj           func() *policyv1beta1.PodUnavailableBudget
		expectErrList int
	}{
		{
			name: "valid pub, targetRef not changed",
			old: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			obj: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			expectErrList: 0,
		},
		{
			name: "invalid pub, targetRef changed",
			old: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			obj: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.Selector = nil
				pub.Spec.MinAvailable = nil
				pub.Spec.TargetReference = &policyv1beta1.TargetReference{APIVersion: "apps", Kind: "Deployment", Name: "deployment-changed"}
				return pub
			},
			expectErrList: 1,
		},
		{
			name: "invalid pub, Selector changed",
			old: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MinAvailable = nil
				return pub
			},
			obj: func() *policyv1beta1.PodUnavailableBudget {
				pub := pubDemo.DeepCopy()
				pub.Spec.TargetReference = nil
				pub.Spec.MinAvailable = nil
				pub.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "pub-changed"}}
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
			errList := pubHandler.validatingPodUnavailableBudgetFnV1beta1(cs.obj(), cs.old())
			if len(errList) != cs.expectErrList {
				t.Fatalf("expect errList(%d) but get(%d) error: %v", cs.expectErrList, len(errList), errList.ToAggregate())
			}
		})
	}
}

func marshalPub(t *testing.T, obj interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("failed to marshal object: %v", err)
	}
	return data
}

func baseBetaPub() *policyv1beta1.PodUnavailableBudget {
	pub := pubDemo.DeepCopy()
	pub.Spec.Selector = nil
	pub.Spec.MinAvailable = nil
	return pub
}

func baseAlphaPub() *policyv1alpha1.PodUnavailableBudget {
	return &policyv1alpha1.PodUnavailableBudget{
		TypeMeta: metav1.TypeMeta{
			APIVersion: policyv1alpha1.GroupVersion.String(),
			Kind:       "PodUnavailableBudget",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Name:        "alpha-pub-handle-test",
			Annotations: map[string]string{},
		},
		Spec: policyv1alpha1.PodUnavailableBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "handle-test"},
			},
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "20%",
			},
		},
	}
}

func TestHandle(t *testing.T) {
	cases := []struct {
		name        string
		request     func() admission.Request
		wantAllowed bool
	}{
		{
			name: "v1beta1 create valid pub",
			request: func() admission.Request {
				return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  metav1.GroupVersionResource{Group: policyv1beta1.GroupVersion.Group, Version: policyv1beta1.GroupVersion.Version, Resource: "podunavailablebudgets"},
					Object:    runtime.RawExtension{Raw: marshalPub(t, baseBetaPub())},
				}}
			},
			wantAllowed: true,
		},
		{
			name: "v1beta1 create invalid pub — selector and targetRef both set",
			request: func() admission.Request {
				pub := baseBetaPub()
				pub.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}}
				return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  metav1.GroupVersionResource{Group: policyv1beta1.GroupVersion.Group, Version: policyv1beta1.GroupVersion.Version, Resource: "podunavailablebudgets"},
					Object:    runtime.RawExtension{Raw: marshalPub(t, pub)},
				}}
			},
			wantAllowed: false,
		},

		{
			name: "v1beta1 update valid pub — targetRef unchanged",
			request: func() admission.Request {
				pub := baseBetaPub()
				return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Resource:  metav1.GroupVersionResource{Group: policyv1beta1.GroupVersion.Group, Version: policyv1beta1.GroupVersion.Version, Resource: "podunavailablebudgets"},
					Object:    runtime.RawExtension{Raw: marshalPub(t, pub)},
					OldObject: runtime.RawExtension{Raw: marshalPub(t, pub)},
				}}
			},
			wantAllowed: true,
		},
		{
			name: "v1beta1 update invalid pub — targetRef changed",
			request: func() admission.Request {
				old := baseBetaPub()
				updated := baseBetaPub()
				updated.Spec.TargetReference = &policyv1beta1.TargetReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "changed"}
				return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Resource:  metav1.GroupVersionResource{Group: policyv1beta1.GroupVersion.Group, Version: policyv1beta1.GroupVersion.Version, Resource: "podunavailablebudgets"},
					Object:    runtime.RawExtension{Raw: marshalPub(t, updated)},
					OldObject: runtime.RawExtension{Raw: marshalPub(t, old)},
				}}
			},
			wantAllowed: false,
		},
		{
			name: "v1alpha1 create valid pub — annotation promoted to spec via conversion",
			request: func() admission.Request {
				pub := baseAlphaPub()
				pub.Annotations[policyv1alpha1.PubProtectOperationAnnotation] = "DELETE,EVICT"
				pub.Annotations[policyv1alpha1.PubProtectTotalReplicasAnnotation] = "10"
				return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  metav1.GroupVersionResource{Group: policyv1alpha1.GroupVersion.Group, Version: policyv1alpha1.GroupVersion.Version, Resource: "podunavailablebudgets"},
					Object:    runtime.RawExtension{Raw: marshalPub(t, pub)},
				}}
			},
			wantAllowed: true,
		},
		{
			name: "v1alpha1 create invalid pub — invalid operation annotation rejected via beta validator",
			request: func() admission.Request {
				pub := baseAlphaPub()
				pub.Annotations[policyv1alpha1.PubProtectOperationAnnotation] = "BOGUS"
				return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  metav1.GroupVersionResource{Group: policyv1alpha1.GroupVersion.Group, Version: policyv1alpha1.GroupVersion.Version, Resource: "podunavailablebudgets"},
					Object:    runtime.RawExtension{Raw: marshalPub(t, pub)},
				}}
			},
			wantAllowed: false,
		},
		{
			name: "v1alpha1 create invalid pub — bad replicas annotation rejected via beta validator",
			request: func() admission.Request {
				pub := baseAlphaPub()
				pub.Annotations[policyv1alpha1.PubProtectTotalReplicasAnnotation] = "not-a-number"
				return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  metav1.GroupVersionResource{Group: policyv1alpha1.GroupVersion.Group, Version: policyv1alpha1.GroupVersion.Version, Resource: "podunavailablebudgets"},
					Object:    runtime.RawExtension{Raw: marshalPub(t, pub)},
				}}
			},
			wantAllowed: false,
		},
		{
			name: "v1alpha1 update valid pub — selector unchanged",
			request: func() admission.Request {
				pub := baseAlphaPub()
				return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Resource:  metav1.GroupVersionResource{Group: policyv1alpha1.GroupVersion.Group, Version: policyv1alpha1.GroupVersion.Version, Resource: "podunavailablebudgets"},
					Object:    runtime.RawExtension{Raw: marshalPub(t, pub)},
					OldObject: runtime.RawExtension{Raw: marshalPub(t, pub)},
				}}
			},
			wantAllowed: true,
		},
		{
			name: "v1alpha1 update invalid pub — selector changed",
			request: func() admission.Request {
				old := baseAlphaPub()
				updated := baseAlphaPub()
				updated.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "changed"}}
				return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Resource:  metav1.GroupVersionResource{Group: policyv1alpha1.GroupVersion.Group, Version: policyv1alpha1.GroupVersion.Version, Resource: "podunavailablebudgets"},
					Object:    runtime.RawExtension{Raw: marshalPub(t, updated)},
					OldObject: runtime.RawExtension{Raw: marshalPub(t, old)},
				}}
			},
			wantAllowed: false,
		},
		{
			name: "unsupported version returns error",
			request: func() admission.Request {
				return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Resource:  metav1.GroupVersionResource{Group: "policy.kruise.io", Version: "v99beta99", Resource: "podunavailablebudgets"},
					Object:    runtime.RawExtension{Raw: []byte(`{}`)},
				}}
			},
			wantAllowed: false,
		},
	}

	decoder := admission.NewDecoder(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := PodUnavailableBudgetCreateUpdateHandler{
		Client:  fakeClient,
		Decoder: decoder,
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			resp := handler.Handle(context.TODO(), cs.request())
			if resp.Allowed != cs.wantAllowed {
				msg := "<nil result>"
				if resp.Result != nil {
					msg = resp.Result.Message
				}
				t.Fatalf("wantAllowed=%v got=%v msg=%s", cs.wantAllowed, resp.Allowed, msg)
			}
		})
	}
}
