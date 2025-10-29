package mutating

import (
	"context"
	"testing"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
	fuzzutils "github.com/openkruise/kruise/test/fuzz"
)

var (
	fakeScheme = runtime.NewScheme()

	defaultPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "fuzz-test"},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:  "init-0",
					Image: "busybox:1.0.0",
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.15.1",
				},
			},
		},
	}

	req = admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation:   admissionv1.Create,
			Object:      runtime.RawExtension{},
			OldObject:   runtime.RawExtension{},
			Resource:    metav1.GroupVersionResource{Group: corev1.SchemeGroupVersion.Group, Version: corev1.SchemeGroupVersion.Version, Resource: "pods"},
			SubResource: "",
		},
	}
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = appsv1beta1.AddToScheme(fakeScheme)
	_ = appsv1beta1.AddToScheme(clientgoscheme.Scheme)
}

func FuzzSidecarSetMutatingPod(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cf := fuzz.NewConsumer(data)

		sidecarSet := &appsv1beta1.SidecarSet{}
		if err := cf.GenerateStruct(sidecarSet); err != nil {
			return
		}

		if err := fuzzutils.GenerateSidecarSetSpec(cf, sidecarSet,
			fuzzutils.GenerateSidecarSetUpdateStrategy,
			fuzzutils.GenerateSidecarSetInjectionStrategy,
			fuzzutils.GenerateSidecarSetInitContainer,
			fuzzutils.GenerateSidecarSetContainer,
			fuzzutils.GenerateSidecarSetPatchPodMetadata); err != nil {
			return
		}
		matched, err := cf.GetBool()
		if err != nil {
			return
		}
		if matched {
			// Make sure can select to defaultPod
			sidecarSet.Spec.Selector.MatchLabels = defaultPod.GetLabels()
			sidecarSet.Spec.Selector.MatchExpressions = nil
			// Set NamespaceSelector to nil to match all namespaces
			sidecarSet.Spec.NamespaceSelector = nil
			sidecarSet.Spec.InjectionStrategy.Revision = nil
		}

		if sidecarSet.GetDeletionTimestamp() != nil && len(sidecarSet.GetFinalizers()) == 0 {
			sidecarSet.SetDeletionTimestamp(nil)
		}

		c := fake.NewClientBuilder().WithObjects(sidecarSet).WithIndex(
			&appsv1beta1.SidecarSet{}, fieldindex.IndexNameForSidecarSetNamespace, fieldindex.IndexSidecarSetV1Beta1,
		).WithScheme(fakeScheme).Build()

		handler := &PodCreateHandler{
			Decoder: admission.NewDecoder(fakeScheme),
			Client:  c,
		}
		_, _ = handler.sidecarsetMutatingPod(context.Background(), req, defaultPod)
	})
}
