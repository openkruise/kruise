package broadcastjob

import (
	"context"

	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/expectations"
)

type podEventHandler struct {
	enqueueHandler handler.TypedEventHandler[*v1.Pod]
}

func isBroadcastJobController(controllerRef *metav1.OwnerReference) bool {
	refGV, err := schema.ParseGroupVersion(controllerRef.APIVersion)
	if err != nil {
		klog.ErrorS(err, "Could not parse APIVersion in OwnerReference", "ownerReference", controllerRef)
		return false
	}
	return controllerRef.Kind == controllerKind.Kind && refGV.Group == controllerKind.Group
}

func (p *podEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	pod := evt.Object
	if pod.DeletionTimestamp != nil {
		p.Delete(ctx, event.TypedDeleteEvent[*v1.Pod]{Object: pod}, q)
		return
	}
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil && isBroadcastJobController(controllerRef) {
		key := types.NamespacedName{Namespace: pod.Namespace, Name: controllerRef.Name}.String()
		scaleExpectations.ObserveScale(key, expectations.Create, getAssignedNode(pod))
		p.enqueueHandler.Create(ctx, evt, q)
	}
}

func (p *podEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	pod := evt.Object
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil && isBroadcastJobController(controllerRef) {
		key := types.NamespacedName{Namespace: pod.Namespace, Name: controllerRef.Name}.String()
		scaleExpectations.ObserveScale(key, expectations.Delete, getAssignedNode(pod))
		p.enqueueHandler.Delete(ctx, evt, q)
	}
}

func (p *podEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	p.enqueueHandler.Update(ctx, evt, q)
}

func (p *podEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
}

type enqueueBroadcastJobForNode struct {
	reader client.Reader
}

func (p *enqueueBroadcastJobForNode) Create(ctx context.Context, evt event.TypedCreateEvent[*v1.Node], q workqueue.RateLimitingInterface) {
	p.addNode(q, evt.Object)
}

func (p *enqueueBroadcastJobForNode) Delete(ctx context.Context, evt event.TypedDeleteEvent[*v1.Node], q workqueue.RateLimitingInterface) {
	p.deleteNode(q, evt.Object)
}

func (p *enqueueBroadcastJobForNode) Generic(ctx context.Context, evt event.TypedGenericEvent[*v1.Node], q workqueue.RateLimitingInterface) {
}

func (p *enqueueBroadcastJobForNode) Update(ctx context.Context, evt event.TypedUpdateEvent[*v1.Node], q workqueue.RateLimitingInterface) {
	p.updateNode(q, evt.ObjectOld, evt.ObjectNew)
}

func (p *enqueueBroadcastJobForNode) addNode(q workqueue.RateLimitingInterface, obj *v1.Node) {
	node := obj
	jobList := &v1alpha1.BroadcastJobList{}
	err := p.reader.List(context.TODO(), jobList)
	if err != nil {
		klog.ErrorS(err, "Failed to enqueue BroadcastJob on addNode")
	}
	for _, bcj := range jobList.Items {
		mockPod := NewMockPod(&bcj, node.Name)
		canFit, err := checkNodeFitness(mockPod, node)
		if !canFit {
			klog.ErrorS(err, "BroadcastJob did not fit on node", "broadcastJob", klog.KObj(&bcj), "nodeName", node.Name)
			continue
		}

		// enqueue the bcj for matching node
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: bcj.Namespace,
				Name:      bcj.Name}})
	}
}

func (p *enqueueBroadcastJobForNode) updateNode(q workqueue.RateLimitingInterface, old, cur *v1.Node) {
	oldNode := old
	curNode := cur
	if shouldIgnoreNodeUpdate(*oldNode, *curNode) {
		return
	}
	jobList := &v1alpha1.BroadcastJobList{}
	err := p.reader.List(context.TODO(), jobList)
	if err != nil {
		klog.ErrorS(err, "Failed to enqueue BroadcastJob on updateNode")
	}
	for _, bcj := range jobList.Items {
		mockPod := NewMockPod(&bcj, oldNode.Name)
		canOldNodeFit, _ := checkNodeFitness(mockPod, oldNode)
		canCurNodeFit, _ := checkNodeFitness(mockPod, curNode)

		if canOldNodeFit != canCurNodeFit {
			// enqueue the broadcast job for matching node
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: bcj.Namespace,
					Name:      bcj.Name}})
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
