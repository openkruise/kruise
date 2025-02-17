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

package nodepodprobe

import (
	"context"
	"flag"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
)

func init() {
	flag.IntVar(&concurrentReconciles, "nodepodprobe-workers", concurrentReconciles, "Max concurrent workers for NodePodProbe controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("NodePodProbe")
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new NodePodProbe Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) ||
		!utilfeature.DefaultFeatureGate.Enabled(features.PodProbeMarkerGate) ||
		!utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cli := utilclient.NewClientFromManager(mgr, "NodePodProbe-controller")
	return &ReconcileNodePodProbe{
		Client: cli,
		scheme: mgr.GetScheme(),
		finder: controllerfinder.Finder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("NodePodProbe-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// watch for changes to NodePodProbe
	if err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.NodePodProbe{}, &enqueueRequestForNodePodProbe{})); err != nil {
		return err
	}

	// watch for changes to pod
	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, &enqueueRequestForPod{reader: mgr.GetClient()})); err != nil {
		return err
	}

	// watch for changes to node
	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Node{}, &enqueueRequestForNode{Reader: mgr.GetClient()})); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileNodePodProbe{}

// ReconcileNodePodProbe reconciles a NodePodProbe object
type ReconcileNodePodProbe struct {
	client.Client
	scheme *runtime.Scheme

	finder *controllerfinder.ControllerFinder
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=nodepodprobes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=nodepodprobes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=nodepodprobes/finalizers,verbs=update

// Reconcile reads that state of the cluster for a NodePodProbe object and makes changes based on the state read
// and what is in the NodePodProbe.Spec
func (r *ReconcileNodePodProbe) Reconcile(_ context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.syncNodePodProbe(req.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ReconcileNodePodProbe) syncNodePodProbe(name string) error {
	// Fetch the NodePodProbe instance
	npp := &appsv1alpha1.NodePodProbe{}
	err := r.Get(context.TODO(), client.ObjectKey{Name: name}, npp)
	if err != nil {
		if errors.IsNotFound(err) {
			npp.Name = name
			return r.Create(context.TODO(), npp)
		}
		// Error reading the object - requeue the request.
		return err
	}
	// Fetch Node
	node := &corev1.Node{}
	err = r.Get(context.TODO(), client.ObjectKey{Name: name}, node)
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) || !node.DeletionTimestamp.IsZero() {
		return r.Delete(context.TODO(), npp)
	}
	// If Pod is deleted, then remove podProbe from NodePodProbe.Spec
	matchedPods, err := r.syncPodFromNodePodProbe(npp)
	if err != nil {
		return err
	}
	for _, status := range npp.Status.PodProbeStatuses {
		pod, ok := matchedPods[status.UID]
		if !ok {
			continue
		}
		// Write podProbe state to Pod metadata and condition
		if err = r.updatePodProbeStatus(pod, status); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcileNodePodProbe) syncPodFromNodePodProbe(npp *appsv1alpha1.NodePodProbe) (map[string]*corev1.Pod, error) {
	// map[pod.uid]=Pod
	matchedPods := map[string]*corev1.Pod{}
	for _, obj := range npp.Spec.PodProbes {
		pod := &corev1.Pod{}
		err := r.Get(context.TODO(), client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, pod)
		if err != nil && !errors.IsNotFound(err) {
			klog.ErrorS(err, "NodePodProbe got pod failed", "pod", klog.KRef(obj.Namespace, obj.Name))
			return nil, err
		}
		if errors.IsNotFound(err) || !kubecontroller.IsPodActive(pod) || string(pod.UID) != obj.UID {
			continue
		}
		matchedPods[string(pod.UID)] = pod
	}

	newSpec := appsv1alpha1.NodePodProbeSpec{}
	for i := range npp.Spec.PodProbes {
		obj := npp.Spec.PodProbes[i]
		if _, ok := matchedPods[obj.UID]; ok {
			newSpec.PodProbes = append(newSpec.PodProbes, obj)
		}
	}
	if reflect.DeepEqual(newSpec, npp.Spec) {
		return matchedPods, nil
	}

	nppClone := &appsv1alpha1.NodePodProbe{}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: npp.Name}, nppClone); err != nil {
			klog.ErrorS(err, "Failed to get updated NodePodProbe from client", "nodePodProbe", klog.KObj(npp))
		}
		if reflect.DeepEqual(newSpec, nppClone.Spec) {
			return nil
		}
		nppClone.Spec = newSpec
		return r.Client.Update(context.TODO(), nppClone)
	})
	if err != nil {
		klog.ErrorS(err, "Failed to update NodePodProbe", "nodePodProbe", klog.KObj(npp))
		return nil, err
	}
	klog.V(3).InfoS("Updated NodePodProbe success", "nodePodProbe", klog.KObj(npp), "oldSpec", util.DumpJSON(npp.Spec), "newSpec", util.DumpJSON(newSpec))
	return matchedPods, nil
}

func (r *ReconcileNodePodProbe) updatePodProbeStatus(pod *corev1.Pod, status appsv1alpha1.PodProbeStatus) error {
	// map[probe.name]->probeState
	currentConditions := make(map[string]*corev1.PodCondition)
	for i := range pod.Status.Conditions {
		condition := &pod.Status.Conditions[i]
		currentConditions[string(condition.Type)] = condition
	}
	type metadata struct {
		Labels      map[string]interface{} `json:"labels,omitempty"`
		Annotations map[string]interface{} `json:"annotations,omitempty"`
	}
	// patch labels or annotations in pod
	probeMetadata := metadata{
		Labels:      map[string]interface{}{},
		Annotations: map[string]interface{}{},
	}
	// pod status condition record probe result
	var probeConditions []corev1.PodCondition
	var err error
	validConditionTypes := sets.NewString()
	for i := range status.ProbeStates {
		probeState := status.ProbeStates[i]
		if probeState.State == "" {
			continue
		}
		// fetch podProbeMarker
		ppmName, probeName := strings.Split(probeState.Name, "#")[0], strings.Split(probeState.Name, "#")[1]
		ppm := &appsv1alpha1.PodProbeMarker{}
		err = r.Get(context.TODO(), client.ObjectKey{Namespace: pod.Namespace, Name: ppmName}, ppm)
		if err != nil {
			// when NodePodProbe is deleted, should delete probes from NodePodProbe.spec
			if errors.IsNotFound(err) {
				continue
			}
			klog.ErrorS(err, "NodePodProbe got pod failed", "podProbeMarkerName", ppmName, "pod", klog.KObj(pod))
			return err
		} else if !ppm.DeletionTimestamp.IsZero() {
			continue
		}
		var policy []appsv1alpha1.ProbeMarkerPolicy
		var conditionType string
		for _, probe := range ppm.Spec.Probes {
			if probe.Name == probeName {
				policy = probe.MarkerPolicy
				conditionType = probe.PodConditionType
				break
			}
		}
		if conditionType != "" && validConditionTypes.Has(conditionType) {
			klog.InfoS("NodePodProbe pod condition was conflict", "podProbeMarkerName", ppmName, "pod", klog.KObj(pod), "conditionType", conditionType)
			// patch pod condition
		} else if conditionType != "" {
			validConditionTypes.Insert(conditionType)
			var conStatus corev1.ConditionStatus
			if probeState.State == appsv1alpha1.ProbeSucceeded {
				conStatus = corev1.ConditionTrue
			} else {
				conStatus = corev1.ConditionFalse
			}
			probeConditions = append(probeConditions, corev1.PodCondition{
				Type:               corev1.PodConditionType(conditionType),
				Status:             conStatus,
				LastProbeTime:      probeState.LastProbeTime,
				LastTransitionTime: probeState.LastTransitionTime,
				Message:            probeState.Message,
			})
		}
		if len(policy) == 0 {
			continue
		}
		// matchedPolicy is when policy.state is equal to probeState.State, otherwise oppositePolicy
		// 1. If policy[0].state = Succeeded, policy[1].state = Failed. probeState.State = Succeeded.
		// So policy[0] is matchedPolicy, policy[1] is oppositePolicy
		// 2. If policy[0].state = Succeeded, and policy[1] does not exist. probeState.State = Succeeded.
		// So policy[0] is matchedPolicy, oppositePolicy is nil
		// 3. If policy[0].state = Succeeded, and policy[1] does not exist. probeState.State = Failed.
		// So policy[0] is oppositePolicy, matchedPolicy is nil
		var matchedPolicy, oppositePolicy *appsv1alpha1.ProbeMarkerPolicy
		for j := range policy {
			if policy[j].State == probeState.State {
				matchedPolicy = &policy[j]
			} else {
				oppositePolicy = &policy[j]
			}
		}
		if oppositePolicy != nil {
			for k := range oppositePolicy.Labels {
				probeMetadata.Labels[k] = nil
			}
			for k := range oppositePolicy.Annotations {
				probeMetadata.Annotations[k] = nil
			}
		}
		if matchedPolicy != nil {
			for k, v := range matchedPolicy.Labels {
				probeMetadata.Labels[k] = v
			}
			for k, v := range matchedPolicy.Annotations {
				probeMetadata.Annotations[k] = v
			}
		}
	}
	// probe condition no changed, continue
	if len(probeConditions) == 0 && len(probeMetadata.Labels) == 0 && len(probeMetadata.Annotations) == 0 {
		return nil
	}
	//update pod metadata and status condition
	podClone := pod.DeepCopy()
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err = r.Client.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, podClone); err != nil {
			klog.ErrorS(err, "Failed to get updated pod from client", "pod", klog.KObj(pod))
			return err
		}
		oldStatus := podClone.Status.DeepCopy()
		for i := range probeConditions {
			condition := probeConditions[i]
			util.SetPodConditionIfMsgChanged(podClone, condition)
		}
		oldMetadata := podClone.ObjectMeta.DeepCopy()
		if podClone.Annotations == nil {
			podClone.Annotations = map[string]string{}
		}
		for k, v := range probeMetadata.Labels {
			// delete the label
			if v == nil {
				delete(podClone.Labels, k)
				// patch the label
			} else {
				podClone.Labels[k] = v.(string)
			}
		}
		for k, v := range probeMetadata.Annotations {
			// delete the annotation
			if v == nil {
				delete(podClone.Annotations, k)
				// patch the annotation
			} else {
				podClone.Annotations[k] = v.(string)
			}
		}
		if reflect.DeepEqual(oldStatus.Conditions, podClone.Status.Conditions) && reflect.DeepEqual(oldMetadata.Labels, podClone.Labels) &&
			reflect.DeepEqual(oldMetadata.Annotations, podClone.Annotations) {
			return nil
		}
		// todo: resolve https://github.com/openkruise/kruise/issues/1597
		return r.Client.Status().Update(context.TODO(), podClone)
	}); err != nil {
		klog.ErrorS(err, "NodePodProbe patched pod status failed", "pod", klog.KObj(podClone))
		return err
	}
	klog.V(3).InfoS("NodePodProbe updated pod metadata and conditions success", "pod", klog.KObj(podClone), "metaData",
		util.DumpJSON(probeMetadata), "conditions", util.DumpJSON(probeConditions))
	return nil
}
