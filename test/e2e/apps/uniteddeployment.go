package apps

import (
	"context"
	"fmt"
	"time"

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
		udManager := tester.NewUnitedDeploymentManager("ud-elastic-test", false)
		udManager.AddSubset("subset-0", nil, getReplicas("1"), getReplicas("2"))
		udManager.AddSubset("subset-1", nil, getReplicas("1"), getReplicas("2"))
		udManager.AddSubset("subset-2", nil, getReplicas("1"), nil)

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
		udManager := tester.NewUnitedDeploymentManager("ud-specific-test", false)
		udManager.AddSubset("subset-0", getReplicas("25%"), nil, nil)
		udManager.AddSubset("subset-1", getReplicas("25%"), nil, nil)
		udManager.AddSubset("subset-2", nil, nil, nil)

		ginkgo.By("create ud")
		udManager.Create(3)
		udManager.CheckSubsets(replicasMap(1, 1, 1))

		ginkgo.By("scale up")
		udManager.Scale(4)
		udManager.CheckSubsets(replicasMap(1, 1, 2))

		ginkgo.By("scale down")
		udManager.Scale(1)
		udManager.CheckSubsets(replicasMap(0, 0, 1))
	})

	ginkgo.It("adaptive united deployment basic", func() {
		udManager := tester.NewUnitedDeploymentManager("adaptive-ud-elastic-test", false)
		// enable adaptive scheduling
		udManager.UnitedDeployment.Spec.Topology.ScheduleStrategy = appsv1alpha1.UnitedDeploymentScheduleStrategy{
			Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
			Adaptive: &appsv1alpha1.AdaptiveUnitedDeploymentStrategy{
				RescheduleCriticalSeconds: ptr.To(int32(20)),
				UnschedulableDuration:     ptr.To(int32(15)),
			},
		}
		udManager.AddSubset("subset-0", nil, nil, getReplicas("2"))
		udManager.AddSubset("subset-1", nil, nil, getReplicas("2"))
		udManager.AddSubset("subset-2", nil, nil, nil)
		// make subset-1 unschedulable
		nodeKey := "ud-e2e/adaptive-ud"
		udManager.UnitedDeployment.Spec.Topology.Subsets[1].NodeSelectorTerm = corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      nodeKey,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"haha"},
				},
			},
		}
		udManager.SetNodeLabel(nodeKey, "")
		ginkgo.By("creating united deployment")
		udManager.Spec.Replicas = ptr.To(int32(3))
		_, err := f.KruiseClientSet.AppsV1alpha1().UnitedDeployments(udManager.Namespace).Create(context.Background(),
			udManager.UnitedDeployment, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("wait for rescheduling, will take long")
		udManager.CheckUnschedulableStatus(unschedulableMap(false, true, false))
		udManager.CheckSubsetPods(replicasMap(2, 0, 1))
		fmt.Println()

		ginkgo.By("scale up while unschedulable")
		udManager.Scale(5)
		udManager.CheckSubsetPods(replicasMap(2, 0, 3))
		fmt.Println()

		ginkgo.By("scale down while unschedulable")
		udManager.Scale(4)
		udManager.CheckSubsetPods(replicasMap(2, 0, 2))
		fmt.Println()

		ginkgo.By("wait subset recovery, will take long")
		udManager.CheckUnschedulableStatus(unschedulableMap(false, false, false))
		udManager.CheckSubsetPods(replicasMap(2, 0, 2))
		fmt.Println()

		ginkgo.By("scale up after recovery")
		udManager.Scale(5)
		udManager.CheckSubsetPods(replicasMap(2, 1, 2))
		fmt.Println()

		ginkgo.By("scale down after recovery, pending first")
		udManager.Scale(4)
		udManager.CheckSubsetPods(replicasMap(2, 0, 2))
		fmt.Println()

		ginkgo.By("scale down after recovery, goes to running")
		udManager.Scale(3)
		udManager.CheckSubsetPods(replicasMap(2, 0, 1))
		fmt.Println()

		ginkgo.By("create new subset")
		udManager.AddSubset("subset-3", nil, getReplicas("2"), nil)
		udManager.Update()
		fmt.Println()

		ginkgo.By("waiting final status after scaling up to new subset, will take long")
		udManager.Scale(6)
		udManager.CheckUnschedulableStatus(unschedulableMap(false, false, false, false))
		udManager.CheckSubsetPods(replicasMap(2, 0, 2, 2))
		fmt.Println()

		ginkgo.By("fix subset-1 and wait recover")
		udManager.SetNodeLabel(nodeKey, "haha")
		udManager.CheckUnschedulableStatus(unschedulableMap(false, false, false, false))

		ginkgo.By("waiting final status after deleting new subset")
		udManager.Spec.Topology.Subsets = udManager.Spec.Topology.Subsets[:3]
		udManager.Update()
		udManager.CheckUnschedulableStatus(unschedulableMap(false, false, false))
		udManager.CheckSubsetPods(replicasMap(2, 2, 2))
		fmt.Println()

		ginkgo.By("scale down after fixed")
		time.Sleep(10 * time.Second)
		udManager.Scale(3)
		udManager.CheckUnschedulableStatus(unschedulableMap(false, false, false))
		udManager.CheckSubsetPods(replicasMap(2, 1, 0))
		fmt.Println()

		ginkgo.By("scale up after fixed")
		udManager.Scale(5)
		udManager.CheckUnschedulableStatus(unschedulableMap(false, false, false))
		udManager.CheckSubsetPods(replicasMap(2, 2, 1))
		fmt.Println()
	})

	ginkgo.It("adaptive united deployment with min replicas", func() {
		udManager := tester.NewUnitedDeploymentManager("adaptive-ud-with-min", false)
		// enable adaptive scheduling
		udManager.UnitedDeployment.Spec.Topology.ScheduleStrategy = appsv1alpha1.UnitedDeploymentScheduleStrategy{
			Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
			Adaptive: &appsv1alpha1.AdaptiveUnitedDeploymentStrategy{
				RescheduleCriticalSeconds: ptr.To(int32(5)),
				UnschedulableDuration:     ptr.To(int32(1)),
			},
		}
		udManager.AddSubset("subset-0", nil, getReplicas("1"), getReplicas("2"))
		udManager.AddSubset("subset-1", nil, nil, nil)
		// make subset-0 unschedulable
		nodeKey := "ud-e2e/adaptive-ud-min"
		udManager.UnitedDeployment.Spec.Topology.Subsets[0].NodeSelectorTerm = corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      nodeKey,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"haha"},
				},
			},
		}
		udManager.SetNodeLabel(nodeKey, "")
		ginkgo.By("creating united deployment")
		udManager.Spec.Replicas = ptr.To(int32(3))
		_, err := f.KruiseClientSet.AppsV1alpha1().UnitedDeployments(udManager.Namespace).Create(context.Background(),
			udManager.UnitedDeployment, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("wait for rescheduling, will take long")
		udManager.CheckUnschedulableStatus(unschedulableMap(true, false))
		udManager.CheckSubsetPods(replicasMap(1, 2))
		fmt.Println()

		ginkgo.By("scale up while unschedulable")
		udManager.Scale(4)
		udManager.CheckSubsetPods(replicasMap(1, 3))
		fmt.Println()

		ginkgo.By("scale down while unschedulable")
		udManager.Scale(3)
		udManager.CheckSubsetPods(replicasMap(1, 2))
		fmt.Println()

		ginkgo.By("wait subset recovery, will take long")
		// If pending Pods persist, subset-0 will never recover. So we have to fix it.
		udManager.SetNodeLabel(nodeKey, "haha")
		udManager.CheckUnschedulableStatus(unschedulableMap(false, false))
		udManager.CheckSubsetPods(replicasMap(1, 2))
		fmt.Println()

		ginkgo.By("scale up after recovery")
		udManager.Scale(5)
		udManager.CheckSubsetPods(replicasMap(2, 3))
		fmt.Println()

		ginkgo.By("scale down after recovery")
		udManager.Scale(4)
		udManager.CheckSubsetPods(replicasMap(2, 2))
		fmt.Println()

		ginkgo.By("scale down after recovery again")
		udManager.Scale(3)
		udManager.CheckSubsetPods(replicasMap(2, 1))
		fmt.Println()

		ginkgo.By("scale down after recovery again and again")
		udManager.Scale(1)
		udManager.CheckSubsetPods(replicasMap(1, 0))
		fmt.Println()
	})

	ginkgo.It("adaptive united deployment with unschedulable pods reserved", func() {
		replicasMap := func(replicas ...int32) map[string]int32 {
			replicaMap := make(map[string]int32)
			for i, r := range replicas {
				replicaMap[fmt.Sprintf("subset-%d", i)] = r
			}
			return replicaMap
		}
		unschedulableMap := func(nonscheduled ...bool) map[string]bool {
			resultMap := make(map[string]bool)
			for i, r := range nonscheduled {
				resultMap[fmt.Sprintf("subset-%d", i)] = r
			}
			return resultMap
		}

		udManager := tester.NewUnitedDeploymentManager("adaptive-ud-tr-test", true)
		// enable adaptive scheduling
		udManager.UnitedDeployment.Spec.Topology.ScheduleStrategy = appsv1alpha1.UnitedDeploymentScheduleStrategy{
			Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
			Adaptive: &appsv1alpha1.AdaptiveUnitedDeploymentStrategy{
				ReserveUnschedulablePods:  true,
				RescheduleCriticalSeconds: ptr.To(int32(20)),
				UnschedulableDuration:     ptr.To(int32(1)),
			},
		}
		udManager.AddSubset("subset-0", nil, nil, nil)
		udManager.AddSubset("subset-1", nil, nil, nil)
		udManager.AddSubset("subset-2", nil, nil, nil)
		subset0EnabledKey := "ud-e2e/temp-adaptive-subset-0"
		subset1EnabledKey := "ud-e2e/temp-adaptive-subset-1"
		udManager.UnitedDeployment.Spec.Topology.Subsets[0].NodeSelectorTerm = corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      subset0EnabledKey,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"true"},
				},
			},
		}
		udManager.UnitedDeployment.Spec.Topology.Subsets[1].NodeSelectorTerm = corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      subset1EnabledKey,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"true"},
				},
			},
		}
		ginkgo.By("initializing node labels")
		udManager.SetNodeLabel(subset0EnabledKey, "false")
		udManager.SetNodeLabel(subset1EnabledKey, "false")
		ginkgo.By("creating united deployment")
		udManager.Spec.Replicas = ptr.To(int32(2))
		_, err := f.KruiseClientSet.AppsV1alpha1().UnitedDeployments(udManager.Namespace).Create(context.Background(),
			udManager.UnitedDeployment, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("wait for rescheduling, will take long")
		udManager.CheckUnschedulableStatus(unschedulableMap(true, true, false))
		udManager.CheckReservedPods(replicasMap(2, 2, 2), replicasMap(2, 2, 0))
		fmt.Println()

		ginkgo.By("scale up while unschedulable")
		udManager.Scale(4)
		udManager.CheckReservedPods(replicasMap(2, 2, 4), replicasMap(2, 2, 0))
		fmt.Println()

		ginkgo.By("scale down while unschedulable")
		udManager.Scale(2)
		udManager.CheckReservedPods(replicasMap(2, 2, 2), replicasMap(2, 2, 0))
		fmt.Println()

		ginkgo.By("scale up again")
		udManager.Scale(4)
		udManager.CheckReservedPods(replicasMap(2, 2, 4), replicasMap(2, 2, 0))
		fmt.Println()

		ginkgo.By("update")
		udManager.SetImage("busybox:1.33")
		udManager.Update()
		udManager.CheckReservedPods(replicasMap(2, 2, 4), replicasMap(2, 2, 0))
		udManager.CheckPodImage("busybox:1.33")

		ginkgo.By("recover subset-0")
		udManager.SetNodeLabel(subset0EnabledKey, "true")
		udManager.CheckUnschedulableStatus(unschedulableMap(false, false, false))
		udManager.CheckReservedPods(replicasMap(4, 0, 0), replicasMap(0, 0, 0))
		fmt.Println()

		ginkgo.By("scale up after recovery")
		udManager.Scale(5)
		udManager.CheckReservedPods(replicasMap(5, 0, 0), replicasMap(0, 0, 0))
		fmt.Println()
	})

	ginkgo.It("adaptive united deployment with reserved pods and min replicas", func() {
		replicasMap := func(replicas ...int32) map[string]int32 {
			replicaMap := make(map[string]int32)
			for i, r := range replicas {
				replicaMap[fmt.Sprintf("subset-%d", i)] = r
			}
			return replicaMap
		}
		unschedulableMap := func(nonscheduled ...bool) map[string]bool {
			resultMap := make(map[string]bool)
			for i, r := range nonscheduled {
				resultMap[fmt.Sprintf("subset-%d", i)] = r
			}
			return resultMap
		}

		udManager := tester.NewUnitedDeploymentManager("adaptive-ud-tr-test", true)
		// enable adaptive scheduling
		udManager.UnitedDeployment.Spec.Topology.ScheduleStrategy = appsv1alpha1.UnitedDeploymentScheduleStrategy{
			Type: appsv1alpha1.AdaptiveUnitedDeploymentScheduleStrategyType,
			Adaptive: &appsv1alpha1.AdaptiveUnitedDeploymentStrategy{
				ReserveUnschedulablePods:  true,
				RescheduleCriticalSeconds: ptr.To(int32(20)),
				UnschedulableDuration:     ptr.To(int32(1)),
			},
		}
		udManager.AddSubset("subset-0", nil, getReplicas("1"), nil)
		udManager.AddSubset("subset-1", nil, getReplicas("1"), nil)
		udManager.AddSubset("subset-2", nil, nil, nil)
		subset0EnabledKey := "ud-e2e/temp-adaptive-subset-min-0"
		subset1EnabledKey := "ud-e2e/temp-adaptive-subset-min-1"
		udManager.UnitedDeployment.Spec.Topology.Subsets[0].NodeSelectorTerm = corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      subset0EnabledKey,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"true"},
				},
			},
		}
		udManager.UnitedDeployment.Spec.Topology.Subsets[1].NodeSelectorTerm = corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      subset1EnabledKey,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"true"},
				},
			},
		}
		ginkgo.By("initializing node labels")
		udManager.SetNodeLabel(subset0EnabledKey, "false")
		udManager.SetNodeLabel(subset1EnabledKey, "false")
		ginkgo.By("creating united deployment")
		udManager.Spec.Replicas = ptr.To(int32(2))
		_, err := f.KruiseClientSet.AppsV1alpha1().UnitedDeployments(udManager.Namespace).Create(context.Background(),
			udManager.UnitedDeployment, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("wait for rescheduling, will take long")
		udManager.CheckUnschedulableStatus(unschedulableMap(true, true, false))
		udManager.CheckReservedPods(replicasMap(2, 2, 2), replicasMap(2, 2, 0))
		fmt.Println()

		ginkgo.By("scale up while unschedulable")
		udManager.Scale(4)
		udManager.CheckReservedPods(replicasMap(2, 2, 4), replicasMap(2, 2, 0))
		fmt.Println()

		ginkgo.By("scale down while unschedulable")
		udManager.Scale(2)
		udManager.CheckReservedPods(replicasMap(2, 2, 2), replicasMap(2, 2, 0))
		fmt.Println()

		ginkgo.By("scale up again")
		udManager.Scale(4)
		udManager.CheckReservedPods(replicasMap(2, 2, 4), replicasMap(2, 2, 0))
		fmt.Println()

		ginkgo.By("update")
		udManager.SetImage("busybox:1.33")
		udManager.Update()
		udManager.CheckReservedPods(replicasMap(2, 2, 4), replicasMap(2, 2, 0))
		udManager.CheckPodImage("busybox:1.33")

		ginkgo.By("recover subset-0")
		udManager.SetNodeLabel(subset0EnabledKey, "true")
		udManager.CheckUnschedulableStatus(unschedulableMap(false, false, false))
		udManager.CheckReservedPods(replicasMap(4, 0, 0), replicasMap(0, 0, 0))
		fmt.Println()

		ginkgo.By("scale up after recovery")
		udManager.Scale(5)
		udManager.CheckReservedPods(replicasMap(5, 0, 0), replicasMap(0, 0, 0))
		fmt.Println()
	})
})

func getReplicas(p string) *intstr.IntOrString { x := intstr.Parse(p); return &x }

func replicasMap(replicas ...int32) map[string]int32 {
	replicaMap := make(map[string]int32)
	for i, r := range replicas {
		replicaMap[fmt.Sprintf("subset-%d", i)] = r
	}
	return replicaMap
}
func unschedulableMap(nonscheduled ...bool) map[string]bool {
	resultMap := make(map[string]bool)
	for i, r := range nonscheduled {
		resultMap[fmt.Sprintf("subset-%d", i)] = r
	}
	return resultMap
}
