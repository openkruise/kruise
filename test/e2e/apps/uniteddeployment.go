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
		framework.ConformanceIt("CloneSet subset patch", func() {
			unitedDeploymentSpec := tester.NewBaseUnitedDeploymentSpec("CloneSet")
			replicas := int32(2)
			subsetReplicas := intstr.FromInt(1)
			topology := appsv1alpha1.Topology{
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
							Raw: []byte(`{"metadata":{"annotations":{"annotation-b":"value-b"},"labels":{"label-a":"value-a-updated"}},
								"spec":{"template":{"spec":{"containers":[{"name":"main","image":"nginx:1-alpine"}]}}}}`),
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

			cloneSets, err := kc.AppsV1alpha1().CloneSets(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-1"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cloneSets.Items).To(gomega.HaveLen(1))
			cloneSet := cloneSets.Items[0]
			gomega.Expect(cloneSet.Labels["label-a"]).To(gomega.Equal("value-a-updated"))
			gomega.Expect(cloneSet.Annotations["annotation-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(cloneSet.Annotations["annotation-b"]).To(gomega.Equal("value-b"))
			subset1Pods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-1"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(subset1Pods.Items).To(gomega.HaveLen(1))
			pod := subset1Pods.Items[0]
			gomega.Expect(pod.Spec.Containers[0].Name).To(gomega.Equal("main"))
			gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal("nginx:1-alpine"))

			cloneSets, err = kc.AppsV1alpha1().CloneSets(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-2"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cloneSets.Items).To(gomega.HaveLen(1))
			cloneSet = cloneSets.Items[0]
			gomega.Expect(cloneSet.Labels["label-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(cloneSet.Annotations["annotation-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(cloneSet.Annotations).ShouldNot(gomega.HaveKey("annotation-b"))
			subset2Pods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-2"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(subset2Pods.Items).To(gomega.HaveLen(1))
			pod = subset2Pods.Items[0]
			gomega.Expect(pod.Spec.Containers[0].Name).To(gomega.Equal("main"))
			gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal("nginx:1"))
		})

		framework.ConformanceIt("Deployment subset patch", func() {
			unitedDeploymentSpec := tester.NewBaseUnitedDeploymentSpec("Deployment")
			replicas := int32(2)
			subsetReplicas := intstr.FromInt(1)
			topology := appsv1alpha1.Topology{
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
							Raw: []byte(`{"metadata":{"annotations":{"annotation-b":"value-b"},"labels":{"label-a":"value-a-updated"}},
								"spec":{"template":{"spec":{"containers":[{"name":"main","image":"nginx:1-alpine"}]}}}}`),
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

			deployments, err := c.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-1"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(deployments.Items).To(gomega.HaveLen(1))
			deployment := deployments.Items[0]
			gomega.Expect(deployment.Labels["label-a"]).To(gomega.Equal("value-a-updated"))
			gomega.Expect(deployment.Annotations["annotation-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(deployment.Annotations["annotation-b"]).To(gomega.Equal("value-b"))
			subset1Pods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-1"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(subset1Pods.Items).To(gomega.HaveLen(1))
			pod := subset1Pods.Items[0]
			gomega.Expect(pod.Spec.Containers[0].Name).To(gomega.Equal("main"))
			gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal("nginx:1-alpine"))

			deployments, err = c.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-2"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(deployments.Items).To(gomega.HaveLen(1))
			deployment = deployments.Items[0]
			gomega.Expect(deployment.Labels["label-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(deployment.Annotations["annotation-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(deployment.Annotations).ShouldNot(gomega.HaveKey("annotation-b"))
			subset2Pods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-2"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(subset2Pods.Items).To(gomega.HaveLen(1))
			pod = subset2Pods.Items[0]
			gomega.Expect(pod.Spec.Containers[0].Name).To(gomega.Equal("main"))
			gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal("nginx:1"))
		})

		framework.ConformanceIt("StatefulSet subset patch", func() {
			unitedDeploymentSpec := tester.NewBaseUnitedDeploymentSpec("StatefulSet")
			replicas := int32(2)
			subsetReplicas := intstr.FromInt(1)
			topology := appsv1alpha1.Topology{
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
							Raw: []byte(`{"metadata":{"annotations":{"annotation-b":"value-b"},"labels":{"label-a":"value-a-updated"}},
								"spec":{"template":{"spec":{"containers":[{"name":"main","image":"nginx:1-alpine"}]}}}}`),
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

			statefulSets, err := c.AppsV1().StatefulSets(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-1"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(statefulSets.Items).To(gomega.HaveLen(1))
			statefulSet := statefulSets.Items[0]
			gomega.Expect(statefulSet.Labels["label-a"]).To(gomega.Equal("value-a-updated"))
			gomega.Expect(statefulSet.Annotations["annotation-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(statefulSet.Annotations["annotation-b"]).To(gomega.Equal("value-b"))
			subset1Pods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-1"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(subset1Pods.Items).To(gomega.HaveLen(1))
			pod := subset1Pods.Items[0]
			gomega.Expect(pod.Spec.Containers[0].Name).To(gomega.Equal("main"))
			gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal("nginx:1-alpine"))

			statefulSets, err = c.AppsV1().StatefulSets(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-2"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(statefulSets.Items).To(gomega.HaveLen(1))
			statefulSet = statefulSets.Items[0]
			gomega.Expect(statefulSet.Labels["label-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(statefulSet.Annotations["annotation-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(statefulSet.Annotations).ShouldNot(gomega.HaveKey("annotation-b"))
			subset2Pods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-2"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(subset2Pods.Items).To(gomega.HaveLen(1))
			pod = subset2Pods.Items[0]
			gomega.Expect(pod.Spec.Containers[0].Name).To(gomega.Equal("main"))
			gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal("nginx:1"))
		})

		framework.ConformanceIt("AdvancedStatefulSet subset patch", func() {
			unitedDeploymentSpec := tester.NewBaseUnitedDeploymentSpec("AdvancedStatefulSet")
			replicas := int32(2)
			subsetReplicas := intstr.FromInt(1)
			topology := appsv1alpha1.Topology{
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
							Raw: []byte(`{"metadata":{"annotations":{"annotation-b":"value-b"},"labels":{"label-a":"value-a-updated"}},
								"spec":{"template":{"spec":{"containers":[{"name":"main","image":"nginx:1-alpine"}]}}}}`),
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

			instances, err := kc.AppsV1alpha1().StatefulSets(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-1"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(instances.Items).To(gomega.HaveLen(1))
			instance := instances.Items[0]
			gomega.Expect(instance.Labels["label-a"]).To(gomega.Equal("value-a-updated"))
			gomega.Expect(instance.Annotations["annotation-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(instance.Annotations["annotation-b"]).To(gomega.Equal("value-b"))
			subset1Pods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-1"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(subset1Pods.Items).To(gomega.HaveLen(1))
			pod := subset1Pods.Items[0]
			gomega.Expect(pod.Spec.Containers[0].Name).To(gomega.Equal("main"))
			gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal("nginx:1-alpine"))

			instances, err = kc.AppsV1alpha1().StatefulSets(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-2"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(instances.Items).To(gomega.HaveLen(1))
			instance = instances.Items[0]
			gomega.Expect(instance.Labels["label-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(instance.Annotations["annotation-a"]).To(gomega.Equal("value-a"))
			gomega.Expect(instance.Annotations).ShouldNot(gomega.HaveKey("annotation-b"))
			subset2Pods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: "apps.kruise.io/subset-name=subset-2"})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(subset2Pods.Items).To(gomega.HaveLen(1))
			pod = subset2Pods.Items[0]
			gomega.Expect(pod.Spec.Containers[0].Name).To(gomega.Equal("main"))
			gomega.Expect(pod.Spec.Containers[0].Image).To(gomega.Equal("nginx:1"))
		})
	})
})
