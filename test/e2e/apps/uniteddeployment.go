package apps

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

var _ = SIGDescribe("uniteddeployment", func() {
	f := framework.NewDefaultFramework("uniteddeployment")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.UnitedDeploymentTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewUnitedDeploymentTester(c, kc, ns)
	})

	ginkgo.It("united deployment with elastic allocator", func() {
		replicas := func(r int) *intstr.IntOrString { p := intstr.FromInt(r); return &p }
		udManager := tester.NewUnitedDeploymentManager("ud-elastic-test")
		udManager.AddSubset("subset-0", nil, replicas(1), replicas(2))
		udManager.AddSubset("subset-1", nil, replicas(1), replicas(2))
		udManager.AddSubset("subset-2", nil, replicas(1), nil)

		replicasMap := func(replicas []int32) map[string]int32 {
			replicaMap := make(map[string]int32)
			for i, r := range replicas {
				replicaMap[fmt.Sprintf("subset-%d", i)] = r
			}
			return replicaMap
		}
		ginkgo.By("test replicas equals to sum of min replicas")
		udManager.Create(3)
		udManager.CheckSubsets(replicasMap([]int32{1, 1, 1}))

		ginkgo.By("test replicas more than sum of min replicas")
		udManager.Scale(7)
		udManager.CheckSubsets(replicasMap([]int32{2, 2, 3}))

		ginkgo.By("test replicas less than sum of min replicas")
		udManager.Scale(1)
		udManager.CheckSubsets(replicasMap([]int32{1, 0, 0}))
	})

	ginkgo.It("united deployment with specific allocator", func() {
		replicas := func(p string) *intstr.IntOrString { x := intstr.FromString(p); return &x }
		udManager := tester.NewUnitedDeploymentManager("ud-specific-test")
		udManager.AddSubset("subset-0", replicas("25%"), nil, nil)
		udManager.AddSubset("subset-1", replicas("25%"), nil, nil)
		udManager.AddSubset("subset-2", nil, nil, nil)

		replicasMap := func(replicas []int32) map[string]int32 {
			replicaMap := make(map[string]int32)
			for i, r := range replicas {
				replicaMap[fmt.Sprintf("subset-%d", i)] = r
			}
			return replicaMap
		}
		ginkgo.By("create ud")
		udManager.Create(3)
		udManager.CheckSubsets(replicasMap([]int32{1, 1, 1}))

		ginkgo.By("scale up")
		udManager.Scale(4)
		udManager.CheckSubsets(replicasMap([]int32{1, 1, 2}))

		ginkgo.By("scale down")
		udManager.Scale(1)
		udManager.CheckSubsets(replicasMap([]int32{0, 0, 1}))
	})

	ginkgo.It("adaptive united deployment with elastic allocator", func() {
		replicas := func(r int) *intstr.IntOrString { p := intstr.FromInt32(int32(r)); return &p }
		replicasMap := func(replicas []int32) map[string]int32 {
			replicaMap := make(map[string]int32)
			for i, r := range replicas {
				replicaMap[fmt.Sprintf("subset-%d", i)] = r
			}
			return replicaMap
		}
		unschedulableMap := func(unschedulables []bool) map[string]bool {
			resultMap := make(map[string]bool)
			for i, r := range unschedulables {
				resultMap[fmt.Sprintf("subset-%d", i)] = r
			}
			return resultMap
		}

		udManager := tester.NewUnitedDeploymentManager("adaptive-ud-elastic-test")
		// enable adaptive scheduling
		udManager.UnitedDeployment.Spec.Topology.ScheduleStrategy = appsv1alpha1.UnitedDeploymentScheduleStrategy{
			Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
			Adaptive: &appsv1alpha1.AdaptiveUnitedDeploymentStrategy{
				RescheduleCriticalSeconds: ptr.To(int32(20)),
				UnschedulableLastSeconds:  ptr.To(int32(15)),
			},
		}
		udManager.AddSubset("subset-0", nil, nil, replicas(2))
		udManager.AddSubset("subset-1", nil, nil, replicas(2))
		udManager.AddSubset("subset-2", nil, nil, nil)
		// make subset-1 unschedulable
		nodeKey := "ud-e2e/to-make-a-bad-subset-elastic"
		udManager.UnitedDeployment.Spec.Topology.Subsets[1].NodeSelectorTerm = corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      nodeKey,
					Operator: corev1.NodeSelectorOpExists,
				},
			},
		}

		ginkgo.By("creating united deployment")
		udManager.Spec.Replicas = ptr.To(int32(3))
		_, err := f.KruiseClientSet.AppsV1alpha1().UnitedDeployments(udManager.Namespace).Create(context.Background(),
			udManager.UnitedDeployment, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("wait for rescheduling, will take long")
		udManager.CheckUnschedulableStatus(unschedulableMap([]bool{false, true, false}))
		udManager.CheckSubsetPods(replicasMap([]int32{2, 0, 1}))
		fmt.Println()

		ginkgo.By("scale up while unschedulable")
		udManager.Scale(4)
		udManager.CheckSubsetPods(replicasMap([]int32{2, 0, 2}))
		fmt.Println()

		ginkgo.By("scale down while unschedulable")
		udManager.Scale(3)
		udManager.CheckSubsetPods(replicasMap([]int32{2, 0, 1}))
		fmt.Println()

		ginkgo.By("wait subset recovery, will take long")
		udManager.CheckUnschedulableStatus(unschedulableMap([]bool{false, false, false}))
		fmt.Println()

		ginkgo.By("scale up after recovery")
		udManager.Scale(4)
		udManager.CheckSubsetPods(replicasMap([]int32{2, 1, 1}))
		fmt.Println()

		ginkgo.By("scale down after recovery")
		udManager.Scale(3)
		udManager.CheckSubsetPods(replicasMap([]int32{2, 1, 0})) // even pods in subset-1 are not ready
		fmt.Println()

		ginkgo.By("create new subset")
		udManager.AddSubset("subset-3", nil, replicas(2), nil)
		udManager.Update()
		fmt.Println()

		ginkgo.By("waiting final status after scaling up to new subset, will take long")
		udManager.Scale(6)
		udManager.CheckUnschedulableStatus(unschedulableMap([]bool{false, false, false, false}))
		udManager.CheckSubsetPods(replicasMap([]int32{2, 0, 2, 2}))
		fmt.Println()

		ginkgo.By("fix subset-1 and wait recover")
		nodeList, err := f.ClientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		someNode := nodeList.Items[1]
		someNode.Labels[nodeKey] = "haha"
		_, err = f.ClientSet.CoreV1().Nodes().Update(context.Background(), &someNode, metav1.UpdateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		udManager.CheckUnschedulableStatus(unschedulableMap([]bool{false, false, false}))

		ginkgo.By("waiting final status after deleting new subset")
		udManager.Spec.Topology.Subsets = udManager.Spec.Topology.Subsets[:3]
		udManager.Update()
		udManager.CheckSubsetPods(replicasMap([]int32{2, 2, 2}))
		fmt.Println()

		ginkgo.By("scale down after fixed")
		udManager.Scale(3)
		udManager.CheckUnschedulableStatus(unschedulableMap([]bool{false, false, false}))
		udManager.CheckSubsetPods(replicasMap([]int32{2, 1, 0}))
		fmt.Println()

		ginkgo.By("scale up after fixed")
		udManager.Scale(5)
		udManager.CheckUnschedulableStatus(unschedulableMap([]bool{false, false, false}))
		udManager.CheckSubsetPods(replicasMap([]int32{2, 2, 1}))
		fmt.Println()
	})
})
