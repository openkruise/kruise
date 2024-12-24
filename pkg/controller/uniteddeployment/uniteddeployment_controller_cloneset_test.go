package uniteddeployment

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCloneSetAll(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	cases := []TestCaseFunc{
		testCsReconcile,
		testTemplateTypeSwitchToCS,
	}

	for _, f := range cases {
		// clear requests
		for {
			var empty bool
			select {
			case <-requests:
			default:
				empty = true
			}
			if empty {
				break
			}
		}
		ns := fmt.Sprintf("ut-uniteddeployment-%s", rand.String(5))
		if err := c.Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}); err != nil {
			t.Fatalf("create namespace %s error: %v", ns, err)
		}
		f(t, g, ns, requests)
	}
}

func testCsReconcile(t *testing.T, g *gomega.GomegaWithT, namespace string, requests chan reconcile.Request) {
	caseName := "asts-reconcile"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: namespace,
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
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
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
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
	expectedCsCount(g, instance, 1)
}

func testTemplateTypeSwitchToCS(t *testing.T, g *gomega.GomegaWithT, namespace string, requests chan reconcile.Request) {
	caseName := "test-template-type-switch-to-cs"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: namespace,
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				AdvancedStatefulSetTemplate: &appsv1alpha1.AdvancedStatefulSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1beta1.StatefulSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
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
						Name: "subset-a",
						Tolerations: []corev1.Toleration{
							{
								Key:      "taint-a",
								Operator: corev1.TolerationOpExists,
								Effect:   corev1.TaintEffectNoSchedule,
							},
							{
								Key:      "taint-b",
								Operator: corev1.TolerationOpEqual,
								Value:    "taint-b-value",
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

	expectedAstsCount(g, instance, 1)
	expectedCsCount(g, instance, 0)

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())

	instance.Spec.Template.AdvancedStatefulSetTemplate = nil
	instance.Spec.Template.CloneSetTemplate = &appsv1alpha1.CloneSetTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"name": caseName,
			},
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": caseName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "container-a",
							Image: "nginx:1.0",
						},
					},
				},
			},
		},
	}

	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)

	expectedAstsCount(g, instance, 0)
	expectedCsCount(g, instance, 1)
}

func TestCsSubsetProvision(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-cs-subset-provision"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
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
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
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

	csList := expectedCsCount(g, instance, 1)
	cs := &csList.Items[0]
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-a"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Topology.Subsets = append(instance.Spec.Topology.Subsets, appsv1alpha1.Subset{
		Name: "subset-b",
		NodeSelectorTerm: corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "node-name",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"node-b"},
				},
			},
		},
	})
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	cs = getSubsetCsByName(csList, "subset-a")
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-a"))

	cs = getSubsetCsByName(csList, "subset-b")
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Topology.Subsets = instance.Spec.Topology.Subsets[1:]
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 1)
	cs = &csList.Items[0]
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(1))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Affinity = &corev1.Affinity{}
	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}

	nodeSelector := &corev1.NodeSelector{}
	nodeSelector.NodeSelectorTerms = append(nodeSelector.NodeSelectorTerms, corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      "test",
				Operator: corev1.NodeSelectorOpExists,
			},
			{
				Key:      "region",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{caseName},
			},
		},
	})
	nodeSelector.NodeSelectorTerms = append(nodeSelector.NodeSelectorTerms, corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      "test",
				Operator: corev1.NodeSelectorOpDoesNotExist,
			},
		},
	})
	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = nodeSelector

	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 1)
	cs = &csList.Items[0]
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).ShouldNot(gomega.BeNil())
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms)).Should(gomega.BeEquivalentTo(2))

	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions)).Should(gomega.BeEquivalentTo(3))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("test"))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpExists))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(0))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Key).Should(gomega.BeEquivalentTo("region"))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[1].Values[0]).Should(gomega.BeEquivalentTo(caseName))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[2].Values[0]).Should(gomega.BeEquivalentTo("node-b"))

	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions)).Should(gomega.BeEquivalentTo(2))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Key).Should(gomega.BeEquivalentTo("test"))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpDoesNotExist))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[0].Values)).Should(gomega.BeEquivalentTo(0))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Key).Should(gomega.BeEquivalentTo("node-name"))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Operator).Should(gomega.BeEquivalentTo(corev1.NodeSelectorOpIn))
	g.Expect(len(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Values)).Should(gomega.BeEquivalentTo(1))
	g.Expect(cs.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[1].MatchExpressions[1].Values[0]).Should(gomega.BeEquivalentTo("node-b"))
}

func TestCsSubsetPatch(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-cs-subset-patch"

	imagePatch := map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []map[string]interface{}{
				{
					"name":  "container-a",
					"image": "nginx:2.0",
				},
			},
		},
	}
	labelPatch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]string{
				"zone": "a",
			},
		},
	}
	resourcePatch := map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []map[string]interface{}{
				{
					"name": "container-a",
					"resources": map[string]interface{}{
						"limits": map[string]interface{}{
							"cpu":    "2",
							"memory": "800Mi",
						},
					},
				},
			},
		},
	}
	envPatch := map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []map[string]interface{}{
				{
					"name": "container-a",
					"env": []map[string]string{
						{
							"name":  "K8S_CONTAINER_NAME",
							"value": "main",
						},
					},
				},
			},
		},
	}
	labelPatchBytes, _ := json.Marshal(labelPatch)
	imagePatchBytes, _ := json.Marshal(imagePatch)
	resourcePatchBytes, _ := json.Marshal(resourcePatch)
	envPatchBytes, _ := json.Marshal(envPatch)
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
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
						Name: "subset-a",
						Patch: runtime.RawExtension{
							Raw: imagePatchBytes,
						},
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						Patch: runtime.RawExtension{
							Raw: labelPatchBytes,
						},
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-b"},
								},
							},
						},
					},
					{
						Name: "subset-c",
						Patch: runtime.RawExtension{
							Raw: resourcePatchBytes,
						},
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-c"},
								},
							},
						},
					},
					{
						Name: "subset-d",
						Patch: runtime.RawExtension{
							Raw: envPatchBytes,
						},
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-d"},
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

	csList := expectedCsCount(g, instance, 4)
	cs := getSubsetCsByName(csList, "subset-a")
	g.Expect(cs.Spec).ShouldNot(gomega.BeNil())
	g.Expect(cs.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	cs = getSubsetCsByName(csList, "subset-b")
	g.Expect(cs.Spec).ShouldNot(gomega.BeNil())
	g.Expect(cs.Spec.Template.Labels).Should(gomega.HaveKeyWithValue("zone", "a"))

	cs = getSubsetCsByName(csList, "subset-c")
	g.Expect(cs.Spec).ShouldNot(gomega.BeNil())
	g.Expect(cs.Spec.Template.Spec.Containers[0].Resources).ShouldNot(gomega.BeNil())
	g.Expect(cs.Spec.Template.Spec.Containers[0].Resources.Limits).ShouldNot(gomega.BeNil())
	g.Expect(cs.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu()).ShouldNot(gomega.BeNil())
	g.Expect(cs.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().Value()).Should(gomega.BeEquivalentTo(2))
	g.Expect(cs.Spec.Template.Spec.Containers[0].Resources.Limits.Memory()).ShouldNot(gomega.BeNil())
	g.Expect(cs.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()).Should(gomega.BeEquivalentTo("800Mi"))

	cs = getSubsetCsByName(csList, "subset-d")
	g.Expect(cs.Spec).ShouldNot(gomega.BeNil())
	g.Expect(cs.Spec.Template.Spec.Containers[0].Env).ShouldNot(gomega.BeNil())
	g.Expect(cs.Spec.Template.Spec.Containers[0].Env[0].Name).Should(gomega.BeEquivalentTo("K8S_CONTAINER_NAME"))
	g.Expect(cs.Spec.Template.Spec.Containers[0].Env[0].Value).Should(gomega.BeEquivalentTo("main"))

}

func TestCsSubsetProvisionWithToleration(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-cs-subset-provision-with-toleration"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
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
						Name: "subset-a",
						Tolerations: []corev1.Toleration{
							{
								Key:      "taint-a",
								Operator: corev1.TolerationOpExists,
								Effect:   corev1.TaintEffectNoSchedule,
							},
							{
								Key:      "taint-b",
								Operator: corev1.TolerationOpEqual,
								Value:    "taint-b-value",
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

	csList := expectedCsCount(g, instance, 1)
	cs := &csList.Items[0]
	g.Expect(cs.Spec.Template.Spec.Tolerations).ShouldNot(gomega.BeNil())
	g.Expect(len(cs.Spec.Template.Spec.Tolerations)).Should(gomega.BeEquivalentTo(2))
	g.Expect(reflect.DeepEqual(cs.Spec.Template.Spec.Tolerations[0], instance.Spec.Topology.Subsets[0].Tolerations[0])).Should(gomega.BeTrue())
	g.Expect(reflect.DeepEqual(cs.Spec.Template.Spec.Tolerations[1], instance.Spec.Topology.Subsets[0].Tolerations[1])).Should(gomega.BeTrue())

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Tolerations = append(instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Tolerations, corev1.Toleration{
		Key:      "taint-0",
		Operator: corev1.TolerationOpExists,
	})

	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 1)
	cs = &csList.Items[0]
	g.Expect(cs.Spec.Template.Spec.Tolerations).ShouldNot(gomega.BeNil())
	g.Expect(len(cs.Spec.Template.Spec.Tolerations)).Should(gomega.BeEquivalentTo(3))
	g.Expect(reflect.DeepEqual(cs.Spec.Template.Spec.Tolerations[1], instance.Spec.Topology.Subsets[0].Tolerations[0])).Should(gomega.BeTrue())
	g.Expect(reflect.DeepEqual(cs.Spec.Template.Spec.Tolerations[2], instance.Spec.Topology.Subsets[0].Tolerations[1])).Should(gomega.BeTrue())
}

func TestCsDupSubset(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-cs-dup-subset"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
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
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"node-a"},
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

	csList := expectedCsCount(g, instance, 1)

	subsetA := csList.Items[0]
	dupCs := subsetA.DeepCopy()
	dupCs.Name = "dup-subset-a"
	dupCs.ResourceVersion = ""
	g.Expect(c.Create(context.TODO(), dupCs)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)
	expectedCsCount(g, instance, 1)
}

func TestCsScale(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-asts-scale"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
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
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
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

	csList := expectedCsCount(g, instance, 2)
	g.Expect(*csList.Items[0].Spec.Replicas + *csList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	var two int32 = 2
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Replicas = &two
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	g.Expect(*csList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(1))
	g.Expect(*csList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.SubsetReplicas).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 1,
		"subset-b": 1,
	}))

	var five int32 = 6
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Replicas = &five
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	g.Expect(*csList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(3))
	g.Expect(*csList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(3))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.SubsetReplicas).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 3,
		"subset-b": 3,
	}))

	var four int32 = 4
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.Replicas = &four
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	g.Expect(*csList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	g.Expect(*csList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(2))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.SubsetReplicas).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 2,
		"subset-b": 2,
	}))
}

func TestCsUpdate(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-cs-update"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &two,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
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
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
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

	csList := expectedCsCount(g, instance, 2)
	g.Expect(*csList.Items[0].Spec.Replicas + *csList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	revisionList := &appsv1.ControllerRevisionList{}
	g.Expect(c.List(context.TODO(), revisionList))
	g.Expect(len(revisionList.Items)).Should(gomega.BeEquivalentTo(1))
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	v1 := revisionList.Items[0].Name
	g.Expect(instance.Status.CurrentRevision).Should(gomega.BeEquivalentTo(v1))

	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	g.Expect(csList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(csList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	revisionList = &appsv1.ControllerRevisionList{}
	g.Expect(c.List(context.TODO(), revisionList))
	g.Expect(len(revisionList.Items)).Should(gomega.BeEquivalentTo(2))
	v2 := revisionList.Items[0].Name
	if v2 == v1 {
		v2 = revisionList.Items[1].Name
	}
	g.Expect(instance.Status.UpdateStatus.UpdatedRevision).Should(gomega.BeEquivalentTo(v2))
}

func TestCsRollingUpdatePartition(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-cs-rolling-update-partition"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.UnitedDeploymentUpdateStrategy{
				Type: appsv1alpha1.ManualUpdateStrategyType,
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
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

	csList := expectedCsCount(g, instance, 2)
	g.Expect(*csList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*csList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	// update with partition
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 4,
			"subset-b": 3,
		},
	}
	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	g.Expect(csList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(csList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	csA := getSubsetCsByName(csList, "subset-a")
	g.Expect(csA).ShouldNot(gomega.BeNil())
	g.Expect(getPartitionCount(csA.Spec.UpdateStrategy.Partition, csA.Spec.Replicas)).Should(gomega.BeEquivalentTo(4))

	csB := getSubsetCsByName(csList, "subset-b")
	g.Expect(csB).ShouldNot(gomega.BeNil())
	g.Expect(getPartitionCount(csB.Spec.UpdateStrategy.Partition, csB.Spec.Replicas)).Should(gomega.BeEquivalentTo(3))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 4,
		"subset-b": 3,
	}))

	// move on
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 0,
			"subset-b": 3,
		},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 4)

	csList = expectedCsCount(g, instance, 2)
	g.Expect(csList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(csList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	csA = getSubsetCsByName(csList, "subset-a")
	g.Expect(csA).ShouldNot(gomega.BeNil())
	g.Expect(getPartitionCount(csA.Spec.UpdateStrategy.Partition, csA.Spec.Replicas)).Should(gomega.BeEquivalentTo(0))

	csB = getSubsetCsByName(csList, "subset-b")
	g.Expect(csB).ShouldNot(gomega.BeNil())
	g.Expect(getPartitionCount(csB.Spec.UpdateStrategy.Partition, csB.Spec.Replicas)).Should(gomega.BeEquivalentTo(3))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 0,
		"subset-b": 3,
	}))

	// move on
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	g.Expect(csList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(csList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	csA = getSubsetCsByName(csList, "subset-a")
	g.Expect(csA).ShouldNot(gomega.BeNil())
	g.Expect(getPartitionCount(csA.Spec.UpdateStrategy.Partition, csA.Spec.Replicas)).Should(gomega.BeEquivalentTo(0))

	csB = getSubsetCsByName(csList, "subset-b")
	g.Expect(csB).ShouldNot(gomega.BeNil())
	g.Expect(getPartitionCount(csB.Spec.UpdateStrategy.Partition, csB.Spec.Replicas)).Should(gomega.BeEquivalentTo(0))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 0,
		"subset-b": 0,
	}))
}

func TestCsOnDelete(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-cs-on-delete"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
										Image: "nginx:1.0",
									},
								},
							},
						},
						UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
							Type: appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType,
						},
					},
				},
			},
			UpdateStrategy: appsv1alpha1.UnitedDeploymentUpdateStrategy{
				Type: appsv1alpha1.ManualUpdateStrategyType,
			},
			Topology: appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
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

	csList := expectedCsCount(g, instance, 2)
	g.Expect(*csList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*csList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	g.Expect(csList.Items[0].Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))
	g.Expect(csList.Items[1].Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))

	// update with partition
	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 4,
			"subset-b": 3,
		},
	}
	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	g.Expect(csList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(csList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	csA := getSubsetCsByName(csList, "subset-a")
	g.Expect(csA).ShouldNot(gomega.BeNil())
	g.Expect(csA.Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))

	csB := getSubsetCsByName(csList, "subset-b")
	g.Expect(csB).ShouldNot(gomega.BeNil())
	g.Expect(csB.Spec.UpdateStrategy.Type).Should(gomega.BeEquivalentTo(appsv1alpha1.InPlaceIfPossibleCloneSetUpdateStrategyType))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Status.UpdateStatus.CurrentPartitions).Should(gomega.BeEquivalentTo(map[string]int32{
		"subset-a": 4,
		"subset-b": 3,
	}))

	// move on
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 0,
			"subset-b": 3,
		},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	g.Expect(csList.Items[0].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	g.Expect(csList.Items[1].Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
}

func TestCsSubsetCount(t *testing.T) {
	g, requests, cancel, mgrStopped := setUp(t)
	defer func() {
		clean(g, c)
		cancel()
		mgrStopped.Wait()
	}()

	caseName := "test-cs-subset-count"
	instance := &appsv1alpha1.UnitedDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      caseName,
			Namespace: "default",
		},
		Spec: appsv1alpha1.UnitedDeploymentSpec{
			Replicas: &ten,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": caseName,
				},
			},
			Template: appsv1alpha1.SubsetTemplate{
				CloneSetTemplate: &appsv1alpha1.CloneSetTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": caseName,
						},
					},
					Spec: appsv1alpha1.CloneSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"name": caseName,
							},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"name": caseName,
								},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "container-a",
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
						Name: "subset-a",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-name",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"nodeA"},
								},
							},
						},
					},
					{
						Name: "subset-b",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
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

	csList := expectedCsCount(g, instance, 2)
	g.Expect(*csList.Items[0].Spec.Replicas).Should(gomega.BeEquivalentTo(5))
	g.Expect(*csList.Items[1].Spec.Replicas).Should(gomega.BeEquivalentTo(5))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	nine := intstr.FromInt(9)
	instance.Spec.Topology.Subsets[0].Replicas = &nine
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	setsubA := getSubsetCsByName(csList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(9))
	setsubB := getSubsetCsByName(csList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(1))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage := intstr.FromString("40%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:2.0"
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	setsubA = getSubsetCsByName(csList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))
	setsubB = getSubsetCsByName(csList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(6))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:2.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("30%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:3.0"
	instance.Spec.UpdateStrategy.Type = appsv1alpha1.ManualUpdateStrategyType
	instance.Spec.UpdateStrategy.ManualUpdate = &appsv1alpha1.ManualUpdate{
		Partitions: map[string]int32{
			"subset-a": 1,
		},
	}
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 2)
	setsubA = getSubsetCsByName(csList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(3))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:3.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.Partition).ShouldNot(gomega.BeNil())
	g.Expect(getPartitionCount(setsubA.Spec.UpdateStrategy.Partition, setsubA.Spec.Replicas)).Should(gomega.BeEquivalentTo(1))
	setsubB = getSubsetCsByName(csList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(7))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:3.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("20%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:4.0"
	instance.Spec.UpdateStrategy.ManualUpdate.Partitions = map[string]int32{
		"subset-a": 2,
	}
	instance.Spec.Topology.Subsets = append(instance.Spec.Topology.Subsets, appsv1alpha1.Subset{
		Name: "subset-c",
		NodeSelectorTerm: corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "node-name",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"nodeC"},
				},
			},
		},
	})
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 2)

	csList = expectedCsCount(g, instance, 3)
	setsubA = getSubsetCsByName(csList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(2))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.Partition).ShouldNot(gomega.BeNil())
	g.Expect(getPartitionCount(setsubA.Spec.UpdateStrategy.Partition, setsubA.Spec.Replicas)).Should(gomega.BeEquivalentTo(2))
	setsubB = getSubsetCsByName(csList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))
	setsubB = getSubsetCsByName(csList, "subset-c")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:4.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	percentage = intstr.FromString("10%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	instance.Spec.Template.CloneSetTemplate.Spec.Template.Spec.Containers[0].Image = "nginx:5.0"
	instance.Spec.UpdateStrategy.ManualUpdate.Partitions = map[string]int32{
		"subset-a": 2,
	}
	instance.Spec.Topology.Subsets = instance.Spec.Topology.Subsets[:2]
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)

	csList = expectedCsCount(g, instance, 2)
	setsubA = getSubsetCsByName(csList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(1))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.Partition).ShouldNot(gomega.BeNil())
	g.Expect(getPartitionCount(setsubA.Spec.UpdateStrategy.Partition, setsubA.Spec.Replicas)).Should(gomega.BeEquivalentTo(1))
	setsubB = getSubsetCsByName(csList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(9))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))

	g.Expect(c.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Name}, instance)).Should(gomega.BeNil())
	g.Expect(instance.Spec.UpdateStrategy.ManualUpdate.Partitions["subset-a"]).Should(gomega.BeEquivalentTo(2))
	percentage = intstr.FromString("40%")
	instance.Spec.Topology.Subsets[0].Replicas = &percentage
	g.Expect(c.Update(context.TODO(), instance)).Should(gomega.BeNil())
	waitReconcilerProcessFinished(g, requests, 3)

	csList = expectedCsCount(g, instance, 2)
	setsubA = getSubsetCsByName(csList, "subset-a")
	g.Expect(*setsubA.Spec.Replicas).Should(gomega.BeEquivalentTo(4))
	g.Expect(setsubA.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
	g.Expect(setsubA.Spec.UpdateStrategy.Partition).ShouldNot(gomega.BeNil())
	g.Expect(getPartitionCount(setsubA.Spec.UpdateStrategy.Partition, setsubA.Spec.Replicas)).Should(gomega.BeEquivalentTo(2))
	setsubB = getSubsetCsByName(csList, "subset-b")
	g.Expect(*setsubB.Spec.Replicas).Should(gomega.BeEquivalentTo(6))
	g.Expect(setsubB.Spec.Template.Spec.Containers[0].Image).Should(gomega.BeEquivalentTo("nginx:5.0"))
}

func expectedCsCount(g *gomega.GomegaWithT, ud *appsv1alpha1.UnitedDeployment, count int) *appsv1alpha1.CloneSetList {
	csList := &appsv1alpha1.CloneSetList{}

	selector, err := metav1.LabelSelectorAsSelector(ud.Spec.Selector)
	g.Expect(err).Should(gomega.BeNil())

	g.Eventually(func() error {
		if err := c.List(context.TODO(), csList, &client.ListOptions{LabelSelector: selector}); err != nil {
			return err
		}

		if len(csList.Items) != count {
			return fmt.Errorf("expected %d asts, got %d", count, len(csList.Items))
		}

		return nil
	}, timeout).Should(gomega.Succeed())

	return csList
}

func getSubsetCsByName(csList *appsv1alpha1.CloneSetList, name string) *appsv1alpha1.CloneSet {
	for _, cs := range csList.Items {
		if cs.Labels[appsv1alpha1.SubSetNameLabelKey] == name {
			return &cs
		}
	}
	return nil
}

func getPartitionCount(partition *intstr.IntOrString, replicas *int32) int {
	partitionCount, _ := intstr.GetValueFromIntOrPercent(partition, int(*replicas), true)
	return partitionCount
}
