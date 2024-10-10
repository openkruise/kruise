package framework

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
)

type UnitedDeploymentTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
	ns string
}

func NewUnitedDeploymentTester(c clientset.Interface, kc kruiseclientset.Interface, ns string) *UnitedDeploymentTester {
	return &UnitedDeploymentTester{
		c:  c,
		kc: kc,
		ns: ns,
	}
}

var zero = int64(0)

func (t *UnitedDeploymentTester) NewUnitedDeploymentManager(name string) *UnitedDeploymentManager {
	return &UnitedDeploymentManager{
		UnitedDeployment: &appsv1alpha1.UnitedDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: t.ns,
			},
			Spec: appsv1alpha1.UnitedDeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": name,
					},
				},
				Template: appsv1alpha1.SubsetTemplate{
					DeploymentTemplate: &appsv1alpha1.DeploymentTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app": name,
							},
						},
						Spec: appsv1.DeploymentSpec{
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": name,
								},
							},
							Template: v1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: map[string]string{
										"app": name,
									},
								},
								Spec: v1.PodSpec{
									TerminationGracePeriodSeconds: &zero,
									Containers: []v1.Container{
										{
											Name:  "busybox",
											Image: "busybox:1.32",
											Command: []string{
												"/bin/sh", "-c", "sleep 100d",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		kc: t.kc,
		c:  t.c,
	}
}

type UnitedDeploymentManager struct {
	*appsv1alpha1.UnitedDeployment
	kc kruiseclientset.Interface
	c  clientset.Interface
}

func (m *UnitedDeploymentManager) AddSubset(name string, replicas, minReplicas, maxReplicas *intstr.IntOrString) {
	m.Spec.Topology.Subsets = append(m.Spec.Topology.Subsets, appsv1alpha1.Subset{
		Name:        name,
		Replicas:    replicas,
		MinReplicas: minReplicas,
		MaxReplicas: maxReplicas,
	})
}

func (m *UnitedDeploymentManager) Scale(replicas int32) {
	m.Spec.Replicas = pointer.Int32(replicas)
	_, err := m.kc.AppsV1alpha1().UnitedDeployments(m.Namespace).Patch(context.TODO(), m.Name, types.MergePatchType,
		[]byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)), metav1.PatchOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	gomega.Eventually(func() bool {
		ud, err := m.kc.AppsV1alpha1().UnitedDeployments(m.Namespace).Get(context.TODO(), m.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		return ud.Status.Replicas == replicas && ud.Generation == ud.Status.ObservedGeneration
	}, time.Minute, time.Second).Should(gomega.BeTrue())
}

func (m *UnitedDeploymentManager) Create(replicas int32) {
	m.Spec.Replicas = pointer.Int32(replicas)
	_, err := m.kc.AppsV1alpha1().UnitedDeployments(m.Namespace).Create(context.TODO(), m.UnitedDeployment, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	time.Sleep(3 * time.Second)
	gomega.Eventually(func() bool {
		ud, err := m.kc.AppsV1alpha1().UnitedDeployments(m.Namespace).Get(context.TODO(), m.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ok := ud.Status.Replicas == replicas && ud.Generation == ud.Status.ObservedGeneration
		if !ok {
			fmt.Printf("UnitedDeploymentManager.Create failed\nud.Status.Replicas: %d, ud.Generation: %d, ud.Status.ObservedGeneration: %d\n",
				ud.Status.Replicas, ud.Generation, ud.Status.ObservedGeneration)
		}
		return ok
	}, time.Minute, time.Second).Should(gomega.BeTrue())
}

func (m *UnitedDeploymentManager) CheckSubsets(replicas map[string]int32) {
	gomega.Eventually(func() bool {
		ud, err := m.kc.AppsV1alpha1().UnitedDeployments(m.Namespace).Get(context.TODO(), m.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ok := ud.GetGeneration() == ud.Status.ObservedGeneration && *ud.Spec.Replicas == ud.Status.Replicas && reflect.DeepEqual(replicas, ud.Status.SubsetReplicas)
		if !ok {
			fmt.Printf("UnitedDeploymentManager.CheckSubsets failed\nud.GetGeneration(): %d, ud.Status.ObservedGeneration: %d, *ud.Spec.Replicas: %d, ud.Status.Replicas: %d, ud.Status.SubsetReplicas: %v\n", ud.GetGeneration(),
				ud.Status.ObservedGeneration, *ud.Spec.Replicas, ud.Status.Replicas, ud.Status.SubsetReplicas)
		}
		return ok
	}, 3*time.Minute, time.Second).Should(gomega.BeTrue())
}

func (m *UnitedDeploymentManager) Update() {
	gomega.Eventually(func(g gomega.Gomega) {
		ud, err := m.kc.AppsV1alpha1().UnitedDeployments(m.Namespace).Get(context.Background(), m.Name, metav1.GetOptions{})
		g.Expect(err).NotTo(gomega.HaveOccurred())
		ud.Spec = m.UnitedDeployment.DeepCopy().Spec
		_, err = m.kc.AppsV1alpha1().UnitedDeployments(m.Namespace).Update(context.Background(), ud, metav1.UpdateOptions{})
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}, time.Minute, time.Second).Should(gomega.Succeed())
}

func (m *UnitedDeploymentManager) CheckSubsetPods(expect map[string]int32) {
	fmt.Print("CheckSubsetPods ")
	ud, err := m.kc.AppsV1alpha1().UnitedDeployments(m.Namespace).Get(context.TODO(), m.Name, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(func(g gomega.Gomega) {
		actual := map[string]int32{}
		for _, subset := range ud.Spec.Topology.Subsets {
			podList, err := m.c.CoreV1().Pods(m.Namespace).List(context.Background(), metav1.ListOptions{
				LabelSelector: fmt.Sprintf("apps.kruise.io/subset-name=%s", subset.Name),
			})
			g.Expect(err).NotTo(gomega.HaveOccurred())
			actual[subset.Name] = int32(len(podList.Items))
		}
		g.Expect(expect).To(gomega.BeEquivalentTo(actual))
	}, time.Minute, 500*time.Millisecond).Should(gomega.Succeed())
	fmt.Println("pass")
}

func (m *UnitedDeploymentManager) CheckUnschedulableStatus(expect map[string]bool) {
	fmt.Print("CheckUnschedulableStatus ")
	gomega.Eventually(func(g gomega.Gomega) {
		ud, err := m.kc.AppsV1alpha1().UnitedDeployments(m.Namespace).Get(context.TODO(), m.Name, metav1.GetOptions{})
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(ud.Status.SubsetStatuses != nil).To(gomega.BeTrue())
		actual := map[string]bool{}
		for name := range expect {
			status := ud.Status.GetSubsetStatus(name)
			g.Expect(status != nil).To(gomega.BeTrue())
			condition := status.GetCondition(appsv1alpha1.UnitedDeploymentSubsetSchedulable)
			actual[name] = condition != nil && condition.Status == v1.ConditionFalse
		}
		g.Expect(expect).To(gomega.BeEquivalentTo(actual))
	}, time.Minute, 500*time.Millisecond).Should(gomega.Succeed())
	fmt.Println("pass")
}
