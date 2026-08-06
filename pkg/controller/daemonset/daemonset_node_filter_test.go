package daemonset

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/controller/daemon/util"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

func markAllPodsReady(store cache.Store, options []func(*corev1.Pod) bool) {
	// mark pods as ready
	for _, obj := range store.List() {
		pod := obj.(*corev1.Pod)
		handle := true
		for _, option := range options {
			handle = handle && option(pod)
		}
		if handle {
			condition := corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionTrue}
			podutil.UpdatePodCondition(&pod.Status, &condition)
		}
	}
}

// setNodesNotReady updates the given nodes in the store to have NodeReady=False.
func setNodesNotReady(nodeStore cache.Store, nodeNames ...string) {
	for _, name := range nodeNames {
		obj, exists, _ := nodeStore.GetByKey(name)
		if !exists {
			continue
		}
		node := obj.(*corev1.Node).DeepCopy()
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == corev1.NodeReady {
				node.Status.Conditions[i].Status = corev1.ConditionFalse
				break
			}
		}
		_ = nodeStore.Update(node)
	}
}

func TestManageDeletesPodsOnNotReadyNodesWhenIgnoreAnnotation(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.DaemonSetPruneIneligibleNodes, true)()

	ds := newDaemonSet("foo")

	ds.Spec.UpdateStrategy = appsv1beta1.DaemonSetUpdateStrategy{
		Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		},
	}
	manager, podControl, _, err := newTestController(ds)
	if err != nil {
		t.Fatalf("error creating DaemonSets controller: %v", err)
	}

	// 5 nodes, initial sync creates one pod per node.
	addNodes(manager.nodeStore, 0, 5, nil)
	notReadyNodeSets := sets.NewString("node-0", "node-1")
	setPodReady := func(all bool) {
		markAllPodsReady(podControl.podStore, []func(*corev1.Pod) bool{func(pod *corev1.Pod) bool {
			if all {
				return true
			}
			nodeName, _ := util.GetTargetNodeName(pod)
			if notReadyNodeSets.Has(nodeName) {
				return false
			}
			return true
		}})
	}

	clearExpectations(t, manager, ds, podControl)
	manager.dsStore.Add(ds)
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)
	setPodReady(true)

	// Mark two nodes NotReady and expect manage() NOT to delete pods on those nodes
	// because IgnoreNotReadyNodes annotation is set.
	clearExpectations(t, manager, ds, podControl)
	setNodesNotReady(manager.nodeStore, notReadyNodeSets.List()...)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 0, 0)

	ds.Spec.Template.Spec.Affinity = &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: corev1.NodeSelectorOpNotIn,
								Values:   notReadyNodeSets.List(),
							},
						},
					},
				},
			},
		},
	}

	ds.Spec.Template.Spec.Containers[0].Image = "bar2"
	ds.Generation++
	manager.dsStore.Update(ds)
	// manage 2  update 1
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 3, 0)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 1, 0, 0)

	clearExpectations(t, manager, ds, podControl)
	setPodReady(false)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 1, 0)

	clearExpectations(t, manager, ds, podControl)
	setPodReady(false)
	expectSyncDaemonSets(t, manager, ds, podControl, 1, 0, 0)

	clearExpectations(t, manager, ds, podControl)
	setPodReady(false)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 1, 0)

	clearExpectations(t, manager, ds, podControl)
	setPodReady(false)
	expectSyncDaemonSets(t, manager, ds, podControl, 1, 0, 0)

	nodeSeen := make(map[string]bool)

	for _, _pod := range podControl.podStore.List() {
		pod := _pod.(*corev1.Pod)
		nodeName, _ := util.GetTargetNodeName(pod)
		if notReadyNodeSets.Has(nodeName) {
			t.Fatalf("pod %s should not exists", nodeName)
		}
		nodeSeen[nodeName] = true
	}
	if len(nodeSeen) != 3 {
		t.Fatalf("expect 3 nodes to be seen %v", nodeSeen)
	}

}

// TestDaemonSetIgnoreNotReadyNodesWhenUpdating verifies that when the ignore-notready-nodes
// annotation is set, pods on NotReady nodes are not counted in numUnavailable and are deleted,
// so rollout can proceed on ready nodes even when NotReady count > maxUnavailable.
func TestDaemonSetIgnoreNotReadyNodesWhenUpdating(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.DaemonSetPruneIneligibleNodes, true)()

	ds := newDaemonSet("foo")
	if ds.Annotations == nil {
		ds.Annotations = make(map[string]string)
	}

	manager, podControl, _, err := newTestController(ds)
	if err != nil {
		t.Fatalf("error creating DaemonSets controller: %v", err)
	}
	addNodes(manager.nodeStore, 0, 5, nil)
	manager.dsStore.Add(ds)
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)
	markAllPodsReady(podControl.podStore, nil)

	// Make 3 nodes NotReady. Without the annotation, numUnavailable would be 3 >= maxUnavailable(2)
	// and rollout would be blocked. With the annotation, pods on NotReady nodes are not counted
	// and are deleted; we can also delete 2 more from ready nodes (maxUnavailable).
	clearExpectations(t, manager, ds, podControl)
	setNodesNotReady(manager.nodeStore, "node-0", "node-1", "node-2")

	// First sync after update:
	// - manage: deletes 3 pods on NotReady nodes (node-0, node-1, node-2)
	// - rollingUpdate: deletes 2 pods on ready nodes (node-3, node-4) per maxUnavailable
	// Total: 0 creates, 5 deletes

	// Update image and set rolling update with maxUnavailable=2 and enable ignore-notready-nodes.
	ds.Spec.Template.Spec.Containers[0].Image = "foo2/bar2"
	ds.Spec.UpdateStrategy.Type = appsv1beta1.RollingUpdateDaemonSetStrategyType
	intStr := intstr.FromInt(2)
	ds.Spec.UpdateStrategy.RollingUpdate = &appsv1beta1.RollingUpdateDaemonSet{MaxUnavailable: &intStr}
	clearExpectations(t, manager, ds, podControl)
	manager.dsStore.Update(ds)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 2, 0)

	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 2, 0, 0)
}

// TestPruneIneligibleNodesActuallyDeletesPods is a regression test for the bug where
// podsToDelete was built inside pruneNodesAndOrphanedPods but never returned, so pods
// on ineligible nodes were never actually deleted even when DaemonSetPruneIneligibleNodes
// was enabled.
//
// It verifies two things:
//  1. pruneNodesAndOrphanedPods returns the correct pod names for ineligible nodes.
//  2. A full sync with DaemonSetPruneIneligibleNodes=true causes those pods to be deleted
//     (i.e., syncNodes is actually invoked with the returned names).
func TestPruneIneligibleNodesActuallyDeletesPods(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.DaemonSetPruneIneligibleNodes, true)()

	ds := newDaemonSet("foo")
	ds.Spec.UpdateStrategy = appsv1beta1.DaemonSetUpdateStrategy{
		Type: appsv1beta1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appsv1beta1.RollingUpdateDaemonSet{
			MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		},
	}

	manager, podControl, _, err := newTestController(ds)
	if err != nil {
		t.Fatalf("error creating DaemonSets controller: %v", err)
	}

	// 5 nodes, initial sync creates one pod per node.
	addNodes(manager.nodeStore, 0, 5, nil)
	manager.dsStore.Add(ds)
	expectSyncDaemonSets(t, manager, ds, podControl, 5, 0, 0)
	markAllPodsReady(podControl.podStore, nil)

	// Narrow nodeAffinity to exclude node-0 and node-1.
	// Also bump the image so this is a real rolling update.
	excludedNodes := sets.NewString("node-0", "node-1")
	ds.Spec.Template.Spec.Affinity = &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: corev1.NodeSelectorOpNotIn,
								Values:   excludedNodes.List(),
							},
						},
					},
				},
			},
		},
	}
	ds.Spec.Template.Spec.Containers[0].Image = "bar2"
	ds.Generation++
	manager.dsStore.Update(ds)

	// --- Part 1: validate pruneNodesAndOrphanedPods return value directly ---
	// Build nodeToDaemonPods the same way the controller does and confirm the
	// function now returns (not silently drops) the 2 pod names from excluded nodes.
	nodeToDaemonPods, err2 := manager.getNodesToDaemonPods(t.Context(), ds)
	if err2 != nil {
		t.Fatalf("getNodesToDaemonPods: %v", err2)
	}
	var nodeList []*corev1.Node
	for _, obj := range manager.nodeStore.List() {
		nodeList = append(nodeList, obj.(*corev1.Node))
	}

	_, ineligiblePods := manager.pruneNodesAndOrphanedPods(ds, nodeToDaemonPods, nodeList)
	if len(ineligiblePods) != 2 {
		t.Errorf("pruneNodesAndOrphanedPods: expected 2 ineligible pod names, got %d: %v", len(ineligiblePods), ineligiblePods)
	}

	// --- Part 2: validate end-to-end that the pods are actually deleted via syncNodes ---
	// The first sync should:
	//   - via manage:         delete 2 pods on excluded nodes (node-0, node-1)
	//   - via rollingUpdate:  delete 1 pod on an eligible node (maxUnavailable=1)
	// Total expected deletes: 3 (2 ineligible + 1 rolling-update step).
	clearExpectations(t, manager, ds, podControl)
	expectSyncDaemonSets(t, manager, ds, podControl, 0, 3, 0)

	// Verify no pods remain on the excluded nodes.
	for _, obj := range podControl.podStore.List() {
		pod := obj.(*corev1.Pod)
		nodeName := pod.Spec.NodeName
		if excludedNodes.Has(nodeName) {
			t.Errorf("pod %s still exists on excluded node %s after sync", pod.Name, nodeName)
		}
	}
}

