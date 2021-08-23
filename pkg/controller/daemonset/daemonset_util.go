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
	"fmt"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
	kubeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// nodeInSameCondition returns true if all effective types ("Status" is true) equals;
// otherwise, returns false.
func nodeInSameCondition(old []corev1.NodeCondition, cur []corev1.NodeCondition) bool {
	if len(old) == 0 && len(cur) == 0 {
		return true
	}

	c1map := map[corev1.NodeConditionType]corev1.ConditionStatus{}
	for _, c := range old {
		if c.Status == corev1.ConditionTrue {
			c1map[c.Type] = c.Status
		}
	}

	for _, c := range cur {
		if c.Status != corev1.ConditionTrue {
			continue
		}

		if _, found := c1map[c.Type]; !found {
			return false
		}

		delete(c1map, c.Type)
	}

	return len(c1map) == 0
}

// nodeShouldRunDaemonPod checks a set of preconditions against a (node,daemonset) and returns a
// summary. Returned booleans are:
// * shouldRun:
//     Returns true when a daemonset should run on the node if a daemonset pod is not already
//     running on that node.
// * shouldContinueRunning:
//     Returns true when a daemonset should continue running on a node if a daemonset pod is already
//     running on that node.
func NodeShouldRunDaemonPod(node *corev1.Node, ds *appsv1alpha1.DaemonSet) (bool, bool, error) {
	pod := NewPod(ds, node.Name)

	// If the daemon set specifies a node name, check that it matches with node.Name.
	if !(ds.Spec.Template.Spec.NodeName == "" || ds.Spec.Template.Spec.NodeName == node.Name) {
		return false, false, nil
	}

	taints := node.Spec.Taints
	fitsNodeName, fitsNodeAffinity, fitsTaints := Predicates(pod, node, taints)
	if !fitsNodeName || !fitsNodeAffinity {
		return false, false, nil
	}

	if !fitsTaints {
		// Scheduled daemon pods should continue running if they tolerate NoExecute taint.
		_, isUntolerated := v1helper.FindMatchingUntoleratedTaint(taints, pod.Spec.Tolerations, func(t *corev1.Taint) bool {
			return t.Effect == corev1.TaintEffectNoExecute
		})
		return false, !isUntolerated, nil
	}

	return true, true, nil
}

func newSchedulerNodeInfo(node *corev1.Node) *schedulernodeinfo.NodeInfo {
	nodeInfo := schedulernodeinfo.NewNodeInfo()
	if extraAllowedPodNumber > 0 {
		rQuant, ok := node.Status.Allocatable[corev1.ResourcePods]
		if ok {
			rQuant.Add(*resource.NewQuantity(extraAllowedPodNumber, resource.DecimalSI))
			nodeCopy := node.DeepCopy()
			nodeCopy.Status.Allocatable[corev1.ResourcePods] = rQuant
			nodeInfo.SetNode(nodeCopy)
			return nodeInfo
		}
	}
	nodeInfo.SetNode(node)
	return nodeInfo
}

func ShouldIgnoreNodeUpdate(oldNode, curNode corev1.Node) bool {
	if !nodeInSameCondition(oldNode.Status.Conditions, curNode.Status.Conditions) {
		return false
	}
	oldNode.ResourceVersion = curNode.ResourceVersion
	oldNode.Status.Conditions = curNode.Status.Conditions
	return apiequality.Semantic.DeepEqual(oldNode, curNode)
}

func getBurstReplicas(ds *appsv1alpha1.DaemonSet) int {
	// Error caught by validation
	burstReplicas, _ := intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(ds.Spec.BurstReplicas, intstrutil.FromInt(0)),
		int(ds.Status.DesiredNumberScheduled),
		false)
	return burstReplicas
}

// GetPodDaemonSets returns a list of DaemonSets that potentially match a pod.
// Only the one specified in the Pod's ControllerRef will actually manage it.
// Returns an error only if no matching DaemonSets are found.
func (dsc *ReconcileDaemonSet) GetPodDaemonSets(pod *corev1.Pod) ([]*appsv1alpha1.DaemonSet, error) {
	var selector labels.Selector
	var daemonSet *appsv1alpha1.DaemonSet

	if len(pod.Labels) == 0 {
		return nil, fmt.Errorf("no daemon sets found for pod %v because it has no labels", pod.Name)
	}

	list := &appsv1alpha1.DaemonSetList{}
	err := dsc.client.List(context.TODO(), list)
	if err != nil {
		return nil, err
	}

	var daemonSets []*appsv1alpha1.DaemonSet
	for i := range list.Items {
		daemonSet = &list.Items[i]
		if daemonSet.Namespace != pod.Namespace {
			continue
		}
		selector, err = metav1.LabelSelectorAsSelector(daemonSet.Spec.Selector)
		if err != nil {
			// this should not happen if the DaemonSet passed validation
			return nil, err
		}

		// If a daemonSet with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		daemonSets = append(daemonSets, daemonSet)
	}

	if len(daemonSets) == 0 {
		return nil, fmt.Errorf("could not find daemon set for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}

	return daemonSets, nil
}

func storeDaemonSetStatus(dsClient kubeClient.Client, ds *appsv1alpha1.DaemonSet, desiredNumberScheduled, currentNumberScheduled, numberMisscheduled, numberReady, updatedNumberScheduled, numberAvailable, numberUnavailable int, updateObservedGen bool, hash string) error {
	key := types.NamespacedName{
		Namespace: ds.Namespace,
		Name:      ds.Name,
	}

	if int(ds.Status.DesiredNumberScheduled) == desiredNumberScheduled &&
		int(ds.Status.CurrentNumberScheduled) == currentNumberScheduled &&
		int(ds.Status.NumberMisscheduled) == numberMisscheduled &&
		int(ds.Status.NumberReady) == numberReady &&
		int(ds.Status.UpdatedNumberScheduled) == updatedNumberScheduled &&
		int(ds.Status.NumberAvailable) == numberAvailable &&
		int(ds.Status.NumberUnavailable) == numberUnavailable &&
		ds.Status.ObservedGeneration >= ds.Generation && ds.Status.DaemonSetHash == hash {
		klog.V(6).Info("storeDaemonSetStatus has no changes and return nil.")
		return nil
	}

	klog.V(6).Infof("toUpdate is %v", ds)

	try := wait.Backoff{
		Steps:    StatusUpdateRetries,
		Duration: 10 * time.Millisecond,
		Factor:   1.0,
		Jitter:   0.1,
	}

	return retry.RetryOnConflict(try, func() error {
		var updateErr, getErr error
		if updateObservedGen {
			ds.Status.ObservedGeneration = ds.Generation
		}
		ds.Status.DesiredNumberScheduled = int32(desiredNumberScheduled)
		ds.Status.CurrentNumberScheduled = int32(currentNumberScheduled)
		ds.Status.NumberMisscheduled = int32(numberMisscheduled)
		ds.Status.NumberReady = int32(numberReady)
		ds.Status.UpdatedNumberScheduled = int32(updatedNumberScheduled)
		ds.Status.NumberAvailable = int32(numberAvailable)
		ds.Status.NumberUnavailable = int32(numberUnavailable)
		ds.Status.DaemonSetHash = hash

		if updateErr = dsClient.Status().Update(context.TODO(), ds); updateErr == nil {
			klog.V(6).Infof("update DaemonSet status succeed. new status is %v", ds.Status)
			return nil
		}

		klog.Errorf("update DaemonSet status %v failed: %v", ds.Status, updateErr)
		// Update the set with the latest resource version for the next poll
		if getErr = dsClient.Get(context.TODO(), key, ds); getErr != nil {
			// If the GET fails we can't trust status.Replicas anymore. This error
			// is bound to be more interesting than the update failure.
			klog.Errorf("get DaemonSet %v failed: %v", ds.Name, getErr)
			return getErr
		}
		return updateErr
	})
}

// GetPodRevision returns revision hash of this pod.
func GetPodRevision(controllerKey string, pod metav1.Object) string {
	return pod.GetLabels()[apps.ControllerRevisionHashLabelKey]
}

// NodeShouldUpdateBySelector checks if the node is selected to upgrade for ds's gray update selector.
// This function does not check NodeShouldRunDaemonPod
func NodeShouldUpdateBySelector(node *corev1.Node, ds *appsv1alpha1.DaemonSet) bool {
	switch ds.Spec.UpdateStrategy.Type {
	case appsv1alpha1.OnDeleteDaemonSetStrategyType:
		return false
	case appsv1alpha1.RollingUpdateDaemonSetStrategyType:
		if ds.Spec.UpdateStrategy.RollingUpdate.Selector == nil {
			return false
		}
		selector, err := metav1.LabelSelectorAsSelector(ds.Spec.UpdateStrategy.RollingUpdate.Selector)
		if err != nil {
			// this should not happen if the DaemonSet passed validation
			klog.Errorf("unexpected rolling update selector for ds %s, err %s", ds.Name, err.Error())
			return false
		}
		if selector.Empty() || !selector.Matches(labels.Set(node.Labels)) {
			return false
		}
		return true
	default:
		klog.Warningf("get unknown update strategy type %s for daemonset %s", ds.Spec.UpdateStrategy.Type, ds.Name)
		return false
	}
}
