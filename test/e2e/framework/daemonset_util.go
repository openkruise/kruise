package framework

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/controller/daemonset"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	schedulernodeinfo "k8s.io/kubernetes/pkg/scheduler/nodeinfo"
)

const (
	// DaemonSetRetryPeriod indicates poll interval for DaemonSet tests
	DaemonSetRetryPeriod = 1 * time.Second
	// DaemonSetRetryTimeout indicates timeout interval for DaemonSet operations
	DaemonSetRetryTimeout = 5 * time.Minute

	DaemonSetLabelPrefix = "daemonset-"
	DaemonSetNameLabel   = DaemonSetLabelPrefix + "name"
	DaemonSetColorLabel  = DaemonSetLabelPrefix + "color"

	OldImage = "busybox:1.29"
	NewImage = "busybox:1.30"
)

type DaemonSetTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
	ns string
}

func NewDaemonSetTester(c clientset.Interface, kc kruiseclientset.Interface, ns string) *DaemonSetTester {
	return &DaemonSetTester{
		c:  c,
		kc: kc,
		ns: ns,
	}
}

func (t *DaemonSetTester) NewDaemonSet(name string, label map[string]string, image string, updateStrategy appsv1alpha1.DaemonSetUpdateStrategy) *appsv1alpha1.DaemonSet {
	return &appsv1alpha1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.ns,
			Name:      name,
		},
		Spec: appsv1alpha1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: label,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: label,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    "busybox",
							Image:   image,
							Command: []string{"/bin/sh", "-c", "sleep 10000000"},
						},
					},
					HostNetwork: true,
				},
			},
			UpdateStrategy: updateStrategy,
		},
	}
}

func (t *DaemonSetTester) CreateDaemonSet(ds *appsv1alpha1.DaemonSet) (*appsv1alpha1.DaemonSet, error) {
	return t.kc.AppsV1alpha1().DaemonSets(t.ns).Create(ds)
}

func (t *DaemonSetTester) GetDaemonSet(name string) (*appsv1alpha1.DaemonSet, error) {
	return t.kc.AppsV1alpha1().DaemonSets(t.ns).Get(name, metav1.GetOptions{})
}

func (t *DaemonSetTester) UpdateDaemonSet(name string, fn func(ds *appsv1alpha1.DaemonSet)) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		ds, err := t.GetDaemonSet(name)
		if err != nil {
			return err
		}

		fn(ds)
		_, err = t.kc.AppsV1alpha1().DaemonSets(t.ns).Update(ds)
		return err
	})
}

func (t *DaemonSetTester) DeleteDaemonSet(namespace, name string) {
	err := t.kc.AppsV1alpha1().DaemonSets(namespace).Delete(name, &metav1.DeleteOptions{})
	if err != nil {
		Logf("delete daemonset(%s.%s) failed: %s", t.ns, name, err.Error())
		return
	}
}

func (t *DaemonSetTester) PatchDaemonSet(name string, patchType types.PatchType, patch []byte) (*appsv1alpha1.DaemonSet, error) {
	return t.kc.AppsV1alpha1().DaemonSets(t.ns).Patch(name, patchType, patch)
}

func (t *DaemonSetTester) WaitForDaemonSetDeleted(namespace, name string) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := t.kc.AppsV1alpha1().DaemonSets(namespace).Get(name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}
			return false, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for daemonset to enter Deleted: %v", pollErr)
	}
}

func (t *DaemonSetTester) CheckDaemonStatus(dsName string) error {
	ds, err := t.GetDaemonSet(dsName)
	if err != nil {
		return fmt.Errorf("Could not get daemon set from v1.")
	}
	desired, scheduled, ready := ds.Status.DesiredNumberScheduled, ds.Status.CurrentNumberScheduled, ds.Status.NumberReady
	if desired != scheduled && desired != ready {
		return fmt.Errorf("Error in daemon status. DesiredScheduled: %d, CurrentScheduled: %d, Ready: %d", desired, scheduled, ready)
	}
	return nil
}

func (t *DaemonSetTester) SeparateDaemonSetNodeLabels(labels map[string]string) (map[string]string, map[string]string) {
	daemonSetLabels := map[string]string{}
	otherLabels := map[string]string{}
	for k, v := range labels {
		if strings.HasPrefix(k, DaemonSetLabelPrefix) {
			daemonSetLabels[k] = v
		} else {
			otherLabels[k] = v
		}
	}
	return daemonSetLabels, otherLabels
}

func (t *DaemonSetTester) ClearDaemonSetNodeLabels(c clientset.Interface) error {
	nodeList := GetReadySchedulableNodesOrDie(c)
	for _, node := range nodeList.Items {
		_, err := t.SetDaemonSetNodeLabels(node.Name, map[string]string{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *DaemonSetTester) SetDaemonSetNodeLabels(nodeName string, labels map[string]string) (*v1.Node, error) {
	nodeClient := t.c.CoreV1().Nodes()
	var newNode *v1.Node
	var newLabels map[string]string
	err := wait.PollImmediate(DaemonSetRetryPeriod, DaemonSetRetryTimeout, func() (bool, error) {
		node, err := nodeClient.Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		// remove all labels this test is creating
		daemonSetLabels, otherLabels := t.SeparateDaemonSetNodeLabels(node.Labels)
		if reflect.DeepEqual(daemonSetLabels, labels) {
			newNode = node
			return true, nil
		}
		node.Labels = otherLabels
		for k, v := range labels {
			node.Labels[k] = v
		}
		newNode, err = nodeClient.Update(node)
		if err == nil {
			newLabels, _ = t.SeparateDaemonSetNodeLabels(newNode.Labels)
			return true, err
		}
		if se, ok := err.(*apierrors.StatusError); ok && se.ErrStatus.Reason == metav1.StatusReasonConflict {
			Logf("failed to update node due to resource version conflict")
			return false, nil
		}
		return false, err
	})
	if err != nil {
		return nil, err
	} else if len(newLabels) != len(labels) {
		return nil, fmt.Errorf("Could not set daemon set test labels as expected.")
	}

	return newNode, nil
}

func (t *DaemonSetTester) CheckRunningOnAllNodes(ds *appsv1alpha1.DaemonSet) func() (bool, error) {
	return func() (bool, error) {
		nodeNames := t.SchedulableNodes(ds)
		return t.CheckDaemonPodOnNodes(ds, nodeNames)()
	}
}

func (t *DaemonSetTester) CheckRunningOnNoNodes(ds *appsv1alpha1.DaemonSet) func() (bool, error) {
	return t.CheckDaemonPodOnNodes(ds, make([]string, 0))
}

func (t *DaemonSetTester) SchedulableNodes(ds *appsv1alpha1.DaemonSet) []string {
	nodeList, err := t.c.CoreV1().Nodes().List(metav1.ListOptions{})
	ExpectNoError(err)
	nodeNames := make([]string, 0)
	for _, node := range nodeList.Items {
		if !t.CanScheduleOnNode(node, ds) {
			Logf("DaemonSet pods can't tolerate node %s with taints %+v, skip checking this node", node.Name, node.Spec.Taints)
			continue
		}
		nodeNames = append(nodeNames, node.Name)
	}
	return nodeNames
}

func (t *DaemonSetTester) CheckDaemonPodOnNodes(ds *appsv1alpha1.DaemonSet, nodeNames []string) func() (bool, error) {
	return func() (bool, error) {
		podList, err := t.c.CoreV1().Pods(t.ns).List(metav1.ListOptions{})
		if err != nil {
			Logf("could not get the pod list: %v", err)
			return false, nil
		}
		pods := podList.Items

		nodesToPodCount := make(map[string]int)
		for _, pod := range pods {
			if !metav1.IsControlledBy(&pod, ds) {
				continue
			}
			if pod.DeletionTimestamp != nil {
				continue
			}
			if podutil.IsPodAvailable(&pod, ds.Spec.MinReadySeconds, metav1.Now()) {
				nodesToPodCount[pod.Spec.NodeName] += 1
			}
		}
		Logf("Number of nodes with available pods: %d", len(nodesToPodCount))

		// Ensure that exactly 1 pod is running on all nodes in nodeNames.
		for _, nodeName := range nodeNames {
			if nodesToPodCount[nodeName] > 1 {
				Logf("Node %s is running more than one daemon pod", nodeName)
				return false, nil
			}
		}

		Logf("Number of running nodes: %d, number of available pods: %d", len(nodeNames), len(nodesToPodCount))
		// Ensure that sizes of the lists are the same. We've verified that every element of nodeNames is in
		// nodesToPodCount, so verifying the lengths are equal ensures that there aren't pods running on any
		// other nodes.
		return len(nodesToPodCount) == len(nodeNames), nil
	}
}

func (t *DaemonSetTester) WaitFailedDaemonPodDeleted(pod *v1.Pod) func() (bool, error) {
	return func() (bool, error) {
		if _, err := t.c.CoreV1().Pods(pod.Namespace).Get(pod.Name, metav1.GetOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, fmt.Errorf("failed to get failed daemon pod %q: %v", pod.Name, err)
		}
		return false, nil
	}
}

func (t *DaemonSetTester) CanScheduleOnNode(node v1.Node, ds *appsv1alpha1.DaemonSet) bool {
	newPod := daemonset.NewPod(ds, node.Name)
	nodeInfo := schedulernodeinfo.NewNodeInfo()
	nodeInfo.SetNode(&node)
	fit, _, err := daemonset.Predicates(newPod, nodeInfo)
	if err != nil {
		Failf("Can't test DaemonSet predicates for node %s: %v", node.Name, err)
		return false
	}
	return fit
}

func (t *DaemonSetTester) ListDaemonPods(label map[string]string) (*v1.PodList, error) {
	selector := labels.Set(label).AsSelector()
	options := metav1.ListOptions{LabelSelector: selector.String()}
	return t.c.CoreV1().Pods(t.ns).List(options)
}
