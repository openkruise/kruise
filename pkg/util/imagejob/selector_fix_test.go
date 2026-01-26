package imagejob_test

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/openkruise/kruise/apis"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
	"github.com/openkruise/kruise/pkg/util/imagejob"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSelectorLogic_Fix(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apis.AddToScheme(scheme)

	// Case 1: Selector is nil (Standard match all)
	jobNil := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job-nil", Namespace: "default"},
		Spec:       appsv1beta1.ImagePullJobSpec{},
	}
	// Case 2: Selector is {} (The Bug Case)
	jobEmpty := &appsv1beta1.ImagePullJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job-empty", Namespace: "default"},
		Spec: appsv1beta1.ImagePullJobSpec{
			ImagePullJobTemplate: appsv1beta1.ImagePullJobTemplate{
				Selector: &appsv1beta1.ImagePullJobNodeSelector{}, // Empty non-nil selector
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(jobNil, jobEmpty).
		WithIndex(&appsv1beta1.ImagePullJob{}, fieldindex.IndexNameForIsActive, func(obj client.Object) []string {
			return []string{"true"}
		}).
		Build()

	nodeImage := &appsv1beta1.NodeImage{
		ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{"foo": "bar"}},
	}

	jobs, _, err := imagejob.GetActiveJobsForNodeImage(client, nodeImage, nil)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	names := sets.NewString()
	for _, j := range jobs {
		names.Insert(j.Name)
	}

	// Both should be found
	g.Expect(names.List()).Should(gomega.ConsistOf("job-nil", "job-empty"))
}
