---
title: PersistentPodState custom workload support
authors:
- "@baxiaoshi"
reviewers:
- ""
creation-date: 2022-08-24
last-updated: 2022-08-25
status:
---

# PersistentPodState custom workload support
Table of Contents
=================

- [PersistentPodState custom workload support](#persistentpodstate-custom-workload-support)
- [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Proposal](#proposal)
    - [1.add dynamic watch](#1add-dynamic-watch)
    - [2.add workload finder](#2add-workload-finder)
    - [3. add persistent annotations](#3-add-persistent-annotations)
## Motivation
PPS(PersistentPodState) currently has support for standard statefulset and kruise's statefulSet. However, some companies will customize workloads(statefulset like) with stable identification pods,  support such workloads, and add support for custom workloads to pps.
## Proposal
Modify PPS controller logic to support custom statefulset like workload：
1.add dynamic watch: for watch custom statefulset like workload, automatically create pps if pps persistent annotations are found
2.add workload finder: used to find workload according to the target of PPS and build ScaleAndSelector object
### 1.add dynamic watch
client-go supports dynamic watch, but requires gvr or gvk to provide the resources. There are two ways to get the resources in the cluster that need to be watched：

1）Get the APIResourceList  in the cluster through the ServerPreferredResources interface of the dynamic client. Because we can't distinguish those workloads, we can only watch all of them here, which will have a lot of unnecessary watch overhead.

2）Find workloads by pod's ownerReferences, and add a whitelist of those that need to be watched. There is a time delay.

3）Dynamic creation of watches via watch whitelist.

It is recommended to use method 2 to get the owner's gvk by reverse lookup and create Informer if there is no watch before.

> Add pps watch whitelist
```go
const PPSWatchCustomWorkloadWhiteList        = "PPS_Watch_Custom_Workload_WhiteList"

type PPSWatchCustomWorkloadWhiteList struct {
	Workloads []metav1.GroupKind `json:"workloads,omitempty"`
}

```

> Add watch pod create logic to dynamically watch new workloads based on pod OwnerReferences.

[https://github.com/openkruise/kruise/blob/master/pkg/controller/persistentpodstate/persistent_pod_state_event_handler.go#L46](https://github.com/openkruise/kruise/blob/master/pkg/controller/persistentpodstate/persistent_pod_state_event_handler.go#L46)
```go
func (p *enqueueRequestForPod) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	pod := evt.Object.(*corev1.Pod)
	// 1. find pod's owners groupVersionKind
	ownerReferencesGVK := make(map[schema.GroupVersionKind]struct{})
	for _, owner := range pod.OwnerReferences {
		gv, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil {
			return
		}

		if owner.Kind == KruiseKindSts.Kind || owner.Kind == KindSts.Kind {
			continue
		}

		ownerReferencesGVK[schema.GroupVersionKind{
			Group:   gv.Group,
			Version: gv.Version,
			Kind:    owner.Kind,
		}] = struct{}{}
	}

	// 2.add watch
	for gvk, _ := range ownerReferencesGVK {
		enable := false
		for _, sgv := range whiteList.Workloads {
			if gvk.GroupKind().String() == sgv.String() {
				enable = true
				break
			}
		}
		if !enable {
			continue
		}

		_, err := util.AddWatcherDynamically(runtimeController, workloadHandler, gvk)
		if err != nil {
			continue
		}
	}
}
```
> workloadController: watch workload change to generate PPS request

```go
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

```
### 2.add workload finder
In the process of reconcile pps need to create ScaleAndSelector object.
```go
// ScaleAndSelector is used to return (controller, scale, selector) fields from the
// controller finder functions.
type ScaleAndSelector struct {
	ControllerReference
	// controller.spec.Replicas
	Scale int32
	// kruise statefulSet.spec.ReserveOrdinals
	ReserveOrdinals []int
	// controller.spec.Selector
	Selector *metav1.LabelSelector
	// metadata
	Metadata metav1.ObjectMeta
}
```
To support the custom workload finder, a new finder with a statefulset like needs to be added:
```go
// getPodStatefulSetLike returns the statefulset like referenced by the provided controllerRef.
func (r *ControllerFinder) getPodStatefulSetLike(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return nil, nil
	}
	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    ref.Kind,
	}
	workload := &unstructured.Unstructured{}
	workload.SetGroupVersionKind(gvk)
	err = r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, workload)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && workload.GetUID() != ref.UID {
		return nil, nil
	}

	scaleSelector := &ScaleAndSelector{
		Scale:    0,
		Selector: nil,
		ControllerReference: ControllerReference{
			APIVersion: workload.GetAPIVersion(),
			Kind:       workload.GetKind(),
			Name:       workload.GetName(),
			UID:        workload.GetUID(),
		},
		Metadata: metav1.ObjectMeta{
			Namespace:   workload.GetNamespace(),
			Name:        workload.GetName(),
			Annotations: workload.GetAnnotations(),
			UID:         workload.GetUID(),
		},
	}
	obj := workload.UnstructuredContent()
	if val, found, err := unstructured.NestedInt64(obj, "spec.replicas"); err == nil && found {
		scaleSelector.Scale = int32(val)
	}
	return scaleSelector, nil
}
```
### 3. add persistent annotations
```go
	// AnnotationPersistentPodAnnotations Pod needs persistent annotations
	// for example kruise.io/persistent-pod-annotations: cni.projectcalico.org/podIP[,xxx]
	// optional
	AnnotationPersistentPodAnnotations = "kruise.io/persistent-pod-annotations"

	// PersistentPodStateSpec defines the desired state of PersistentPodState
type PersistentPodStateSpec struct {

	// Persist the annotations information of the pods that need to be saved
	PersistentPodAnnotations []string `json:"persistentPodAnnotations,omitempty"`
}

type PodState struct {
	// pod persistent annotations
	Annotations map[string]string `json:"annotations,omitempty"`
}
```