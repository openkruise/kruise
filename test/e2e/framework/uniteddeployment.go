package framework

import (
	"context"
	"fmt"
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
	"reflect"
	"time"
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
	}
}

type UnitedDeploymentManager struct {
	*appsv1alpha1.UnitedDeployment
	kc kruiseclientset.Interface
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
		return ud.Status.Replicas == replicas && ud.Generation == ud.Status.ObservedGeneration
	}, time.Minute, time.Second).Should(gomega.BeTrue())
}

func (m *UnitedDeploymentManager) CheckSubsets(replicas map[string]int32) {
	gomega.Eventually(func() bool {
		ud, err := m.kc.AppsV1alpha1().UnitedDeployments(m.Namespace).Get(context.TODO(), m.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		return ud.GetGeneration() == ud.Status.ObservedGeneration && *ud.Spec.Replicas == ud.Status.Replicas && reflect.DeepEqual(replicas, ud.Status.SubsetReplicas)
	}, time.Minute, time.Second).Should(gomega.BeTrue())
}
