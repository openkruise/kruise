package uniteddeployment

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

var (
	one int32 = 1
)

func TestRevisionManage(t *testing.T) {
	g, requests, stopMgr, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		close(stopMgr)
		mgrStopped.Wait()
	}()

	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "foo",
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				StatefulSetTemplate: &appsv1alpha1.StatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": "foo",
						},
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": "foo",
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "containerA",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subsetA",
						NodeSelector: corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "node-name",
											Operator: corev1.NodeSelectorOpIn,
											Values:   []string{"nodeA"},
										},
									},
								},
							},
						},
					},
				},
			},
			RevisionHistoryLimit: &one,
		},
	}

	// Create the UnitedDeployment object and expect the Reconcile and Deployment to be created
	err := c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	revisionList := &appsv1.ControllerRevisionList{}
	g.Expect(c.List(context.TODO(), &client.ListOptions{}, revisionList)).Should(gomega.BeNil())
	g.Expect(len(revisionList.Items)).Should(gomega.BeEquivalentTo(1))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Template.StatefulSetTemplate.Labels["version"] = "v2"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	revisionList = &appsv1.ControllerRevisionList{}
	g.Expect(c.List(context.TODO(), &client.ListOptions{}, revisionList)).Should(gomega.BeNil())
	g.Expect(len(revisionList.Items)).Should(gomega.BeEquivalentTo(2))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Template.StatefulSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:1.1"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	revisionList = &appsv1.ControllerRevisionList{}
	g.Expect(c.List(context.TODO(), &client.ListOptions{}, revisionList)).Should(gomega.BeNil())
	g.Expect(len(revisionList.Items)).Should(gomega.BeEquivalentTo(2))
}

func setUp(t *testing.T) (*gomega.GomegaWithT, chan reconcile.Request, chan struct{}, *sync.WaitGroup) {
	g := gomega.NewGomegaWithT(t)
	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()
	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())
	stopMgr, mgrStopped := StartTestManager(mgr, g)

	return g, requests, stopMgr, mgrStopped
}

func clean(g *gomega.GomegaWithT, c client.Client) {
	udList := &appsv1alpha1.UnitedDeploymentList{}
	if err := c.List(context.TODO(), &client.ListOptions{}, udList); err == nil {
		for _, ud := range udList.Items {
			c.Delete(context.TODO(), &ud)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), &client.ListOptions{}, udList); err != nil {
			return err
		}

		if len(udList.Items) != 0 {
			return fmt.Errorf("expected %d sts, got %d", 0, len(udList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	rList := &appsv1.ControllerRevisionList{}
	if err := c.List(context.TODO(), &client.ListOptions{}, rList); err == nil {
		for _, ud := range rList.Items {
			c.Delete(context.TODO(), &ud)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), &client.ListOptions{}, rList); err != nil {
			return err
		}

		if len(rList.Items) != 0 {
			return fmt.Errorf("expected %d sts, got %d", 0, len(rList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	stsList := &appsv1.StatefulSetList{}
	if err := c.List(context.TODO(), &client.ListOptions{}, stsList); err == nil {
		for _, sts := range stsList.Items {
			c.Delete(context.TODO(), &sts)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), &client.ListOptions{}, stsList); err != nil {
			return err
		}

		if len(stsList.Items) != 0 {
			return fmt.Errorf("expected %d sts, got %d", 0, len(stsList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())

	podList := &corev1.PodList{}
	if err := c.List(context.TODO(), &client.ListOptions{}, podList); err == nil {
		for _, pod := range podList.Items {
			c.Delete(context.TODO(), &pod)
		}
	}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), &client.ListOptions{}, podList); err != nil {
			return err
		}

		if len(podList.Items) != 0 {
			return fmt.Errorf("expected %d sts, got %d", 0, len(podList.Items))
		}

		return nil
	}, timeout, time.Second).Should(gomega.Succeed())
}
