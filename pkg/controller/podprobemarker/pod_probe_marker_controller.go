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
	"flag"
	"reflect"
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func init() {
	flag.IntVar(&concurrentReconciles, "podprobemarker-workers", concurrentReconciles, "Max concurrent workers for PodProbeMarker controller.")
}

var (
	concurrentReconciles = 3
)

const (
	ReconPodProbeMarker = "PodProbeMarker"
	ReconNodePodProbe   = "NodePodProbe"

	// PodProbeMarkerFinalizer is used to remove podProbe from NodePodProbe.Spec
	PodProbeMarkerFinalizer = "kruise.io/pod-probe-marker"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new PodProbeMarker Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cli := utilclient.NewClientFromManager(mgr, "PodProbeMarker-controller")
	return &ReconcilePodProbeMarker{
		Client: cli,
		scheme: mgr.GetScheme(),
		finder: controllerfinder.Finder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("PodProbeMarker-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// watch for changes to PodProbeMarker
	if err = c.Watch(&source.Kind{Type: &appsv1alpha1.PodProbeMarker{}}, &enqueueRequestForPodProbeMarker{}); err != nil {
		return err
	}

	// watch for changes to NodePodProbe
	if err = c.Watch(&source.Kind{Type: &appsv1alpha1.NodePodProbe{}}, &enqueueRequestForNodePodProbe{}); err != nil {
		return err
	}

	// watch for changes to pod
	if err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &enqueueRequestForPod{reader: mgr.GetClient()}); err != nil {
		return err
	}

	// watch for changes to node
	if err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &enqueueRequestForNode{Reader: mgr.GetClient()}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePodProbeMarker{}

// ReconcilePodProbeMarker reconciles a PodProbeMarker object
type ReconcilePodProbeMarker struct {
	client.Client
	scheme *runtime.Scheme

	finder *controllerfinder.ControllerFinder
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=podprobemarkers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=podprobemarkers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=nodepodprobes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=nodepodprobes/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a PodProbeMarker object and makes changes based on the state read
// and what is in the PodProbeMarker.Spec
func (r *ReconcilePodProbeMarker) Reconcile(_ context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	if strings.Contains(req.Name, ReconPodProbeMarker) {
		err = r.syncPodProbeMarker(req.Namespace, strings.Split(req.Name, "#")[1])
	} else {
		err = r.syncNodePodProbe(strings.Split(req.Name, "#")[1])
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ReconcilePodProbeMarker) syncPodProbeMarker(ns, name string) error {
	// Fetch the PodProbeMarker instance
	ppm := &appsv1alpha1.PodProbeMarker{}
	err := r.Get(context.TODO(), client.ObjectKey{Namespace: ns, Name: name}, ppm)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return nil
		}
		// Error reading the object - requeue the request.
		return err
	}
	pods, err := r.getMatchingPods(ppm)
	if err != nil {
		klog.Errorf("PodProbeMarker ppm(%s/%s) list pods failed: %s", ppm.Namespace, ppm.Name, err.Error())
		return err
	}
	// remove podProbe from NodePodProbe.Spec
	if !ppm.DeletionTimestamp.IsZero() {
		return r.handlerPodProbeMarkerFinalizer(ppm, pods)
	}
	// add finalizer
	if !controllerutil.ContainsFinalizer(ppm, PodProbeMarkerFinalizer) {
		err = util.UpdateFinalizer(r.Client, ppm, util.AddFinalizerOpType, PodProbeMarkerFinalizer)
		if err != nil {
			klog.Errorf("add PodProbeMarker(%s/%s) finalizer failed: %s", ppm.Namespace, ppm.Name, err.Error())
			return err
		}
		klog.V(3).Infof("add PodProbeMarker(%s/%s) finalizer success", ppm.Namespace, ppm.Name)
	}
	// add podProbe in NodePodProbe
	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			continue
		}
		// add podProbe to NodePodProbe.Spec
		if err = r.updateNodePodProbe(ppm, pod); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcilePodProbeMarker) handlerPodProbeMarkerFinalizer(ppm *appsv1alpha1.PodProbeMarker, pods []*corev1.Pod) error {
	if !controllerutil.ContainsFinalizer(ppm, PodProbeMarkerFinalizer) {
		return nil
	}
	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			continue
		}
		if err := r.removePodProbeFromNodePodProbe(ppm.Name, pod.Spec.NodeName); err != nil {
			return err
		}
	}
	err := util.UpdateFinalizer(r.Client, ppm, util.RemoveFinalizerOpType, PodProbeMarkerFinalizer)
	if err != nil {
		klog.Errorf("remove PodProbeMarker(%s/%s) finalizer failed: %s", ppm.Namespace, ppm.Name, err.Error())
		return err
	}
	klog.V(3).Infof("remove PodProbeMarker(%s/%s) finalizer success", ppm.Namespace, ppm.Name)
	return nil
}

func (r *ReconcilePodProbeMarker) updateNodePodProbe(ppm *appsv1alpha1.PodProbeMarker, pod *corev1.Pod) error {
	npp := &appsv1alpha1.NodePodProbe{}
	err := r.Get(context.TODO(), client.ObjectKey{Name: pod.Spec.NodeName}, npp)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Warningf("PodProbeMarker ppm(%s/%s) NodePodProbe(%s) is Not Found", ppm.Namespace, ppm.Name, npp.Name)
			return nil
		}
		// Error reading the object - requeue the request.
		klog.Errorf("PodProbeMarker ppm(%s/%s) get NodePodProbe(%s) failed: %s", ppm.Namespace, ppm.Name, pod.Spec.NodeName, err.Error())
		return err
	}

	oldSpec := npp.Spec.DeepCopy()
	exist := false
	for i := range npp.Spec.PodProbes {
		podProbe := &npp.Spec.PodProbes[i]
		if podProbe.Name == pod.Name && podProbe.Namespace == pod.Namespace && podProbe.Uid == string(pod.UID) {
			exist = true
			for j := range ppm.Spec.Probes {
				probe := ppm.Spec.Probes[j]
				probe.PodProbeMarkerName = ppm.Name
				probe.MarkerPolicy = nil
				setPodContainerProbes(podProbe, probe)
			}
			break
		}
	}
	if !exist {
		podProbe := appsv1alpha1.PodProbe{Name: pod.Name, Namespace: pod.Namespace, Uid: string(pod.UID)}
		for j := range ppm.Spec.Probes {
			probe := ppm.Spec.Probes[j]
			probe.PodProbeMarkerName = ppm.Name
			probe.MarkerPolicy = nil
			podProbe.Probes = append(podProbe.Probes, probe)
		}
		npp.Spec.PodProbes = append(npp.Spec.PodProbes, podProbe)
	}

	if reflect.DeepEqual(npp.Spec, oldSpec) {
		return nil
	}
	err = r.Update(context.TODO(), npp)
	if err != nil {
		klog.Errorf("PodProbeMarker ppm(%s/%s) Update NodePodProbe(%s) failed: %s", ppm.Namespace, ppm.Name, npp.Name, err.Error())
		return err
	}
	klog.V(3).Infof("PodProbeMarker ppm(%s/%s) update NodePodProbe(%s) from(%s) -> to(%s) success",
		ppm.Namespace, ppm.Name, npp.Name, util.DumpJSON(oldSpec), util.DumpJSON(npp.Spec))
	return nil
}

func setPodContainerProbes(podProbe *appsv1alpha1.PodProbe, probe appsv1alpha1.ContainerProbe) {
	for i, obj := range podProbe.Probes {
		if obj.Name == probe.Name {
			other := obj.DeepCopy()
			// PodProbeMarkerName is used internally by the index(for PodProbeMarker Object), so no comparison is required
			other.PodProbeMarkerName = ""
			if !reflect.DeepEqual(other, probe) {
				podProbe.Probes[i] = probe
			}
			return
		}
	}
	podProbe.Probes = append(podProbe.Probes, probe)
}

// If you need update the pod object, you must DeepCopy it
func (r *ReconcilePodProbeMarker) getMatchingPods(ppm *appsv1alpha1.PodProbeMarker) ([]*corev1.Pod, error) {
	// get more faster selector
	selector, err := util.ValidatedLabelSelectorAsSelector(ppm.Spec.Selector)
	if err != nil {
		return nil, err
	}
	// DisableDeepCopy:true, indicates must be deep copy before update pod objection
	listOpts := &client.ListOptions{LabelSelector: selector, Namespace: ppm.Namespace}
	podList := &corev1.PodList{}
	if listErr := r.Client.List(context.TODO(), podList, listOpts, utilclient.DisableDeepCopy); listErr != nil {
		return nil, err
	}
	pods := make([]*corev1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
		if kubecontroller.IsPodActive(pod) {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}
