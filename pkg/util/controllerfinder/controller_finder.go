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

package controllerfinder

import (
	"context"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/configuration"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	scaleclient "k8s.io/client-go/scale"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var Finder *ControllerFinder

func InitControllerFinder(mgr manager.Manager) error {
	Finder = &ControllerFinder{
		Client: mgr.GetClient(),
		mapper: mgr.GetRESTMapper(),
	}
	cfg := mgr.GetConfig()
	if cfg.GroupVersion == nil {
		cfg.GroupVersion = &schema.GroupVersion{}
	}
	codecs := serializer.NewCodecFactory(mgr.GetScheme())
	cfg.NegotiatedSerializer = codecs.WithoutConversion()
	restClient, err := rest.RESTClientFor(cfg)
	if err != nil {
		return err
	}
	k8sClient, err := clientset.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	Finder.discoveryClient = k8sClient.Discovery()
	scaleKindResolver := scaleclient.NewDiscoveryScaleKindResolver(Finder.discoveryClient)
	Finder.scaleNamespacer = scaleclient.New(restClient, Finder.mapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)
	return nil
}

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

type ControllerReference struct {
	// API version of the referent.
	APIVersion string `json:"apiVersion" protobuf:"bytes,5,opt,name=apiVersion"`
	// Kind of the referent.
	Kind string `json:"kind" protobuf:"bytes,1,opt,name=kind"`
	// Name of the referent.
	Name string `json:"name" protobuf:"bytes,3,opt,name=name"`
	// UID of the referent.
	UID types.UID `json:"uid" protobuf:"bytes,4,opt,name=uid,casttype=k8s.io/apimachinery/pkg/types.UID"`
}

// PodControllerFinder is a function type that maps a pod to a list of
// controllers and their scale.
type PodControllerFinder func(ref ControllerReference, namespace string) (*ScaleAndSelector, error)

type ControllerFinder struct {
	client.Client

	mapper          meta.RESTMapper
	scaleNamespacer scaleclient.ScalesGetter
	discoveryClient discovery.DiscoveryInterface
}

func (r *ControllerFinder) GetExpectedScaleForPods(pods []*corev1.Pod) (int32, error) {
	// 1. Find the controller for each pod.  If any pod has 0 controllers,
	// that's an error. With ControllerRef, a pod can only have 1 controller.
	// A mapping from controllers to their scale.
	podRefs := sets.NewString()
	controllerScale := map[types.UID]int32{}
	for _, pod := range pods {
		ref := metav1.GetControllerOf(pod)
		// ref has already been got, so there is no need to get again
		if ref == nil || podRefs.Has(string(ref.UID)) {
			continue
		}
		podRefs.Insert(string(ref.UID))
		// Check all the supported controllers to find the desired scale.
		workload, err := r.GetScaleAndSelectorForRef(ref.APIVersion, ref.Kind, pod.Namespace, ref.Name, ref.UID)
		if err != nil {
			return 0, err
		} else if workload == nil || !workload.Metadata.DeletionTimestamp.IsZero() {
			continue
		}
		controllerScale[workload.UID] = workload.Scale
	}
	// 2. Add up all the controllers.
	var expectedCount int32
	for _, count := range controllerScale {
		expectedCount += count
	}

	return expectedCount, nil
}

func (r *ControllerFinder) GetScaleAndSelectorForRef(apiVersion, kind, ns, name string, uid types.UID) (*ScaleAndSelector, error) {
	targetRef := ControllerReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		UID:        uid,
	}

	for _, finder := range r.Finders() {
		scale, err := finder(targetRef, ns)
		if scale != nil || err != nil {
			return scale, err
		}
	}
	return nil, nil
}

func (r *ControllerFinder) Finders() []PodControllerFinder {
	return []PodControllerFinder{r.getPodReplicationController, r.getPodDeployment, r.getPodReplicaSet,
		r.getPodStatefulSet, r.getPodKruiseCloneSet, r.getPodKruiseStatefulSet, r.getPodStatefulSetLike, r.getScaleController}
}

var (
	ControllerKindRS       = apps.SchemeGroupVersion.WithKind("ReplicaSet")
	ControllerKindSS       = apps.SchemeGroupVersion.WithKind("StatefulSet")
	ControllerKindRC       = corev1.SchemeGroupVersion.WithKind("ReplicationController")
	ControllerKindDep      = apps.SchemeGroupVersion.WithKind("Deployment")
	ControllerKruiseKindCS = appsv1alpha1.SchemeGroupVersion.WithKind("CloneSet")
	ControllerKruiseKindSS = appsv1beta1.SchemeGroupVersion.WithKind("StatefulSet")

	validWorkloadList = []schema.GroupVersionKind{ControllerKindRS, ControllerKindSS, ControllerKindRC, ControllerKindDep, ControllerKruiseKindCS, ControllerKruiseKindSS}
)

// getPodReplicaSet finds a replicaset which has no matching deployments.
func (r *ControllerFinder) getPodReplicaSet(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref.APIVersion, ref.Kind, ControllerKindRS)
	if !ok {
		return nil, nil
	}
	replicaSet, err := r.getReplicaSet(ref, namespace)
	if err != nil {
		return nil, err
	}
	if replicaSet == nil {
		return nil, nil
	}
	controllerRef := metav1.GetControllerOf(replicaSet)
	if controllerRef != nil && controllerRef.Kind == ControllerKindDep.Kind {
		refSs := ControllerReference{
			APIVersion: controllerRef.APIVersion,
			Kind:       controllerRef.Kind,
			Name:       controllerRef.Name,
			UID:        controllerRef.UID,
		}
		return r.getPodDeployment(refSs, namespace)
	}

	return &ScaleAndSelector{
		Scale:    *(replicaSet.Spec.Replicas),
		Selector: replicaSet.Spec.Selector,
		ControllerReference: ControllerReference{
			APIVersion: replicaSet.APIVersion,
			Kind:       replicaSet.Kind,
			Name:       replicaSet.Name,
			UID:        replicaSet.UID,
		},
		Metadata: replicaSet.ObjectMeta,
	}, nil
}

// getPodReplicaSet finds a replicaset which has no matching deployments.
func (r *ControllerFinder) getReplicaSet(ref ControllerReference, namespace string) (*apps.ReplicaSet, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref.APIVersion, ref.Kind, ControllerKindRS)
	if !ok {
		return nil, nil
	}
	replicaSet := &apps.ReplicaSet{}
	err := r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, replicaSet)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && replicaSet.UID != ref.UID {
		return nil, nil
	}
	return replicaSet, nil
}

// getPodStatefulSet returns the statefulset referenced by the provided controllerRef.
func (r *ControllerFinder) getPodStatefulSet(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref.APIVersion, ref.Kind, ControllerKindSS)
	if !ok {
		return nil, nil
	}
	statefulSet := &apps.StatefulSet{}
	err := r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, statefulSet)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && statefulSet.UID != ref.UID {
		return nil, nil
	}

	return &ScaleAndSelector{
		Scale:    *(statefulSet.Spec.Replicas),
		Selector: statefulSet.Spec.Selector,
		ControllerReference: ControllerReference{
			APIVersion: statefulSet.APIVersion,
			Kind:       statefulSet.Kind,
			Name:       statefulSet.Name,
			UID:        statefulSet.UID,
		},
		Metadata: statefulSet.ObjectMeta,
	}, nil
}

// getPodDeployments finds deployments for any replicasets which are being managed by deployments.
func (r *ControllerFinder) getPodDeployment(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref.APIVersion, ref.Kind, ControllerKindDep)
	if !ok {
		return nil, nil
	}
	deployment := &apps.Deployment{}
	err := r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, deployment)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && deployment.UID != ref.UID {
		return nil, nil
	}
	return &ScaleAndSelector{
		Scale:    *(deployment.Spec.Replicas),
		Selector: deployment.Spec.Selector,
		ControllerReference: ControllerReference{
			APIVersion: deployment.APIVersion,
			Kind:       deployment.Kind,
			Name:       deployment.Name,
			UID:        deployment.UID,
		},
		Metadata: deployment.ObjectMeta,
	}, nil
}

func (r *ControllerFinder) getPodReplicationController(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref.APIVersion, ref.Kind, ControllerKindRC)
	if !ok {
		return nil, nil
	}
	rc := &corev1.ReplicationController{}
	err := r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, rc)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && rc.UID != ref.UID {
		return nil, nil
	}
	return &ScaleAndSelector{
		Scale: *(rc.Spec.Replicas),
		ControllerReference: ControllerReference{
			APIVersion: rc.APIVersion,
			Kind:       rc.Kind,
			Name:       rc.Name,
			UID:        rc.UID,
		},
		Metadata: rc.ObjectMeta,
	}, nil
}

// getPodStatefulSet returns the kruise cloneSet referenced by the provided controllerRef.
func (r *ControllerFinder) getPodKruiseCloneSet(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref.APIVersion, ref.Kind, ControllerKruiseKindCS)
	if !ok {
		return nil, nil
	}
	cloneSet := &appsv1alpha1.CloneSet{}
	err := r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, cloneSet)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && cloneSet.UID != ref.UID {
		return nil, nil
	}

	return &ScaleAndSelector{
		Scale:    *(cloneSet.Spec.Replicas),
		Selector: cloneSet.Spec.Selector,
		ControllerReference: ControllerReference{
			APIVersion: cloneSet.APIVersion,
			Kind:       cloneSet.Kind,
			Name:       cloneSet.Name,
			UID:        cloneSet.UID,
		},
		Metadata: cloneSet.ObjectMeta,
	}, nil
}

// getPodStatefulSet returns the kruise statefulset referenced by the provided controllerRef.
func (r *ControllerFinder) getPodKruiseStatefulSet(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref.APIVersion, ref.Kind, ControllerKruiseKindSS)
	if !ok {
		return nil, nil
	}
	ss := &appsv1beta1.StatefulSet{}
	err := r.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: ref.Name}, ss)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && ss.UID != ref.UID {
		return nil, nil
	}

	return &ScaleAndSelector{
		Scale:           *(ss.Spec.Replicas),
		ReserveOrdinals: ss.Spec.ReserveOrdinals,
		Selector:        ss.Spec.Selector,
		ControllerReference: ControllerReference{
			APIVersion: ss.APIVersion,
			Kind:       ss.Kind,
			Name:       ss.Name,
			UID:        ss.UID,
		},
		Metadata: ss.ObjectMeta,
	}, nil
}

// getPodStatefulSetLike returns the statefulset like referenced by the provided controllerRef.
func (r *ControllerFinder) getPodStatefulSetLike(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return nil, nil
	}
	whiteList, err := configuration.GetPPSWatchCustomWorkloadWhiteList(r.Client)
	if err != nil {
		return nil, err
	}
	if !whiteList.ValidateAPIVersionAndKind(ref.APIVersion, ref.Kind) {
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

func (r *ControllerFinder) getScaleController(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	if isValidGroupVersionKind(ref.APIVersion, ref.Kind) {
		return nil, nil
	}
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return nil, err
	}
	gk := schema.GroupKind{
		Group: gv.Group,
		Kind:  ref.Kind,
	}

	mapping, err := r.mapper.RESTMapping(gk, gv.Version)
	if err != nil {
		return nil, err
	}
	gr := mapping.Resource.GroupResource()
	scale, err := r.scaleNamespacer.Scales(namespace).Get(context.TODO(), gr, ref.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// TODO, implementsScale
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && scale.UID != ref.UID {
		return nil, nil
	}
	selector, err := metav1.ParseToLabelSelector(scale.Status.Selector)
	if err != nil {
		return nil, err
	}
	return &ScaleAndSelector{
		Scale: scale.Spec.Replicas,
		ControllerReference: ControllerReference{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
			UID:        scale.UID,
		},
		Metadata: scale.ObjectMeta,
		Selector: selector,
	}, nil
}

func verifyGroupKind(apiVersion, kind string, gvk schema.GroupVersionKind) (bool, error) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return false, err
	}
	return gv.Group == gvk.Group && kind == gvk.Kind, nil
}

func isValidGroupVersionKind(apiVersion, kind string) bool {
	for _, gvk := range validWorkloadList {
		valid, err := verifyGroupKind(apiVersion, kind, gvk)
		if err != nil {
			return false
		} else if valid {
			return true
		}
	}
	return false
}
