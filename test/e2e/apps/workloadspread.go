/*
Copyright 2021 The Kruise Authors.

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

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util/workloadspread"
	"github.com/openkruise/kruise/test/e2e/framework"
)

const WorkloadSpreadFakeZoneKey = "e2e.kruise.io/workloadspread-fake-zone"

var (
	KruiseKindCloneSet    = appsv1alpha1.SchemeGroupVersion.WithKind("CloneSet")
	KruiseKindStatefulSet = appsv1alpha1.SchemeGroupVersion.WithKind("StatefulSet")
	controllerKindDep     = appsv1.SchemeGroupVersion.WithKind("Deployment")
	//controllerKindJob  = batchv1.SchemeGroupVersion.WithKind("Job")
)

var _ = SIGDescribe("workloadspread", func() {
	f := framework.NewDefaultFramework("workloadspread")
	workloadSpreadName := "test-workload-spread"
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var ns string
	var tester *framework.WorkloadSpreadTester

	IsKubernetesVersionLessThan122 := func() bool {
		if v, err := c.Discovery().ServerVersion(); err != nil {
			framework.Logf("Failed to discovery server version: %v", err)
		} else if minor, err := strconv.Atoi(v.Minor); err != nil || minor < 22 {
			return true
		}
		return false
	}

	ginkgo.BeforeEach(func() {
		ns = f.Namespace.Name
		c = f.ClientSet
		kc = f.KruiseClientSet
		tester = framework.NewWorkloadSpreadTester(c, kc, f.DynamicClient)

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
			// The node-role.kubernetes.io/master will be only set in taints since Kubernetes v1.24
			var isMaster bool
			for _, taint := range node.Spec.Taints {
				// 1.24
				if taint.Key == "node-role.kubernetes.io/master" {
					isMaster = true
					break
				}
				// 1.26
				if taint.Key == "node-role.kubernetes.io/control-plane" {
					isMaster = true
					break
				}
			}
			if isMaster {
				continue
			}
			workers = append(workers, &node)
		}
		gomega.Expect(len(workers) > 2).Should(gomega.Equal(true))
		// subset-a
		worker0 := workers[0]
		tester.SetNodeLabel(c, worker0, WorkloadSpreadFakeZoneKey, "zone-a")
		// subset-b
		worker1 := workers[1]
		tester.SetNodeLabel(c, worker1, WorkloadSpreadFakeZoneKey, "zone-b")
		worker2 := workers[2]
		tester.SetNodeLabel(c, worker2, WorkloadSpreadFakeZoneKey, "zone-b")
	})

	f.AfterEachActions = []func(){
		func() {
			nodeList, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			patchBody := fmt.Sprintf(`{"metadata":{"labels":{"%s":null}}}`, WorkloadSpreadFakeZoneKey)
			for i := range nodeList.Items {
				node := nodeList.Items[i]
				if _, exist := node.GetLabels()[WorkloadSpreadFakeZoneKey]; !exist {
					continue
				}
				_, err = c.CoreV1().Nodes().Patch(context.TODO(), node.Name, types.StrategicMergePatchType, []byte(patchBody), metav1.PatchOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			}
		},
	}

	framework.KruiseDescribe("WorkloadSpread functionality", func() {
		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
		})

		framework.ConformanceIt("deploy in two zone, the type of maxReplicas is Integer", func() {
			cloneSet := tester.NewBaseCloneSet(ns)
			// create workloadSpread
			targetRef := appsv1alpha1.TargetReference{
				APIVersion: KruiseKindCloneSet.GroupVersion().String(),
				Kind:       KruiseKindCloneSet.Kind,
				Name:       cloneSet.Name,
			}
			subset1 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "zone-a",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-a"},
						},
					},
				},
				MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-a"}}}`),
				},
			}
			subset2 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "zone-b",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-b"},
						},
					},
				},
				MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-b"}}}`),
				},
			}
			workloadSpread := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, []appsv1alpha1.WorkloadSpreadSubset{subset1, subset2})
			workloadSpread = tester.CreateWorkloadSpread(workloadSpread)

			// create cloneset, replicas = 6
			cloneSet.Spec.Replicas = ptr.To(int32(6))
			cloneSet = tester.CreateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err := tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(6))
			subset1Pods := 0
			subset2Pods := 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(3))
			gomega.Expect(subset2Pods).To(gomega.Equal(3))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(3)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[1].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(3)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].DeletingPods)).To(gomega.Equal(0))

			// update cloneset image
			ginkgo.By(fmt.Sprintf("update cloneSet(%s/%s) image=%s", cloneSet.Namespace, cloneSet.Name, NewWebserverImage))
			cloneSet.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			tester.UpdateCloneSet(cloneSet)
			tester.WaiteCloneSetUpdate(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(6))
			subset1Pods = 0
			subset2Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(3))
			gomega.Expect(subset2Pods).To(gomega.Equal(3))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(3)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[1].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(3)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].DeletingPods)).To(gomega.Equal(0))

			//scale down cloneSet.replicas = 4, maxReplicas = 2.
			ginkgo.By(fmt.Sprintf("scale down cloneSet(%s/%s) replicas=4", cloneSet.Namespace, cloneSet.Name))
			workloadSpread.Spec.Subsets[0].MaxReplicas.IntVal = 2
			workloadSpread.Spec.Subsets[1].MaxReplicas.IntVal = 2
			tester.UpdateWorkloadSpread(workloadSpread)
			cloneSet.Spec.Replicas = ptr.To(int32(4))
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(4))
			subset1Pods = 0
			subset2Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(2))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[1].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].DeletingPods)).To(gomega.Equal(0))

			//scale up cloneSet.replicas = 8
			ginkgo.By(fmt.Sprintf("scale up cloneSet(%s/%s) replicas=8, maxReplicas=4", cloneSet.Namespace, cloneSet.Name))
			workloadSpread.Spec.Subsets[0].MaxReplicas.IntVal = 4
			workloadSpread.Spec.Subsets[1].MaxReplicas.IntVal = 4
			tester.UpdateWorkloadSpread(workloadSpread)
			cloneSet.Spec.Replicas = ptr.To(int32(8))
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(8))
			subset1Pods = 0
			subset2Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(4))
			gomega.Expect(subset2Pods).To(gomega.Equal(4))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(4)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[1].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(4)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].DeletingPods)).To(gomega.Equal(0))

			ginkgo.By("deploy in two zone, the type of maxReplicas is Integer, done")
		})

		framework.ConformanceIt("elastic deployment, zone-a=2, zone-b=nil", func() {
			cloneSet := tester.NewBaseCloneSet(ns)
			// create workloadSpread
			targetRef := appsv1alpha1.TargetReference{
				APIVersion: KruiseKindCloneSet.GroupVersion().String(),
				Kind:       KruiseKindCloneSet.Kind,
				Name:       cloneSet.Name,
			}
			subset1 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "zone-a",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-a"},
						},
					},
				},
				MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-a"}}}`),
				},
			}
			subset2 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "zone-b",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-b"},
						},
					},
				},
				MaxReplicas: nil,
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-b"}}}`),
				},
			}
			workloadSpread := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, []appsv1alpha1.WorkloadSpreadSubset{subset1, subset2})
			workloadSpread = tester.CreateWorkloadSpread(workloadSpread)

			// create cloneset, replicas = 2
			cloneSet = tester.CreateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err := tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(2))
			subset1Pods := 0
			subset2Pods := 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(0))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[1].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].MissingReplicas).To(gomega.Equal(int32(-1)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(0)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].DeletingPods)).To(gomega.Equal(0))

			//scale up cloneSet.replicas = 6
			ginkgo.By(fmt.Sprintf("scale up cloneSet(%s/%s) replicas=6", cloneSet.Namespace, cloneSet.Name))
			cloneSet.Spec.Replicas = ptr.To(int32(6))
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(6))
			subset1Pods = 0
			subset2Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(4))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[1].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].MissingReplicas).To(gomega.Equal(int32(-1)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(4)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].DeletingPods)).To(gomega.Equal(0))

			// update cloneset image
			ginkgo.By(fmt.Sprintf("update cloneSet(%s/%s) image=%s", cloneSet.Namespace, cloneSet.Name, NewWebserverImage))
			cloneSet.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(6))
			subset1Pods = 0
			subset2Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(4))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[1].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].MissingReplicas).To(gomega.Equal(int32(-1)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(4)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].DeletingPods)).To(gomega.Equal(0))

			//scale down cloneSet.replicas = 2
			ginkgo.By(fmt.Sprintf("scale down cloneSet(%s/%s) replicas=2", cloneSet.Namespace, cloneSet.Name))
			cloneSet.Spec.Replicas = ptr.To(int32(2))
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(2))
			subset1Pods = 0
			subset2Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(0))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			time.Sleep(2 * time.Second)
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[1].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].MissingReplicas).To(gomega.Equal(int32(-1)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(0)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].DeletingPods)).To(gomega.Equal(0))

			ginkgo.By("elastic deployment, zone-a=2, zone-b=nil, done")
		})

		framework.ConformanceIt("reschedule subset-a", func() {
			cloneSet := tester.NewBaseCloneSet(ns)
			// create workloadSpread
			targetRef := appsv1alpha1.TargetReference{
				APIVersion: KruiseKindCloneSet.GroupVersion().String(),
				Kind:       KruiseKindCloneSet.Kind,
				Name:       cloneSet.Name,
			}
			subset1 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "subset-a",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							// Pod is not schedulable due to incorrect configuration
							Values: []string{"zone-x"},
						},
					},
				},
				MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
				},
			}
			subset2 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "subset-b",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-b"},
						},
					},
				},
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-b"}}}`),
				},
			}
			workloadSpread := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, []appsv1alpha1.WorkloadSpreadSubset{subset1, subset2})
			workloadSpread.Spec.ScheduleStrategy = appsv1alpha1.WorkloadSpreadScheduleStrategy{
				Type: appsv1alpha1.FixedWorkloadSpreadScheduleStrategyType,
			}
			workloadSpread = tester.CreateWorkloadSpread(workloadSpread)

			// create cloneset, replicas = 5
			cloneSet.Spec.Replicas = ptr.To(int32(5))
			cloneSet = tester.CreateCloneSet(cloneSet)
			tester.WaitForCloneSetRunReplicas(cloneSet, int32(3))

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err := tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(5))
			subset1Pods := 0
			subset1RunningPods := 0
			subset2Pods := 0
			subset2RunningPods := 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						if pod.Status.Phase == corev1.PodRunning {
							subset1RunningPods++
						}
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
						gomega.Expect(pod.Annotations["subset"]).To(gomega.Equal(subset1.Name))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						if pod.Status.Phase == corev1.PodRunning {
							subset2RunningPods++
						}
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
						gomega.Expect(pod.Annotations["subset"]).To(gomega.Equal(subset2.Name))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset1RunningPods).To(gomega.Equal(0))
			gomega.Expect(subset2Pods).To(gomega.Equal(3))
			gomega.Expect(subset2RunningPods).To(gomega.Equal(3))

			// check workloadSpread status
			ginkgo.By(fmt.Sprintf("check workloadSpread(%s/%s) status", workloadSpread.Namespace, workloadSpread.Name))
			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].Conditions)).To(gomega.Equal(0))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[1].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].MissingReplicas).To(gomega.Equal(int32(-1)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[1].Replicas).To(gomega.Equal(int32(3)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[1].Conditions)).To(gomega.Equal(0))

			// wait for subset schedulabe
			ginkgo.By(fmt.Sprintf("wait workloadSpread(%s/%s) subset-a reschedulabe", workloadSpread.Namespace, workloadSpread.Name))
			workloadSpread.Spec.ScheduleStrategy = appsv1alpha1.WorkloadSpreadScheduleStrategy{
				Type: appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType,
				Adaptive: &appsv1alpha1.AdaptiveWorkloadSpreadStrategy{
					DisableSimulationSchedule: true,
					RescheduleCriticalSeconds: ptr.To(int32(5)),
				},
			}
			tester.UpdateWorkloadSpread(workloadSpread)
			tester.WaitForWorkloadSpreadRunning(workloadSpread)

			err = wait.PollUntilContextTimeout(context.Background(), time.Second, time.Minute*6, true, func(ctx context.Context) (bool, error) {
				ws, err := kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				for _, condition := range ws.Status.SubsetStatuses[0].Conditions {
					if condition.Type == appsv1alpha1.SubsetSchedulable && condition.Status == corev1.ConditionFalse {
						return true, nil
					}
				}
				return false, nil
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			tester.WaitForCloneSetRunReplicas(cloneSet, int32(5))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(5))
			subset1Pods = 0
			subset2Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
						gomega.Expect(pod.Annotations["subset"]).To(gomega.Equal(subset2.Name))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(0))
			gomega.Expect(subset2Pods).To(gomega.Equal(5))

			// wait subset-a to schedulable
			ginkgo.By("wait subset-a to schedulable")
			err = wait.PollUntilContextTimeout(context.Background(), time.Second, time.Minute*5, true, func(ctx context.Context) (bool, error) {
				ws, err := kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				for _, condition := range ws.Status.SubsetStatuses[0].Conditions {
					if condition.Type == appsv1alpha1.SubsetSchedulable && condition.Status == corev1.ConditionTrue {
						return true, nil
					}
				}
				return false, nil
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ginkgo.By("workloadSpread reschedule subset-a, done")
		})

		framework.ConformanceIt("manage the pods that were created before workloadspread", func() {
			// build cloneSet, default to schedule its pods to zone-a
			cloneSet := tester.NewBaseCloneSet(ns)
			cloneSet.Spec.Replicas = ptr.To(int32(4))
			cloneSet.Spec.Template.Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
					Weight: 100,
					Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key:      WorkloadSpreadFakeZoneKey,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"zone-a"},
					}}}},
				}},
			}
			// build workloadSpread
			targetRef := appsv1alpha1.TargetReference{
				APIVersion: KruiseKindCloneSet.GroupVersion().String(),
				Kind:       KruiseKindCloneSet.Kind,
				Name:       cloneSet.Name,
			}
			subset1 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "zone-a",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-a"},
						},
					},
				},
				MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-a"}}}`),
				},
			}
			subset2 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "zone-b",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-b"},
						},
					},
				},
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-b"}}}`),
				},
			}
			workloadSpread := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, []appsv1alpha1.WorkloadSpreadSubset{subset1, subset2})
			ginkgo.By("Creating CloneSet with 4 replicas before creating workloadSpread...")
			cloneSet = tester.CreateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			ginkgo.By("annotate some pods with an unmatched WorkloadSpread...")
			pods, err := tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			fakeWS := &workloadspread.InjectWorkloadSpread{
				Name:   "fakeWS",
				Subset: "subset-a",
			}
			by, _ := json.Marshal(fakeWS)
			f.PodClient().Update(pods[0].Name, func(updatePod *corev1.Pod) {
				updatePod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations] = string(by)
			})
			f.PodClient().Update(pods[1].Name, func(updatePod *corev1.Pod) {
				updatePod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations] = string(by)
			})

			ginkgo.By("Creating workloadSpread, check its subsetStatus...")
			workloadSpread = tester.CreateWorkloadSpread(workloadSpread)
			gomega.Eventually(func() int32 {
				workloadSpread, err := tester.GetWorkloadSpread(workloadSpread.Namespace, workloadSpread.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return workloadSpread.Status.SubsetStatuses[0].Replicas
			}, 5*time.Minute, time.Second).Should(gomega.Equal(int32(4)))

			ginkgo.By("Extend CloneSet to 6 replicas, check subsetStatus and pods...")
			cloneSet.Spec.Replicas = ptr.To(int32(6))
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)
			gomega.Eventually(func() int32 {
				workloadSpread, err := tester.GetWorkloadSpread(workloadSpread.Namespace, workloadSpread.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return workloadSpread.Status.SubsetStatuses[1].Replicas
			}, 5*time.Minute, time.Second).Should(gomega.Equal(int32(2)))

			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			subset1Pods := 0
			subset2Pods := 0
			uninjectPods := 0
			zeroDeletionCost := 0
			negativeDeletionCost := 0
			positiveDeletionCost := 0
			for _, pod := range pods {
				// count for different deletion costs
				cost, err := strconv.Atoi(pod.Annotations[workloadspread.PodDeletionCostAnnotation])
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				if cost > 0 {
					positiveDeletionCost++
				} else if cost == 0 {
					zeroDeletionCost++
				} else {
					negativeDeletionCost++
				}
				// count for different subsets
				str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]
				if ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Name == workloadSpreadName {
						if injectWorkloadSpread.Subset == subset1.Name {
							subset1Pods++
						} else if injectWorkloadSpread.Subset == subset2.Name {
							subset2Pods++
							gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
							gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
							gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
							gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
							gomega.Expect(pod.Annotations["subset"]).To(gomega.Equal(subset2.Name))
						}
						continue
					}
				}
				uninjectPods++
			}
			gomega.Expect(zeroDeletionCost).To(gomega.Equal(0))
			gomega.Expect(positiveDeletionCost).To(gomega.Equal(4))
			gomega.Expect(negativeDeletionCost).To(gomega.Equal(2))
			gomega.Expect(subset1Pods).To(gomega.Equal(4))
			gomega.Expect(subset2Pods).To(gomega.Equal(2))
			gomega.Expect(uninjectPods).To(gomega.Equal(0))

			ginkgo.By("Shrink CloneSet to 3 replicas, check subsetStatus and pods...")
			cloneSet.Spec.Replicas = ptr.To(int32(3))
			cloneSet.Spec.Template.Spec.NodeSelector = nil
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)
			gomega.Eventually(func() int32 {
				workloadSpread, err := tester.GetWorkloadSpread(workloadSpread.Namespace, workloadSpread.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return workloadSpread.Status.SubsetStatuses[0].Replicas
			}, 5*time.Minute, time.Second).Should(gomega.Equal(int32(2)))

			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			subset1Pods = 0
			subset2Pods = 0
			uninjectPods = 0
			zeroDeletionCost = 0
			negativeDeletionCost = 0
			positiveDeletionCost = 0
			for _, pod := range pods {
				// count for different deletion costs
				cost, err := strconv.Atoi(pod.Annotations[workloadspread.PodDeletionCostAnnotation])
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				if cost > 0 {
					positiveDeletionCost++
				} else if cost == 0 {
					zeroDeletionCost++
				} else {
					negativeDeletionCost++
				}
				// count for different subsets
				str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]
				if ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Name == workloadSpreadName {
						if injectWorkloadSpread.Subset == subset1.Name {
							subset1Pods++
						} else if injectWorkloadSpread.Subset == subset2.Name {
							subset2Pods++
							gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
							gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
							gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
							gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
							gomega.Expect(pod.Annotations["subset"]).To(gomega.Equal(subset2.Name))
						}
						continue
					}
				}
				uninjectPods++
			}
			gomega.Expect(zeroDeletionCost).To(gomega.Equal(0))
			gomega.Expect(positiveDeletionCost).To(gomega.Equal(3))
			gomega.Expect(negativeDeletionCost).To(gomega.Equal(0))
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(1))
			gomega.Expect(uninjectPods).To(gomega.Equal(0))
		})

		framework.ConformanceIt("only one subset, zone-a=nil", func() {
			priorityClass := &schedulingv1.PriorityClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-priority-class",
				},
				Value:         100,
				GlobalDefault: false,
			}
			tester.CreatePriorityClass(priorityClass)
			cloneSet := tester.NewBaseCloneSet(ns)
			// create workloadSpread
			targetRef := appsv1alpha1.TargetReference{
				APIVersion: KruiseKindCloneSet.GroupVersion().String(),
				Kind:       KruiseKindCloneSet.Kind,
				Name:       cloneSet.Name,
			}
			subset1 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "zone-a",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-a"},
						},
					},
				},
				MaxReplicas: nil,
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-a"}},"spec":{"priorityClassName":"test-priority-class"}}`),
				},
			}

			workloadSpread := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, []appsv1alpha1.WorkloadSpreadSubset{subset1})
			workloadSpread = tester.CreateWorkloadSpread(workloadSpread)

			// create cloneset, replicas = 2
			cloneSet = tester.CreateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err := tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(2))
			subset1Pods := 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Spec.PriorityClassName).To(gomega.Equal("test-priority-class"))
						gomega.Expect(*pod.Spec.Priority).To(gomega.BeEquivalentTo(100))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(-1)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))

			//scale up cloneSet.replicas = 6
			ginkgo.By(fmt.Sprintf("scale up cloneSet(%s/%s) replicas=6", cloneSet.Namespace, cloneSet.Name))
			cloneSet.Spec.Replicas = ptr.To(int32(6))
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(6))
			subset1Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Spec.PriorityClassName).To(gomega.Equal("test-priority-class"))
						gomega.Expect(*pod.Spec.Priority).To(gomega.BeEquivalentTo(100))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(6))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(-1)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(6)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))

			// update cloneset image
			ginkgo.By(fmt.Sprintf("update cloneSet(%s/%s) image=%s", cloneSet.Namespace, cloneSet.Name, NewWebserverImage))
			cloneSet.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(6))
			subset1Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Spec.PriorityClassName).To(gomega.Equal("test-priority-class"))
						gomega.Expect(*pod.Spec.Priority).To(gomega.BeEquivalentTo(100))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(6))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(-1)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(6)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))

			ginkgo.By("elastic deployment, zone-a=2, zone-b=nil, done")
		})

		framework.ConformanceIt("only one subset, zone-a=2", func() {
			cloneSet := tester.NewBaseCloneSet(ns)
			// create workloadSpread
			targetRef := appsv1alpha1.TargetReference{
				APIVersion: KruiseKindCloneSet.GroupVersion().String(),
				Kind:       KruiseKindCloneSet.Kind,
				Name:       cloneSet.Name,
			}
			subset1 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "zone-a",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-a"},
						},
					},
				},
				MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-a"}}}`),
				},
			}

			workloadSpread := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, []appsv1alpha1.WorkloadSpreadSubset{subset1})
			workloadSpread = tester.CreateWorkloadSpread(workloadSpread)

			// create cloneset, replicas = 2
			cloneSet = tester.CreateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err := tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(2))
			subset1Pods := 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))

			//scale up cloneSet.replicas = 6
			ginkgo.By(fmt.Sprintf("scale up cloneSet(%s/%s) replicas=6", cloneSet.Namespace, cloneSet.Name))
			cloneSet.Spec.Replicas = ptr.To(int32(6))
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(6))
			subset1Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))

			// scale down cloneset image
			//scale up cloneSet.replicas = 2
			ginkgo.By(fmt.Sprintf("scale up cloneSet(%s/%s) replicas=6", cloneSet.Namespace, cloneSet.Name))
			cloneSet.Spec.Replicas = ptr.To(int32(2))
			tester.UpdateCloneSet(cloneSet)
			tester.WaitForCloneSetRunning(cloneSet)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get cloneSet(%s/%s) pods, and check workloadSpread(%s/%s) status", cloneSet.Namespace, cloneSet.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(cloneSet.Namespace, cloneSet.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(2))
			subset1Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Replicas).To(gomega.Equal(int32(2)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))

			ginkgo.By("elastic deployment, zone-a=2, zone-b=nil, done")
		})

		framework.ConformanceIt("manage existing pods by only preferredNodeSelector, then deletion subset-b", func() {
			cs := tester.NewBaseCloneSet(ns)
			// create workloadSpread
			targetRef := appsv1alpha1.TargetReference{
				APIVersion: KruiseKindCloneSet.GroupVersion().String(),
				Kind:       KruiseKindCloneSet.Kind,
				Name:       cs.Name,
			}
			subsets := []appsv1alpha1.WorkloadSpreadSubset{
				{
					Name: "subset-a",
					Tolerations: []corev1.Toleration{{
						Key:      "node-taint",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					}},
					PreferredNodeSelectorTerms: []corev1.PreferredSchedulingTerm{
						{
							Weight: 20,
							Preference: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{{
									Key:      WorkloadSpreadFakeZoneKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-a"},
								}},
							},
						},
						{
							Weight: 10,
							Preference: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{{
									Key:      WorkloadSpreadFakeZoneKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-b"},
								}},
							},
						},
					},
					MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 3},
				},
				{
					Name: "subset-b",
					Tolerations: []corev1.Toleration{{
						Key:      "node-taint",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					}},
					PreferredNodeSelectorTerms: []corev1.PreferredSchedulingTerm{
						{
							Weight: 10,
							Preference: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{{
									Key:      WorkloadSpreadFakeZoneKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-a"},
								}},
							},
						},
						{
							Weight: 20,
							Preference: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{{
									Key:      WorkloadSpreadFakeZoneKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-b"},
								}},
							},
						},
					},
				},
			}

			cs.Spec.Replicas = ptr.To(int32(3))
			cs.Spec.Template.Spec.Affinity = &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
						{
							Weight: 100,
							Preference: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{{
									Key:      WorkloadSpreadFakeZoneKey,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"zone-b"},
								}},
							},
						},
					},
				},
			}

			ginkgo.By("Creating cloneset and waiting for pods to be ready...")
			cs = tester.CreateCloneSet(cs)
			tester.WaitForCloneSetRunning(cs)

			ginkgo.By("Creating workloadspread and waiting for its reconcile...")
			ws := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, subsets)
			ws = tester.CreateWorkloadSpread(ws)
			tester.WaitForWorkloadSpreadRunning(ws)
			ws, err := tester.GetWorkloadSpread(ws.Namespace, ws.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Check workloadspread status...")
			var replicasForSubsetA, replicasForSubsetB int32
			for _, subset := range ws.Status.SubsetStatuses {
				switch subset.Name {
				case "subset-a":
					replicasForSubsetA += subset.Replicas
				case "subset-b":
					replicasForSubsetB += subset.Replicas
				}
			}
			gomega.Expect(replicasForSubsetA).To(gomega.Equal(int32(0)))
			gomega.Expect(replicasForSubsetB).To(gomega.Equal(int32(3)))

			ginkgo.By("List pods of cloneset and check their deletion cost...")
			pods, err := tester.GetSelectorPods(ns, cs.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
			}

			ginkgo.By("Deletion subset-b and waiting for workloadspread's reconcile...")
			ws.Spec.Subsets = []appsv1alpha1.WorkloadSpreadSubset{
				ws.Spec.Subsets[0],
			}
			tester.UpdateWorkloadSpread(ws)
			tester.WaitForWorkloadSpreadRunning(ws)

			ginkgo.By("List pods of cloneset and check their deletion cost again...")
			pods, err = tester.GetSelectorPods(ns, cs.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("-200"))
			}

			ginkgo.By("manage existing pods by only preferredNodeSelector, then deletion subset-b, done")
		})

		framework.ConformanceIt("manage statefulset pods only with patch", func() {
			sts, svc := tester.NewBaseHeadlessStatefulSet(ns)
			// create workloadSpread
			targetRef := appsv1alpha1.TargetReference{
				APIVersion: KruiseKindStatefulSet.GroupVersion().String(),
				Kind:       KruiseKindStatefulSet.Kind,
				Name:       sts.Name,
			}
			subsets := []appsv1alpha1.WorkloadSpreadSubset{
				{
					Name:        "subset-a",
					MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
					Patch: runtime.RawExtension{
						Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-a"}}}`),
					},
				},
				{
					Name:        "subset-b",
					MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
					Patch: runtime.RawExtension{
						Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-b"}}}`),
					},
				},
				{
					Name: "subset-c",
					Patch: runtime.RawExtension{
						Raw: []byte(`{"metadata":{"annotations":{"subset":"subset-c"}}}`),
					},
				},
			}

			ginkgo.By("Creating workloadspread and waiting for its reconcile...")
			ws := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, subsets)
			ws = tester.CreateWorkloadSpread(ws)
			tester.WaitForWorkloadSpreadRunning(ws)

			ginkgo.By("Creating statefulset with 5 replicas and waiting for pods to be ready...")
			sts.Spec.Replicas = ptr.To(int32(5))
			tester.CreateService(svc)
			statefulSet := tester.CreateStatefulSet(sts)
			tester.WaitForStatefulSetRunning(statefulSet)

			ginkgo.By("Check workloadspread status...")
			ws, err := tester.GetWorkloadSpread(ws.Namespace, ws.Name)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var replicasForSubsetA, replicasForSubsetB, replicasForSubsetC int32
			for _, subset := range ws.Status.SubsetStatuses {
				switch subset.Name {
				case "subset-a":
					replicasForSubsetA += subset.Replicas
				case "subset-b":
					replicasForSubsetB += subset.Replicas
				case "subset-c":
					replicasForSubsetC += subset.Replicas
				}
			}
			gomega.Expect(replicasForSubsetA).To(gomega.Equal(int32(2)))
			gomega.Expect(replicasForSubsetB).To(gomega.Equal(int32(2)))
			gomega.Expect(replicasForSubsetC).To(gomega.Equal(int32(1)))

			ginkgo.By("List pods of cloneset and check their patch...")
			pods, err := tester.GetSelectorPods(ns, statefulSet.Spec.Selector)
			var podForSubsetA, podForSubsetB, podForSubsetC int32
			for _, pod := range pods {
				switch pod.Annotations["subset"] {
				case "subset-a":
					podForSubsetA++
				case "subset-b":
					podForSubsetB++
				case "subset-c":
					podForSubsetC++
				}
			}
			gomega.Expect(podForSubsetA).To(gomega.Equal(int32(2)))
			gomega.Expect(podForSubsetB).To(gomega.Equal(int32(2)))
			gomega.Expect(podForSubsetC).To(gomega.Equal(int32(1)))
			ginkgo.By("manage statefulset pods only with patch, done")
		})

		framework.ConformanceIt("job-like custom workload", func() {
			newTargetReference := func(name string, apiVersion, kind string) *appsv1alpha1.TargetReference {
				return &appsv1alpha1.TargetReference{
					APIVersion: apiVersion,
					Kind:       kind,
					Name:       name,
				}
			}
			newSubsets := func(aMax, bMax intstr.IntOrString) []appsv1alpha1.WorkloadSpreadSubset {
				return []appsv1alpha1.WorkloadSpreadSubset{
					{
						Name:        "subset-a",
						MaxReplicas: &aMax,
					},
					{
						Name:        "subset-b",
						MaxReplicas: &bMax,
					},
				}
			}
			checkWorkloadSpread := func(ws *appsv1alpha1.WorkloadSpread, replicasA, missA, replicasB, missB int) func(gomega.Gomega) {
				return func(g gomega.Gomega) {
					ws, err := tester.GetWorkloadSpread(ws.Namespace, ws.Name)
					g.Expect(err).NotTo(gomega.HaveOccurred())
					statuses := ws.Status.SubsetStatuses
					g.Expect(len(statuses)).To(gomega.BeEquivalentTo(2))
					g.Expect(statuses[0].Replicas).To(gomega.BeEquivalentTo(replicasA))
					g.Expect(statuses[0].MissingReplicas).To(gomega.BeEquivalentTo(missA))
					g.Expect(statuses[1].Replicas).To(gomega.BeEquivalentTo(replicasB))
					g.Expect(statuses[1].MissingReplicas).To(gomega.BeEquivalentTo(missB))
				}
			}
			ginkgo.By("invalid targetFilter")
			ws := tester.NewWorkloadSpread(ns, "ws-invalid-target-filter", newTargetReference("invalid-target-filter", "apps.kruise.io/v1alpha1", "DaemonSet"), newSubsets(intstr.FromInt32(2), intstr.FromInt32(5)))
			ws.Spec.TargetFilter = &appsv1alpha1.TargetFilter{
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "key",
							Operator: metav1.LabelSelectorOpIn,
						},
					},
				},
			}
			_, err := tester.KC.AppsV1alpha1().WorkloadSpreads(ws.Namespace).Create(context.Background(), ws, metav1.CreateOptions{})
			gomega.Expect(err).To(gomega.HaveOccurred())

			ginkgo.By("custom workload with no replicas: replicas 5, 5 => 2/2, 1/5")
			ws = tester.NewWorkloadSpread(ns, "ws-no-replicas", newTargetReference("no-replicas", "apps.kruise.io/v1alpha1", "DaemonSet"), newSubsets(intstr.FromInt32(2), intstr.FromInt32(5)))
			tester.CreateWorkloadSpread(ws)
			ads := tester.NewBaseDaemonSet("no-replicas", ns)
			tester.CreateDaemonSet(ads)
			gomega.Eventually(checkWorkloadSpread(ws, 2, 0, 1, 4)).WithTimeout(time.Minute).WithPolling(time.Second).Should(gomega.Succeed())

			ginkgo.By("custom workload with replicas path in whitelist (which is MASTER), want replicas 5 and 1 ps + 3 master created (pods not filtered), subset replicas 20%, 80% => 1/1, 3/4")
			ws = tester.NewWorkloadSpread(ns, "ws-replicas-whitelist", newTargetReference("replicas-whitelist", "kubeflow.org/v1", "TFJob"), newSubsets(intstr.FromString("20%"), intstr.FromString("80%")))
			tfjob := tester.NewTFJob("replicas-whitelist", ns, 1, 5, 0)
			tester.CreateWorkloadSpread(ws)
			tester.CreateTFJob(tfjob, 1, 3, 0)
			gomega.Eventually(checkWorkloadSpread(ws, 1, 0, 3, 1)).WithTimeout(time.Minute).WithPolling(time.Second).Should(gomega.Succeed())

			ginkgo.By("custom workload with target filter (which is worker), want replicas 4 and 1 ps + 1 master + 2 worker created (pods filtered), subset replicas 25%, 75% => 1/1, 1/3")
			ws = tester.NewWorkloadSpread(ns, "ws-with-filter", newTargetReference("with-filter", "kubeflow.org/v1", "TFJob"), newSubsets(intstr.FromString("25%"), intstr.FromString("75%")))
			ws.Spec.TargetFilter = &appsv1alpha1.TargetFilter{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"role": "worker",
					},
				},
				ReplicasPathList: []string{
					"spec.tfReplicaSpecs.Worker.replicas",
				},
			}
			tester.CreateWorkloadSpread(ws)
			tfjob = tester.NewTFJob("with-filter", ns, 1, 1, 4)
			tester.CreateTFJob(tfjob, 1, 1, 2)
			gomega.Eventually(checkWorkloadSpread(ws, 1, 0, 1, 2)).WithTimeout(time.Minute).WithPolling(time.Second).Should(gomega.Succeed())
		})

		//test k8s cluster version >= 1.21
		ginkgo.It("elastic deploy for deployment, zone-a=2, zone-b=nil", func() {
			if IsKubernetesVersionLessThan122() {
				ginkgo.Skip("kip this e2e case, it can only run on K8s >= 1.22")
			}
			deployment := tester.NewBaseDeployment(ns)
			// create workloadSpread
			targetRef := appsv1alpha1.TargetReference{
				APIVersion: controllerKindDep.GroupVersion().String(),
				Kind:       controllerKindDep.Kind,
				Name:       deployment.Name,
			}
			subset1 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "zone-a",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-a"},
						},
					},
				},
				MaxReplicas: &intstr.IntOrString{Type: intstr.Int, IntVal: 2},
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-a"}}}`),
				},
			}
			subset2 := appsv1alpha1.WorkloadSpreadSubset{
				Name: "zone-b",
				RequiredNodeSelectorTerm: &corev1.NodeSelectorTerm{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      WorkloadSpreadFakeZoneKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"zone-b"},
						},
					},
				},
				MaxReplicas: nil,
				Patch: runtime.RawExtension{
					Raw: []byte(`{"metadata":{"annotations":{"subset":"zone-b"}}}`),
				},
			}
			workloadSpread := tester.NewWorkloadSpread(ns, workloadSpreadName, &targetRef, []appsv1alpha1.WorkloadSpreadSubset{subset1, subset2})
			workloadSpread = tester.CreateWorkloadSpread(workloadSpread)
			tester.WaitForWorkloadSpreadRunning(workloadSpread)

			// create deployment, replicas = 2
			deployment = tester.CreateDeployment(deployment)
			tester.WaitForDeploymentRunning(deployment)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get deployment(%s/%s) pods, and check workloadSpread(%s/%s) status", deployment.Namespace, deployment.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(2))
			subset1Pods := 0
			subset2Pods := 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(0))

			//scale up deployment.replicas = 6
			ginkgo.By(fmt.Sprintf("scale up deployment(%s/%s) replicas=6", deployment.Namespace, deployment.Name))
			deployment.Spec.Replicas = ptr.To(int32(6))
			tester.UpdateDeployment(deployment)
			tester.WaitForDeploymentRunning(deployment)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get deployment(%s/%s) pods, and check workloadSpread(%s/%s) status", deployment.Namespace, deployment.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(6))
			subset1Pods = 0
			subset2Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(4))

			workloadSpread, err = kc.AppsV1alpha1().WorkloadSpreads(workloadSpread.Namespace).Get(context.TODO(), workloadSpread.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].Name).To(gomega.Equal(workloadSpread.Spec.Subsets[0].Name))
			gomega.Expect(workloadSpread.Status.SubsetStatuses[0].MissingReplicas).To(gomega.Equal(int32(0)))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].CreatingPods)).To(gomega.Equal(0))
			gomega.Expect(len(workloadSpread.Status.SubsetStatuses[0].DeletingPods)).To(gomega.Equal(0))

			// update deployment image
			ginkgo.By(fmt.Sprintf("update deployment(%s/%s) image=%s", deployment.Namespace, deployment.Name, NewWebserverImage))
			deployment.Spec.Template.Spec.Containers[0].Image = NewWebserverImage
			tester.UpdateDeployment(deployment)
			tester.WaitForDeploymentRunning(deployment)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get deployment(%s/%s) pods, and check workloadSpread(%s/%s) status", deployment.Namespace, deployment.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(6))
			subset1Pods = 0
			subset2Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(4))

			//scale down deployment.replicas = 2
			ginkgo.By(fmt.Sprintf("scale down deployment(%s/%s) replicas=2", deployment.Namespace, deployment.Name))
			deployment.Spec.Replicas = ptr.To(int32(2))
			tester.UpdateDeployment(deployment)
			tester.WaitForDeploymentRunning(deployment)

			// get pods, and check workloadSpread
			ginkgo.By(fmt.Sprintf("get deployment(%s/%s) pods, and check workloadSpread(%s/%s) status", deployment.Namespace, deployment.Name, workloadSpread.Namespace, workloadSpread.Name))
			pods, err = tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(2))
			subset1Pods = 0
			subset2Pods = 0
			for _, pod := range pods {
				if str, ok := pod.Annotations[workloadspread.MatchedWorkloadSpreadSubsetAnnotations]; ok {
					var injectWorkloadSpread *workloadspread.InjectWorkloadSpread
					err := json.Unmarshal([]byte(str), &injectWorkloadSpread)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					if injectWorkloadSpread.Subset == subset1.Name {
						subset1Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset1.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("200"))
					} else if injectWorkloadSpread.Subset == subset2.Name {
						subset2Pods++
						gomega.Expect(injectWorkloadSpread.Name).To(gomega.Equal(workloadSpread.Name))
						gomega.Expect(pod.Spec.Affinity).NotTo(gomega.BeNil())
						gomega.Expect(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions).To(gomega.Equal(subset2.RequiredNodeSelectorTerm.MatchExpressions))
						gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal("100"))
					}
				} else {
					// others PodDeletionCostAnnotation not set
					gomega.Expect(pod.Annotations[workloadspread.PodDeletionCostAnnotation]).To(gomega.Equal(""))
				}
			}
			gomega.Expect(subset1Pods).To(gomega.Equal(2))
			gomega.Expect(subset2Pods).To(gomega.Equal(0))

			ginkgo.By("elastic deploy for deployment, zone-a=2, zone-b=nil, done")
		})
	})
})
