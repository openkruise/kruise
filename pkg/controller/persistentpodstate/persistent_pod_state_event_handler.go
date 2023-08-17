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

package persistentpodstate

import (
	"context"
	"fmt"

	"github.com/openkruise/kruise/pkg/util/configuration"

	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/webhook/pod/mutating"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	reader client.Reader
	client client.Client
}

func (p *enqueueRequestForPod) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	pod := evt.Object.(*corev1.Pod)
	pps := p.fetchPersistentPodState(pod)
	if pps == nil {
		return
	}
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      pps.Name,
			Namespace: pps.Namespace,
		},
	})
}

func (p *enqueueRequestForPod) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.updatePod(q, evt.ObjectOld, evt.ObjectNew)
}

func (p *enqueueRequestForPod) updatePod(q workqueue.RateLimitingInterface, old, cur runtime.Object) {
	newPod := cur.(*corev1.Pod)
	oldPod := old.(*corev1.Pod)
	if !isPodValidChanged(oldPod, newPod) {
		return
	}
	pps := p.fetchPersistentPodState(newPod)
	if pps == nil {
		return
	}
	q.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      pps.Name,
			Namespace: pps.Namespace,
		},
	})
}

func (p *enqueueRequestForPod) fetchPersistentPodState(pod *corev1.Pod) *appsv1alpha1.PersistentPodState {
	ref := metav1.GetControllerOf(pod)
	whiteList, err := configuration.GetPPSWatchCustomWorkloadWhiteList(p.client)
	if err != nil {
		klog.Errorf("Failed to get persistent pod state config white list, error: %v\n", err.Error())
		return nil
	}
	if ref == nil || !whiteList.ValidateAPIVersionAndKind(ref.APIVersion, ref.Kind) {
		return nil
	}
	ppsName := pod.Annotations[mutating.InjectedPersistentPodStateKey]
	if ppsName != "" {
		obj := &appsv1alpha1.PersistentPodState{}
		if err := p.reader.Get(context.TODO(), client.ObjectKey{Namespace: pod.Namespace, Name: ppsName}, obj); err != nil {
			klog.Errorf("fetch pod(%s/%s) PersistentPodState(%s) failed: %s", pod.Namespace, pod.Name, ppsName, err.Error())
			return nil
		}
		return obj
	}

	return mutating.SelectorPersistentPodState(p.reader, appsv1alpha1.TargetReference{
		APIVersion: ref.APIVersion,
		Kind:       ref.Kind,
		Name:       ref.Name,
	}, pod.Namespace)
}

func isPodValidChanged(oldPod, newPod *corev1.Pod) bool {
	if newPod.ResourceVersion == oldPod.ResourceVersion {
		return false
	}
	// If the pod's deletion timestamp is set, reconcile
	if oldPod.DeletionTimestamp.IsZero() && !newPod.DeletionTimestamp.IsZero() {
		return true
	}

	// when pod ready, then reconcile
	if !podutil.IsPodReady(oldPod) && podutil.IsPodReady(newPod) {
		return true
	}
	return false
}

var _ handler.EventHandler = &enqueueRequestForStatefulSet{}

type enqueueRequestForStatefulSet struct {
	reader client.Reader
}

func (p *enqueueRequestForStatefulSet) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	sts := evt.Object.(*appsv1.StatefulSet)
	if sts.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] == "true" &&
		(sts.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] != "" ||
			sts.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] != "") {
		enqueuePersistentPodStateRequest(q, KindSts.GroupVersion().String(), KindSts.Kind, sts.Namespace, sts.Name)
	}
}

func (p *enqueueRequestForStatefulSet) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	sts := evt.Object.(*appsv1.StatefulSet)
	if pps := mutating.SelectorPersistentPodState(p.reader, appsv1alpha1.TargetReference{
		APIVersion: KruiseKindSts.GroupVersion().String(),
		Kind:       KruiseKindSts.Kind,
		Name:       sts.Name,
	}, sts.Namespace); pps != nil {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pps.Name,
				Namespace: pps.Namespace,
			},
		})
	}
}

func (p *enqueueRequestForStatefulSet) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForStatefulSet) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oSts := evt.ObjectOld.(*appsv1.StatefulSet)
	nSts := evt.ObjectNew.(*appsv1.StatefulSet)
	if oSts.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] != nSts.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] ||
		oSts.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] != nSts.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] ||
		oSts.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] != nSts.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] {
		enqueuePersistentPodStateRequest(q, KindSts.GroupVersion().String(), KindSts.Kind, nSts.Namespace, nSts.Name)
	}

	// delete statefulSet scenario
	if oSts.DeletionTimestamp.IsZero() && !nSts.DeletionTimestamp.IsZero() {
		if pps := mutating.SelectorPersistentPodState(p.reader, appsv1alpha1.TargetReference{
			APIVersion: KindSts.GroupVersion().String(),
			Kind:       KindSts.Kind,
			Name:       nSts.Name,
		}, nSts.Namespace); pps != nil {
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pps.Name,
					Namespace: pps.Namespace,
				},
			})
		}
	}
}

func enqueuePersistentPodStateRequest(q workqueue.RateLimitingInterface, apiVersion, kind, ns, name string) {
	// name Format = generate#{apiVersion}#{workload.Kind}#{workload.Name}
	// example for generate#apps/v1#StatefulSet#echoserver
	qName := fmt.Sprintf("%s%s#%s#%s", AutoGeneratePersistentPodStatePrefix, apiVersion, kind, name)
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Namespace: ns,
		Name:      qName,
	}})
	klog.V(3).Infof("enqueuePersistentPodStateRequest(%s)", qName)
}

var _ handler.EventHandler = &enqueueRequestForKruiseStatefulSet{}

type enqueueRequestForKruiseStatefulSet struct {
	reader client.Reader
}

func (p *enqueueRequestForKruiseStatefulSet) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	sts := evt.Object.(*appsv1beta1.StatefulSet)
	if sts.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] == "true" &&
		(sts.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] != "" ||
			sts.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] != "") {
		enqueuePersistentPodStateRequest(q, KruiseKindSts.GroupVersion().String(), KruiseKindSts.Kind, sts.Namespace, sts.Name)
	}
}

func (p *enqueueRequestForKruiseStatefulSet) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	sts := evt.Object.(*appsv1beta1.StatefulSet)
	if pps := mutating.SelectorPersistentPodState(p.reader, appsv1alpha1.TargetReference{
		APIVersion: KruiseKindSts.GroupVersion().String(),
		Kind:       KruiseKindSts.Kind,
		Name:       sts.Name,
	}, sts.Namespace); pps != nil {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pps.Name,
				Namespace: pps.Namespace,
			},
		})
	}
}

func (p *enqueueRequestForKruiseStatefulSet) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForKruiseStatefulSet) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oSts := evt.ObjectOld.(*appsv1beta1.StatefulSet)
	nSts := evt.ObjectNew.(*appsv1beta1.StatefulSet)
	if oSts.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] != nSts.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] ||
		oSts.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] != nSts.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] ||
		oSts.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] != nSts.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] {
		enqueuePersistentPodStateRequest(q, KruiseKindSts.GroupVersion().String(), KruiseKindSts.Kind, nSts.Namespace, nSts.Name)
	}

	// delete statefulSet scenario
	if oSts.DeletionTimestamp.IsZero() && !nSts.DeletionTimestamp.IsZero() {
		if pps := mutating.SelectorPersistentPodState(p.reader, appsv1alpha1.TargetReference{
			APIVersion: KruiseKindSts.GroupVersion().String(),
			Kind:       KruiseKindSts.Kind,
			Name:       nSts.Name,
		}, nSts.Namespace); pps != nil {
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pps.Name,
					Namespace: pps.Namespace,
				},
			})
		}
	}
}

var _ handler.EventHandler = &enqueueRequestForStatefulSetLike{}

type enqueueRequestForStatefulSetLike struct {
	reader client.Reader
}

func (p *enqueueRequestForStatefulSetLike) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	workload := evt.Object.(*unstructured.Unstructured)
	annotations := workload.GetAnnotations()
	if annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] == "true" &&
		(annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] != "" ||
			annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] != "") {
		enqueuePersistentPodStateRequest(q, workload.GetAPIVersion(), workload.GetKind(), workload.GetNamespace(), workload.GetName())
	}
}

func (p *enqueueRequestForStatefulSetLike) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oWorkload := evt.ObjectOld.(*unstructured.Unstructured)
	nWorkload := evt.ObjectNew.(*unstructured.Unstructured)
	oAnnotations := oWorkload.GetAnnotations()
	nAnnotations := nWorkload.GetAnnotations()
	if oAnnotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] != nAnnotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] ||
		oAnnotations[appsv1alpha1.AnnotationRequiredPersistentTopology] != nAnnotations[appsv1alpha1.AnnotationRequiredPersistentTopology] ||
		oAnnotations[appsv1alpha1.AnnotationPreferredPersistentTopology] != nAnnotations[appsv1alpha1.AnnotationPreferredPersistentTopology] {
		enqueuePersistentPodStateRequest(q, nWorkload.GetAPIVersion(), nWorkload.GetKind(), nWorkload.GetNamespace(), nWorkload.GetName())
	}

	// delete statefulSet scenario
	if oWorkload.GetDeletionTimestamp().IsZero() && !nWorkload.GetDeletionTimestamp().IsZero() {
		if pps := mutating.SelectorPersistentPodState(p.reader, appsv1alpha1.TargetReference{
			APIVersion: oWorkload.GetAPIVersion(),
			Kind:       oWorkload.GetKind(),
			Name:       nWorkload.GetName(),
		}, nWorkload.GetNamespace()); pps != nil {
			q.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pps.Name,
					Namespace: pps.Namespace,
				},
			})
		}
	}
}

func (p *enqueueRequestForStatefulSetLike) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	workload := evt.Object.(*unstructured.Unstructured)
	if pps := mutating.SelectorPersistentPodState(p.reader, appsv1alpha1.TargetReference{
		APIVersion: workload.GetAPIVersion(),
		Kind:       workload.GetKind(),
		Name:       workload.GetName(),
	}, workload.GetNamespace()); pps != nil {
		q.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      pps.Name,
				Namespace: pps.Namespace,
			},
		})
	}
}

func (p *enqueueRequestForStatefulSetLike) Generic(genericEvent event.GenericEvent, limitingInterface workqueue.RateLimitingInterface) {
}
