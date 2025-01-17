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
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	flag.IntVar(&concurrentReconciles, "podprobemarker-workers", concurrentReconciles, "Max concurrent workers for PodProbeMarker controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("PodProbeMarker")
)

const (
	// PodProbeMarkerFinalizer is on PodProbeMarker, and used to remove podProbe from NodePodProbe.Spec
	PodProbeMarkerFinalizer = "kruise.io/node-pod-probe-cleanup"

	VirtualKubelet = "virtual-kubelet"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new PodProbeMarker Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) || !utilfeature.DefaultFeatureGate.Enabled(features.PodProbeMarkerGate) {
		return nil
	}
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
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// watch for changes to PodProbeMarker
	if err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.PodProbeMarker{}, &enqueueRequestForPodProbeMarker{})); err != nil {
		return err
	}
	// watch for changes to pod
	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, &enqueueRequestForPod{reader: mgr.GetClient()})); err != nil {
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
// +kubebuilder:rbac:groups=apps.kruise.io,resources=podprobemarkers/finalizers,verbs=update

// Reconcile reads that state of the cluster for a PodProbeMarker object and makes changes based on the state read
// and what is in the PodProbeMarker.Spec
func (r *ReconcilePodProbeMarker) Reconcile(_ context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.syncPodProbeMarker(req.Namespace, req.Name)
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
	normalPods, serverlessPods, err := r.getMatchingPods(ppm)
	if err != nil {
		klog.ErrorS(err, "PodProbeMarker listed pods failed", "podProbeMarker", klog.KObj(ppm))
		return err
	}
	// remove podProbe from NodePodProbe.Spec
	if !ppm.DeletionTimestamp.IsZero() {
		return r.handlerPodProbeMarkerFinalizer(ppm, normalPods)
	}
	// add finalizer
	if !controllerutil.ContainsFinalizer(ppm, PodProbeMarkerFinalizer) {
		err = util.UpdateFinalizer(r.Client, ppm, util.AddFinalizerOpType, PodProbeMarkerFinalizer)
		if err != nil {
			klog.ErrorS(err, "Failed to add PodProbeMarker finalizer", "podProbeMarker", klog.KObj(ppm))
			return err
		}
		klog.V(3).InfoS("Added PodProbeMarker finalizer success", "podProbeMarker", klog.KObj(ppm))
	}

	// map[probe.PodConditionType] = probe.MarkerPolicy
	markers := make(map[string][]appsv1alpha1.ProbeMarkerPolicy)
	for _, probe := range ppm.Spec.Probes {
		if probe.PodConditionType == "" || len(probe.MarkerPolicy) == 0 {
			continue
		}
		markers[probe.PodConditionType] = probe.MarkerPolicy
	}
	if utilfeature.DefaultFeatureGate.Enabled(features.EnablePodProbeMarkerOnServerless) && len(markers) != 0 {
		for _, pod := range serverlessPods {
			if err = r.markServerlessPod(pod, markers); err != nil {
				klog.ErrorS(err, "Failed to marker serverless pod", "podProbeMarker", klog.KObj(ppm), "pod", klog.KObj(pod))
				return err
			}
		}
	}

	groupByNode := make(map[string][]*corev1.Pod)
	for i, pod := range normalPods {
		groupByNode[pod.Spec.NodeName] = append(groupByNode[pod.Spec.NodeName], normalPods[i])
	}

	// add podProbe in NodePodProbe
	for nodeName := range groupByNode {
		// add podProbe to NodePodProbe.Spec
		if err = r.updateNodePodProbes(ppm, nodeName, groupByNode[nodeName]); err != nil {
			return err
		}
	}
	// update podProbeMarker status
	ppmClone := ppm.DeepCopy()
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.Client.Get(context.TODO(), client.ObjectKey{Namespace: ppm.Namespace, Name: ppm.Name}, ppmClone); err != nil {
			klog.ErrorS(err, "Failed to get updated PodProbeMarker from client", "podProbeMarker", klog.KObj(ppm))
		}
		matchedPods := len(normalPods) + len(serverlessPods)
		if ppmClone.Status.ObservedGeneration == ppmClone.Generation && int(ppmClone.Status.MatchedPods) == matchedPods {
			return nil
		}
		ppmClone.Status.ObservedGeneration = ppmClone.Generation
		ppmClone.Status.MatchedPods = int64(matchedPods)
		return r.Client.Status().Update(context.TODO(), ppmClone)
	}); err != nil {
		klog.ErrorS(err, "PodProbeMarker update status failed", "podProbeMarker", klog.KObj(ppm))
		return err
	}
	klog.V(3).InfoS("PodProbeMarker update status success", "podProbeMarker", klog.KObj(ppm), "status", util.DumpJSON(ppmClone.Status))
	return nil
}

func (r *ReconcilePodProbeMarker) handlerPodProbeMarkerFinalizer(ppm *appsv1alpha1.PodProbeMarker, pods []*corev1.Pod) error {
	if !controllerutil.ContainsFinalizer(ppm, PodProbeMarkerFinalizer) {
		return nil
	}
	for _, pod := range pods {
		if err := r.removePodProbeFromNodePodProbe(ppm.Name, pod.Spec.NodeName); err != nil {
			return err
		}
	}
	err := util.UpdateFinalizer(r.Client, ppm, util.RemoveFinalizerOpType, PodProbeMarkerFinalizer)
	if err != nil {
		klog.ErrorS(err, "Failed to remove PodProbeMarker finalizer", "podProbeMarker", klog.KObj(ppm))
		return err
	}
	klog.V(3).InfoS("Removed PodProbeMarker finalizer success", "podProbeMarker", klog.KObj(ppm))
	return nil
}

// marker labels or annotations on Pod based on probing results
// markers is map[probe.PodConditionType] = probe.MarkerPolicy
func (r *ReconcilePodProbeMarker) markServerlessPod(pod *corev1.Pod, markers map[string][]appsv1alpha1.ProbeMarkerPolicy) error {
	newObjectMeta := *pod.ObjectMeta.DeepCopy()
	if newObjectMeta.Annotations == nil {
		newObjectMeta.Annotations = make(map[string]string)
	}
	if newObjectMeta.Labels == nil {
		newObjectMeta.Labels = make(map[string]string)
	}

	for cond, policy := range markers {
		condition := util.GetCondition(pod, corev1.PodConditionType(cond))
		if condition == nil {
			continue
		}

		for _, obj := range policy {
			if !obj.State.IsEqualPodConditionStatus(condition.Status) {
				continue
			}
			for k, v := range obj.Annotations {
				newObjectMeta.Annotations[k] = v
			}
			for k, v := range obj.Labels {
				newObjectMeta.Labels[k] = v
			}
		}
	}

	// no change
	if reflect.DeepEqual(pod.ObjectMeta, newObjectMeta) {
		return nil
	}

	// patch change
	oldBytes, _ := json.Marshal(corev1.Pod{ObjectMeta: pod.ObjectMeta})
	newBytes, _ := json.Marshal(corev1.Pod{ObjectMeta: newObjectMeta})
	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldBytes, newBytes, &corev1.Pod{})
	if err != nil {
		return err
	}
	obj := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name}}
	if err = r.Patch(context.TODO(), obj, client.RawPatch(types.StrategicMergePatchType, patchBytes)); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	klog.V(3).InfoS("PodProbeMarker marker pod success", "pod", klog.KObj(pod), "patch", string(patchBytes))
	return nil
}

func (r *ReconcilePodProbeMarker) updateNodePodProbes(ppm *appsv1alpha1.PodProbeMarker, nodeName string, pods []*corev1.Pod) error {
	npp := &appsv1alpha1.NodePodProbe{}
	err := r.Get(context.TODO(), client.ObjectKey{Name: nodeName}, npp)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.InfoS("PodProbeMarker NodePodProbe was Not Found", "podProbeMarker", klog.KObj(ppm), "nodePodProbe", klog.KObj(npp))
			return nil
		}
		// Error reading the object - requeue the request.
		klog.ErrorS(err, "PodProbeMarker got NodePodProbe failed", "podProbeMarker", klog.KObj(ppm), "nodePodProbe", klog.KObj(npp))
		return err
	}

	oldSpec := npp.Spec.DeepCopy()
	for _, pod := range pods {
		exist := false
		for i := range npp.Spec.PodProbes {
			podProbe := &npp.Spec.PodProbes[i]
			if podProbe.Name == pod.Name && podProbe.Namespace == pod.Namespace && podProbe.UID == string(pod.UID) {
				exist = true
				for j := range ppm.Spec.Probes {
					probe := ppm.Spec.Probes[j]
					if podProbe.IP == "" {
						podProbe.IP = pod.Status.PodIP
					}
					if probe.Probe.TCPSocket != nil {
						probe, err = convertTcpSocketProbeCheckPort(probe, pod)
						if err != nil {
							klog.ErrorS(err, "Failed to convert tcpSocket probe port", "pod", klog.KObj(pod))
							continue
						}
					}
					if probe.Probe.HTTPGet != nil {
						probe, err = convertHttpGetProbeCheckPort(probe, pod)
						if err != nil {
							klog.ErrorS(err, "Failed to convert httpGet probe port", "pod", klog.KObj(pod))
							continue
						}
					}
					setPodContainerProbes(podProbe, probe, ppm.Name)
				}
				break
			}
		}
		if !exist {
			podProbe := appsv1alpha1.PodProbe{Name: pod.Name, Namespace: pod.Namespace, UID: string(pod.UID), IP: pod.Status.PodIP}
			for j := range ppm.Spec.Probes {
				probe := ppm.Spec.Probes[j]
				// look up a port in a container by name & convert container name port
				if probe.Probe.TCPSocket != nil {
					probe, err = convertTcpSocketProbeCheckPort(probe, pod)
					if err != nil {
						klog.ErrorS(err, "Failed to convert tcpSocket probe port", "pod", klog.KObj(pod))
						continue
					}
				}
				if probe.Probe.HTTPGet != nil {
					probe, err = convertHttpGetProbeCheckPort(probe, pod)
					if err != nil {
						klog.ErrorS(err, "Failed to convert httpGet probe port", "pod", klog.KObj(pod))
						continue
					}
				}
				podProbe.Probes = append(podProbe.Probes, appsv1alpha1.ContainerProbe{
					Name:          fmt.Sprintf("%s#%s", ppm.Name, probe.Name),
					ContainerName: probe.ContainerName,
					Probe:         probe.Probe,
				})
			}
			npp.Spec.PodProbes = append(npp.Spec.PodProbes, podProbe)
		}
	}

	if reflect.DeepEqual(npp.Spec, oldSpec) {
		return nil
	}
	err = r.Update(context.TODO(), npp)
	if err != nil {
		klog.ErrorS(err, "PodProbeMarker updated NodePodProbe failed", "podProbeMarker", klog.KObj(ppm), "nodePodProbeName", npp.Name)
		return err
	}
	klog.V(3).InfoS("PodProbeMarker updated NodePodProbe success", "podProbeMarker", klog.KObj(ppm), "nodePodProbeName", npp.Name,
		"oldSpec", util.DumpJSON(oldSpec), "newSpec", util.DumpJSON(npp.Spec))
	return nil
}

func convertHttpGetProbeCheckPort(probe appsv1alpha1.PodContainerProbe, pod *corev1.Pod) (appsv1alpha1.PodContainerProbe, error) {
	probeNew := probe.DeepCopy()
	if probe.Probe.HTTPGet.Port.Type == intstr.Int {
		return *probeNew, nil
	}
	container := util.GetPodContainerByName(probe.ContainerName, pod)
	if container == nil {
		return *probeNew, fmt.Errorf("Failed to get container by name: %v in pod: %v/%v", probe.ContainerName, pod.Namespace, pod.Name)
	}
	portInt, err := util.ExtractPort(probe.Probe.HTTPGet.Port, *container)
	if err != nil {
		return *probeNew, fmt.Errorf("Failed to extract port for container: %v in pod: %v/%v", container.Name, pod.Namespace, pod.Name)
	}
	// If you need to parse integer values with specific bit sizes, avoid strconv.Atoi,
	// and instead use strconv.ParseInt or strconv.ParseUint, which also allow specifying the bit size.
	// https://codeql.github.com/codeql-query-help/go/go-incorrect-integer-conversion/
	probeNew.Probe.HTTPGet.Port = intstr.FromInt(portInt)
	return *probeNew, nil
}

func convertTcpSocketProbeCheckPort(probe appsv1alpha1.PodContainerProbe, pod *corev1.Pod) (appsv1alpha1.PodContainerProbe, error) {
	probeNew := probe.DeepCopy()
	if probe.Probe.TCPSocket.Port.Type == intstr.Int {
		return *probeNew, nil
	}
	container := util.GetPodContainerByName(probe.ContainerName, pod)
	if container == nil {
		return *probeNew, fmt.Errorf("Failed to get container by name: %v in pod: %v/%v", probe.ContainerName, pod.Namespace, pod.Name)
	}
	portInt, err := util.ExtractPort(probe.Probe.TCPSocket.Port, *container)
	if err != nil {
		return *probeNew, fmt.Errorf("Failed to extract port for container: %v in pod: %v/%v", container.Name, pod.Namespace, pod.Name)
	}
	probeNew.Probe.TCPSocket.Port = intstr.FromInt(portInt)
	return *probeNew, nil
}

func setPodContainerProbes(podProbe *appsv1alpha1.PodProbe, probe appsv1alpha1.PodContainerProbe, ppmName string) {
	newProbe := appsv1alpha1.ContainerProbe{
		Name:          fmt.Sprintf("%s#%s", ppmName, probe.Name),
		ContainerName: probe.ContainerName,
		Probe:         probe.Probe,
	}
	for i, obj := range podProbe.Probes {
		if obj.Name == newProbe.Name {
			if !reflect.DeepEqual(obj, newProbe) {
				podProbe.Probes[i] = newProbe
			}
			return
		}
	}
	podProbe.Probes = append(podProbe.Probes, newProbe)
}

// If you need update the pod object, you must DeepCopy it
// para1: normal pods, which is on normal node
// para2: serverless pods
func (r *ReconcilePodProbeMarker) getMatchingPods(ppm *appsv1alpha1.PodProbeMarker) ([]*corev1.Pod, []*corev1.Pod, error) {
	// get more faster selector
	selector, err := util.ValidatedLabelSelectorAsSelector(ppm.Spec.Selector)
	if err != nil {
		return nil, nil, err
	}
	// DisableDeepCopy:true, indicates must be deep copy before update pod objection
	listOpts := &client.ListOptions{LabelSelector: selector, Namespace: ppm.Namespace}
	podList := &corev1.PodList{}
	if listErr := r.Client.List(context.TODO(), podList, listOpts, utilclient.DisableDeepCopy); listErr != nil {
		return nil, nil, err
	}
	normalPods := make([]*corev1.Pod, 0, len(podList.Items))
	serverlessPods := make([]*corev1.Pod, 0, len(podList.Items))
	nodes := map[string]*corev1.Node{}
	for i := range podList.Items {
		pod := &podList.Items[i]
		condition := util.GetCondition(pod, corev1.PodInitialized)
		if !kubecontroller.IsPodActive(pod) || condition == nil ||
			condition.Status != corev1.ConditionTrue || pod.Spec.NodeName == "" {
			continue
		}

		node, ok := nodes[pod.Spec.NodeName]
		if !ok {
			node = &corev1.Node{}
			if err = r.Get(context.TODO(), client.ObjectKey{Name: pod.Spec.NodeName}, node); err != nil {
				if errors.IsNotFound(err) {
					continue
				}
				return nil, nil, err
			}
			nodes[node.Name] = node
		}

		if node.Labels["type"] == VirtualKubelet {
			serverlessPods = append(serverlessPods, pod)
		} else {
			normalPods = append(normalPods, pod)
		}
	}
	return normalPods, serverlessPods, nil
}

func (r *ReconcilePodProbeMarker) removePodProbeFromNodePodProbe(ppmName, nppName string) error {
	npp := &appsv1alpha1.NodePodProbe{}
	err := r.Get(context.TODO(), client.ObjectKey{Name: nppName}, npp)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	newSpec := appsv1alpha1.NodePodProbeSpec{}
	for _, podProbe := range npp.Spec.PodProbes {
		newPodProbe := appsv1alpha1.PodProbe{Name: podProbe.Name, Namespace: podProbe.Namespace, UID: podProbe.UID}
		for i := range podProbe.Probes {
			probe := podProbe.Probes[i]
			// probe.Name -> podProbeMarker.Name#probe.Name
			if !strings.Contains(probe.Name, fmt.Sprintf("%s#", ppmName)) {
				newPodProbe.Probes = append(newPodProbe.Probes, probe)
			}
		}
		if len(newPodProbe.Probes) > 0 {
			newSpec.PodProbes = append(newSpec.PodProbes, newPodProbe)
		}
	}
	if reflect.DeepEqual(npp.Spec, newSpec) {
		return nil
	}
	npp.Spec = newSpec
	err = r.Update(context.TODO(), npp)
	if err != nil {
		klog.ErrorS(err, "NodePodProbe removed PodProbeMarker failed", "nodePodProbeName", nppName, "podProbeMarkerName", ppmName)
		return err
	}
	klog.V(3).InfoS("NodePodProbe removed PodProbeMarker success", "nodePodProbeName", nppName, "podProbeMarkerName", ppmName)
	return nil
}
