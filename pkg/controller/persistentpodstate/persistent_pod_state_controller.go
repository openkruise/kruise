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
	"flag"
	"reflect"
	"strconv"
	"strings"

	"github.com/openkruise/kruise/pkg/util/configuration"
	"k8s.io/utils/ptr"

	ctrlUtil "github.com/openkruise/kruise/pkg/controller/util"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
)

func init() {
	flag.IntVar(&concurrentReconciles, "persistentpodstate-workers", concurrentReconciles, "Max concurrent workers for PersistentPodState controller.")
}

var (
	concurrentReconciles = 3

	// kubernetes
	KindSts = appsv1.SchemeGroupVersion.WithKind("StatefulSet")
	// kruise
	KruiseKindSts = appsv1beta1.SchemeGroupVersion.WithKind("StatefulSet")
	KruiseKindPps = appsv1alpha1.SchemeGroupVersion.WithKind("PersistentPodState")
	// AutoGeneratePersistentPodStatePrefix auto generate PersistentPodState crd
	AutoGeneratePersistentPodStatePrefix = "generate#"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new PersistentPodState Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !discovery.DiscoverGVK(KruiseKindPps) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cli := utilclient.NewClientFromManager(mgr, "persistentpodstate-controller")
	return &ReconcilePersistentPodState{
		Client: cli,
		scheme: mgr.GetScheme(),
		finder: controllerfinder.Finder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("persistentpodstate-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, &enqueueRequestForPod{reader: mgr.GetClient(), client: mgr.GetClient()})); err != nil {
		return err
	}

	// watch for changes to PersistentPodState
	if err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.PersistentPodState{}, &handler.TypedEnqueueRequestForObject[*appsv1alpha1.PersistentPodState]{})); err != nil {
		return err
	}

	// watch for changes to StatefulSet
	if err = c.Watch(source.Kind(mgr.GetCache(), &appsv1.StatefulSet{}, &enqueueRequestForStatefulSet{reader: mgr.GetClient()})); err != nil {
		return err
	}

	// watch for changes to kruise StatefulSet
	if err = c.Watch(source.Kind(mgr.GetCache(), &appsv1beta1.StatefulSet{}, &enqueueRequestForKruiseStatefulSet{reader: mgr.GetClient()})); err != nil {
		return err
	}

	whiteList, err := configuration.GetPPSWatchCustomWorkloadWhiteList(mgr.GetClient())
	if err != nil {
		return err
	}
	if whiteList != nil {
		workloadHandler := &enqueueRequestForStatefulSetLike{reader: mgr.GetClient()}
		for _, workload := range whiteList.Workloads {
			if _, err := ctrlUtil.AddWatcherDynamically(mgr, c, workloadHandler, workload, "PPS"); err != nil {
				return err
			}
		}
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePersistentPodState{}

type innerStatefulset struct {
	// replicas
	Replicas int32
	// kruise statefulset filed
	ReserveOrdinals   sets.Set[int]
	DeletionTimestamp *metav1.Time
}

// ReconcilePersistentPodState reconciles a PersistentPodState object
type ReconcilePersistentPodState struct {
	client.Client
	scheme *runtime.Scheme

	finder *controllerfinder.ControllerFinder
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=persistentpodstates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=persistentpodstates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=persistentpodstates/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a PersistentPodState object and makes changes based on the state read
// and what is in the PersistentPodState.Spec
func (r *ReconcilePersistentPodState) Reconcile(_ context.Context, req ctrl.Request) (ctrl.Result, error) {
	// auto generate PersistentPodState crd
	if strings.Contains(req.Name, AutoGeneratePersistentPodStatePrefix) {
		return ctrl.Result{}, r.autoGeneratePersistentPodState(req)
	}

	// Fetch the Statefulset instance
	persistentPodState := &appsv1alpha1.PersistentPodState{}
	err := r.Client.Get(context.TODO(), client.ObjectKey{Namespace: req.Namespace, Name: req.Name}, persistentPodState)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	klog.V(3).InfoS("Begin to reconcile PersistentPodState", "persistentPodState", klog.KObj(persistentPodState))
	pods, innerSts, err := r.getPodsAndStatefulset(persistentPodState)
	if err != nil {
		return ctrl.Result{}, err
		// delete workload scenario
	} else if innerSts == nil || !innerSts.DeletionTimestamp.IsZero() {
		newStatus := appsv1alpha1.PersistentPodStateStatus{}
		newStatus.ObservedGeneration = persistentPodState.Generation
		newStatus.PodStates = nil
		return ctrl.Result{}, r.updatePersistentPodStateStatus(persistentPodState, newStatus)
	}

	klog.V(3).InfoS("Reconcile statefulset pods for PersistentPodState", "persistentPodState", klog.KObj(persistentPodState), "podCount", len(pods))
	newStatus := persistentPodState.Status.DeepCopy()
	newStatus.ObservedGeneration = persistentPodState.Generation
	if newStatus.PodStates == nil {
		newStatus.PodStates = make(map[string]appsv1alpha1.PodState)
	}
	nodeTopologyKeys := sets.NewString()
	// required node labels
	if persistentPodState.Spec.RequiredPersistentTopology != nil {
		nodeTopologyKeys.Insert(persistentPodState.Spec.RequiredPersistentTopology.NodeTopologyKeys...)
	}
	// preferred node labels
	for _, item := range persistentPodState.Spec.PreferredPersistentTopology {
		nodeTopologyKeys.Insert(item.Preference.NodeTopologyKeys...)
	}

	annotationKeys := sets.NewString()
	for _, item := range persistentPodState.Spec.PersistentPodAnnotations {
		annotationKeys.Insert(item.Key)
	}

	// create sts scenario
	for _, pod := range pods {
		// 1. pod not ready, continue
		if !podutil.IsPodReady(pod) || pod.Spec.NodeName == "" {
			continue
		}
		// 2. check old pod state
		if podState, ok := newStatus.PodStates[pod.Name]; ok {
			currentKeys := sets.NewString()
			for key := range podState.NodeTopologyLabels {
				currentKeys.Insert(key)
			}
			currentAns := sets.NewString()
			for _, key := range podState.Annotations {
				currentAns.Insert(key)
			}

			// already recorded, no need to regenerate
			if podState.NodeName == pod.Spec.NodeName && nodeTopologyKeys.Equal(currentKeys) && annotationKeys.Equal(currentAns) {
				continue
			}
		}
		// 3. create new pod state
		newState, err := r.getPodState(pod, nodeTopologyKeys, annotationKeys)
		if err != nil {
			continue
		}
		// 4. store PodState
		newStatus.PodStates[pod.Name] = newState
	}

	// scale down statefulSet scenario
	if persistentPodState.Spec.PersistentPodStateRetentionPolicy != appsv1alpha1.PersistentPodStateRetentionPolicyWhenDeleted {
		for podName := range newStatus.PodStates {
			index, err := parseStsPodIndex(podName)
			if err != nil {
				klog.ErrorS(err, "Failed to parse PersistentPodState podName", "persistentPodState", klog.KObj(persistentPodState), "podName", podName)
				continue
			}
			if isInStatefulSetReplicas(index, innerSts) {
				continue
			}
			// others will be deleted for scaling down sts
			// if pod not exists, then delete PersistentPodState
			if _, ok := pods[podName]; !ok {
				delete(newStatus.PodStates, podName)
			}
		}
	}
	return ctrl.Result{}, r.updatePersistentPodStateStatus(persistentPodState, *newStatus)
}

func (r *ReconcilePersistentPodState) updatePersistentPodStateStatus(pps *appsv1alpha1.PersistentPodState, newStatus appsv1alpha1.PersistentPodStateStatus) error {
	if reflect.DeepEqual(pps.Status, newStatus) {
		return nil
	}
	// update PersistentPodState status
	persistentPodStateClone := &appsv1alpha1.PersistentPodState{}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.Client.Get(context.TODO(), client.ObjectKey{Namespace: pps.Namespace, Name: pps.Name}, persistentPodStateClone); err != nil {
			return err
		}
		persistentPodStateClone.Status = newStatus
		return r.Client.Status().Update(context.TODO(), persistentPodStateClone)
	}); err != nil {
		klog.ErrorS(err, "Failed to update PersistentPodState status", "persistentPodState", klog.KObj(pps))
		return err
	}
	klog.V(3).InfoS("Updated PersistentPodState status success", "persistentPodState", klog.KObj(pps), "oldPodStatesCount", len(pps.Status.PodStates), "newPodStatesCount", len(newStatus.PodStates))
	return nil
}

func parseStsPodIndex(podName string) (int, error) {
	index := strings.LastIndex(podName, "-")
	return strconv.Atoi(podName[index+1:])
}

// map[string]*corev1.Pod -> map[Pod.Name]*corev1.Pod
func (r *ReconcilePersistentPodState) getPodsAndStatefulset(persistentPodState *appsv1alpha1.PersistentPodState) (map[string]*corev1.Pod, *innerStatefulset, error) {
	inner := &innerStatefulset{}
	ref := persistentPodState.Spec.TargetReference
	workload, err := r.finder.GetScaleAndSelectorForRef(ref.APIVersion, ref.Kind, persistentPodState.Namespace, ref.Name, "")
	if err != nil {
		klog.ErrorS(err, "PersistentPodState fetch statefulSet failed", "persistentPodState", klog.KObj(persistentPodState), "statefulSetName", ref.Name)
		return nil, nil, err
	} else if workload == nil {
		return nil, nil, nil
	}
	inner.Replicas = workload.Scale
	inner.ReserveOrdinals = workload.ReserveOrdinals
	inner.DeletionTimestamp = workload.Metadata.DeletionTimestamp

	// DisableDeepCopy:true, indicates must be deep copy before update pod objection
	pods, _, err := r.finder.GetPodsForRef(ref.APIVersion, ref.Kind, persistentPodState.Namespace, ref.Name, true)
	if err != nil {
		klog.ErrorS(err, "Failed to list persistentPodState pods", "persistentPodState", klog.KObj(persistentPodState))
		return nil, nil, err
	}
	matchedPods := make(map[string]*corev1.Pod, len(pods))
	for i := range pods {
		pod := pods[i]
		matchedPods[pod.Name] = pod
	}
	return matchedPods, inner, nil
}

func (r *ReconcilePersistentPodState) getPodState(pod *corev1.Pod, nodeTopologyKeys sets.String, annotationKeys sets.String) (appsv1alpha1.PodState, error) {
	// pod state
	podState := appsv1alpha1.PodState{
		NodeTopologyLabels: map[string]string{},
		Annotations:        map[string]string{},
	}
	//get node of pod
	node := &corev1.Node{}
	err := r.Get(context.TODO(), client.ObjectKey{Name: pod.Spec.NodeName}, node)
	if err != nil {
		klog.ErrorS(err, "Fetched node of pod error", "pod", klog.KObj(pod), "nodeName", pod.Spec.NodeName)
		return podState, err
	}
	podState.NodeName = pod.Spec.NodeName
	for _, key := range nodeTopologyKeys.List() {
		if val, ok := node.Labels[key]; ok {
			podState.NodeTopologyLabels[key] = val
		}
	}
	for _, key := range annotationKeys.List() {
		if val, ok := pod.Annotations[key]; ok {
			podState.Annotations[key] = val
		}
	}
	return podState, nil
}

func isInStatefulSetReplicas(index int, sts *innerStatefulset) bool {
	replicas := sets.NewInt()
	replicaIndex := 0
	for realReplicaCount := 0; realReplicaCount < int(sts.Replicas); replicaIndex++ {
		if sts.ReserveOrdinals.Has(replicaIndex) {
			continue
		}
		realReplicaCount++
		replicas.Insert(replicaIndex)
	}
	return replicas.Has(index)
}

// auto generate PersistentPodState crd
func (r *ReconcilePersistentPodState) autoGeneratePersistentPodState(req ctrl.Request) error {
	// req.Name Format = generate#{apiVersion}#{workload.Kind}#{workload.Name}
	// example for generate#apps/v1#StatefulSet#echoserver
	arr := strings.Split(req.Name, "#")
	if len(arr) != 4 {
		klog.InfoS("Reconcile PersistentPodState workload is invalid", "workload", req)
		return nil
	}
	// fetch workload
	apiVersion, kind, ns, name := arr[1], arr[2], req.Namespace, arr[3]
	workload, err := r.finder.GetScaleAndSelectorForRef(apiVersion, kind, ns, name, "")
	if err != nil {
		return err
	} else if workload == nil {
		klog.InfoS("Reconcile PersistentPodState workload is Not Found", "workload", req)
		return nil
	}
	// fetch persistentPodState crd
	oldObj := &appsv1alpha1.PersistentPodState{}
	err = r.Client.Get(context.TODO(), client.ObjectKey{Namespace: ns, Name: name}, oldObj)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		// not found
		oldObj = nil
	}

	// auto generate persistentPodState crd object
	if workload.Metadata.Annotations[appsv1alpha1.AnnotationAutoGeneratePersistentPodState] == "true" {
		if workload.Metadata.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] == "" &&
			workload.Metadata.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] == "" {
			klog.InfoS("StatefulSet persistentPodState annotation was incomplete", "statefulSet", klog.KRef(workload.Metadata.Namespace, workload.Name))
			return nil
		}

		newObj := newStatefulSetPersistentPodState(workload)
		// create new obj
		if oldObj == nil {
			if err = r.Create(context.TODO(), newObj); err != nil {
				if errors.IsAlreadyExists(err) {
					return nil
				}
				return err
			}
			klog.V(3).InfoS("Created StatefulSet persistentPodState success", "statefulSet", klog.KRef(ns, name), "persistentPodState", klog.KObj(newObj))
			return nil
		}
		// compare with old object
		if reflect.DeepEqual(oldObj.Spec, newObj.Spec) {
			return nil
		}
		objClone := &appsv1alpha1.PersistentPodState{}
		if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err = r.Client.Get(context.TODO(), client.ObjectKey{Namespace: newObj.Namespace, Name: newObj.Name}, objClone); err != nil {
				return err
			}
			objClone.Spec = *newObj.Spec.DeepCopy()
			return r.Client.Update(context.TODO(), objClone)
		}); err != nil {
			klog.ErrorS(err, "Failed to update persistentPodState", "persistentPodState", klog.KObj(newObj))
			return err
		}
		klog.V(3).InfoS("Updated persistentPodState success", "persistentPodState", klog.KObj(newObj), "oldSpec",
			util.DumpJSON(oldObj.Spec), "newSpec", util.DumpJSON(newObj.Spec))
		return nil
	}

	// delete auto generated persistentPodState crd object
	if oldObj == nil {
		return nil
	}
	if err = r.Delete(context.TODO(), oldObj); err != nil {
		return err
	}
	klog.V(3).InfoS("Deleted StatefulSet persistentPodState", "statefulSet", klog.KRef(ns, name))
	return nil
}

func newStatefulSetPersistentPodState(workload *controllerfinder.ScaleAndSelector) *appsv1alpha1.PersistentPodState {
	obj := &appsv1alpha1.PersistentPodState{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workload.Name,
			Namespace: workload.Metadata.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: workload.APIVersion,
					Kind:       workload.Kind,
					Name:       workload.Name,
					Controller: ptr.To(true),
					UID:        workload.UID,
				},
			},
		},
		Spec: appsv1alpha1.PersistentPodStateSpec{
			TargetReference: appsv1alpha1.TargetReference{
				APIVersion: workload.APIVersion,
				Kind:       workload.Kind,
				Name:       workload.Name,
			},
		},
	}
	// required topology term
	if workload.Metadata.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology] != "" {
		requiredTopologyKeys := strings.Split(workload.Metadata.Annotations[appsv1alpha1.AnnotationRequiredPersistentTopology], ",")
		obj.Spec.RequiredPersistentTopology = &appsv1alpha1.NodeTopologyTerm{
			NodeTopologyKeys: requiredTopologyKeys,
		}
	}

	// persistent pod annotations
	if workload.Metadata.Annotations[appsv1alpha1.AnnotationPersistentPodAnnotations] != "" {
		annotationKeys := strings.Split(workload.Metadata.Annotations[appsv1alpha1.AnnotationPersistentPodAnnotations], ",")
		for _, key := range annotationKeys {
			obj.Spec.PersistentPodAnnotations = append(obj.Spec.PersistentPodAnnotations,
				appsv1alpha1.PersistentPodAnnotation{Key: key})
		}
	}

	// preferred topology term
	if workload.Metadata.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology] != "" {
		preferredTopologyKeys := strings.Split(workload.Metadata.Annotations[appsv1alpha1.AnnotationPreferredPersistentTopology], ",")
		obj.Spec.PreferredPersistentTopology = []appsv1alpha1.PreferredTopologyTerm{
			{
				Weight: 100,
				Preference: appsv1alpha1.NodeTopologyTerm{
					NodeTopologyKeys: preferredTopologyKeys,
				},
			},
		}
	}
	return obj
}
