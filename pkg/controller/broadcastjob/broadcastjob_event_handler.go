package broadcastjob

import (
	"context"

	"github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type enqueueBroadcastJobForNode struct {
	client client.Client
}

func (p *enqueueBroadcastJobForNode) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.addNode(q, evt.Object)
}

func (p *enqueueBroadcastJobForNode) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	p.deleteNode(q, evt.Object)
}

func (p *enqueueBroadcastJobForNode) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueBroadcastJobForNode) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.updateNode(q, evt.ObjectOld, evt.ObjectNew)
}

func (p *enqueueBroadcastJobForNode) addNode(q workqueue.RateLimitingInterface, obj runtime.Object) {
	node, ok := obj.(*v1.Node)
	if !ok {
		return
	}
	jobList := &v1alpha1.BroadcastJobList{}
	err := p.client.List(context.TODO(), &client.ListOptions{}, jobList)
	if err != nil {
		klog.Errorf("Error enqueueing broadcastjob on addNode %v", err)
	}
	for _, bj := range jobList.Items {
		mockPod := NewPod(&bj, node.Name)
		canFit, err := checkNodeFitness(mockPod, node)
		if err != nil {
			klog.Errorf("failed to checkNodeFitness for job %s/%s, on node %s, %v", bj.Namespace, bj.Name, node.Name, err)
			continue
		}
		if !canFit {
			klog.Infof("Job %s/%s does not fit on node %s", bj.Namespace, bj.Name, node.Name)
			continue
		}

		// enqueue the bj for matching node
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: bj.Namespace,
				Name:      bj.Name}})
	}
}

func (p *enqueueBroadcastJobForNode) updateNode(q workqueue.RateLimitingInterface, old, cur runtime.Object) {
	oldNode := old.(*v1.Node)
	curNode := cur.(*v1.Node)
	if shouldIgnoreNodeUpdate(*oldNode, *curNode) {
		return
	}
	jobList := &v1alpha1.BroadcastJobList{}
	err := p.client.List(context.TODO(), &client.ListOptions{}, jobList)
	if err != nil {
		klog.Errorf("Error enqueueing broadcastjob on updateNode %v", err)
	}
	for _, bj := range jobList.Items {
		mockPod := NewPod(&bj, oldNode.Name)
		canOldNodeFit, err := checkNodeFitness(mockPod, oldNode)
		if err != nil {
			klog.Errorf("failed to checkNodeFitness for job %s/%s, on old node %s, %v", bj.Namespace, bj.Name, oldNode.Name, err)
			continue
		}

		canCurNodeFit, err := checkNodeFitness(mockPod, curNode)
		if err != nil {
			klog.Errorf("failed to checkNodeFitness for job %s/%s, on cur node %s, %v", bj.Namespace, bj.Name, curNode.Name, err)
			continue
		}

		if canOldNodeFit != canCurNodeFit {
			// enqueue the bj for matching node
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: bj.Namespace,
					Name:      bj.Name}})
		}
	}
}

func (p *enqueueBroadcastJobForNode) deleteNode(q workqueue.RateLimitingInterface, obj runtime.Object) {

}

// nodeInSameCondition returns true if all effective types ("Status" is true) equals;
// otherwise, returns false.
func nodeInSameCondition(old []v1.NodeCondition, cur []v1.NodeCondition) bool {
	if len(old) == 0 && len(cur) == 0 {
		return true
	}

	c1map := map[v1.NodeConditionType]v1.ConditionStatus{}
	for _, c := range old {
		if c.Status == v1.ConditionTrue {
			c1map[c.Type] = c.Status
		}
	}

	for _, c := range cur {
		if c.Status != v1.ConditionTrue {
			continue
		}

		if _, found := c1map[c.Type]; !found {
			return false
		}

		delete(c1map, c.Type)
	}

	return len(c1map) == 0
}

func shouldIgnoreNodeUpdate(oldNode, curNode v1.Node) bool {
	if !nodeInSameCondition(oldNode.Status.Conditions, curNode.Status.Conditions) {
		return false
	}
	oldNode.ResourceVersion = curNode.ResourceVersion
	oldNode.Status.Conditions = curNode.Status.Conditions
	return apiequality.Semantic.DeepEqual(oldNode, curNode)
}
