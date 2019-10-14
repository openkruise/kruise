package utils

import (
	"testing"

	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	val int32 = 2
)

func Test(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
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

	getOwner := func() (runtime.Object, error) {
		return sts, nil
	}

	updateOwnee := func(obj runtime.Object) (err error) {
		return nil
	}
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, &appsv1.StatefulSet{})
	m, err := NewRefManager(getOwner, updateOwnee, sts.Spec.Selector, sts, scheme)
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
}
