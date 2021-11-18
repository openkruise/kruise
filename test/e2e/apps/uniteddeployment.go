package apps

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"

)

const UnitedDeploymentFakeZoneKey = "e2e.kruise.io/uniteddeployment-fake-zone"

var _ = SIGDescribe("uniteddeployment", func() {
	f := framework.NewDefaultFramework("uniteddeployment")
	unitedDeploymentName := "test-united-deployment"
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var ns string
	var tester *framework.UnitedDeploymentTester
	ginkgo.BeforeEach(func() {
		ns = f.Namespace.Name
		c = f.ClientSet
		kc = f.KruiseClientSet
		tester = framework.NewUnitedDeploymentTester(c, kc)

		// label nodes
		nodeList, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(len(nodeList.Items) > 2).Should(gomega.Equal(true))

		workers := make([]*corev1.Node, 0)
		for i := range nodeList.Items {
			node := nodeList.Items[i]
			if _, exist := node.GetLabels()["node-role.kubernetes.io/master"]; exist {
				continue
			}
			workers = append(workers, &node)
		}
		gomega.Expect(len(workers) > 2).Should(gomega.Equal(true))
		// subset-1
		worker0 := workers[0]
		tester.SetNodeLabel(c, worker0, UnitedDeploymentFakeZoneKey, "zone-a")
		// subset-2
		worker1 := workers[1]
		tester.SetNodeLabel(c, worker1, UnitedDeploymentFakeZoneKey, "zone-b")
		worker2 := workers[2]
		tester.SetNodeLabel(c, worker2, UnitedDeploymentFakeZoneKey, "zone-b")
	})
	f.AfterEachActions = []func(){
		func() {
			nodeList, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			patchBody := fmt.Sprintf(`{"metadata":{"labels":{"%s":null}}}`, UnitedDeploymentFakeZoneKey)
			for i := range nodeList.Items {
				node := nodeList.Items[i]
				if _, exist := node.GetLabels()[UnitedDeploymentFakeZoneKey]; !exist {
					continue
				}
				_, err = c.CoreV1().Nodes().Patch(context.TODO(), node.Name, types.StrategicMergePatchType, []byte(patchBody), metav1.PatchOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}
		},
	}

	framework.KruiseDescribe("UnitedDeployment functionality", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
		})
		framework.ConformanceIt("cloneSet subset patch", func() {
			unitedDeploymentSpec := tester.NewBaseUnitedDeploymentSpec("CloneSet")
			replicas := int32(2)
			subsetReplicas := intstr.FromInt(1)
			topology:= appsv1alpha1.Topology{
				Subsets: []appsv1alpha1.Subset{
					{
						Name: "subset-1",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      UnitedDeploymentFakeZoneKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-a"},
								},
							},
						},
						Replicas: &subsetReplicas,
						Patch: runtime.RawExtension{
							Raw: []byte(`{"spec":{"template":{"metadata":{"annotations":{"annotation-b":"value-b"},"labels":{"label-a":"value-a-updated"}},
										"spec":{"containers":[{"name":"main","image":"nginx:1-alpine"}]}}}}`),
						},
					},
					{
						Name: "subset-2",
						NodeSelectorTerm: corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      UnitedDeploymentFakeZoneKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-b"},
								},
							},
						},
					},
				},
			}
			unitedDeploymentSpec.Replicas = &replicas
			unitedDeploymentSpec.Topology = topology
			unitedDeployment := &appsv1alpha1.UnitedDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      unitedDeploymentName,
					Namespace: ns,
				},
				Spec: *unitedDeploymentSpec,
			}
			unitedDeployment = tester.CreateUnitedDeployment(unitedDeployment)
			tester.WaitForUnitedDeploymentRunning(unitedDeployment)
			subset1Pods, err := tester.GetSelectorPods(unitedDeployment.Namespace, &metav1.LabelSelector{MatchLabels: map[string]string{
				"apps.kruise.io/subset-name": "subset-1",
			}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(subset1Pods).To(gomega.HaveLen(1))
			for _, pod := range subset1Pods {
				gomega.Expect(pod.Labels["label-a"]).To(gomega.Equal("value-a-updated"))
				gomega.Expect(pod.Annotations["annotation-a"]).To(gomega.Equal("value-a"))
				gomega.Expect(pod.Annotations["annotation-b"]).To(gomega.Equal("value-b"))
				gomega.Expect(pod.Spec.Containers).To(gomega.HaveLen(1))
				gomega.Expect(pod.Spec.Containers[0].Name).To(gomega.Equal("main"))
				gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal("1-alpine"))
			}

			subset2Pods, err := tester.GetSelectorPods(unitedDeployment.Namespace, &metav1.LabelSelector{MatchLabels: map[string]string{
				"apps.kruise.io/subset-name": "subset-2",
			}})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(subset1Pods).To(gomega.HaveLen(1))
			for _, pod := range subset2Pods {
				gomega.Expect(pod.Labels["label-a"]).To(gomega.Equal("value-a"))
				gomega.Expect(pod.Annotations["annotation-a"]).To(gomega.Equal("value-a"))
				gomega.Expect(pod.Annotations).ShouldNot(gomega.HaveKey("annotation-b"))
				gomega.Expect(pod.Spec.Containers).To(gomega.HaveLen(1))
				gomega.Expect(pod.Spec.Containers[0].Name).To(gomega.Equal("main"))
				gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal("1.0"))
			}
		})
	})
})
