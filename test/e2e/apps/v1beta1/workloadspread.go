/*
Copyright 2026 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util/workloadspread"
	"github.com/openkruise/kruise/test/e2e/framework/common"
	"github.com/openkruise/kruise/test/e2e/framework/v1beta1"
)

const WorkloadSpreadFakeZoneKey = "e2e.kruise.io/workloadspread-fake-zone-v1beta1"

var (
	KruiseKindCloneSet = appsv1alpha1.SchemeGroupVersion.WithKind("CloneSet")
)

var _ = ginkgo.Describe("WorkloadSpread v1beta1", ginkgo.Label("WorkloadSpread", "operation"), func() {
	f := v1beta1.NewDefaultFramework("workloadspread-v1beta1")
	workloadSpreadName := "test-workload-spread"
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var ns string
	var tester *v1beta1.WorkloadSpreadTester

	IsKubernetesVersionLessThan122 := func() bool {
		if v, err := c.Discovery().ServerVersion(); err != nil {
			common.Logf("Failed to discovery server version: %v", err)
		} else if minor, err := strconv.Atoi(v.Minor); err != nil || minor < 22 {
			return true
		}
		return false
	}

	ginkgo.BeforeEach(func() {
		ns = f.Namespace.Name
		c = f.ClientSet
		kc = f.KruiseClientSet
		tester = v1beta1.NewWorkloadSpreadTester(c, kc, f.DynamicClient)

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
			var isMaster bool
			for _, taint := range node.Spec.Taints {
				if taint.Key == "node-role.kubernetes.io/master" || taint.Key == "node-role.kubernetes.io/control-plane" {
					isMaster = true
					break
				}
			}
			if isMaster {
				continue
			}
			workers = append(workers, &node)
		}
		sort.Slice(workers, func(i, j int) bool {
			return workers[i].Name < workers[j].Name
		})
		gomega.Expect(len(workers) > 2).Should(gomega.Equal(true))
		// subset-a
		tester.SetNodeLabel(c, workers[0], WorkloadSpreadFakeZoneKey, "zone-a")
		// subset-b
		tester.SetNodeLabel(c, workers[1], WorkloadSpreadFakeZoneKey, "zone-b")
		tester.SetNodeLabel(c, workers[2], WorkloadSpreadFakeZoneKey, "zone-b")
	})

	ginkgo.Context("WorkloadSpread v1beta1 functionality", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentSpecReport().Failed() {
				common.DumpDebugInfo(c, ns)
			}
		})

		ginkgo.It("v1beta1: deploy in two zones, maxReplicas is Integer", func() {
			if IsKubernetesVersionLessThan122() {
				ginkgo.Skip("requires Kubernetes >= 1.22")
			}
			cloneSet := tester.NewBaseCloneSet(ns)
			targetRef := appsv1beta1.TargetReference{
				APIVersion: KruiseKindCloneSet.GroupVersion().String(),
				Kind:       KruiseKindCloneSet.Kind,
				Name:       cloneSet.Name,
			}
			subset1 := appsv1beta1.WorkloadSpreadSubset{
				Name: "zone-a",
				RequiredNodeSelector: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: WorkloadSpreadFakeZoneKey, Operator: corev1.NodeSelectorOpIn, Values: []string{"zone-a"}},
					},
				},
				MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
				Patch:       runtime.RawExtension{Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-a"}}}`)},
			}
			subset2 := appsv1beta1.WorkloadSpreadSubset{
				Name: "zone-b",
				RequiredNodeSelector: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: WorkloadSpreadFakeZoneKey, Operator: corev1.NodeSelectorOpIn, Values: []string{"zone-b"}},
					},
				},
				MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
				Patch:       runtime.RawExtension{Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-b"}}}`)},
			}
			workloadSpread := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, []appsv1beta1.WorkloadSpreadSubset{subset1, subset2})
			workloadSpread = tester.CreateWorkloadSpread(workloadSpread)

			cloneSet.Spec.Replicas = ptr.To(int32(6))
			cloneSet = tester.CreateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			ginkgo.By(fmt.Sprintf("check pod affinity and WS status for %s/%s", ns, workloadSpreadName))
			pods, err := tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(6))

			subset1Pods, subset2Pods := 0, 0
			for _, pod := range pods {
				str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]
				if !ok {
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
					continue
				}
				var inj *workloadspread.InjectWorkloadSpread
				gomega.Expect(json.Unmarshal([]byte(str), &inj)).To(gomega.Succeed())
				switch inj.Subset {
				case subset1.Name:
					subset1Pods++
					gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
					gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).
						To(gomega.Equal(subset1.RequiredNodeSelector.MatchExpressions))
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
				case subset2.Name:
					subset2Pods++
					gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
					gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).
						To(gomega.Equal(subset2.RequiredNodeSelector.MatchExpressions))
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(3))
			gomega.Expect(subset2Pods).To(gomega.Equal(3))

			workloadSpread, err = kc.AppsV1beta1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(3)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(3)))

			// scale down
			ginkgo.By("scale down replicas=4, maxReplicas=2 per zone")
			workloadSpread.Spec.Subsets[0].MaxReplicas.IntVal = 2
			workloadSpread.Spec.Subsets[1].MaxReplicas.IntVal = 2
			tester.UpdateWorkloadSpread(workloadSpread)
			cloneSet.Spec.Replicas = ptr.To(int32(4))
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(4))

			workloadSpread, err = kc.AppsV1beta1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(2)))

			ginkgo.By("v1beta1: deploy in two zones done")
		})

		ginkgo.It("v1beta1: elastic deployment, zone-a=2, zone-b=nil", func() {
			if IsKubernetesVersionLessThan122() {
				ginkgo.Skip("requires Kubernetes >= 1.22")
			}
			cloneSet := tester.NewBaseCloneSet(ns)
			targetRef := appsv1beta1.TargetReference{
				APIVersion: KruiseKindCloneSet.GroupVersion().String(),
				Kind:       KruiseKindCloneSet.Kind,
				Name:       cloneSet.Name,
			}
			subset1 := appsv1beta1.WorkloadSpreadSubset{
				Name: "zone-a",
				RequiredNodeSelector: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: WorkloadSpreadFakeZoneKey, Operator: corev1.NodeSelectorOpIn, Values: []string{"zone-a"}},
					},
				},
				MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				Patch:       runtime.RawExtension{Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-a"}}}`)},
			}
			// zone-b has no maxReplicas — elastic overflow subset
			subset2 := appsv1beta1.WorkloadSpreadSubset{
				Name: "zone-b",
				RequiredNodeSelector: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: WorkloadSpreadFakeZoneKey, Operator: corev1.NodeSelectorOpIn, Values: []string{"zone-b"}},
					},
				},
				Patch: runtime.RawExtension{Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-b"}}}`)},
			}
			workloadSpread := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, []appsv1beta1.WorkloadSpreadSubset{subset1, subset2})
			workloadSpread = tester.CreateWorkloadSpread(workloadSpread)

			cloneSet.Spec.Replicas = ptr.To(int32(5))
			cloneSet = tester.CreateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			ginkgo.By("check 2 pods in zone-a and 3 in zone-b")
			pods, err := tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(5))

			subset1Pods, subset2Pods := 0, 0
			for _, pod := range pods {
				str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]
				if !ok {
					continue
				}
				var inj *workloadspread.InjectWorkloadSpread
				gomega.Expect(json.Unmarshal([]byte(str), &inj)).To(gomega.Succeed())
				switch inj.Subset {
				case subset1.Name:
					subset1Pods++
					gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
					gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).
						To(gomega.Equal(subset1.RequiredNodeSelector.MatchExpressions))
				case subset2.Name:
					subset2Pods++
					gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(3))

			ginkgo.By("v1beta1: elastic deployment done")
		})

		// Cross-version scenario: create WS via v1alpha1 client (using old field names),
		// verify the API server converted it to v1beta1 correctly and the controller
		// sees the right fields.
		ginkgo.It("cross-version: v1alpha1 create is visible as v1beta1 with correct field mapping", func() {
			if IsKubernetesVersionLessThan122() {
				ginkgo.Skip("requires Kubernetes >= 1.22")
			}
			cloneSet := tester.NewBaseCloneSet(ns)
			alphaWS := &appsv1alpha1.WorkloadSpread{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      workloadSpreadName,
				},
				Spec: appsv1alpha1.WorkloadSpreadSpec{
					TargetReference: &appsv1alpha1.TargetReference{
						APIVersion: KruiseKindCloneSet.GroupVersion().String(),
						Kind:       KruiseKindCloneSet.Kind,
						Name:       cloneSet.Name,
					},
					Subsets: []appsv1alpha1.WorkloadSpreadSubset{
						{
							Name: "zone-a",
							// v1alpha1 field name
							RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{Key: WorkloadSpreadFakeZoneKey, Operator: corev1.NodeSelectorOpIn, Values: []string{"zone-a"}},
								},
							},
							MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
						},
						{Name: "zone-b"},
					},
				},
			}

			ginkgo.By("create WorkloadSpread via v1alpha1 client")
			beta := tester.CreateWorkloadSpreadV1alpha1(alphaWS)

			ginkgo.By("verify v1beta1 view has correct RequiredNodeSelector (renamed field)")
			gomega.Expect(beta.Spec.Subsets[0].RequiredNodeSelector).NotTo(gomega.BeNil(),
				"RequiredNodeSelector must be non-nil after conversion from RequiredNodeSelectorTerm")
			gomega.Expect(beta.Spec.Subsets[0].RequiredNodeSelector.MatchExpressions).
				To(gomega.Equal(alphaWS.Spec.Subsets[0].RequiredNodeSelectorTerm.MatchExpressions))
			gomega.Expect(beta.Spec.Subsets[0].RequiredNodeSelector.MatchExpressions[0].Key).
				To(gomega.Equal(WorkloadSpreadFakeZoneKey))

			ginkgo.By("deploy cloneSet and verify pods get correct affinity from v1beta1 WS")
			cloneSet.Spec.Replicas = ptr.To(int32(3))
			cloneSet = tester.CreateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			pods, err := tester.GetSelectorPods(ns, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]
				if !ok {
					continue
				}
				var inj *workloadspread.InjectWorkloadSpread
				gomega.Expect(json.Unmarshal([]byte(str), &inj)).To(gomega.Succeed())
				if inj.Subset == "zone-a" {
					gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil(),
						"node affinity must be injected for zone-a subset created via v1alpha1 API")
					gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).
						NotTo(gomega.BeNil())
				}
			}

			ginkgo.By("cross-version round-trip done")
		})
	})
})
