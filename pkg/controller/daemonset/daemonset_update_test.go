/*
Copyright 2020 The Kruise Authors.

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

package daemonset

import (
	"context"
	"reflect"
	"testing"
	"time"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/controller/daemon/util"
	testingclock "k8s.io/utils/clock/testing"
	utilpointer "k8s.io/utils/pointer"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

func TestDaemonSetUpdatesPods(t *testing.T) {
	ds := newDaemonSet("foo")
	manager, podControl, _, err := newTestController(ds)
	if err != nil {
		t.Fatalf("error creating DaemonSets controller: %v", err)
	}
	maxUnavailable := 2
	addNodes(manager.nodeStore, 0, 5, nil)
	manager.dsStore.Add(ds)
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)
	markPodsReady(podControl.podStore)

	ds.Spec.Template.Spec.Containers[0].Image = "foo2/bar2"
	ds.Spec.UpdateStrategy.Type = appsv1alpha1.RollingUpdateDaemonSetStrategyType
	intStr := intstr.FromInt(maxUnavailable)
	ds.Spec.UpdateStrategy.RollingUpdate = &appsv1alpha1.RollingUpdateDaemonSet{MaxUnavailable: &intStr}
	manager.dsStore.Update(ds)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, maxUnavailable, 0)
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, maxUnavailable, 0, 0)
	markPodsReady(podControl.podStore)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, maxUnavailable, 0)
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, maxUnavailable, 0, 0)
	markPodsReady(podControl.podStore)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 1, 0)
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 1, 0, 0)
	markPodsReady(podControl.podStore)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 0, 0)
	clearExpectations(t, manager, ds, podControl)
}

func TestDaemonSetUpdatesPodsWithMaxSurge(t *testing.T) {
	ds := newDaemonSet("foo")
	manager, podControl, _, err := newTestController(ds)
	if err != nil {
		t.Fatalf("error creating DaemonSets controller: %v", err)
	}
	addNodes(manager.nodeStore, 0, 5, nil)
	manager.dsStore.Add(ds)
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)
	markPodsReady(podControl.podStore)

	// surge is the controlling amount
	maxSurge := 2
	ds.Spec.Template.Spec.Containers[0].Image = "foo2/bar2"
	ds.Spec.UpdateStrategy = newUpdateSurge(intstr.FromInt(maxSurge))
	manager.dsStore.Update(ds)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, maxSurge, 0, 0)
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 0, 0)
	markPodsReady(podControl.podStore)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, maxSurge, maxSurge, 0)
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 0, 0)
	markPodsReady(podControl.podStore)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 5%maxSurge, maxSurge, 0)
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 0, 0)
	markPodsReady(podControl.podStore)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 5%maxSurge, 0)
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 0, 0)
}

func TestDaemonSetUpdatesWhenNewPosIsNotReady(t *testing.T) {
	ds := newDaemonSet("foo")
	manager, podControl, _, err := newTestController(ds)
	if err != nil {
		t.Fatalf("error creating DaemonSets controller: %v", err)
	}
	maxUnavailable := 3
	addNodes(manager.nodeStore, 0, 5, nil)
	err = manager.dsStore.Add(ds)
	if err != nil {
		t.Fatal(err)
	}
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)
	markPodsReady(podControl.podStore)

	ds.Spec.Template.Spec.Containers[0].Image = "foo2/bar2"
	ds.Spec.UpdateStrategy.Type = appsv1alpha1.RollingUpdateDaemonSetStrategyType
	intStr := intstr.FromInt(maxUnavailable)
	ds.Spec.UpdateStrategy.RollingUpdate = &appsv1alpha1.RollingUpdateDaemonSet{MaxUnavailable: &intStr}
	err = manager.dsStore.Update(ds)
	if err != nil {
		t.Fatal(err)
	}

	// new pods are not ready numUnavailable == maxUnavailable
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, maxUnavailable, 0)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, maxUnavailable, 0, 0)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 0, 0)
	clearExpectations(t, manager, ds, podControl)
}

func TestDaemonSetUpdatesAllOldPodsNotReady(t *testing.T) {
	ds := newDaemonSet("foo")
	manager, podControl, _, err := newTestController(ds)
	if err != nil {
		t.Fatalf("error creating DaemonSets controller: %v", err)
	}
	maxUnavailable := 3
	addNodes(manager.nodeStore, 0, 5, nil)
	err = manager.dsStore.Add(ds)
	if err != nil {
		t.Fatal(err)
	}
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)

	ds.Spec.Template.Spec.Containers[0].Image = "foo2/bar2"
	ds.Spec.UpdateStrategy.Type = appsv1alpha1.RollingUpdateDaemonSetStrategyType
	intStr := intstr.FromInt(maxUnavailable)
	ds.Spec.UpdateStrategy.RollingUpdate = &appsv1alpha1.RollingUpdateDaemonSet{MaxUnavailable: &intStr}
	err = manager.dsStore.Update(ds)
	if err != nil {
		t.Fatal(err)
	}

	// all old pods are unavailable so should be removed
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 5, 0)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 0, 0)
	clearExpectations(t, manager, ds, podControl)
}

func TestDaemonSetUpdatesAllOldPodsNotReadyMaxSurge(t *testing.T) {
	ds := newDaemonSet("foo")
	manager, podControl, _, err := newTestController(ds)
	if err != nil {
		t.Fatalf("error creating DaemonSets controller: %v", err)
	}
	addNodes(manager.nodeStore, 0, 5, nil)
	manager.dsStore.Add(ds)
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)

	maxSurge := 3
	ds.Spec.Template.Spec.Containers[0].Image = "foo2/bar2"
	ds.Spec.UpdateStrategy = newUpdateSurge(intstr.FromInt(maxSurge))
	manager.dsStore.Update(ds)

	// all old pods are unavailable so should be surged
	manager.failedPodsBackoff.Clock = testingclock.NewFakeClock(time.Unix(100, 0))
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)

	// waiting for pods to go ready, old pods are deleted
	manager.failedPodsBackoff.Clock = testingclock.NewFakeClock(time.Unix(200, 0))
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 5, 0)

	setPodReadiness(t, manager, true, 5, func(_ *corev1.Pod) bool { return true })
	ds.Spec.MinReadySeconds = 15
	ds.Spec.Template.Spec.Containers[0].Image = "foo3/bar3"
	manager.dsStore.Update(ds)

	manager.failedPodsBackoff.Clock = testingclock.NewFakeClock(time.Unix(300, 0))
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 3, 0, 0)

	hash, err := currentDSHash(context.TODO(), manager, ds)
	if err != nil {
		t.Fatal(err)
	}
	currentPods := podsByNodeMatchingHash(manager, hash)
	// mark two updated pods as ready at time 300
	setPodReadiness(t, manager, true, 2, func(pod *corev1.Pod) bool {
		return pod.Labels[apps.ControllerRevisionHashLabelKey] == hash
	})
	// mark one of the old pods that is on a node without an updated pod as unready
	setPodReadiness(t, manager, false, 1, func(pod *corev1.Pod) bool {
		nodeName, err := util.GetTargetNodeName(pod)
		if err != nil {
			t.Fatal(err)
		}
		return pod.Labels[apps.ControllerRevisionHashLabelKey] != hash && len(currentPods[nodeName]) == 0
	})

	// the new pods should still be considered waiting to hit min readiness, so one pod should be created to replace
	// the deleted old pod
	manager.failedPodsBackoff.Clock = testingclock.NewFakeClock(time.Unix(310, 0))
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 1, 0, 0)

	// the new pods are now considered available, so delete the old pods
	manager.failedPodsBackoff.Clock = testingclock.NewFakeClock(time.Unix(320, 0))
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 1, 3, 0)

	// mark all updated pods as ready at time 320
	currentPods = podsByNodeMatchingHash(manager, hash)
	setPodReadiness(t, manager, true, 3, func(pod *corev1.Pod) bool {
		return pod.Labels[apps.ControllerRevisionHashLabelKey] == hash
	})

	// the new pods are now considered available, so delete the old pods
	manager.failedPodsBackoff.Clock = testingclock.NewFakeClock(time.Unix(340, 0))
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 2, 0)

	// controller has completed upgrade
	manager.failedPodsBackoff.Clock = testingclock.NewFakeClock(time.Unix(350, 0))
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 0, 0)
}

func TestDaemonSetUpdatesNoTemplateChanged(t *testing.T) {
	ds := newDaemonSet("foo")
	manager, podControl, _, err := newTestController(ds)
	if err != nil {
		t.Fatalf("error creating DaemonSets controller: %v", err)
	}
	maxUnavailable := 3
	addNodes(manager.nodeStore, 0, 5, nil)
	manager.dsStore.Add(ds)
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)

	ds.Spec.UpdateStrategy.Type = appsv1alpha1.RollingUpdateDaemonSetStrategyType
	intStr := intstr.FromInt(maxUnavailable)
	ds.Spec.UpdateStrategy.RollingUpdate = &appsv1alpha1.RollingUpdateDaemonSet{MaxUnavailable: &intStr}
	manager.dsStore.Update(ds)

	// template is not changed no pod should be removed
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 0, 0)
	clearExpectations(t, manager, ds, podControl)
}

func podsByNodeMatchingHash(dsc *daemonSetsController, hash string) map[string][]string {
	byNode := make(map[string][]string)
	for _, obj := range dsc.podStore.List() {
		pod := obj.(*corev1.Pod)
		if pod.Labels[apps.ControllerRevisionHashLabelKey] != hash {
			continue
		}
		nodeName, err := util.GetTargetNodeName(pod)
		if err != nil {
			panic(err)
		}
		byNode[nodeName] = append(byNode[nodeName], pod.Name)
	}
	return byNode
}

func setPodReadiness(t *testing.T, dsc *daemonSetsController, ready bool, count int, fn func(*corev1.Pod) bool) {
	t.Helper()
	for _, obj := range dsc.podStore.List() {
		if count <= 0 {
			break
		}
		pod := obj.(*corev1.Pod)
		if pod.DeletionTimestamp != nil {
			continue
		}
		if podutil.IsPodReady(pod) == ready {
			continue
		}
		if !fn(pod) {
			continue
		}
		condition := corev1.PodCondition{Type: corev1.PodReady}
		if ready {
			condition.Status = corev1.ConditionTrue
		} else {
			condition.Status = corev1.ConditionFalse
		}
		if !podutil.UpdatePodCondition(&pod.Status, &condition) {
			t.Fatal("failed to update pod")
		}
		// TODO: workaround UpdatePodCondition calling time.Now() directly
		setCondition := podutil.GetPodReadyCondition(pod.Status)
		setCondition.LastTransitionTime.Time = dsc.failedPodsBackoff.Clock.Now()
		klog.InfoS("Marked pod ready status", "pod", klog.KObj(pod), "ready", ready)
		count--
	}
	if count > 0 {
		t.Fatalf("could not mark %d pods ready=%t", count, ready)
	}
}

func currentDSHash(ctx context.Context, dsc *daemonSetsController, ds *appsv1alpha1.DaemonSet) (string, error) {
	// Construct histories of the DaemonSet, and get the hash of current history
	cur, _, err := dsc.constructHistory(ctx, ds)
	if err != nil {
		return "", err
	}
	return cur.Labels[apps.DefaultDaemonSetUniqueLabelKey], nil
}

func newUpdateSurge(value intstr.IntOrString) appsv1alpha1.DaemonSetUpdateStrategy {
	zero := intstr.FromInt(0)
	return appsv1alpha1.DaemonSetUpdateStrategy{
		Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
			MaxUnavailable: &zero,
			MaxSurge:       &value,
		},
	}
}

func newUpdateUnavailable(value intstr.IntOrString) appsv1alpha1.DaemonSetUpdateStrategy {
	return appsv1alpha1.DaemonSetUpdateStrategy{
		Type: appsv1alpha1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appsv1alpha1.RollingUpdateDaemonSet{
			MaxUnavailable: &value,
		},
	}
}

func TestGetUnavailableNumbers(t *testing.T) {
	cases := []struct {
		name           string
		Manager        *daemonSetsController
		ds             *appsv1alpha1.DaemonSet
		nodeToPods     map[string][]*corev1.Pod
		enableSurge    bool
		maxSurge       int
		maxUnavailable int
		emptyNodes     int
		Err            error
	}{
		{
			name: "No nodes",
			Manager: func() *daemonSetsController {
				manager, _, _, err := newTestController()
				if err != nil {
					t.Fatalf("error creating DaemonSets controller: %v", err)
				}
				return manager
			}(),
			ds: func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("x")
				ds.Spec.UpdateStrategy = newUpdateUnavailable(intstr.FromInt(0))
				return ds
			}(),
			nodeToPods:     make(map[string][]*corev1.Pod),
			maxUnavailable: 0,
			emptyNodes:     0,
		},
		{
			name: "Two nodes with ready pods",
			Manager: func() *daemonSetsController {
				manager, _, _, err := newTestController()
				if err != nil {
					t.Fatalf("error creating DaemonSets controller: %v", err)
				}
				addNodes(manager.nodeStore, 0, 2, nil)
				return manager
			}(),
			ds: func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("x")
				ds.Spec.UpdateStrategy = newUpdateUnavailable(intstr.FromInt(1))
				return ds
			}(),
			nodeToPods: func() map[string][]*corev1.Pod {
				mapping := make(map[string][]*corev1.Pod)
				pod0 := newPod("pod-0", "node-0", simpleDaemonSetLabel, nil)
				pod1 := newPod("pod-1", "node-1", simpleDaemonSetLabel, nil)
				markPodReady(pod0)
				markPodReady(pod1)
				mapping["node-0"] = []*corev1.Pod{pod0}
				mapping["node-1"] = []*corev1.Pod{pod1}
				return mapping
			}(),
			maxUnavailable: 1,
			emptyNodes:     0,
		},
		{
			name: "Two nodes, one node without pods",
			Manager: func() *daemonSetsController {
				manager, _, _, err := newTestController()
				if err != nil {
					t.Fatalf("error creating DaemonSets controller: %v", err)
				}
				addNodes(manager.nodeStore, 0, 2, nil)
				return manager
			}(),
			ds: func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("x")
				ds.Spec.UpdateStrategy = newUpdateUnavailable(intstr.FromInt(0))
				return ds
			}(),
			nodeToPods: func() map[string][]*corev1.Pod {
				mapping := make(map[string][]*corev1.Pod)
				pod0 := newPod("pod-0", "node-0", simpleDaemonSetLabel, nil)
				markPodReady(pod0)
				mapping["node-0"] = []*corev1.Pod{pod0}
				return mapping
			}(),
			maxUnavailable: 1,
			emptyNodes:     1,
		},
		{
			name: "Two nodes, one node without pods, surge",
			Manager: func() *daemonSetsController {
				manager, _, _, err := newTestController()
				if err != nil {
					t.Fatalf("error creating DaemonSets controller: %v", err)
				}
				addNodes(manager.nodeStore, 0, 2, nil)
				return manager
			}(),
			ds: func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("x")
				ds.Spec.UpdateStrategy = newUpdateSurge(intstr.FromInt(0))
				return ds
			}(),
			nodeToPods: func() map[string][]*corev1.Pod {
				mapping := make(map[string][]*corev1.Pod)
				pod0 := newPod("pod-0", "node-0", simpleDaemonSetLabel, nil)
				markPodReady(pod0)
				mapping["node-0"] = []*corev1.Pod{pod0}
				return mapping
			}(),
			maxUnavailable: 1,
			emptyNodes:     1,
		},
		{
			name: "Two nodes with pods, MaxUnavailable in percents",
			Manager: func() *daemonSetsController {
				manager, _, _, err := newTestController()
				if err != nil {
					t.Fatalf("error creating DaemonSets controller: %v", err)
				}
				addNodes(manager.nodeStore, 0, 2, nil)
				return manager
			}(),
			ds: func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("x")
				ds.Spec.UpdateStrategy = newUpdateUnavailable(intstr.FromString("50%"))
				return ds
			}(),
			nodeToPods: func() map[string][]*corev1.Pod {
				mapping := make(map[string][]*corev1.Pod)
				pod0 := newPod("pod-0", "node-0", simpleDaemonSetLabel, nil)
				pod1 := newPod("pod-1", "node-1", simpleDaemonSetLabel, nil)
				markPodReady(pod0)
				markPodReady(pod1)
				mapping["node-0"] = []*corev1.Pod{pod0}
				mapping["node-1"] = []*corev1.Pod{pod1}
				return mapping
			}(),
			maxUnavailable: 1,
			emptyNodes:     0,
		},
		{
			name: "Two nodes with pods, MaxUnavailable in percents, surge",
			Manager: func() *daemonSetsController {
				manager, _, _, err := newTestController()
				if err != nil {
					t.Fatalf("error creating DaemonSets controller: %v", err)
				}
				addNodes(manager.nodeStore, 0, 2, nil)
				return manager
			}(),
			ds: func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("x")
				ds.Spec.UpdateStrategy = newUpdateSurge(intstr.FromString("50%"))
				return ds
			}(),
			nodeToPods: func() map[string][]*corev1.Pod {
				mapping := make(map[string][]*corev1.Pod)
				pod0 := newPod("pod-0", "node-0", simpleDaemonSetLabel, nil)
				pod1 := newPod("pod-1", "node-1", simpleDaemonSetLabel, nil)
				markPodReady(pod0)
				markPodReady(pod1)
				mapping["node-0"] = []*corev1.Pod{pod0}
				mapping["node-1"] = []*corev1.Pod{pod1}
				return mapping
			}(),
			enableSurge:    true,
			maxSurge:       1,
			maxUnavailable: 0,
			emptyNodes:     0,
		},
		{
			name: "Two nodes with pods, MaxUnavailable is 100%, surge",
			Manager: func() *daemonSetsController {
				manager, _, _, err := newTestController()
				if err != nil {
					t.Fatalf("error creating DaemonSets controller: %v", err)
				}
				addNodes(manager.nodeStore, 0, 2, nil)
				return manager
			}(),
			ds: func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("x")
				ds.Spec.UpdateStrategy = newUpdateSurge(intstr.FromString("100%"))
				return ds
			}(),
			nodeToPods: func() map[string][]*corev1.Pod {
				mapping := make(map[string][]*corev1.Pod)
				pod0 := newPod("pod-0", "node-0", simpleDaemonSetLabel, nil)
				pod1 := newPod("pod-1", "node-1", simpleDaemonSetLabel, nil)
				markPodReady(pod0)
				markPodReady(pod1)
				mapping["node-0"] = []*corev1.Pod{pod0}
				mapping["node-1"] = []*corev1.Pod{pod1}
				return mapping
			}(),
			enableSurge:    true,
			maxSurge:       2,
			maxUnavailable: 0,
			emptyNodes:     0,
		},
		{
			name: "Two nodes with pods, MaxUnavailable in percents, pod terminating",
			Manager: func() *daemonSetsController {
				manager, _, _, err := newTestController()
				if err != nil {
					t.Fatalf("error creating DaemonSets controller: %v", err)
				}
				addNodes(manager.nodeStore, 0, 3, nil)
				return manager
			}(),
			ds: func() *appsv1alpha1.DaemonSet {
				ds := newDaemonSet("x")
				ds.Spec.UpdateStrategy = newUpdateUnavailable(intstr.FromString("50%"))
				return ds
			}(),
			nodeToPods: func() map[string][]*corev1.Pod {
				mapping := make(map[string][]*corev1.Pod)
				pod0 := newPod("pod-0", "node-0", simpleDaemonSetLabel, nil)
				pod1 := newPod("pod-1", "node-1", simpleDaemonSetLabel, nil)
				now := metav1.Now()
				markPodReady(pod0)
				markPodReady(pod1)
				pod1.DeletionTimestamp = &now
				mapping["node-0"] = []*corev1.Pod{pod0}
				mapping["node-1"] = []*corev1.Pod{pod1}
				return mapping
			}(),
			maxUnavailable: 2,
			emptyNodes:     1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			c.Manager.dsStore.Add(c.ds)
			nodeList, err := c.Manager.nodeLister.List(labels.Everything())
			if err != nil {
				t.Fatalf("error listing nodes: %v", err)
			}
			maxSurge, maxUnavailable, err := c.Manager.updatedDesiredNodeCounts(c.ds, nodeList, c.nodeToPods)
			if err != nil && c.Err != nil {
				if c.Err != err {
					t.Fatalf("Expected error: %v but got: %v", c.Err, err)
				}
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if maxSurge != c.maxSurge || maxUnavailable != c.maxUnavailable {
				t.Errorf("Wrong values. maxSurge: %d, expected %d, maxUnavailable: %d, expected: %d", maxSurge, c.maxSurge, maxUnavailable, c.maxUnavailable)
			}
			var emptyNodes int
			for _, pods := range c.nodeToPods {
				if len(pods) == 0 {
					emptyNodes++
				}
			}
			if emptyNodes != c.emptyNodes {
				t.Errorf("expected numEmpty to be %d, was %d", c.emptyNodes, emptyNodes)
			}
		})
	}
}

func Test_maxRevision(t *testing.T) {
	type args struct {
		histories []*apps.ControllerRevision
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "GetMaxRevision",
			args: args{
				histories: []*apps.ControllerRevision{
					{
						Revision: 123456789,
					},
					{
						Revision: 213456789,
					},
					{
						Revision: 312456789,
					},
				},
			},
			want: 312456789,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maxRevision(tt.args.histories); got != tt.want {
				t.Errorf("maxRevision() = %v, want %v", got, tt.want)
			}
			t.Logf("maxRevision() = %v", tt.want)
		})
	}
}

func TestGetTemplateGeneration(t *testing.T) {
	type args struct {
		ds *appsv1alpha1.DaemonSet
	}
	constNum := int64(1000)
	tests := []struct {
		name    string
		args    args
		want    *int64
		wantErr bool
	}{
		{
			name: "GetTemplateGeneration",
			args: args{
				ds: &appsv1alpha1.DaemonSet{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							apps.DeprecatedTemplateGeneration: "1000",
						},
					},
					Spec:   appsv1alpha1.DaemonSetSpec{},
					Status: appsv1alpha1.DaemonSetStatus{},
				},
			},
			want:    &constNum,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetTemplateGeneration(tt.args.ds)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTemplateGeneration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if *got != *tt.want {
				t.Errorf("GetTemplateGeneration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterDaemonPodsNodeToUpdate(t *testing.T) {
	now := metav1.Now()
	type testcase struct {
		name             string
		rolling          *appsv1alpha1.RollingUpdateDaemonSet
		hash             string
		nodeToDaemonPods map[string][]*corev1.Pod
		nodes            []*corev1.Node
		expectNodes      []string
	}

	tests := []testcase{
		{
			name: "Standard,partition=0",
			rolling: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:      appsv1alpha1.StandardRollingUpdateType,
				Partition: utilpointer.Int32Ptr(0),
			},
			hash: "v2",
			nodeToDaemonPods: map[string][]*corev1.Pod{
				"n1": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n2": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v2"}}},
				},
				"n3": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
			},
			expectNodes: []string{"n2", "n3", "n1"},
		},
		{
			name: "Standard,partition=1",
			rolling: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:      appsv1alpha1.StandardRollingUpdateType,
				Partition: utilpointer.Int32Ptr(1),
			},
			hash: "v2",
			nodeToDaemonPods: map[string][]*corev1.Pod{
				"n1": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v2"}}},
				},
				"n2": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n3": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
			},
			expectNodes: []string{"n1", "n3"},
		},
		{
			name: "Standard,partition=1,selector=1",
			rolling: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:      appsv1alpha1.StandardRollingUpdateType,
				Partition: utilpointer.Int32Ptr(1),
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"node-type": "canary"}},
			},
			hash: "v2",
			nodeToDaemonPods: map[string][]*corev1.Pod{
				"n1": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n2": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v2"}}},
				},
				"n3": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n4": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
			},
			nodes: []*corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n3", Labels: map[string]string{"node-type": "canary"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n4"}},
			},
			expectNodes: []string{"n2", "n3"},
		},
		{
			name: "Standard,partition=2,selector=3",
			rolling: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:      appsv1alpha1.StandardRollingUpdateType,
				Partition: utilpointer.Int32Ptr(2),
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"node-type": "canary"}},
			},
			hash: "v2",
			nodeToDaemonPods: map[string][]*corev1.Pod{
				"n1": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n2": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v2"}}},
				},
				"n3": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n4": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
			},
			nodes: []*corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"node-type": "canary"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n3", Labels: map[string]string{"node-type": "canary"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n4", Labels: map[string]string{"node-type": "canary"}}},
			},
			expectNodes: []string{"n2", "n4"},
		},
		{
			name: "Standard,partition=0,selector=3,terminating",
			rolling: &appsv1alpha1.RollingUpdateDaemonSet{
				Type:      appsv1alpha1.StandardRollingUpdateType,
				Partition: utilpointer.Int32Ptr(0),
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"node-type": "canary"}},
			},
			hash: "v2",
			nodeToDaemonPods: map[string][]*corev1.Pod{
				"n1": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
				"n2": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v2"}}},
				},
				"n3": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}, DeletionTimestamp: &now}},
				},
				"n4": {
					{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{apps.DefaultDaemonSetUniqueLabelKey: "v1"}}},
				},
			},
			nodes: []*corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"node-type": "canary"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n3", Labels: map[string]string{"node-type": "canary"}}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n4"}},
			},
			expectNodes: []string{"n2", "n3", "n1"},
		},
	}

	testFn := func(test *testcase, t *testing.T) {
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
		for _, node := range test.nodes {
			if err := indexer.Add(node); err != nil {
				t.Fatalf("failed to add node into indexer: %v", err)
			}
		}
		nodeLister := corelisters.NewNodeLister(indexer)
		dsc := &ReconcileDaemonSet{nodeLister: nodeLister}

		ds := &appsv1alpha1.DaemonSet{Spec: appsv1alpha1.DaemonSetSpec{UpdateStrategy: appsv1alpha1.DaemonSetUpdateStrategy{
			Type:          appsv1alpha1.RollingUpdateDaemonSetStrategyType,
			RollingUpdate: test.rolling,
		}}}
		got, err := dsc.filterDaemonPodsNodeToUpdate(ds, test.hash, test.nodeToDaemonPods)
		if err != nil {
			t.Fatalf("failed to call filterDaemonPodsNodeToUpdate: %v", err)
		}
		if !reflect.DeepEqual(got, test.expectNodes) {
			t.Fatalf("expected %v, got %v", test.expectNodes, got)
		}
	}

	for i := range tests {
		t.Run(tests[i].name, func(t *testing.T) {
			testFn(&tests[i], t)
		})
	}
}
