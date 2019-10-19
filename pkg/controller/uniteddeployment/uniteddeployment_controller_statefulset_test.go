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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/uniteddeployment/subset"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var deploy = client.ObjectKey{Namespace: "default", Name: "foo"}

const timeout = time.Second * 2

var (
	one  int32 = 1
	two  int32 = 2
	five int32 = 5
	ten  int32 = 10
)

func TestStsReconcile(t *testing.T) {
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
			RevisionHistoryLimit: &ten,
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
	waitReconcilerProcessFinished(g, requests, 3)
	expectedStsCount(g, 1)
}

func TestStsSubsetProvision(t *testing.T) {
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
			RevisionHistoryLimit: &ten,
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
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, 1)
	sts := &stsList.Items[0]
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("nodeA"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Topology.Subsets = append(instance.Spec.Topology.Subsets, appsv1alpha1.Subset{
		Name: "subsetB",
		NodeSelector: corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      "node-name",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"nodeB"},
						},
					},
				},
			},
		},
	})
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, 2)
	sts = getSubsetByName(stsList, "subsetA")
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("nodeA"))

	sts = getSubsetByName(stsList, "subsetB")
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("nodeB"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Topology.Subsets = instance.Spec.Topology.Subsets[1:]
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	stsList = expectedStsCount(g, 1)
	sts = &stsList.Items[0]
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(sts.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("nodeB"))
}

func TestStsDupSubset(t *testing.T) {
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
			RevisionHistoryLimit: &ten,
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
	waitReconcilerProcessFinished(g, requests, 3)

	stsList := expectedStsCount(g, 1)

	subsetA := stsList.Items[0]
	dupSts := subsetA.DeepCopy()
	dupSts.Name = "dup-subset-a"
	dupSts.ResourceVersion = ""
	g.Expect(c.Create(context.TODO(), dupSts)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)
	expectedStsCount(g, 1)
}

func waitReconcilerProcessFinished(g *gomega.GomegaWithT, requests chan reconcile.Request, minCount int) {
	timeout := time.After(timeout)
	for {
		minCount--
		select {
		case <-requests:
			continue
		case <-timeout:
			if minCount <= 0 {
				return
			}
		}
	}
}

func getSubsetByName(stsList *appsv1.StatefulSetList, name string) *appsv1.StatefulSet {
	for _, sts := range stsList.Items {
		if sts.Labels[appsv1alpha1.SubSetNameLabelKey] == name {
			return &sts
		}
	}

	return nil
}

func expectedStsCount(g *gomega.GomegaWithT, count int) *appsv1.StatefulSetList {
	stsList := &appsv1.StatefulSetList{}
	g.Eventually(func() error {
		if err := c.List(context.TODO(), &client.ListOptions{}, stsList); err != nil {
			return err
		}

		if len(stsList.Items) != count {
			return fmt.Errorf("expected %d sts, got %d", count, len(stsList.Items))
		}

		return nil
	}, timeout).Should(gomega.Succeed())

	return stsList
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

	subsetReplicasFn = func(subset *subset.Subset) int32 {
		return subset.Spec.Replicas
	}

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
