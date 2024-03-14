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
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	if err = c.Watch(&source.Kind{Type: &appsv1alpha1.PodProbeMarker{}}, &enqueueRequestForPodProbeMarker{}); err != nil {
		return err
	}
	// watch for changes to pod
	if err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &enqueueRequestForPod{reader: mgr.GetClient()}); err != nil {
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

	groupByNode := make(map[string][]*corev1.Pod)
	for i, pod := range pods {
		groupByNode[pod.Spec.NodeName] = append(groupByNode[pod.Spec.NodeName], pods[i])
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
			klog.Errorf("error getting updated podProbeMarker %s from client", ppm.Name)
		}
		if ppmClone.Status.ObservedGeneration == ppmClone.Generation && int(ppmClone.Status.MatchedPods) == len(pods) {
			return nil
		}
		ppmClone.Status.ObservedGeneration = ppmClone.Generation
		ppmClone.Status.MatchedPods = int64(len(pods))
		return r.Client.Status().Update(context.TODO(), ppmClone)
	}); err != nil {
		klog.Errorf("PodProbeMarker(%s/%s) update status failed: %s", ppm.Namespace, ppm.Name, err.Error())
		return err
	}
	klog.V(3).Infof("PodProbeMarker(%s/%s) update status(%s) success", ppm.Namespace, ppm.Name, util.DumpJSON(ppmClone.Status))
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
		klog.Errorf("remove PodProbeMarker(%s/%s) finalizer failed: %s", ppm.Namespace, ppm.Name, err.Error())
		return err
	}
	klog.V(3).Infof("remove PodProbeMarker(%s/%s) finalizer success", ppm.Namespace, ppm.Name)
	return nil
}

func (r *ReconcilePodProbeMarker) updateNodePodProbes(ppm *appsv1alpha1.PodProbeMarker, nodeName string, pods []*corev1.Pod) error {
	npp := &appsv1alpha1.NodePodProbe{}
	err := r.Get(context.TODO(), client.ObjectKey{Name: nodeName}, npp)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Warningf("PodProbeMarker ppm(%s/%s) NodePodProbe(%s) is Not Found", ppm.Namespace, ppm.Name, npp.Name)
			return nil
		}
		// Error reading the object - requeue the request.
		klog.Errorf("PodProbeMarker ppm(%s/%s) get NodePodProbe(%s) failed: %s", ppm.Namespace, ppm.Name, nodeName, err.Error())
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
							klog.Errorf("Failed to convert tcpSocket probe port, err: %v, pod: %v/%v", err, pod.Namespace, pod.Name)
							continue
						}
					}
					if probe.Probe.HTTPGet != nil {
						probe, err = convertHttpGetProbeCheckPort(probe, pod)
						if err != nil {
							klog.Errorf("Failed to convert httpGet probe port, err: %v, pod: %v/%v", err, pod.Namespace, pod.Name)
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
						klog.Errorf("Failed to convert tcpSocket probe port, err: %v, pod: %v/%v", err, pod.Namespace, pod.Name)
						continue
					}
				}
				if probe.Probe.HTTPGet != nil {
					probe, err = convertHttpGetProbeCheckPort(probe, pod)
					if err != nil {
						klog.Errorf("Failed to convert httpGet probe port, err: %v, pod: %v/%v", err, pod.Namespace, pod.Name)
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
		klog.Errorf("PodProbeMarker ppm(%s/%s) Update NodePodProbe(%s) failed: %s", ppm.Namespace, ppm.Name, npp.Name, err.Error())
		return err
	}
	klog.V(3).Infof("PodProbeMarker ppm(%s/%s) update NodePodProbe(%s) from(%s) -> to(%s) success",
		ppm.Namespace, ppm.Name, npp.Name, util.DumpJSON(oldSpec), util.DumpJSON(npp.Spec))
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
		condition := util.GetCondition(pod, corev1.PodInitialized)
		if kubecontroller.IsPodActive(pod) && pod.Spec.NodeName != "" &&
			condition != nil && condition.Status == corev1.ConditionTrue {
			pods = append(pods, pod)
		}
	}
	return pods, nil
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
		klog.Errorf("NodePodProbe(%s) remove PodProbe(%s) failed: %s", nppName, ppmName, err.Error())
		return err
	}
	klog.V(3).Infof("NodePodProbe(%s) remove PodProbe(%s) success", nppName, ppmName)
	return nil
}
