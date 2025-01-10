/*
Copyright 2022 The Kruise Authors.

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

package podprobemarker

import (
	"context"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

var _ handler.EventHandler = &enqueueRequestForPodProbeMarker{}

type enqueueRequestForPodProbeMarker struct{}

func (p *enqueueRequestForPodProbeMarker) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.queue(q, evt.Object)
}

func (p *enqueueRequestForPodProbeMarker) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPodProbeMarker) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPodProbeMarker) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.queue(q, evt.ObjectNew)
}

func (p *enqueueRequestForPodProbeMarker) queue(q workqueue.RateLimitingInterface, obj runtime.Object) {
	ppm, ok := obj.(*appsalphav1.PodProbeMarker)
	if !ok {
		return
	}
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ppm.Namespace,
			Name:      ppm.Name,
		},
	})
}

var _ handler.EventHandler = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	reader client.Reader
}

func (p *enqueueRequestForPod) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	new, ok := evt.ObjectNew.(*corev1.Pod)
	if !ok {
		return
	}
	old, ok := evt.ObjectOld.(*corev1.Pod)
	if !ok {
		return
	}
	// add pod probe to nodePodProbe.spec
	oldInitialCondition := util.GetCondition(old, corev1.PodInitialized)
	newInitialCondition := util.GetCondition(new, corev1.PodInitialized)
	if !kubecontroller.IsPodActive(new) || newInitialCondition == nil ||
		newInitialCondition.Status != corev1.ConditionTrue || new.Spec.NodeName == "" {
		return
	}

	// normal pod
	if ((oldInitialCondition == nil || oldInitialCondition.Status == corev1.ConditionFalse) &&
		newInitialCondition.Status == corev1.ConditionTrue) || old.Status.PodIP != new.Status.PodIP {
		ppms, err := p.getPodProbeMarkerForPod(new)
		if err != nil {
			klog.ErrorS(err, "Failed to List PodProbeMarker")
			return
		}
		for _, ppm := range ppms {
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: ppm.Namespace,
					Name:      ppm.Name,
				},
			})
		}
	}

	// serverless pod
	if utilfeature.DefaultFeatureGate.Enabled(features.EnablePodProbeMarkerOnServerless) {
		if reflect.DeepEqual(old.Status.Conditions, new.Status.Conditions) {
			return
		}
		node := &corev1.Node{}
		if err := p.reader.Get(context.TODO(), client.ObjectKey{Name: new.Spec.NodeName}, node); err != nil {
			klog.ErrorS(err, "Failed to get Node", "nodeName", new.Spec.NodeName)
			return
		}
		if node.Labels["type"] != VirtualKubelet {
			return
		}
		ppms, err := p.getPodProbeMarkerForPod(new)
		if err != nil {
			klog.ErrorS(err, "Failed to List PodProbeMarker")
			return
		}
		for _, ppm := range ppms {
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: ppm.Namespace,
					Name:      ppm.Name,
				},
			})
		}
	}
}

func (p *enqueueRequestForPod) getPodProbeMarkerForPod(pod *corev1.Pod) ([]*appsalphav1.PodProbeMarker, error) {
	var ppms []*appsalphav1.PodProbeMarker
	// new pod have annotation kruise.io/podprobemarker-list
	if str, ok := pod.Annotations[appsalphav1.PodProbeMarkerListAnnotationKey]; ok && str != "" {
		names := strings.Split(str, ",")
		for _, name := range names {
			ppm := &appsalphav1.PodProbeMarker{}
			if err := p.reader.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: name}, ppm); err != nil {
				klog.ErrorS(err, "Failed to get PodProbeMarker", "name", name)
				continue
			}
			ppms = append(ppms, ppm)
		}
		return ppms, nil
	}

	ppmList := &appsalphav1.PodProbeMarkerList{}
	if err := p.reader.List(context.TODO(), ppmList, &client.ListOptions{Namespace: pod.Namespace}, utilclient.DisableDeepCopy); err != nil {
		return nil, err
	}
	for i := range ppmList.Items {
		ppm := &ppmList.Items[i]
		// This error is irreversible, so continue
		labelSelector, err := util.ValidatedLabelSelectorAsSelector(ppm.Spec.Selector)
		if err != nil {
			continue
		}
		// If a PUB with a nil or empty selector creeps in, it should match nothing, not everything.
		if labelSelector.Empty() || !labelSelector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		ppms = append(ppms, ppm)
	}
	return ppms, nil
}
