/*
Copyright 2019 The Kruise Authors.
Copyright 2014 The Kubernetes Authors.

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
	"reflect"
	"regexp"
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	watchtools "k8s.io/client-go/tools/watch"
	imageutils "k8s.io/kubernetes/test/utils/image"
	"k8s.io/utils/pointer"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"
)

// GCE Quota requirements: 3 pds, one per stateful pod manifest declared above.
// GCE Api requirements: nodes and master need storage r/w permissions.
var _ = SIGDescribe("AppStatefulSetStorage", func() {
	f := framework.NewDefaultFramework("statefulset-storage")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var serverMinorVersion int

	canExpandSC, cannotExpandSC := "allow-volume-expansion", "disallow-volume-expansion"

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		if v, err := c.Discovery().ServerVersion(); err != nil {
			framework.Logf("Failed to discovery server version: %v", err)
		} else {
			if serverMinorVersion, err = strconv.Atoi(v.Minor); err != nil {
				framework.Logf("Failed to convert server version %+v: %v", v, err)
			}
		}
		sc, err := c.StorageV1().StorageClasses().Get(context.TODO(), canExpandSC, metav1.GetOptions{})
		if errors.IsNotFound(err) || sc == nil {
			framework.Failf("no sc %v found", canExpandSC)
		}
	})
	_ = serverMinorVersion

	ginkgo.Describe("Resize PVC", func() {
		oldSize, newSize := "1Gi", "2Gi"
		injectSC := func(podUpdatePolicy appsv1beta1.PodUpdateStrategyType, ss *appsv1beta1.StatefulSet, volumeClaimUpdateStrategy appsv1beta1.VolumeClaimUpdateStrategyType, scNames ...string) {
			if podUpdatePolicy == appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType {
				ss.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{
					PodUpdatePolicy: podUpdatePolicy,
				}
				ss.Spec.Template.Spec.ReadinessGates = append(ss.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
			}

			ss.Spec.VolumeClaimUpdateStrategy = appsv1beta1.VolumeClaimUpdateStrategy{
				Type: volumeClaimUpdateStrategy,
			}
			if len(ss.Spec.VolumeClaimTemplates) != len(scNames) {
				return
			}
			quantity, _ := resource.ParseQuantity(oldSize)
			for i := range scNames {
				ss.Spec.VolumeClaimTemplates[i].Spec.StorageClassName = &scNames[i]
				ss.Spec.VolumeClaimTemplates[i].Spec.Resources.Requests[v1.ResourceStorage] = quantity
			}
		}
		resizeVCT := func(ss *appsv1beta1.StatefulSet, size string, resizeElementSize int) {
			quantity, _ := resource.ParseQuantity(size)
			for i := range ss.Spec.VolumeClaimTemplates {
				if i >= resizeElementSize {
					return
				}
				ss.Spec.VolumeClaimTemplates[i].Spec.Resources.Requests[v1.ResourceStorage] = quantity
			}
		}
		ssName := "ss"
		labels := map[string]string{
			"foo": "bar",
			"baz": "blah",
		}
		headlessSvcName := "test"
		var ss *appsv1beta1.StatefulSet
		var sst *framework.StatefulSetTester

		ginkgo.BeforeEach(func() {
			ginkgo.By("Creating service " + headlessSvcName + " in namespace " + ns)
			headlessService := framework.CreateServiceSpec(headlessSvcName, "", true, labels)
			_, err := c.CoreV1().Services(ns).Create(context.TODO(), headlessService, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			sst = framework.NewStatefulSetTester(c, kc)
		})

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all statefulset in ns %v", ns)
			framework.DeleteAllStatefulSets(c, kc, ns)
		})
		newImage := NewNginxImage
		validateExpandVCT := func(vctNumber int, injectSCFn func(ss *appsv1beta1.StatefulSet), updateFn func(ss *appsv1beta1.StatefulSet), expectErr bool) {
			ctx := context.TODO()
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			vms := []v1.VolumeMount{}
			for i := 0; i < vctNumber; i++ {
				vms = append(vms, v1.VolumeMount{
					Name:      fmt.Sprintf("data%d", i),
					MountPath: fmt.Sprintf("/data%d", i),
				})
			}
			ss = framework.NewStatefulSet(ssName, ns, headlessSvcName, 3, vms, nil, labels)
			injectSCFn(ss)

			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(ctx, ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			sst = framework.NewStatefulSetTester(c, kc)
			waitForStatus(ctx, c, kc, ss)
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)

			ginkgo.By("expand volume claim size")
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, updateFn)
			if expectErr {
				// error is expected
				if err == nil {
					framework.Failf("unexpected to update pvc with sc can not expand, but get error %v", err)
				}
				return
			} else {
				framework.ExpectNoError(err)
			}

			// we need to ensure we wait for all the new ones to show up, not
			// just for any random 3
			waitForStatus(ctx, c, kc, ss)
			waitForPVCCapacity(ctx, c, kc, ss, func(pvc, template resource.Quantity) bool {
				return pvc.Cmp(template) == 0
			})

			ginkgo.By("Confirming 3 pvc capacity consistent with spec")
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)
			sst.WaitForStatusPVCReadyReplicas(ss, 3)
		}

		ginkgo.It("recreate_expand_vct_with_sc_can_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(1, injectFn, updateFn, false)
		})

		ginkgo.It("recreate_expand_vct_with_sc_cannot_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(1, injectFn, updateFn, true)
		})

		ginkgo.It("recreate_expand_vct_with_2sc_can_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, false)
		})

		ginkgo.It("recreate_expand_vct_with_2sc_cannot_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		ginkgo.It("recreate_expand_only_can_expand_vct_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 1)
			}
			validateExpandVCT(2, injectFn, updateFn, false)
		})

		ginkgo.It("recreate_expand_only_cannot_expand_vct_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 1)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		ginkgo.It("recreate_expand_both_cannot_expand_vct_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		ginkgo.It("inplace_expand_vct_with_sc_can_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(1, injectFn, updateFn, false)
		})

		ginkgo.It("inplace_expand_vct_with_sc_cannot_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(1, injectFn, updateFn, true)
		})

		ginkgo.It("inplace_expand_vct_with_2sc_can_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, false)
		})

		ginkgo.It("inplace_expand_vct_with_2sc_cannot_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		ginkgo.It("inplace_expand_only_can_expand_vct_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 1)
			}
			validateExpandVCT(2, injectFn, updateFn, false)
		})

		ginkgo.It("inplace_expand_only_cannot_expand_vct_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 1)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		ginkgo.It("inplace_expand_both_cannot_expand_vct_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		framework.ConformanceIt("should perform rolling updates with specified-deleted with pvc", func() {
			ginkgo.By("Creating a new StatefulSet")
			vms := []v1.VolumeMount{}
			for i := 0; i < 1; i++ {
				vms = append(vms, v1.VolumeMount{
					Name:      fmt.Sprintf("data%d", i),
					MountPath: fmt.Sprintf("/data%d", i),
				})
			}
			ss = framework.NewStatefulSet(ssName, ns, headlessSvcName, 4, vms, nil, labels)
			injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC)

			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse("20")
				update.Spec.VolumeClaimUpdateStrategy = appsv1beta1.VolumeClaimUpdateStrategy{
					Type: appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType,
				}
				resizeVCT(update, newSize, 2)
			}
			testWithSpecifiedDeleted(c, kc, ns, ss, updateFn)
			notEqualPVCNum := 0
			sst := framework.NewStatefulSetTester(c, kc)
			ss = sst.WaitForStatus(ss)
			sst.WaitForStatusPVCReadyReplicas(ss, 2)

			ss = sst.WaitForStatus(ss)
			waitForPVCCapacity(context.TODO(), c, kc, ss, func(pvc, template resource.Quantity) bool {
				if pvc.Cmp(template) != 0 {
					notEqualPVCNum++
				}
				return true
			})
			gomega.Expect(2).To(gomega.Equal(notEqualPVCNum))
		})
	})

	ginkgo.Describe("Resize PVC only", func() {
		oldSize, newSize := "1Gi", "2Gi"
		injectSC := func(podUpdatePolicy appsv1beta1.PodUpdateStrategyType, ss *appsv1beta1.StatefulSet, volumeClaimUpdateStrategy appsv1beta1.VolumeClaimUpdateStrategyType, scNames ...string) {
			if podUpdatePolicy == appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType {
				ss.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{
					PodUpdatePolicy: podUpdatePolicy,
				}
				ss.Spec.Template.Spec.ReadinessGates = append(ss.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
			}

			ss.Spec.VolumeClaimUpdateStrategy = appsv1beta1.VolumeClaimUpdateStrategy{
				Type: volumeClaimUpdateStrategy,
			}
			if len(ss.Spec.VolumeClaimTemplates) != len(scNames) {
				return
			}
			quantity, _ := resource.ParseQuantity(oldSize)
			for i := range scNames {
				ss.Spec.VolumeClaimTemplates[i].Spec.StorageClassName = &scNames[i]
				ss.Spec.VolumeClaimTemplates[i].Spec.Resources.Requests[v1.ResourceStorage] = quantity
			}
		}
		resizeVCT := func(ss *appsv1beta1.StatefulSet, size string, resizeElementSize int) {
			quantity, _ := resource.ParseQuantity(size)
			for i := range ss.Spec.VolumeClaimTemplates {
				if i >= resizeElementSize {
					return
				}
				ss.Spec.VolumeClaimTemplates[i].Spec.Resources.Requests[v1.ResourceStorage] = quantity
			}
		}
		ssName := "ss"
		labels := map[string]string{
			"foo": "bar",
			"baz": "blah",
		}
		headlessSvcName := "test"
		var ss *appsv1beta1.StatefulSet
		var sst *framework.StatefulSetTester

		ginkgo.BeforeEach(func() {
			ginkgo.By("Creating service " + headlessSvcName + " in namespace " + ns)
			headlessService := framework.CreateServiceSpec(headlessSvcName, "", true, labels)
			_, err := c.CoreV1().Services(ns).Create(context.TODO(), headlessService, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			sst = framework.NewStatefulSetTester(c, kc)
		})

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all statefulset in ns %v", ns)
			framework.DeleteAllStatefulSets(c, kc, ns)
		})
		validateExpandVCT := func(vctNumber int, injectSCFn func(ss *appsv1beta1.StatefulSet), updateFn func(ss *appsv1beta1.StatefulSet), expectErr bool) {
			ctx := context.TODO()
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			vms := []v1.VolumeMount{}
			for i := 0; i < vctNumber; i++ {
				vms = append(vms, v1.VolumeMount{
					Name:      fmt.Sprintf("data%d", i),
					MountPath: fmt.Sprintf("/data%d", i),
				})
			}
			ss = framework.NewStatefulSet(ssName, ns, headlessSvcName, 3, vms, nil, labels)
			injectSCFn(ss)

			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(ctx, ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			sst = framework.NewStatefulSetTester(c, kc)
			waitForStatus(ctx, c, kc, ss)
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)

			ginkgo.By("expand volume claim size")
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, updateFn)
			if expectErr {
				// error is expected
				if err == nil {
					framework.Failf("unexpected to update pvc with sc can not expand, but get error %v", err)
				}
				return
			} else {
				framework.ExpectNoError(err)
			}

			// we need to ensure we wait for all the new ones to show up, not
			// just for any random 3
			waitForStatus(ctx, c, kc, ss)
			waitForPVCCapacity(ctx, c, kc, ss, func(pvc, template resource.Quantity) bool {
				return pvc.Cmp(template) == 0
			})

			ginkgo.By("Confirming 3 pvc capacity consistent with spec")
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)
			sst.WaitForStatusPVCReadyReplicas(ss, 3)
		}

		ginkgo.It("recreate_with_sc_can_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(1, injectFn, updateFn, false)
		})

		ginkgo.It("recreate_with_sc_cannot_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(1, injectFn, updateFn, true)
		})

		ginkgo.It("recreate_with_2sc_can_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, false)
		})

		ginkgo.It("recreate_with_2sc_cannot_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		ginkgo.It("recreate_expand_only_can_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 1)
			}
			validateExpandVCT(2, injectFn, updateFn, false)
		})

		ginkgo.It("recreate_expand_only_cannot_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 1)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		ginkgo.It("recreate_expand_both_cannot_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		ginkgo.It("inplace_with_sc_can_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(1, injectFn, updateFn, false)
		})

		ginkgo.It("inplace_with_sc_cannot_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(1, injectFn, updateFn, true)
		})

		ginkgo.It("inplace_with_2sc_can_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, false)
		})

		ginkgo.It("inplace_with_2sc_cannot_expand", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		ginkgo.It("inplace_expand_only_can_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC, cannotExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 1)
			}
			validateExpandVCT(2, injectFn, updateFn, false)
		})

		ginkgo.It("inplace_expand_only_cannot_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 1)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})

		ginkgo.It("inplace_expand_both_cannot_with_mixed_sc", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, cannotExpandSC, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				resizeVCT(update, newSize, 2)
			}
			validateExpandVCT(2, injectFn, updateFn, true)
		})
	})

	ginkgo.Describe("Resize PVC with rollback", func() {
		oldSize, newSize := "1Gi", "2Gi"
		injectSC := func(podUpdatePolicy appsv1beta1.PodUpdateStrategyType, ss *appsv1beta1.StatefulSet, volumeClaimUpdateStrategy appsv1beta1.VolumeClaimUpdateStrategyType, scNames ...string) {
			if podUpdatePolicy == appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType {
				ss.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{
					PodUpdatePolicy: podUpdatePolicy,
				}
				ss.Spec.Template.Spec.ReadinessGates = append(ss.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
			}

			ss.Spec.VolumeClaimUpdateStrategy = appsv1beta1.VolumeClaimUpdateStrategy{
				Type: volumeClaimUpdateStrategy,
			}
			if len(ss.Spec.VolumeClaimTemplates) != len(scNames) {
				return
			}
			quantity, _ := resource.ParseQuantity(oldSize)
			for i := range scNames {
				ss.Spec.VolumeClaimTemplates[i].Spec.StorageClassName = &scNames[i]
				ss.Spec.VolumeClaimTemplates[i].Spec.Resources.Requests[v1.ResourceStorage] = quantity
			}
		}
		// resize only first resizeElementSize elements
		resizeVCT := func(ss *appsv1beta1.StatefulSet, size string, resizeElementSize int) {
			quantity, _ := resource.ParseQuantity(size)
			for i := range ss.Spec.VolumeClaimTemplates {
				if i >= resizeElementSize {
					return
				}
				ss.Spec.VolumeClaimTemplates[i].Spec.Resources.Requests[v1.ResourceStorage] = quantity
			}
		}
		ssName := "ss"
		labels := map[string]string{
			"foo": "bar",
			"baz": "blah",
		}
		headlessSvcName := "test"
		var ss *appsv1beta1.StatefulSet
		var sst *framework.StatefulSetTester

		ginkgo.BeforeEach(func() {
			ginkgo.By("Creating service " + headlessSvcName + " in namespace " + ns)
			headlessService := framework.CreateServiceSpec(headlessSvcName, "", true, labels)
			_, err := c.CoreV1().Services(ns).Create(context.TODO(), headlessService, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			sst = framework.NewStatefulSetTester(c, kc)
		})

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all statefulset in ns %v", ns)
			framework.DeleteAllStatefulSets(c, kc, ns)
		})
		oldImage := NginxImage
		newImage := NewNginxImage
		validateUpdateVCTAndRollback := func(vctNumber int, injectSCFn func(ss *appsv1beta1.StatefulSet), updateFn, rollbackFn func(ss *appsv1beta1.StatefulSet)) {
			ctx := context.TODO()
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			vms := []v1.VolumeMount{}
			for i := 0; i < vctNumber; i++ {
				vms = append(vms, v1.VolumeMount{
					Name:      fmt.Sprintf("data%d", i),
					MountPath: fmt.Sprintf("/data%d", i),
				})
			}
			ss = framework.NewStatefulSet(ssName, ns, headlessSvcName, 3, vms, nil, labels)
			injectSCFn(ss)

			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(ctx, ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			sst = framework.NewStatefulSetTester(c, kc)
			waitForStatus(ctx, c, kc, ss)
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)

			ginkgo.By("expand volume claim size")
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, updateFn)
			framework.ExpectNoError(err)

			sst.WaitForStatusReadyReplicas(ss, 3)
			sst.WaitForStatusPVCReadyReplicas(ss, 1)

			var pods *v1.PodList
			ss, pods = sst.WaitForPartitionedRollingUpdate(ss)
			for i := range pods.Items {
				if i < int(*ss.Spec.UpdateStrategy.RollingUpdate.Partition) {
					gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
						fmt.Sprintf("Pod %s/%s has image %s not equal to current image %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Spec.Containers[0].Image,
							oldImage))
					gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(ss.Status.CurrentRevision),
						fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
							ss.Status.CurrentRevision))
				} else {
					gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
						fmt.Sprintf("Pod %s/%s has image %s not equal to new image  %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Spec.Containers[0].Image,
							newImage))
					gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(ss.Status.UpdateRevision),
						fmt.Sprintf("Pod %s/%s has revision %s not equal to new revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
							ss.Status.UpdateRevision))
				}
			}

			ginkgo.By("run rollback fn")
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, rollbackFn)
			framework.ExpectNoError(err)

			sst.WaitForStatusReadyReplicas(ss, 3)

			// we need to ensure we wait for all the new ones to show up, not
			// just for any random 3
			waitForStatus(ctx, c, kc, ss)

			ginkgo.By("Confirming 3 pvc capacity consistent with spec")
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)
		}

		// support reconcile when vct resize only
		ginkgo.It("partition 2, rollback only image", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
				partition := int32(2)
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			}
			rollbackFn := func(update *appsv1beta1.StatefulSet) {
				ginkgo.By("rollback only image, remain new pvc size")
				update.Spec.Template.Spec.Containers[0].Image = oldImage
				partition := int32(0)
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			}
			validateUpdateVCTAndRollback(1, injectFn, updateFn, rollbackFn)
			ctx := context.TODO()
			waitForPVCCapacity(ctx, c, kc, ss, func(pvc, template resource.Quantity) bool {
				return pvc.Cmp(template) == 0
			})

			ginkgo.By("Confirming 3 pvc capacity consistent with spec")
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)
			sst.WaitForStatusPVCReadyReplicas(ss, 3)

			// update to another version
			ginkgo.By("After rollbacked, continue to expand pvc size")
			var err error
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, func(set *appsv1beta1.StatefulSet) {
				resizeVCT(set, "3Gi", 2)
			})
			framework.ExpectNoError(err)

			sst.WaitForStatusReadyReplicas(ss, 3)
			waitForStatus(ctx, c, kc, ss)
			waitForPVCCapacity(ctx, c, kc, ss, func(pvc, template resource.Quantity) bool {
				return pvc.Cmp(template) == 0
			})

			ginkgo.By("Confirming 3 pvc capacity consistent with spec")
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)
			sst.WaitForStatusPVCReadyReplicas(ss, 3)
		})

		ginkgo.It("partition 2, rollback only image and update to another image", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
				partition := int32(2)
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			}
			rollbackFn := func(update *appsv1beta1.StatefulSet) {
				ginkgo.By("rollback only image, remain new pvc size")
				update.Spec.Template.Spec.Containers[0].Image = oldImage
				partition := int32(0)
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			}
			validateUpdateVCTAndRollback(1, injectFn, updateFn, rollbackFn)

			ginkgo.By("After rollbacked, update a new image")
			// update to another version
			ctx := context.TODO()
			var err error
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, func(set *appsv1beta1.StatefulSet) {
				set.Spec.Template.Spec.Containers[0].Image = WebserverImage
			})
			framework.ExpectNoError(err)

			sst.WaitForStatusReadyReplicas(ss, 3)
			waitForStatus(ctx, c, kc, ss)
			waitForPVCCapacity(ctx, c, kc, ss, func(pvc, template resource.Quantity) bool {
				return pvc.Cmp(template) == 0
			})

			ginkgo.By("Confirming 3 pvc capacity consistent with spec")
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)
			sst.WaitForStatusPVCReadyReplicas(ss, 3)
		})

		ginkgo.It("partition 2, rollback image and pvc size", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
				partition := int32(2)
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			}
			rollbackFn := func(update *appsv1beta1.StatefulSet) {
				ginkgo.By("rollback image and pvc size")
				update.Spec.Template.Spec.Containers[0].Image = oldImage
				resizeVCT(update, oldSize, 2)
				partition := int32(0)
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			}
			validateUpdateVCTAndRollback(1, injectFn, updateFn, rollbackFn)
			ctx := context.TODO()
			waitForPVCCapacity(ctx, c, kc, ss, func(pvc, template resource.Quantity) bool {
				// only 2 size is newSize which is compatible with oldSize
				return pvc.Cmp(template) >= 0
			})
			sst.WaitForStatusPVCReadyReplicas(ss, 3)

			// update to another version
			ginkgo.By("After rollbacked, update a new image")
			var err error
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, func(set *appsv1beta1.StatefulSet) {
				set.Spec.Template.Spec.Containers[0].Image = WebserverImage
			})
			framework.ExpectNoError(err)

			sst.WaitForStatusReadyReplicas(ss, 3)
			waitForStatus(ctx, c, kc, ss)
			waitForPVCCapacity(ctx, c, kc, ss, func(pvc, template resource.Quantity) bool {
				// only 2 size is newSize which is compatible with oldSize
				return pvc.Cmp(template) >= 0
			})

			ginkgo.By("Confirming 3 pvc capacity consistent with spec")
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)
			sst.WaitForStatusPVCReadyReplicas(ss, 3)
		})

		ginkgo.It("partition 2, rollback image and change to OnPVCDelete update strategy", func() {
			injectFn := func(ss *appsv1beta1.StatefulSet) {
				injectSC(appsv1beta1.RecreatePodUpdateStrategyType, ss, appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType, canExpandSC)
			}
			updateFn := func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				resizeVCT(update, newSize, 2)
				partition := int32(2)
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			}
			rollbackFn := func(update *appsv1beta1.StatefulSet) {
				ginkgo.By("rollback image and use OnPVCDelete update strategy")
				update.Spec.Template.Spec.Containers[0].Image = oldImage
				partition := int32(0)
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
				update.Spec.VolumeClaimUpdateStrategy.Type = appsv1beta1.OnPVCDeleteVolumeClaimUpdateStrategyType
			}
			validateUpdateVCTAndRollback(1, injectFn, updateFn, rollbackFn)
			sst.WaitForStatusPVCReadyReplicas(ss, 1)

			ctx := context.TODO()
			// update to another version
			ginkgo.By("After rollbacked, update a new image")
			var err error
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, func(set *appsv1beta1.StatefulSet) {
				set.Spec.Template.Spec.Containers[0].Image = WebserverImage
			})
			framework.ExpectNoError(err)

			sst.WaitForStatusReadyReplicas(ss, 3)
			waitForStatus(ctx, c, kc, ss)

			ginkgo.By("Confirming 3 pvc capacity consistent with spec")
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)
			sst.WaitForStatusPVCReadyReplicas(ss, 1)
		})
	})
})

// GCE Quota requirements: 3 pds, one per stateful pod manifest declared above.
// GCE Api requirements: nodes and master need storage r/w permissions.
var _ = SIGDescribe("StatefulSet", func() {
	f := framework.NewDefaultFramework("statefulset")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var serverMinorVersion int

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		if v, err := c.Discovery().ServerVersion(); err != nil {
			framework.Logf("Failed to discovery server version: %v", err)
		} else {
			if serverMinorVersion, err = strconv.Atoi(v.Minor); err != nil {
				framework.Logf("Failed to convert server version %+v: %v", v, err)
			}
		}
	})

	framework.KruiseDescribe("Basic StatefulSet functionality [StatefulSetBasic]", func() {
		ssName := "ss"
		labels := map[string]string{
			"foo": "bar",
			"baz": "blah",
		}
		headlessSvcName := "test"
		var statefulPodMounts, podMounts []v1.VolumeMount
		var ss *appsv1beta1.StatefulSet

		ginkgo.BeforeEach(func() {
			statefulPodMounts = []v1.VolumeMount{{Name: "datadir", MountPath: "/data/"}}
			podMounts = []v1.VolumeMount{{Name: "home", MountPath: "/home"}}
			ss = framework.NewStatefulSet(ssName, ns, headlessSvcName, 2, statefulPodMounts, podMounts, labels)

			ginkgo.By("Creating service " + headlessSvcName + " in namespace " + ns)
			headlessService := framework.CreateServiceSpec(headlessSvcName, "", true, labels)
			_, err := c.CoreV1().Services(ns).Create(context.TODO(), headlessService, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all statefulset in ns %v", ns)
			framework.DeleteAllStatefulSets(c, kc, ns)
		})

		// This can't be Conformance yet because it depends on a default
		// StorageClass and a dynamic provisioner.
		ginkgo.It("should provide basic identity", func() {
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			sst := framework.NewStatefulSetTester(c, kc)
			sst.PauseNewPods(ss)

			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Saturating stateful set " + ss.Name)
			sst.Saturate(ss)

			ginkgo.By("Verifying statefulset mounted data directory is usable")
			framework.ExpectNoError(sst.CheckMount(ss, "/data"))

			ginkgo.By("Verifying statefulset provides a stable hostname for each pod")
			framework.ExpectNoError(sst.CheckHostname(ss))

			ginkgo.By("Verifying statefulset set proper service name")
			framework.ExpectNoError(sst.CheckServiceName(ss, headlessSvcName))

			cmd := "echo $(hostname) | dd of=/data/hostname conv=fsync"
			ginkgo.By("Running " + cmd + " in all stateful pods")
			framework.ExpectNoError(sst.ExecInStatefulPods(ss, cmd))

			ginkgo.By("Restarting statefulset " + ss.Name)
			sst.Restart(ss)
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)

			ginkgo.By("Verifying statefulset mounted data directory is usable")
			framework.ExpectNoError(sst.CheckMount(ss, "/data"))

			cmd = "if [ \"$(cat /data/hostname)\" = \"$(hostname)\" ]; then exit 0; else exit 1; fi"
			ginkgo.By("Running " + cmd + " in all stateful pods")
			framework.ExpectNoError(sst.ExecInStatefulPods(ss, cmd))
		})

		// This can't be Conformance yet because it depends on a default
		// StorageClass and a dynamic provisioner.
		ginkgo.It("should adopt matching orphans and release non-matching pods", func() {
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 1
			sst := framework.NewStatefulSetTester(c, kc)
			sst.PauseNewPods(ss)

			// Replace ss with the one returned from Create() so it has the UID.
			// Save Kind since it won't be populated in the returned ss.
			kind := ss.Kind
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ss.Kind = kind

			ginkgo.By("Saturating stateful set " + ss.Name)
			sst.Saturate(ss)
			pods := sst.GetPodList(ss)
			gomega.Expect(pods.Items).To(gomega.HaveLen(int(*ss.Spec.Replicas)))

			ginkgo.By("Checking that stateful set pods are created with ControllerRef")
			pod := pods.Items[0]
			controllerRef := metav1.GetControllerOf(&pod)
			gomega.Expect(controllerRef).ToNot(gomega.BeNil())
			gomega.Expect(controllerRef.Kind).To(gomega.Equal(ss.Kind))
			gomega.Expect(controllerRef.Name).To(gomega.Equal(ss.Name))
			gomega.Expect(controllerRef.UID).To(gomega.Equal(ss.UID))

			ginkgo.By("Orphaning one of the stateful set's pods")
			f.PodClient().Update(pod.Name, func(pod *v1.Pod) {
				pod.OwnerReferences = nil
			})

			ginkgo.By("Checking that the stateful set readopts the pod")
			gomega.Expect(framework.WaitForPodCondition(c, pod.Namespace, pod.Name, "adopted", framework.StatefulSetTimeout,
				func(pod *v1.Pod) (bool, error) {
					controllerRef := metav1.GetControllerOf(pod)
					if controllerRef == nil {
						return false, nil
					}
					if controllerRef.Kind != ss.Kind || controllerRef.Name != ss.Name || controllerRef.UID != ss.UID {
						return false, fmt.Errorf("pod has wrong controllerRef: %v", controllerRef)
					}
					return true, nil
				},
			)).To(gomega.Succeed(), "wait for pod %q to be readopted", pod.Name)

			ginkgo.By("Removing the labels from one of the stateful set's pods")
			prevLabels := pod.Labels
			f.PodClient().Update(pod.Name, func(pod *v1.Pod) {
				pod.Labels = nil
			})

			ginkgo.By("Checking that the stateful set releases the pod")
			gomega.Expect(framework.WaitForPodCondition(c, pod.Namespace, pod.Name, "released", framework.StatefulSetTimeout,
				func(pod *v1.Pod) (bool, error) {
					controllerRef := metav1.GetControllerOf(pod)
					if controllerRef != nil {
						return false, nil
					}
					return true, nil
				},
			)).To(gomega.Succeed(), "wait for pod %q to be released", pod.Name)

			// If we don't do this, the test leaks the Pod and PVC.
			ginkgo.By("Readding labels to the stateful set's pod")
			f.PodClient().Update(pod.Name, func(pod *v1.Pod) {
				pod.Labels = prevLabels
			})

			ginkgo.By("Checking that the stateful set readopts the pod")
			gomega.Expect(framework.WaitForPodCondition(c, pod.Namespace, pod.Name, "adopted", framework.StatefulSetTimeout,
				func(pod *v1.Pod) (bool, error) {
					controllerRef := metav1.GetControllerOf(pod)
					if controllerRef == nil {
						return false, nil
					}
					if controllerRef.Kind != ss.Kind || controllerRef.Name != ss.Name || controllerRef.UID != ss.UID {
						return false, fmt.Errorf("pod has wrong controllerRef: %v", controllerRef)
					}
					return true, nil
				},
			)).To(gomega.Succeed(), "wait for pod %q to be readopted", pod.Name)
		})

		// This can't be Conformance yet because it depends on a default
		// StorageClass and a dynamic provisioner.
		ginkgo.It("should not deadlock when a pod's predecessor fails", func() {
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 2
			sst := framework.NewStatefulSetTester(c, kc)
			sst.PauseNewPods(ss)

			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			time.Sleep(time.Minute)
			sst.WaitForRunning(1, 0, ss)

			ginkgo.By("Resuming stateful pod at index 0.")
			sst.ResumeNextPod(ss)

			ginkgo.By("Waiting for stateful pod at index 1 to enter running.")
			sst.WaitForRunning(2, 1, ss)

			// Now we have 1 healthy and 1 unhealthy stateful pod. Deleting the healthy stateful pod should *not*
			// create a new stateful pod till the remaining stateful pod becomes healthy, which won't happen till
			// we set the healthy bit.

			ginkgo.By("Deleting healthy stateful pod at index 0.")
			sst.DeleteStatefulPodAtIndex(0, ss)

			ginkgo.By("Confirming stateful pod at index 0 is recreated.")
			sst.WaitForRunning(2, 1, ss)

			ginkgo.By("Resuming stateful pod at index 1.")
			sst.ResumeNextPod(ss)

			ginkgo.By("Confirming all stateful pods in statefulset are created.")
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
		})

		// This can't be Conformance yet because it depends on a default
		// StorageClass and a dynamic provisioner.
		ginkgo.It("should perform rolling updates and roll backs of template modifications with PVCs", func() {
			ginkgo.By("Creating a new StatefulSet with PVCs")
			*(ss.Spec.Replicas) = 3
			rollbackTest(c, kc, ns, ss)
		})

		/*
			Release : v1.9
			Testname: StatefulSet, Rolling Update
			Description: StatefulSet MUST support the RollingUpdate strategy to automatically replace Pods one at a time when the Pod template changes. The StatefulSet's status MUST indicate the CurrentRevision and UpdateRevision. If the template is changed to match a prior revision, StatefulSet MUST detect this as a rollback instead of creating a new revision. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("should perform rolling updates and roll backs of template modifications", func() {
			ginkgo.By("Creating a new StatefulSet")
			ss := framework.NewStatefulSet("ss2", ns, headlessSvcName, 3, nil, nil, labels)
			rollbackTest(c, kc, ns, ss)
		})

		/*
			Release : v1.9
			Testname: StatefulSet, Rolling Update with Partition
			Description: StatefulSet's RollingUpdate strategy MUST support the Partition parameter for canaries and phased rollouts. If a Pod is deleted while a rolling update is in progress, StatefulSet MUST restore the Pod without violating the Partition. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("should perform canary updates and phased rolling updates of template modifications", func() {
			ginkgo.By("Creating a new StaefulSet")
			ss := framework.NewStatefulSet("ss2", ns, headlessSvcName, 3, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
					return &appsv1beta1.RollingUpdateStatefulSetStrategy{
						Partition: func() *int32 {
							i := int32(3)
							return &i
						}()}
				}(),
			}
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
					ss.Namespace, ss.Name, updateRevision, currentRevision))
			pods := sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to currentRevision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}
			newImage := NewNginxImage
			oldImage := ss.Spec.Template.Spec.Containers[0].Image

			ginkgo.By(fmt.Sprintf("Updating stateful set template: update image from %s to %s", oldImage, newImage))
			gomega.Expect(oldImage).NotTo(gomega.Equal(newImage), "Incorrect test setup: should update to a different image")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating a new revision")
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
				"Current revision should not equal update revision during rolling update")

			ginkgo.By("Not applying an update when the partition is greater than the number of replicas")
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
					fmt.Sprintf("Pod %s/%s has image %s not equal to current image %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Spec.Containers[0].Image,
						oldImage))
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Performing a canary update")
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
					return &appsv1beta1.RollingUpdateStatefulSetStrategy{
						Partition: func() *int32 {
							i := int32(2)
							return &i
						}()}
				}(),
			}
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
					Type: apps.RollingUpdateStatefulSetStrategyType,
					RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
						return &appsv1beta1.RollingUpdateStatefulSetStrategy{
							Partition: func() *int32 {
								i := int32(2)
								return &i
							}()}
					}(),
				}
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ss, pods = sst.WaitForPartitionedRollingUpdate(ss)
			for i := range pods.Items {
				if i < int(*ss.Spec.UpdateStrategy.RollingUpdate.Partition) {
					gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
						fmt.Sprintf("Pod %s/%s has image %s not equal to current image %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Spec.Containers[0].Image,
							oldImage))
					gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
						fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
							currentRevision))
				} else {
					gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
						fmt.Sprintf("Pod %s/%s has image %s not equal to new image  %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Spec.Containers[0].Image,
							newImage))
					gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
						fmt.Sprintf("Pod %s/%s has revision %s not equal to new revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
							updateRevision))
				}
			}

			ginkgo.By("Restoring Pods to the correct revision when they are deleted")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				if i < int(*ss.Spec.UpdateStrategy.RollingUpdate.Partition) {
					gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
						fmt.Sprintf("Pod %s/%s has image %s not equal to current image %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Spec.Containers[0].Image,
							oldImage))
					gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
						fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
							currentRevision))
				} else {
					gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
						fmt.Sprintf("Pod %s/%s has image %s not equal to new image  %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Spec.Containers[0].Image,
							newImage))
					gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
						fmt.Sprintf("Pod %s/%s has revision %s not equal to new revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
							updateRevision))
				}
			}

			ginkgo.By("Performing a phased rolling update")
			for i := int(*ss.Spec.UpdateStrategy.RollingUpdate.Partition) - 1; i >= 0; i-- {
				ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
					update.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
						Type: apps.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: func() *appsv1beta1.RollingUpdateStatefulSetStrategy {
							j := int32(i)
							return &appsv1beta1.RollingUpdateStatefulSetStrategy{
								Partition: &j,
							}
						}(),
					}
				})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				ss, pods = sst.WaitForPartitionedRollingUpdate(ss)
				for i := range pods.Items {
					if i < int(*ss.Spec.UpdateStrategy.RollingUpdate.Partition) {
						gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
							fmt.Sprintf("Pod %s/%s has image %s not equal to current image %s",
								pods.Items[i].Namespace,
								pods.Items[i].Name,
								pods.Items[i].Spec.Containers[0].Image,
								oldImage))
						gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
							fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
								pods.Items[i].Namespace,
								pods.Items[i].Name,
								pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
								currentRevision))
					} else {
						gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
							fmt.Sprintf("Pod %s/%s has image %s not equal to new image  %s",
								pods.Items[i].Namespace,
								pods.Items[i].Name,
								pods.Items[i].Spec.Containers[0].Image,
								newImage))
						gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
							fmt.Sprintf("Pod %s/%s has revision %s not equal to new revision %s",
								pods.Items[i].Namespace,
								pods.Items[i].Name,
								pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
								updateRevision))
					}
				}
			}
			gomega.Expect(ss.Status.CurrentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s current revision %s does not equal update revision %s on update completion",
					ss.Namespace,
					ss.Name,
					ss.Status.CurrentRevision,
					updateRevision))

		})

		// Do not mark this as Conformance.
		// The legacy OnDelete strategy only exists for backward compatibility with pre-v1 APIs.
		ginkgo.It("should implement legacy replacement when the update strategy is OnDelete", func() {
			ginkgo.By("Creating a new StatefulSet")
			ss := framework.NewStatefulSet("ss2", ns, headlessSvcName, 3, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.OnDeleteStatefulSetStrategyType,
			}
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
					ss.Namespace, ss.Name, updateRevision, currentRevision))
			pods := sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Restoring Pods to the current revision")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(1, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}
			newImage := NewNginxImage
			oldImage := ss.Spec.Template.Spec.Containers[0].Image

			ginkgo.By(fmt.Sprintf("Updating stateful set template: update image from %s to %s", oldImage, newImage))
			gomega.Expect(oldImage).NotTo(gomega.Equal(newImage), "Incorrect test setup: should update to a different image")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating a new revision")
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
				"Current revision should not equal update revision during rolling update")

			ginkgo.By("Recreating Pods at the new revision")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(1, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
					fmt.Sprintf("Pod %s/%s has image %s not equal to new image %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Spec.Containers[0].Image,
						newImage))
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
					fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						updateRevision))
			}
		})

		/*
			Testname: AdvancedStatefulSet, InPlaceUpdate
			Description: StatefulSet MUST in-place update pods for pod inplace update strategy. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("should in-place update pods when the pod update strategy is InPlace", func() {
			ginkgo.By("Creating a new StatefulSet")
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 3, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
					PodUpdatePolicy:       appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
					InPlaceUpdateStrategy: &appspub.InPlaceUpdateStrategy{GracePeriodSeconds: 10},
				},
			}
			ss.Spec.Template.Spec.ReadinessGates = append(ss.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
					ss.Namespace, ss.Name, updateRevision, currentRevision))
			pods := sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Restoring Pods to the current revision")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(1, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}
			newImage := NewNginxImage
			oldImage := ss.Spec.Template.Spec.Containers[0].Image

			ginkgo.By(fmt.Sprintf("Updating stateful set template: update image from %s to %s", oldImage, newImage))
			gomega.Expect(oldImage).NotTo(gomega.Equal(newImage), "Incorrect test setup: should update to a different image")
			var partition int32 = 3
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Image = newImage
				if update.Spec.UpdateStrategy.RollingUpdate == nil {
					update.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{}
				}
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating a new revision")
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
				"Current revision should not equal update revision during rolling update")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				partition = 0
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("InPlace update Pods at the new revision")
			sst.WaitForPodUpdatedAndRunning(ss, pods.Items[0].Name, currentRevision)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
					fmt.Sprintf("Pod %s/%s has image %s not equal to new image %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Spec.Containers[0].Image,
						newImage))
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
					fmt.Sprintf("Pod %s/%s has revision %s not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						updateRevision))
			}
		})

		framework.ConformanceIt("should in-place update env from label", func() {
			ginkgo.By("Creating a new StatefulSet")
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 3, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
					PodUpdatePolicy:       appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
					InPlaceUpdateStrategy: &appspub.InPlaceUpdateStrategy{GracePeriodSeconds: 10},
				},
			}
			ss.Spec.Template.ObjectMeta.Labels = map[string]string{"test-env": "foo"}
			for k, v := range labels {
				ss.Spec.Template.ObjectMeta.Labels[k] = v
			}
			ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
				Name:      "TEST_ENV",
				ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['test-env']"}},
			})
			ss.Spec.Template.Spec.ReadinessGates = append(ss.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
					ss.Namespace, ss.Name, updateRevision, currentRevision))
			pods := sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Restoring Pods to the current revision")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(1, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Updating stateful set template: update label for env")
			var partition int32 = 3
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.ObjectMeta.Labels["test-env"] = "bar"
				if update.Spec.UpdateStrategy.RollingUpdate == nil {
					update.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{}
				}
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating a new revision")
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
				"Current revision should not equal update revision during rolling update")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				partition = 0
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("InPlace update Pods at the new revision")
			sst.WaitForPodUpdatedAndRunning(ss, pods.Items[0].Name, currentRevision)
			sst.WaitForRunningAndReady(3, ss)

			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Status.ContainerStatuses[0].RestartCount).To(gomega.Equal(int32(1)))
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision))
			}
		})

		framework.ConformanceIt("should recreate update when pod qos changed", func() {
			ginkgo.By("Creating a new StatefulSet")
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 3, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss.Spec.UpdateStrategy = appsv1beta1.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1beta1.RollingUpdateStatefulSetStrategy{
					PodUpdatePolicy:       appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
					InPlaceUpdateStrategy: &appspub.InPlaceUpdateStrategy{GracePeriodSeconds: 10},
				},
			}
			ss.Spec.Template.ObjectMeta.Labels = map[string]string{"test-env": "foo"}
			for k, v := range labels {
				ss.Spec.Template.ObjectMeta.Labels[k] = v
			}
			ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
				Name:      "TEST_ENV",
				ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.labels['test-env']"}},
			})
			ss.Spec.Template.Spec.ReadinessGates = append(ss.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
				fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
					ss.Namespace, ss.Name, updateRevision, currentRevision))
			pods := sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			ginkgo.By("Restoring Pods to the current revision")
			sst.DeleteStatefulPodAtIndex(0, ss)
			sst.DeleteStatefulPodAtIndex(1, ss)
			sst.DeleteStatefulPodAtIndex(2, ss)
			sst.WaitForRunningAndReady(3, ss)
			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
					fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
						pods.Items[i].Namespace,
						pods.Items[i].Name,
						pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
						currentRevision))
			}

			updateTime := time.Now()
			ginkgo.By("Updating stateful set template: change pod qos")
			var partition int32 = 3
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Limits: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("1Gi"),
					},
				}
				if update.Spec.UpdateStrategy.RollingUpdate == nil {
					update.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{}
				}
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating a new revision")
			ss = sst.WaitForStatus(ss)
			currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
			gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
				"Current revision should not equal update revision during rolling update")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				partition = 0
				update.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("recreate update Pods at the new revision")
			sst.WaitForPodUpdatedAndRunning(ss, pods.Items[0].Name, currentRevision)
			sst.WaitForRunningAndReady(3, ss)

			ss = sst.GetStatefulSet(ss.Namespace, ss.Name)
			pods = sst.GetPodList(ss)
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Status.ContainerStatuses[0].RestartCount).To(gomega.Equal(int32(0)))
				gomega.Expect(pods.Items[i].CreationTimestamp.After(updateTime)).To(gomega.Equal(true))
				gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision))
			}
		})

		/*
			Release : v1.9
			Testname: StatefulSet, Scaling
			Description: StatefulSet MUST create Pods in ascending order by ordinal index when scaling up, and delete Pods in descending order when scaling down. Scaling up or down MUST pause if any Pods belonging to the StatefulSet are unhealthy. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("Scaling should happen in predictable order and halt if any stateful pod is unhealthy", func() {
			psLabels := klabels.Set(labels)
			ginkgo.By("Initializing watcher for selector " + psLabels.String())
			watcher, err := f.ClientSet.CoreV1().Pods(ns).Watch(context.TODO(), metav1.ListOptions{
				LabelSelector: psLabels.AsSelector().String(),
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Creating stateful set " + ssName + " in namespace " + ns)
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 1, nil, nil, psLabels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss, err = kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Waiting until all stateful set " + ssName + " replicas will be running in namespace " + ns)
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)

			ginkgo.By("Confirming that stateful set scale up will halt with unhealthy stateful pod")
			_ = sst.BreakHTTPProbe(ss)
			sst.WaitForRunningAndNotReady(*ss.Spec.Replicas, ss)
			sst.WaitForStatusReadyReplicas(ss, 0)
			sst.UpdateReplicas(ss, 3)
			sst.ConfirmStatefulPodCount(1, ss, 10*time.Second, true)

			ginkgo.By("Scaling up stateful set " + ssName + " to 3 replicas and waiting until all of them will be running in namespace " + ns)
			_ = sst.RestoreHTTPProbe(ss)
			sst.WaitForRunningAndReady(3, ss)

			ginkgo.By("Verifying that stateful set " + ssName + " was scaled up in order")
			expectedOrder := []string{ssName + "-0", ssName + "-1", ssName + "-2"}
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), framework.StatefulSetTimeout)
			defer cancel()
			_, err = watchtools.UntilWithoutRetry(ctx, watcher, func(event watch.Event) (bool, error) {
				if event.Type != watch.Added {
					return false, nil
				}
				pod := event.Object.(*v1.Pod)
				if pod.Name == expectedOrder[0] {
					expectedOrder = expectedOrder[1:]
				}
				return len(expectedOrder) == 0, nil

			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Scale down will halt with unhealthy stateful pod")
			watcher, err = f.ClientSet.CoreV1().Pods(ns).Watch(context.TODO(), metav1.ListOptions{
				LabelSelector: psLabels.AsSelector().String(),
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			_ = sst.BreakHTTPProbe(ss)
			sst.WaitForStatusReadyReplicas(ss, 0)
			sst.WaitForRunningAndNotReady(3, ss)
			sst.UpdateReplicas(ss, 0)
			sst.ConfirmStatefulPodCount(3, ss, 10*time.Second, true)

			ginkgo.By("Scaling down stateful set " + ssName + " to 0 replicas and waiting until none of pods will run in namespace" + ns)
			_ = sst.RestoreHTTPProbe(ss)
			_, _ = sst.Scale(ss, 0)

			ginkgo.By("Verifying that stateful set " + ssName + " was scaled down in reverse order")
			expectedOrder = []string{ssName + "-2", ssName + "-1", ssName + "-0"}
			ctx, cancel = watchtools.ContextWithOptionalTimeout(context.Background(), framework.StatefulSetTimeout)
			defer cancel()
			_, err = watchtools.UntilWithoutRetry(ctx, watcher, func(event watch.Event) (bool, error) {
				if event.Type != watch.Deleted {
					return false, nil
				}
				pod := event.Object.(*v1.Pod)
				if pod.Name == expectedOrder[0] {
					expectedOrder = expectedOrder[1:]
				}
				return len(expectedOrder) == 0, nil

			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		/*
			Release : v1.9
			Testname: StatefulSet, Burst Scaling
			Description: StatefulSet MUST support the Parallel PodManagementPolicy for burst scaling. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("Burst scaling should run to completion even with unhealthy pods", func() {
			psLabels := klabels.Set(labels)

			ginkgo.By("Creating stateful set " + ssName + " in namespace " + ns)
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 1, nil, nil, psLabels)
			ss.Spec.PodManagementPolicy = apps.ParallelPodManagement
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Waiting until all stateful set " + ssName + " replicas will be running in namespace " + ns)
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)

			ginkgo.By("Confirming that stateful set scale up will not halt with unhealthy stateful pod")
			_ = sst.BreakHTTPProbe(ss)
			sst.WaitForRunningAndNotReady(*ss.Spec.Replicas, ss)
			sst.WaitForStatusReadyReplicas(ss, 0)
			sst.UpdateReplicas(ss, 3)
			sst.ConfirmStatefulPodCount(3, ss, 10*time.Second, false)

			ginkgo.By("Scaling up stateful set " + ssName + " to 3 replicas and waiting until all of them will be running in namespace " + ns)
			_ = sst.RestoreHTTPProbe(ss)
			sst.WaitForRunningAndReady(3, ss)

			ginkgo.By("Scale down will not halt with unhealthy stateful pod")
			_ = sst.BreakHTTPProbe(ss)
			sst.WaitForStatusReadyReplicas(ss, 0)
			sst.WaitForRunningAndNotReady(3, ss)
			sst.UpdateReplicas(ss, 0)
			sst.ConfirmStatefulPodCount(0, ss, 10*time.Second, false)

			ginkgo.By("Scaling down stateful set " + ssName + " to 0 replicas and waiting until none of pods will run in namespace" + ns)
			_ = sst.RestoreHTTPProbe(ss)
			_, _ = sst.Scale(ss, 0)
			sst.WaitForStatusReplicas(ss, 0)
		})

		/*
			Release : v1.9
			Testname: StatefulSet, Recreate Failed Pod
			Description: StatefulSet MUST delete and recreate Pods it owns that go into a Failed state, such as when they are rejected or evicted by a Node. This test does not depend on a preexisting default StorageClass or a dynamic provisioner.
		*/
		framework.ConformanceIt("Should recreate evicted statefulset", func() {
			podName := "test-pod"
			statefulPodName := ssName + "-0"
			ginkgo.By("Looking for a node to schedule stateful set and pod")
			nodes := framework.GetReadySchedulableNodesOrDie(f.ClientSet)
			node := nodes.Items[0]

			ginkgo.By("Creating pod with conflicting port in namespace " + f.Namespace.Name)
			conflictingPort := v1.ContainerPort{HostPort: 21017, ContainerPort: 21017, Name: "conflict"}
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: podName,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "nginx",
							Image: imageutils.GetE2EImage(imageutils.Nginx),
							Ports: []v1.ContainerPort{conflictingPort},
						},
					},
					NodeName: node.Name,
				},
			}
			pod, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pod, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Creating statefulset with conflicting port in namespace " + f.Namespace.Name)
			ss := framework.NewStatefulSet(ssName, f.Namespace.Name, headlessSvcName, 1, nil, nil, labels)
			statefulPodContainer := &ss.Spec.Template.Spec.Containers[0]
			statefulPodContainer.Ports = append(statefulPodContainer.Ports, conflictingPort)
			ss.Spec.Template.Spec.NodeName = node.Name
			_, err = kc.AppsV1beta1().StatefulSets(f.Namespace.Name).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Waiting until pod " + podName + " will start running in namespace " + f.Namespace.Name)
			if err := f.WaitForPodRunning(podName); err != nil {
				framework.Failf("Pod %v did not start running: %v", podName, err)
			}

			var initialStatefulPodUID types.UID
			ginkgo.By("Waiting until stateful pod " + statefulPodName + " will be recreated and deleted at least once in namespace " + f.Namespace.Name)
			w, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Watch(context.TODO(), metav1.SingleObject(metav1.ObjectMeta{Name: statefulPodName}))
			framework.ExpectNoError(err)
			ctx, cancel := watchtools.ContextWithOptionalTimeout(context.Background(), framework.StatefulPodTimeout)
			defer cancel()
			// we need to get UID from pod in any state and wait until stateful set controller will remove pod at least once
			_, err = watchtools.UntilWithoutRetry(ctx, w, func(event watch.Event) (bool, error) {
				pod := event.Object.(*v1.Pod)
				switch event.Type {
				case watch.Deleted:
					framework.Logf("Observed delete event for stateful pod %v in namespace %v", pod.Name, pod.Namespace)
					if initialStatefulPodUID == "" {
						return false, nil
					}
					return true, nil
				}
				framework.Logf("Observed stateful pod in namespace: %v, name: %v, uid: %v, status phase: %v. Waiting for statefulset controller to delete.",
					pod.Namespace, pod.Name, pod.UID, pod.Status.Phase)
				initialStatefulPodUID = pod.UID
				return false, nil
			})
			if err != nil {
				framework.Failf("Pod %v expected to be re-created at least once", statefulPodName)
			}

			ginkgo.By("Removing pod with conflicting port in namespace " + f.Namespace.Name)
			err = f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, *metav1.NewDeleteOptions(0))
			framework.ExpectNoError(err)

			ginkgo.By("Waiting when stateful pod " + statefulPodName + " will be recreated in namespace " + f.Namespace.Name + " and will be in running state")
			// we may catch delete event, that's why we are waiting for running phase like this, and not with watchtools.UntilWithoutRetry
			gomega.Eventually(func() error {
				statefulPod, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Get(context.TODO(), statefulPodName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if statefulPod.Status.Phase != v1.PodRunning {
					return fmt.Errorf("Pod %v is not in running phase: %v", statefulPod.Name, statefulPod.Status.Phase)
				} else if statefulPod.UID == initialStatefulPodUID {
					return fmt.Errorf("Pod %v wasn't recreated: %v == %v", statefulPod.Name, statefulPod.UID, initialStatefulPodUID)
				}
				return nil
			}, framework.StatefulPodTimeout, 2*time.Second).Should(gomega.BeNil())
		})

		/*
			Release : v1.16
			Testname: StatefulSet resource Replica scaling
			Description: Create a StatefulSet resource.
			Newly created StatefulSet resource MUST have a scale of one.
			Bring the scale of the StatefulSet resource up to two. StatefulSet scale MUST be at two replicas.
		*/
		framework.ConformanceIt("should have a working scale subresource", func() {
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 1, nil, nil, labels)
			sst := framework.NewStatefulSetTester(c, kc)
			sst.SetHTTPProbe(ss)
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
			ss = sst.WaitForStatus(ss)

			ginkgo.By("getting scale subresource")
			scale, err := kc.AppsV1beta1().StatefulSets(ns).GetScale(context.TODO(), ssName, metav1.GetOptions{})
			if err != nil {
				framework.Failf("Failed to get scale subresource: %v", err)
			}
			framework.ExpectEqual(scale.Spec.Replicas, int32(1))
			framework.ExpectEqual(scale.Status.Replicas, int32(1))

			ginkgo.By("updating a scale subresource")
			if serverMinorVersion >= 18 {
				scale.ResourceVersion = "" // indicate the scale update should be unconditional
			}
			scale.Spec.Replicas = 2
			scaleResult, err := kc.AppsV1beta1().StatefulSets(ns).UpdateScale(context.TODO(), ssName, scale, metav1.UpdateOptions{})
			if err != nil {
				framework.Failf("Failed to put scale subresource: %v", err)
			}
			framework.ExpectEqual(scaleResult.Spec.Replicas, int32(2))

			ginkgo.By("verifying the statefulset Spec.Replicas was modified")
			ss, err = kc.AppsV1beta1().StatefulSets(ns).Get(context.TODO(), ssName, metav1.GetOptions{})
			if err != nil {
				framework.Failf("Failed to get statefulset resource: %v", err)
			}
			framework.ExpectEqual(*(ss.Spec.Replicas), int32(2))
		})

		/*
			Testname: StatefulSet, ScaleStrategy
			Description: StatefulSet resource MUST support the MaxUnavailable ScaleStrategy for scaling.
			It only affects when create new pod, terminating pod and unavailable pod at the Parallel PodManagementPolicy.
		*/
		framework.ConformanceIt("Should can update pods when the statefulset scale strategy is set", func() {
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			maxUnavailable := intstr.FromInt32(2)
			ss := framework.NewStatefulSet(ssName, ns, headlessSvcName, 3, nil, nil, labels)
			ss.Spec.Template.Spec.Containers[0].Name = "busybox"
			ss.Spec.Template.Spec.Containers[0].Image = BusyboxImage
			ss.Spec.Template.Spec.Containers[0].Command = []string{"sleep", "3600"}
			ss.Spec.PodManagementPolicy = apps.ParallelPodManagement
			ss.Spec.Template.Spec.RestartPolicy = v1.RestartPolicyAlways
			ss.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{
				MinReadySeconds: pointer.Int32(3),
				PodUpdatePolicy: appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType,
			}
			ss.Spec.ScaleStrategy = &appsv1beta1.StatefulSetScaleStrategy{MaxUnavailable: &maxUnavailable}
			ss.Spec.Template.Spec.ReadinessGates = append(ss.Spec.Template.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
			sst := framework.NewStatefulSetTester(c, kc)
			// sst.SetHTTPProbe(ss)
			ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)

			ginkgo.By("Scaling up stateful set " + ssName + " to 10 replicas and check create new pod equal MaxUnavailable")
			sst.UpdateReplicas(ss, 10)
			sst.ConfirmStatefulPodCount(5, ss, time.Second, false)
			sst.WaitForRunningAndReady(10, ss)

			ginkgo.By("Confirming that stateful can update all pods to be unhealthy")
			maxUnavailable = intstr.FromString("100%")
			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.ScaleStrategy.MaxUnavailable = &maxUnavailable
				update.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = &intstr.IntOrString{Type: intstr.String, StrVal: "100%"}
				update.Spec.Template.Spec.Containers[0].Command = []string{}
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			sst.WaitForRunningAndNotReady(10, ss)
			sst.WaitForStatusReadyReplicas(ss, 0)
			ss = sst.WaitForStatus(ss)

			ginkgo.By("Confirming that stateful can update all pods if any stateful pod is unhealthy")

			ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Template.Labels["test-update"] = "yes"
				update.Spec.Template.Spec.Containers[0].Command = []string{"sleep", "180"}
			})
			sst.WaitForRunningAndReady(10, ss)
			sst.WaitForStatusReadyReplicas(ss, 10)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var pods *v1.PodList
			sst.WaitForState(ss, func(set *appsv1beta1.StatefulSet, pl *v1.PodList) (bool, error) {
				ss = set
				pods = pl
				sst.SortStatefulPods(pods)
				for i := range pods.Items {
					if pods.Items[i].Labels[apps.StatefulSetRevisionLabel] != set.Status.UpdateRevision {
						framework.Logf("Waiting for Pod %s/%s to have revision %s update revision %s",
							pods.Items[i].Namespace,
							pods.Items[i].Name,
							set.Status.UpdateRevision,
							pods.Items[i].Labels[apps.StatefulSetRevisionLabel])
						return false, nil
					}
				}
				return true, nil
			})

			ginkgo.By("Confirming Pods were updated successful")
			for i := range pods.Items {
				gomega.Expect(pods.Items[i].Labels["test-update"]).To(gomega.Equal("yes"))
			}
		})

		/*
			Testname: StatefulSet, Specified delete
			Description: Specified delete pod MUST under maxUnavailable constrain.
		*/
		framework.ConformanceIt("should perform rolling updates with specified-deleted", func() {
			ginkgo.By("Creating a new StatefulSet")
			ss = framework.NewStatefulSet("ss2", ns, headlessSvcName, 3, nil, nil, labels)
			testWithSpecifiedDeleted(c, kc, ns, ss)
		})
	})

	//ginkgo.Describe("Deploy clustered applications [Feature:StatefulSet] [Slow]", func() {
	//	var appTester *clusterAppTester
	//
	//	ginkgo.BeforeEach(func() {
	//		appTester = &clusterAppTester{client: c, ns: ns}
	//	})
	//
	//	ginkgo.AfterEach(func() {
	//		if ginkgo.CurrentGinkgoTestDescription().Failed {
	//			framework.DumpDebugInfo(c, ns)
	//		}
	//		framework.Logf("Deleting all statefulset in ns %v", ns)
	//		e2estatefulset.DeleteAllStatefulSets(c, ns)
	//	})
	//
	//	// Do not mark this as Conformance.
	//	// StatefulSet Conformance should not be dependent on specific applications.
	//	ginkgo.It("should creating a working zookeeper cluster", func() {
	//		e2epv.SkipIfNoDefaultStorageClass(c)
	//		appTester.statefulPod = &zookeeperTester{client: c}
	//		appTester.run()
	//	})
	//
	//	// Do not mark this as Conformance.
	//	// StatefulSet Conformance should not be dependent on specific applications.
	//	ginkgo.It("should creating a working redis cluster", func() {
	//		e2epv.SkipIfNoDefaultStorageClass(c)
	//		appTester.statefulPod = &redisTester{client: c}
	//		appTester.run()
	//	})
	//
	//	// Do not mark this as Conformance.
	//	// StatefulSet Conformance should not be dependent on specific applications.
	//	ginkgo.It("should creating a working mysql cluster", func() {
	//		e2epv.SkipIfNoDefaultStorageClass(c)
	//		appTester.statefulPod = &mysqlGaleraTester{client: c}
	//		appTester.run()
	//	})
	//
	//	// Do not mark this as Conformance.
	//	// StatefulSet Conformance should not be dependent on specific applications.
	//	ginkgo.It("should creating a working CockroachDB cluster", func() {
	//		e2epv.SkipIfNoDefaultStorageClass(c)
	//		appTester.statefulPod = &cockroachDBTester{client: c}
	//		appTester.run()
	//	})
	//})
	//
	//// Make sure minReadySeconds is honored
	//// Don't mark it as conformance yet
	//ginkgo.It("MinReadySeconds should be honored when enabled", func() {
	//	ssName := "test-ss"
	//	headlessSvcName := "test"
	//	// Define StatefulSet Labels
	//	ssPodLabels := map[string]string{
	//		"name": "sample-pod",
	//		"pod":  WebserverImageName,
	//	}
	//	ss := e2estatefulset.NewStatefulSet(ssName, ns, headlessSvcName, 1, nil, nil, ssPodLabels)
	//	setHTTPProbe(ss)
	//	ss, err := c.AppsV1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
	//	framework.ExpectNoError(err)
	//	e2estatefulset.WaitForStatusAvailableReplicas(c, ss, 1)
	//})
	//
	//ginkgo.It("AvailableReplicas should get updated accordingly when MinReadySeconds is enabled", func() {
	//	ssName := "test-ss"
	//	headlessSvcName := "test"
	//	// Define StatefulSet Labels
	//	ssPodLabels := map[string]string{
	//		"name": "sample-pod",
	//		"pod":  WebserverImageName,
	//	}
	//	ss := e2estatefulset.NewStatefulSet(ssName, ns, headlessSvcName, 2, nil, nil, ssPodLabels)
	//	ss.Spec.MinReadySeconds = 30
	//	setHTTPProbe(ss)
	//	ss, err := c.AppsV1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
	//	framework.ExpectNoError(err)
	//	e2estatefulset.WaitForStatusAvailableReplicas(c, ss, 0)
	//	// let's check that the availableReplicas have still not updated
	//	time.Sleep(5 * time.Second)
	//	ss, err = c.AppsV1().StatefulSets(ns).Get(context.TODO(), ss.Name, metav1.GetOptions{})
	//	framework.ExpectNoError(err)
	//	if ss.Status.AvailableReplicas != 0 {
	//		framework.Failf("invalid number of availableReplicas: expected=%v received=%v", 0, ss.Status.AvailableReplicas)
	//	}
	//	e2estatefulset.WaitForStatusAvailableReplicas(c, ss, 2)
	//
	//	ss, err = updateStatefulSetWithRetries(c, ns, ss.Name, func(update *appsv1.StatefulSet) {
	//		update.Spec.MinReadySeconds = 3600
	//	})
	//	framework.ExpectNoError(err)
	//	// We don't expect replicas to be updated till 1 hour, so the availableReplicas should be 0
	//	e2estatefulset.WaitForStatusAvailableReplicas(c, ss, 0)
	//
	//	ss, err = updateStatefulSetWithRetries(c, ns, ss.Name, func(update *appsv1.StatefulSet) {
	//		update.Spec.MinReadySeconds = 0
	//	})
	//	framework.ExpectNoError(err)
	//	e2estatefulset.WaitForStatusAvailableReplicas(c, ss, 2)
	//
	//	ginkgo.By("check availableReplicas are shown in status")
	//	out, err := framework.RunKubectl(ns, "get", "statefulset", ss.Name, "-o=yaml")
	//	framework.ExpectNoError(err)
	//	if !strings.Contains(out, "availableReplicas: 2") {
	//		framework.Failf("invalid number of availableReplicas: expected=%v received=%v", 2, out)
	//	}
	//})

	ginkgo.Describe("Non-retain StatefulSetPersistentVolumeClaimPolicy [Feature:StatefulSetAutoDeletePVC]", func() {
		ssName := "ss"
		labels := map[string]string{
			"foo": "bar",
			"baz": "blah",
		}
		headlessSvcName := "test"
		var statefulPodMounts, podMounts []v1.VolumeMount
		var ss *appsv1beta1.StatefulSet

		ginkgo.BeforeEach(func() {
			statefulPodMounts = []v1.VolumeMount{{Name: "datadir", MountPath: "/data/"}}
			podMounts = []v1.VolumeMount{{Name: "home", MountPath: "/home"}}
			ss = framework.NewStatefulSet(ssName, ns, headlessSvcName, 2, statefulPodMounts, podMounts, labels)

			ginkgo.By("Creating service " + headlessSvcName + " in namespace " + ns)
			headlessService := framework.CreateServiceSpec(headlessSvcName, "", true, labels)
			_, err := c.CoreV1().Services(ns).Create(context.TODO(), headlessService, metav1.CreateOptions{})
			framework.ExpectNoError(err)
		})

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all statefulset in ns %v", ns)
			framework.DeleteAllStatefulSets(c, kc, ns)
		})

		ginkgo.It("should delete PVCs with a WhenDeleted policy", func() {
			if framework.SkipIfNoDefaultStorageClass(c) {
				return
			}
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming all 3 PVCs exist with their owner refs")
			err = verifyStatefulSetPVCsExistWithOwnerRefs(c, kc, ss, []int{0, 1, 2}, true, false)
			framework.ExpectNoError(err)

			ginkgo.By("Deleting stateful set " + ss.Name)
			err = kc.AppsV1beta1().StatefulSets(ns).Delete(context.TODO(), ss.Name, metav1.DeleteOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Verifying PVCs deleted")
			err = verifyStatefulSetPVCsExist(c, ss, []int{})
			framework.ExpectNoError(err)
		})

		ginkgo.It("should delete PVCs with a OnScaledown policy", func() {
			if framework.SkipIfNoDefaultStorageClass(c) {
				return
			}
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenScaled: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming all 3 PVCs exist")
			err = verifyStatefulSetPVCsExist(c, ss, []int{0, 1, 2})
			framework.ExpectNoError(err)

			ginkgo.By("Scaling stateful set " + ss.Name + " to one replica")
			ss, err = framework.NewStatefulSetTester(c, kc).Scale(ss, 1)
			framework.ExpectNoError(err)

			ginkgo.By("Verifying all but one PVC deleted")
			err = verifyStatefulSetPVCsExist(c, ss, []int{0})
			framework.ExpectNoError(err)
		})

		ginkgo.It("should delete PVCs with a OnScaledown policy and range reserveOrdinals=[0,2-5]", func() {
			if framework.SkipIfNoDefaultStorageClass(c) {
				return
			}
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenScaled: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			}
			ss.Spec.ReserveOrdinals = []intstr.IntOrString{
				intstr.FromInt32(0),
				intstr.FromString("2-5"),
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming all 3 PVCs exist")
			err = verifyStatefulSetPVCsExist(c, ss, []int{1, 6, 7})
			framework.ExpectNoError(err)

			ginkgo.By("Scaling stateful set " + ss.Name + " to one replica")
			ss, err = framework.NewStatefulSetTester(c, kc).Scale(ss, 1)
			framework.ExpectNoError(err)

			ginkgo.By("Verifying all but one PVC deleted")
			err = verifyStatefulSetPVCsExist(c, ss, []int{1})
			framework.ExpectNoError(err)
		})

		ginkgo.It("should delete PVCs after adopting pod (WhenDeleted)", func() {
			if framework.SkipIfNoDefaultStorageClass(c) {
				return
			}
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming all 3 PVCs exist with their owner refs")
			err = verifyStatefulSetPVCsExistWithOwnerRefs(c, kc, ss, []int{0, 1, 2}, true, false)
			framework.ExpectNoError(err)

			ginkgo.By("Orphaning the 3rd pod")
			patch, err := json.Marshal(metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{},
			})
			framework.ExpectNoError(err, "Could not Marshal JSON for patch payload")
			_, err = c.CoreV1().Pods(ns).Patch(context.TODO(), fmt.Sprintf("%s-2", ss.Name), types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{}, "")
			framework.ExpectNoError(err, "Could not patch payload")

			ginkgo.By("Deleting stateful set " + ss.Name)
			err = kc.AppsV1beta1().StatefulSets(ns).Delete(context.TODO(), ss.Name, metav1.DeleteOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Verifying PVCs deleted")
			err = verifyStatefulSetPVCsExist(c, ss, []int{})
			framework.ExpectNoError(err)
		})

		ginkgo.It("should delete PVCs after adopting pod (WhenScaled) [Feature:StatefulSetAutoDeletePVC]", func() {
			if framework.SkipIfNoDefaultStorageClass(c) {
				return
			}
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 3
			ss.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1beta1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenScaled: appsv1beta1.DeletePersistentVolumeClaimRetentionPolicyType,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming all 3 PVCs exist")
			err = verifyStatefulSetPVCsExist(c, ss, []int{0, 1, 2})
			framework.ExpectNoError(err)

			// why 3rd -> 2rd? patch 3rd pod maybe failed when pod has not been created
			ginkgo.By("Orphaning the 2rd pod")
			patch, err := json.Marshal(metav1.ObjectMeta{
				OwnerReferences: []metav1.OwnerReference{},
			})
			framework.ExpectNoError(err, "Could not Marshal JSON for patch payload")
			_, err = c.CoreV1().Pods(ns).Patch(context.TODO(), fmt.Sprintf("%s-1", ss.Name), types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{}, "")
			framework.ExpectNoError(err, "Could not patch payload")

			ginkgo.By("Scaling stateful set " + ss.Name + " to one replica")
			ss, err = framework.NewStatefulSetTester(c, kc).Scale(ss, 1)
			framework.ExpectNoError(err)

			ginkgo.By("Verifying all but one PVC deleted")
			err = verifyStatefulSetPVCsExist(c, ss, []int{0})
			framework.ExpectNoError(err)
		})
	})

	ginkgo.Describe("Automatically recreate PVC for pending pod when PVC is missing", func() {
		ssName := "ss"
		labels := map[string]string{
			"foo": "bar",
			"baz": "blah",
		}
		headlessSvcName := "test"
		var statefulPodMounts []v1.VolumeMount
		var ss *appsv1beta1.StatefulSet

		ginkgo.BeforeEach(func() {
			statefulPodMounts = []v1.VolumeMount{{Name: "datadir", MountPath: "/data/"}}
			ss = framework.NewStatefulSet(ssName, ns, headlessSvcName, 1, statefulPodMounts, nil, labels)
		})

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all statefulset in ns %v", ns)
			framework.DeleteAllStatefulSets(c, kc, ns)
		})

		//ginkgo.It("PVC should be recreated when pod is pending due to missing PVC", f.WithDisruptive(), f.WithSerial(), func() {
		ginkgo.It("PVC should be recreated when pod is pending due to missing PVC", func() {
			ctx := context.TODO()
			framework.SkipIfNoDefaultStorageClass(c)

			readyNode, err := framework.GetRandomReadySchedulableNode(ctx, c)
			framework.ExpectNoError(err)
			hostLabel := "kubernetes.io/hostname"
			hostLabelVal := readyNode.Labels[hostLabel]

			ss.Spec.Template.Spec.NodeSelector = map[string]string{hostLabel: hostLabelVal} // force the pod on a specific node
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			_, err = kc.AppsV1beta1().StatefulSets(ns).Create(ctx, ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming PVC exists")
			err = verifyStatefulSetPVCsExist(c, ss, []int{0})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming Pod is ready")
			sst := framework.NewStatefulSetTester(c, kc)
			sst.WaitForStatusReadyReplicas(ss, 1)
			podName := getStatefulSetPodNameAtIndex(0, ss)
			pod, err := c.CoreV1().Pods(ns).Get(ctx, podName, metav1.GetOptions{})
			framework.ExpectNoError(err)

			nodeName := pod.Spec.NodeName
			gomega.Expect(nodeName).To(gomega.Equal(readyNode.Name))
			node, err := c.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
			framework.ExpectNoError(err)

			oldData, err := json.Marshal(node)
			framework.ExpectNoError(err)

			node.Spec.Unschedulable = true

			newData, err := json.Marshal(node)
			framework.ExpectNoError(err)

			// cordon node, to make sure pod does not get scheduled to the node until the pvc is deleted
			patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, v1.Node{})
			framework.ExpectNoError(err)
			ginkgo.By("Cordoning Node")
			_, err = c.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
			framework.ExpectNoError(err)
			cordoned := true

			defer func() {
				if cordoned {
					uncordonNode(ctx, c, oldData, newData, nodeName)
				}
			}()

			// wait for the node to be unschedulable
			framework.WaitForNodeSchedulable(ctx, c, nodeName, 10*time.Second, false)

			ginkgo.By("Deleting Pod")
			err = c.CoreV1().Pods(ns).Delete(ctx, podName, metav1.DeleteOptions{})
			framework.ExpectNoError(err)

			// wait for the pod to be recreated
			waitForStatusCurrentReplicas(ctx, c, kc, ss, 1)
			_, err = c.CoreV1().Pods(ns).Get(ctx, podName, metav1.GetOptions{})
			framework.ExpectNoError(err)

			pvcList, err := c.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{LabelSelector: klabels.Everything().String()})
			framework.ExpectNoError(err)
			gomega.Expect(pvcList.Items).To(gomega.HaveLen(1))
			pvcName := pvcList.Items[0].Name

			ginkgo.By("Deleting PVC")
			err = c.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, pvcName, metav1.DeleteOptions{})
			framework.ExpectNoError(err)

			uncordonNode(ctx, c, oldData, newData, nodeName)
			cordoned = false

			ginkgo.By("Confirming PVC recreated")
			err = verifyStatefulSetPVCsExist(c, ss, []int{0})
			framework.ExpectNoError(err)

			ginkgo.By("Confirming Pod is ready after being recreated")
			sst.WaitForStatusReadyReplicas(ss, 1)
			pod, err = c.CoreV1().Pods(ns).Get(ctx, podName, metav1.GetOptions{})
			framework.ExpectNoError(err)
			gomega.Expect(pod.Spec.NodeName).To(gomega.Equal(readyNode.Name)) // confirm the pod was scheduled back to the original node
		})
	})

	ginkgo.Describe("Scaling StatefulSetStartOrdinal", func() {
		ssName := "ss"
		labels := map[string]string{
			"foo": "bar",
			"baz": "blah",
		}
		headlessSvcName := "test"
		var ss *appsv1beta1.StatefulSet
		var sst *framework.StatefulSetTester

		ginkgo.BeforeEach(func() {
			ss = framework.NewStatefulSet(ssName, ns, headlessSvcName, 2, nil, nil, labels)

			ginkgo.By("Creating service " + headlessSvcName + " in namespace " + ns)
			headlessService := framework.CreateServiceSpec(headlessSvcName, "", true, labels)
			_, err := c.CoreV1().Services(ns).Create(context.TODO(), headlessService, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			sst = framework.NewStatefulSetTester(c, kc)
		})

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all statefulset in ns %v", ns)
			framework.DeleteAllStatefulSets(c, kc, ns)
		})

		ginkgo.It("Setting .start.ordinal", func() {
			ctx := context.TODO()
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 2
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(ctx, ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			sst := framework.NewStatefulSetTester(c, kc)
			waitForStatus(ctx, c, kc, ss)
			sst.WaitForStatusReplicas(ss, 2)
			sst.WaitForStatusReadyReplicas(ss, 2)

			ginkgo.By("Confirming 2 replicas, with start ordinal 0")
			pods := sst.GetPodList(ss)
			err = expectPodNames(pods, []string{"ss-0", "ss-1"})
			framework.ExpectNoError(err)

			ginkgo.By("Setting .spec.replicas = 3 .spec.ordinals.start = 2")
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Ordinals = &appsv1beta1.StatefulSetOrdinals{
					Start: 2,
				}
				*(update.Spec.Replicas) = 3
			})
			framework.ExpectNoError(err)

			// we need to ensure we wait for all the new ones to show up, not
			// just for any random 3
			waitForStatus(ctx, c, kc, ss)
			waitForPodNames(ctx, c, kc, ss, []string{"ss-2", "ss-3", "ss-4"})
			ginkgo.By("Confirming 3 replicas, with start ordinal 2")
			sst.WaitForStatusReplicas(ss, 3)
			sst.WaitForStatusReadyReplicas(ss, 3)
		})

		ginkgo.It("Increasing .start.ordinal", func() {
			ctx := context.TODO()
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 2
			ss.Spec.Ordinals = &appsv1beta1.StatefulSetOrdinals{
				Start: 2,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(ctx, ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			waitForStatus(ctx, c, kc, ss)
			sst.WaitForStatusReplicas(ss, 2)
			sst.WaitForStatusReadyReplicas(ss, 2)

			ginkgo.By("Confirming 2 replicas, with start ordinal 2")
			pods := sst.GetPodList(ss)
			err = expectPodNames(pods, []string{"ss-2", "ss-3"})
			framework.ExpectNoError(err)

			ginkgo.By("Increasing .spec.ordinals.start = 4")
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Ordinals = &appsv1beta1.StatefulSetOrdinals{
					Start: 4,
				}
			})
			framework.ExpectNoError(err)

			// since we are replacing 2 pods for 2, we need to ensure we wait
			// for the new ones to show up, not just for any random 2
			ginkgo.By("Confirming 2 replicas, with start ordinal 4")
			waitForStatus(ctx, c, kc, ss)
			waitForPodNames(ctx, c, kc, ss, []string{"ss-4", "ss-5"})
			sst.WaitForStatusReplicas(ss, 2)
			sst.WaitForStatusReadyReplicas(ss, 2)
		})

		ginkgo.It("Decreasing .start.ordinal", func() {
			ctx := context.TODO()
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 2
			ss.Spec.Ordinals = &appsv1beta1.StatefulSetOrdinals{
				Start: 3,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(ctx, ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			waitForStatus(ctx, c, kc, ss)
			sst.WaitForStatusReplicas(ss, 2)
			sst.WaitForStatusReadyReplicas(ss, 2)

			ginkgo.By("Confirming 2 replicas, with start ordinal 3")
			pods := sst.GetPodList(ss)
			err = expectPodNames(pods, []string{"ss-3", "ss-4"})
			framework.ExpectNoError(err)

			ginkgo.By("Decreasing .spec.ordinals.start = 2")
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Ordinals = &appsv1beta1.StatefulSetOrdinals{
					Start: 2,
				}
			})
			framework.ExpectNoError(err)

			// since we are replacing 2 pods for 2, we need to ensure we wait
			// for the new ones to show up, not just for any random 2
			ginkgo.By("Confirming 2 replicas, with start ordinal 2")
			waitForStatus(ctx, c, kc, ss)
			waitForPodNames(ctx, c, kc, ss, []string{"ss-2", "ss-3"})
			sst.WaitForStatusReplicas(ss, 2)
			sst.WaitForStatusReadyReplicas(ss, 2)
		})

		ginkgo.It("Removing .start.ordinal", func() {
			ctx := context.TODO()
			ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
			*(ss.Spec.Replicas) = 2
			ss.Spec.Ordinals = &appsv1beta1.StatefulSetOrdinals{
				Start: 3,
			}
			_, err := kc.AppsV1beta1().StatefulSets(ns).Create(ctx, ss, metav1.CreateOptions{})
			framework.ExpectNoError(err)
			sst.WaitForStatusReplicas(ss, 2)
			sst.WaitForStatusReadyReplicas(ss, 2)

			ginkgo.By("Confirming 2 replicas, with start ordinal 3")
			pods := sst.GetPodList(ss)
			err = expectPodNames(pods, []string{"ss-3", "ss-4"})
			framework.ExpectNoError(err)

			ginkgo.By("Removing .spec.ordinals")
			ss, err = updateStatefulSetWithRetries(ctx, kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
				update.Spec.Ordinals = nil
			})
			framework.ExpectNoError(err)

			// since we are replacing 2 pods for 2, we need to ensure we wait
			// for the new ones to show up, not just for any random 2
			framework.Logf("Confirming 2 replicas, with start ordinal 0")
			waitForStatus(ctx, c, kc, ss)
			waitForPodNames(ctx, c, kc, ss, []string{"ss-0", "ss-1"})
			sst.WaitForStatusReplicas(ss, 2)
			sst.WaitForStatusReadyReplicas(ss, 2)
		})
	})
})

// This function is used by two tests to test StatefulSet rollbacks: one using
// PVCs and one using no storage.
func rollbackTest(c clientset.Interface, kc kruiseclientset.Interface, ns string, ss *appsv1beta1.StatefulSet) {
	sst := framework.NewStatefulSetTester(c, kc)
	sst.SetHTTPProbe(ss)
	ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
	ss = sst.WaitForStatus(ss)
	currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
	gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
		fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
			ss.Namespace, ss.Name, updateRevision, currentRevision))
	pods := sst.GetPodList(ss)
	for i := range pods.Items {
		gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
			fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
				currentRevision))
	}
	sst.SortStatefulPods(pods)
	err = sst.BreakPodHTTPProbe(ss, &pods.Items[1])
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ss, pods = sst.WaitForPodNotReady(ss, pods.Items[1].Name)
	newImage := NewNginxImage
	oldImage := ss.Spec.Template.Spec.Containers[0].Image

	ginkgo.By(fmt.Sprintf("Updating StatefulSet template: update image from %s to %s", oldImage, newImage))
	gomega.Expect(oldImage).NotTo(gomega.Equal(newImage), "Incorrect test setup: should update to a different image")
	ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
		update.Spec.Template.Spec.Containers[0].Image = newImage
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("Creating a new revision")
	ss = sst.WaitForStatus(ss)
	currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
	gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
		"Current revision should not equal update revision during rolling update")

	ginkgo.By("Updating Pods in reverse ordinal order")
	pods = sst.GetPodList(ss)
	sst.SortStatefulPods(pods)
	err = sst.RestorePodHTTPProbe(ss, &pods.Items[1])
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ss, pods = sst.WaitForPodReady(ss, pods.Items[1].Name)
	ss, pods = sst.WaitForRollingUpdate(ss)
	gomega.Expect(ss.Status.CurrentRevision).To(gomega.Equal(updateRevision),
		fmt.Sprintf("StatefulSet %s/%s current revision %s does not equal update revision %s on update completion",
			ss.Namespace,
			ss.Name,
			ss.Status.CurrentRevision,
			updateRevision))
	for i := range pods.Items {
		gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(newImage),
			fmt.Sprintf(" Pod %s/%s has image %s not have new image %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Spec.Containers[0].Image,
				newImage))
		gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
			fmt.Sprintf("Pod %s/%s revision %s is not equal to update revision %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
				updateRevision))
	}

	ginkgo.By("Rolling back to a previous revision")
	err = sst.BreakPodHTTPProbe(ss, &pods.Items[1])
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ss, pods = sst.WaitForPodNotReady(ss, pods.Items[1].Name)
	priorRevision := currentRevision
	ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
		update.Spec.Template.Spec.Containers[0].Image = oldImage
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ss = sst.WaitForStatus(ss)
	currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
	gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
		"Current revision should not equal update revision during roll back")
	gomega.Expect(priorRevision).To(gomega.Equal(updateRevision),
		"Prior revision should equal update revision during roll back")

	ginkgo.By("Rolling back update in reverse ordinal order")
	pods = sst.GetPodList(ss)
	sst.SortStatefulPods(pods)
	_ = sst.RestorePodHTTPProbe(ss, &pods.Items[1])
	ss, pods = sst.WaitForPodReady(ss, pods.Items[1].Name)
	ss, pods = sst.WaitForRollingUpdate(ss)
	gomega.Expect(ss.Status.CurrentRevision).To(gomega.Equal(priorRevision),
		fmt.Sprintf("StatefulSet %s/%s current revision %s does not equal prior revision %s on rollback completion",
			ss.Namespace,
			ss.Name,
			ss.Status.CurrentRevision,
			updateRevision))

	for i := range pods.Items {
		gomega.Expect(pods.Items[i].Spec.Containers[0].Image).To(gomega.Equal(oldImage),
			fmt.Sprintf("Pod %s/%s has image %s not equal to previous image %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Spec.Containers[0].Image,
				oldImage))
		gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(priorRevision),
			fmt.Sprintf("Pod %s/%s revision %s is not equal to prior revision %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
				priorRevision))
	}
}

// verifyStatefulSetPVCsExist confirms that exactly the PVCs for ss with the specified ids exist. This polls until the situation occurs, an error happens, or until timeout (in the latter case an error is also returned). Beware that this cannot tell if a PVC will be deleted at some point in the future, so if used to confirm that no PVCs are deleted, the caller should wait for some event giving the PVCs a reasonable chance to be deleted, before calling this function.
func verifyStatefulSetPVCsExist(c clientset.Interface, ss *appsv1beta1.StatefulSet, claimIds []int) error {
	idSet := map[int]struct{}{}
	for _, id := range claimIds {
		idSet[id] = struct{}{}
	}
	return wait.PollUntilContextTimeout(context.Background(), framework.StatefulSetPoll, framework.StatefulSetTimeout, true, func(_ context.Context) (bool, error) {
		pvcList, err := c.CoreV1().PersistentVolumeClaims(ss.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: klabels.Everything().String()})
		if err != nil {
			framework.Logf("WARNING: Failed to list pvcs for verification, retrying: %v", err)
			return false, nil
		}
		for _, claim := range ss.Spec.VolumeClaimTemplates {
			pvcNameRE := regexp.MustCompile(fmt.Sprintf("^%s-%s-([0-9]+)$", claim.Name, ss.Name))
			seenPVCs := map[int]struct{}{}
			for _, pvc := range pvcList.Items {
				matches := pvcNameRE.FindStringSubmatch(pvc.Name)
				if len(matches) != 2 {
					continue
				}
				ordinal, err := strconv.ParseInt(matches[1], 10, 32)
				if err != nil {
					framework.Logf("ERROR: bad pvc name %s (%v)", pvc.Name, err)
					return false, err
				}
				if _, found := idSet[int(ordinal)]; !found {
					return false, nil // Retry until the PVCs are consistent.
				} else {
					seenPVCs[int(ordinal)] = struct{}{}
				}
			}
			if len(seenPVCs) != len(idSet) {
				framework.Logf("Found %d of %d PVCs", len(seenPVCs), len(idSet))
				return false, nil // Retry until the PVCs are consistent.
			}
		}
		return true, nil
	})
}

// verifyStatefulSetPVCsExistWithOwnerRefs works as verifyStatefulSetPVCsExist, but also waits for the ownerRefs to match.
func verifyStatefulSetPVCsExistWithOwnerRefs(c clientset.Interface, kc kruiseclientset.Interface, ss *appsv1beta1.StatefulSet, claimIndices []int, wantSetRef, wantPodRef bool) error {
	indexSet := map[int]struct{}{}
	for _, id := range claimIndices {
		indexSet[id] = struct{}{}
	}
	set, _ := kc.AppsV1beta1().StatefulSets(ss.Namespace).Get(context.TODO(), ss.Name, metav1.GetOptions{})
	setUID := set.GetUID()
	if setUID == "" {
		framework.Failf("Statefulset %s missing UID", ss.Name)
	}
	return wait.PollUntilContextTimeout(context.Background(), framework.StatefulSetPoll, framework.StatefulSetTimeout, true, func(_ context.Context) (bool, error) {
		pvcList, err := c.CoreV1().PersistentVolumeClaims(ss.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: klabels.Everything().String()})
		if err != nil {
			framework.Logf("WARNING: Failed to list pvcs for verification, retrying: %v", err)
			return false, nil
		}
		for _, claim := range ss.Spec.VolumeClaimTemplates {
			pvcNameRE := regexp.MustCompile(fmt.Sprintf("^%s-%s-([0-9]+)$", claim.Name, ss.Name))
			seenPVCs := map[int]struct{}{}
			for _, pvc := range pvcList.Items {
				matches := pvcNameRE.FindStringSubmatch(pvc.Name)
				if len(matches) != 2 {
					continue
				}
				ordinal, err := strconv.ParseInt(matches[1], 10, 32)
				if err != nil {
					framework.Logf("ERROR: bad pvc name %s (%v)", pvc.Name, err)
					return false, err
				}
				if _, found := indexSet[int(ordinal)]; !found {
					framework.Logf("Unexpected, retrying")
					return false, nil // Retry until the PVCs are consistent.
				}
				var foundSetRef, foundPodRef bool
				for _, ref := range pvc.GetOwnerReferences() {
					if ref.Kind == "StatefulSet" && ref.UID == setUID {
						foundSetRef = true
					}
					if ref.Kind == "Pod" {
						podName := fmt.Sprintf("%s-%d", ss.Name, ordinal)
						pod, err := c.CoreV1().Pods(ss.Namespace).Get(context.TODO(), podName, metav1.GetOptions{})
						if err != nil {
							framework.Logf("Pod %s not found, retrying (%v)", podName, err)
							return false, nil
						}
						podUID := pod.GetUID()
						if podUID == "" {
							framework.Failf("Pod %s is missing UID", pod.Name)
						}
						if ref.UID == podUID {
							foundPodRef = true
						}
					}
				}
				if foundSetRef == wantSetRef && foundPodRef == wantPodRef {
					seenPVCs[int(ordinal)] = struct{}{}
				}
			}
			if len(seenPVCs) != len(indexSet) {
				framework.Logf("Only %d PVCs, retrying", len(seenPVCs))
				return false, nil // Retry until the PVCs are consistent.
			}
		}
		return true, nil
	})
}

// getStatefulSetPodNameAtIndex gets formatted pod name given index.
func getStatefulSetPodNameAtIndex(index int, ss *appsv1beta1.StatefulSet) string {
	// TODO: we won't use "-index" as the name strategy forever,
	// pull the name out from an identity mapper.
	return fmt.Sprintf("%v-%v", ss.Name, index)
}

func uncordonNode(ctx context.Context, c clientset.Interface, oldData, newData []byte, nodeName string) {
	ginkgo.By("Uncordoning Node")
	// uncordon node, by reverting patch
	revertPatchBytes, err := strategicpatch.CreateTwoWayMergePatch(newData, oldData, v1.Node{})
	framework.ExpectNoError(err)
	_, err = c.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, revertPatchBytes, metav1.PatchOptions{})
	framework.ExpectNoError(err)
}

// waitForStatus waits for the StatefulSetStatus's CurrentReplicas to be equal to expectedReplicas
// The returned StatefulSet contains such a StatefulSetStatus
func waitForStatusCurrentReplicas(_ context.Context, c clientset.Interface, kc kruiseclientset.Interface, set *appsv1beta1.StatefulSet, expectedReplicas int32) *appsv1beta1.StatefulSet {
	sst := framework.NewStatefulSetTester(c, kc)
	sst.WaitForState(set, func(set2 *appsv1beta1.StatefulSet, pods *v1.PodList) (bool, error) {
		if set2.Status.ObservedGeneration >= set.Generation && set2.Status.CurrentReplicas == expectedReplicas {
			set = set2
			return true, nil
		}
		return false, nil
	})
	return set
}

// waitForStatus waits for the StatefulSetStatus's ObservedGeneration to be greater than or equal to set's Generation.
// The returned StatefulSet contains such a StatefulSetStatus
func waitForStatus(_ context.Context, c clientset.Interface, kc kruiseclientset.Interface, set *appsv1beta1.StatefulSet) *appsv1beta1.StatefulSet {
	sst := framework.NewStatefulSetTester(c, kc)
	sst.WaitForState(set, func(set2 *appsv1beta1.StatefulSet, pods *v1.PodList) (bool, error) {
		if set2.Status.ObservedGeneration >= set.Generation {
			set = set2
			return true, nil
		}
		return false, nil
	})
	return set
}

// waitForPodNames waits for the StatefulSet's pods to match expected names.
func waitForPodNames(_ context.Context, c clientset.Interface, kc kruiseclientset.Interface, set *appsv1beta1.StatefulSet, expectedPodNames []string) {
	sst := framework.NewStatefulSetTester(c, kc)
	sst.WaitForState(set,
		func(intSet *appsv1beta1.StatefulSet, pods *v1.PodList) (bool, error) {
			if err := expectPodNames(pods, expectedPodNames); err != nil {
				framework.Logf("Currently %v", err)
				return false, nil
			}
			return true, nil
		})
}

// expectPodNames compares the names of the pods from actualPods with expectedPodNames.
// actualPods can be in any list, since we'll sort by their ordinals and filter
// active ones. expectedPodNames should be ordered by statefulset ordinals.
func expectPodNames(actualPods *v1.PodList, expectedPodNames []string) error {
	framework.SortStatefulPods(actualPods)
	pods := []string{}
	for _, pod := range actualPods.Items {
		// ignore terminating pods, similarly to how the controller does it
		// when calculating status information
		if IsPodActive(&pod) {
			pods = append(pods, pod.Name)
		}
	}
	if !reflect.DeepEqual(expectedPodNames, pods) {
		diff := cmp.Diff(expectedPodNames, pods)
		return fmt.Errorf("pod names don't match, diff (- for expected, + for actual):\n%s", diff)
	}
	return nil
}

// IsPodActive return true if the pod meets certain conditions.
func IsPodActive(p *v1.Pod) bool {
	return v1.PodSucceeded != p.Status.Phase &&
		v1.PodFailed != p.Status.Phase &&
		p.DeletionTimestamp == nil
}

type updateStatefulSetFunc func(*appsv1beta1.StatefulSet)

// updateStatefulSetWithRetries updates statefulset template with retries.
func updateStatefulSetWithRetries(ctx context.Context, kc kruiseclientset.Interface, namespace, name string, applyUpdate updateStatefulSetFunc) (statefulSet *appsv1beta1.StatefulSet, err error) {
	statefulSets := kc.AppsV1beta1().StatefulSets(namespace)
	var updateErr error
	pollErr := wait.PollUntilContextTimeout(ctx, 10*time.Millisecond, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
		if statefulSet, err = statefulSets.Get(ctx, name, metav1.GetOptions{}); err != nil {
			return false, err
		}
		// Apply the update, then attempt to push it to the apiserver.
		applyUpdate(statefulSet)
		if statefulSet, err = statefulSets.Update(ctx, statefulSet, metav1.UpdateOptions{}); err == nil {
			framework.Logf("Updating stateful set %s", name)
			return true, nil
		}
		updateErr = err
		return false, nil
	})
	if wait.Interrupted(pollErr) {
		pollErr = fmt.Errorf("couldn't apply the provided updated to stateful set %q: %v", name, updateErr)
	}
	return statefulSet, pollErr
}

// waitForPVCCapacity waits for the StatefulSet's pods to match expected names.
func waitForPVCCapacity(_ context.Context, c clientset.Interface, kc kruiseclientset.Interface, set *appsv1beta1.StatefulSet, cmp func(resource.Quantity, resource.Quantity) bool) {
	sst := framework.NewStatefulSetTester(c, kc)
	capacityMap := map[string]resource.Quantity{}
	for _, pvc := range set.Spec.VolumeClaimTemplates {
		capacityMap[pvc.Name] = *pvc.Spec.Resources.Requests.Storage()
	}
	sst.WaitForPVCState(set,
		func(intSet *appsv1beta1.StatefulSet, pvcs *v1.PersistentVolumeClaimList) (bool, error) {
			for _, pvc := range pvcs.Items {
				templateName, err := framework.GetVolumeTemplateName(pvc.Name, set.Name)
				if err != nil {
					continue
				}
				if pvc.Status.Capacity != nil {
					capacity := pvc.Status.Capacity[v1.ResourceStorage]
					templateCap := capacityMap[templateName]
					framework.Logf("template %v, spec %v, status %v",
						templateCap.String(), pvc.Spec.Resources.Requests.Storage().String(), capacity.String())
					// status capacity == spec request
					if capacity.Cmp(*pvc.Spec.Resources.Requests.Storage()) != 0 {
						return false, nil
					}
					// status capacity == template request
					if !cmp(capacity, templateCap) {
						return false, nil
					}
				}
			}
			return true, nil
		})
}

// This function is used by two tests to test StatefulSet rollbacks: one using
// PVCs and one using no storage.
func testWithSpecifiedDeleted(c clientset.Interface, kc kruiseclientset.Interface, ns string, ss *appsv1beta1.StatefulSet,
	fns ...func(update *appsv1beta1.StatefulSet)) {
	sst := framework.NewStatefulSetTester(c, kc)
	*(ss.Spec.Replicas) = 4
	ss, err := kc.AppsV1beta1().StatefulSets(ns).Create(context.TODO(), ss, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	sst.WaitForRunningAndReady(*ss.Spec.Replicas, ss)
	ss = sst.WaitForStatus(ss)
	currentRevision, updateRevision := ss.Status.CurrentRevision, ss.Status.UpdateRevision
	gomega.Expect(currentRevision).To(gomega.Equal(updateRevision),
		fmt.Sprintf("StatefulSet %s/%s created with update revision %s not equal to current revision %s",
			ss.Namespace, ss.Name, updateRevision, currentRevision))
	pods := sst.GetPodList(ss)
	for i := range pods.Items {
		gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
			fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
				pods.Items[i].Namespace,
				pods.Items[i].Name,
				pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
				currentRevision))
	}
	specifiedDeletePod := func(idx int) {
		sst.SortStatefulPods(pods)
		oldUid := pods.Items[idx].UID
		err = setPodSpecifiedDelete(c, ns, pods.Items[idx].Name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ss = sst.WaitForStatus(ss)
		name := pods.Items[idx].Name
		// wait be deleted
		sst.WaitForState(ss, func(set2 *appsv1beta1.StatefulSet, pods2 *v1.PodList) (bool, error) {
			ss = set2
			pods = pods2
			for i := range pods.Items {
				if pods.Items[i].Name == name {
					return pods.Items[i].UID != oldUid, nil
				}
			}
			return false, nil
		})
		sst.WaitForPodReady(ss, pods.Items[idx].Name)
		pods = sst.GetPodList(ss)
		sst.SortStatefulPods(pods)
	}
	specifiedDeletePod(1)
	newImage := NewNginxImage
	oldImage := ss.Spec.Template.Spec.Containers[0].Image

	ginkgo.By(fmt.Sprintf("Updating StatefulSet template: update image from %s to %s", oldImage, newImage))
	gomega.Expect(oldImage).NotTo(gomega.Equal(newImage), "Incorrect test setup: should update to a different image")
	var partition int32 = 2
	ss, err = framework.UpdateStatefulSetWithRetries(kc, ns, ss.Name, func(update *appsv1beta1.StatefulSet) {
		update.Spec.Template.Spec.Containers[0].Image = newImage
		if update.Spec.UpdateStrategy.RollingUpdate == nil {
			update.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{}
		}
		update.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateStatefulSetStrategy{
			Partition: &partition,
		}
		for _, fn := range fns {
			fn(update)
		}
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	specifiedDeletePod(2)

	ginkgo.By("Creating a new revision")
	ss = sst.WaitForStatus(ss)
	currentRevision, updateRevision = ss.Status.CurrentRevision, ss.Status.UpdateRevision
	gomega.Expect(currentRevision).NotTo(gomega.Equal(updateRevision),
		"Current revision should not equal update revision during rolling update")
	specifiedDeletePod(1)
	for i := range pods.Items {
		if i >= int(partition) {
			gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(updateRevision),
				fmt.Sprintf("Pod %s/%s revision %s is not equal to updated revision %s",
					pods.Items[i].Namespace,
					pods.Items[i].Name,
					pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
					updateRevision))
		} else {
			gomega.Expect(pods.Items[i].Labels[apps.StatefulSetRevisionLabel]).To(gomega.Equal(currentRevision),
				fmt.Sprintf("Pod %s/%s revision %s is not equal to current revision %s",
					pods.Items[i].Namespace,
					pods.Items[i].Name,
					pods.Items[i].Labels[apps.StatefulSetRevisionLabel],
					currentRevision))
		}
	}
}

func setPodSpecifiedDelete(c clientset.Interface, ns, name string) error {
	_, err := c.CoreV1().Pods(ns).Patch(context.TODO(), name, types.StrategicMergePatchType, []byte(`{"metadata":{"labels":{"apps.kruise.io/specified-delete":"true"}}}`), metav1.PatchOptions{})
	return err
}
