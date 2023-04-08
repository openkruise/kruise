package broadcastjob

import (
	"context"

	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
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
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"

	"github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/expectations"
)

type podEventHandler struct {
	enqueueHandler handler.EnqueueRequestForOwner
}

func isBroadcastJobController(controllerRef *metav1.OwnerReference) bool {
	refGV, err := schema.ParseGroupVersion(controllerRef.APIVersion)
	if err != nil {
		klog.Errorf("Could not parse OwnerReference %v APIVersion: %v", controllerRef, err)
		return false
	}
	return controllerRef.Kind == controllerKind.Kind && refGV.Group == controllerKind.Group
}

func (p *podEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	pod := evt.Object.(*v1.Pod)
	if pod.DeletionTimestamp != nil {
		p.Delete(event.DeleteEvent{Object: evt.Object}, q)
		return
	}
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil && isBroadcastJobController(controllerRef) {
		key := types.NamespacedName{Namespace: pod.Namespace, Name: controllerRef.Name}.String()
		scaleExpectations.ObserveScale(key, expectations.Create, getAssignedNode(pod))
		p.enqueueHandler.Create(evt, q)
	}
}

func (p *podEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	pod := evt.Object.(*v1.Pod)
	if controllerRef := metav1.GetControllerOf(pod); controllerRef != nil && isBroadcastJobController(controllerRef) {
		key := types.NamespacedName{Namespace: pod.Namespace, Name: controllerRef.Name}.String()
		scaleExpectations.ObserveScale(key, expectations.Delete, getAssignedNode(pod))
		p.enqueueHandler.Delete(evt, q)
	}
}

func (p *podEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.enqueueHandler.Update(evt, q)
}

func (p *podEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

var _ inject.Mapper = &podEventHandler{}

func (p *podEventHandler) InjectScheme(s *runtime.Scheme) error {
	return p.enqueueHandler.InjectScheme(s)
}

var _ inject.Mapper = &podEventHandler{}

func (p *podEventHandler) InjectMapper(m meta.RESTMapper) error {
	return p.enqueueHandler.InjectMapper(m)
}

type enqueueBroadcastJobForNode struct {
	reader client.Reader
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
	err := p.reader.List(context.TODO(), jobList)
	if err != nil {
		klog.Errorf("Error enqueueing broadcastjob on addNode %v", err)
	}
	for _, bcj := range jobList.Items {
		mockPod := NewMockPod(&bcj, node.Name)
		canFit, err := checkNodeFitness(mockPod, node)
		if !canFit {
			klog.Infof("Job %s/%s does not fit on node %s due to %v", bcj.Namespace, bcj.Name, node.Name, err)
			continue
		}

		// enqueue the bcj for matching node
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: bcj.Namespace,
				Name:      bcj.Name}})
	}
}

func (p *enqueueBroadcastJobForNode) updateNode(q workqueue.RateLimitingInterface, old, cur runtime.Object) {
	oldNode := old.(*v1.Node)
	curNode := cur.(*v1.Node)
	if shouldIgnoreNodeUpdate(*oldNode, *curNode) {
		return
	}
	jobList := &v1alpha1.BroadcastJobList{}
	err := p.reader.List(context.TODO(), jobList)
	if err != nil {
		klog.Errorf("Error enqueueing broadcastjob on updateNode %v", err)
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
