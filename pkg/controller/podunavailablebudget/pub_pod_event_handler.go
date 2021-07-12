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

package podunavailablebudget

import (
	"reflect"
	"time"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	client           client.Client
	controllerFinder *controllerfinder.ControllerFinder
}

func (p *enqueueRequestForPod) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.addPod(q, evt.Object)
}

func (p *enqueueRequestForPod) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {}

func (p *enqueueRequestForPod) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.updatePod(q, evt.ObjectOld, evt.ObjectNew)
}

func (p *enqueueRequestForPod) addPod(q workqueue.RateLimitingInterface, obj runtime.Object) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	pub, _ := pubcontrol.GetPodUnavailableBudgetForPod(p.client, p.controllerFinder, pod)
	if pub == nil {
		return
	}

	klog.V(3).Infof("add pod(%s.%s) reconcile pub(%s.%s)", pod.Namespace, pod.Name, pub.Namespace, pub.Name)
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      pub.Name,
			Namespace: pub.Namespace,
		},
	})
}

func (p *enqueueRequestForPod) updatePod(q workqueue.RateLimitingInterface, old, cur runtime.Object) {
	newPod := cur.(*corev1.Pod)
	oldPod := old.(*corev1.Pod)
	if newPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}

	//labels changed, and reconcile union pubs
	if !reflect.DeepEqual(newPod.Labels, oldPod.Labels) {
		oldPub, _ := pubcontrol.GetPodUnavailableBudgetForPod(p.client, p.controllerFinder, oldPod)
		newPub, _ := pubcontrol.GetPodUnavailableBudgetForPod(p.client, p.controllerFinder, newPod)
		if oldPub != nil && newPub != nil && oldPub.Name == newPub.Name {
			control := pubcontrol.NewPubControl(newPub)
			if isReconcile, enqueueDelayTime := isPodAvailableChanged(oldPod, newPod, newPub, control); isReconcile {
				q.AddAfter(reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      newPub.Name,
						Namespace: newPub.Namespace,
					},
				}, enqueueDelayTime)
			}
			return
		}
		if oldPub != nil {
			klog.V(3).Infof("pod(%s.%s) labels changed, and reconcile pub(%s.%s)", oldPod.Namespace, oldPod.Name, oldPub.Namespace, oldPub.Name)
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      oldPub.Name,
					Namespace: oldPub.Namespace,
				},
			})
		}
		if newPub != nil {
			klog.V(3).Infof("pod(%s.%s) labels changed, and reconcile pub(%s.%s)", newPod.Namespace, newPod.Name, newPub.Namespace, newPub.Name)
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      newPub.Name,
					Namespace: newPub.Namespace,
				},
			})
		}

		return
	}

	pub, _ := pubcontrol.GetPodUnavailableBudgetForPod(p.client, p.controllerFinder, newPod)
	if pub == nil {
		return
	}
	control := pubcontrol.NewPubControl(pub)
	if isReconcile, enqueueDelayTime := isPodAvailableChanged(oldPod, newPod, pub, control); isReconcile {
		q.AddAfter(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pub.Name,
				Namespace: pub.Namespace,
			},
		}, enqueueDelayTime)
	}

}

func isPodAvailableChanged(oldPod, newPod *corev1.Pod, pub *policyv1alpha1.PodUnavailableBudget, control pubcontrol.PubControl) (bool, time.Duration) {
	var enqueueDelayTime time.Duration
	// If the pod's deletion timestamp is set, remove endpoint from ready address.
	if oldPod.DeletionTimestamp.IsZero() && !newPod.DeletionTimestamp.IsZero() {
		enqueueDelayTime = time.Second * 5
		klog.V(3).Infof("pod(%s.%s) DeletionTimestamp changed, and reconcile pub(%s.%s) delayTime(5s)", newPod.Namespace, newPod.Name, pub.Namespace, pub.Name)
		return true, enqueueDelayTime
		// oldPod Deletion is set, then no reconcile
	} else if !oldPod.DeletionTimestamp.IsZero() {
		return false, enqueueDelayTime
	}

	// If the pod's readiness has changed, the associated endpoint address
	// will move from the unready endpoints set to the ready endpoints.
	// So for the purposes of an endpoint, a readiness change on a pod
	// means we have a changed pod.
	oldReady := control.IsPodReady(oldPod) && control.IsPodStateConsistent(oldPod)
	newReady := control.IsPodReady(newPod) && control.IsPodStateConsistent(newPod)
	if oldReady != newReady {
		klog.V(3).Infof("pod(%s.%s) ConsistentAndReady changed(from %v to %v), and reconcile pub(%s.%s)",
			newPod.Namespace, newPod.Name, oldReady, newReady, pub.Namespace, pub.Name)
		return true, enqueueDelayTime
	}

	return false, enqueueDelayTime
}
