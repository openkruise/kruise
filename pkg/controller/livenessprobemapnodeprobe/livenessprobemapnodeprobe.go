package enhancedlivenessprobe2nodeprobe

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
)

const (
	concurrentReconciles = 10
	controllerName       = "livenessprobemapnodeprobe-controller"

	FinalizerPodEnhancedLivenessProbe = "pod.kruise.io/enhanced-liveness-probe-cleanup"

	AddNodeProbeConfigOpType = "addNodeProbe"
	DelNodeProbeConfigOpType = "delNodeProbe"
)

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

var _ reconcile.Reconciler = &ReconcileEnhancedLivenessProbe2NodeProbe{}

// ReconcileEnhancedLivenessProbe reconciles a Pod object
type ReconcileEnhancedLivenessProbe2NodeProbe struct {
	client.Client
	recorder record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileEnhancedLivenessProbe2NodeProbe {
	return &ReconcileEnhancedLivenessProbe2NodeProbe{
		Client:   utilclient.NewClientFromManager(mgr, controllerName),
		recorder: mgr.GetEventRecorderFor(controllerName),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileEnhancedLivenessProbe2NodeProbe) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	// watch events of pod
	if err = c.Watch(&source.Kind{Type: &v1.Pod{}}, &enqueueRequestForPod{reader: mgr.GetClient()}); err != nil {
		return err
	}
	return nil
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=nodepodprobes,verbs=get;list;watch;create;update;patch:delete
func (r *ReconcileEnhancedLivenessProbe2NodeProbe) Reconcile(_ context.Context, request reconcile.Request) (res reconcile.Result, err error) {
	start := time.Now()
	klog.V(3).Infof("Starting to process Pod %v", request.NamespacedName)
	defer func() {
		if err != nil {
			klog.Warningf("Failed to process Pod %v, elapsedTime %v, error: %v", request.NamespacedName, time.Since(start), err)
		} else {
			klog.Infof("Finish to process Pod %v, elapsedTime %v", request.NamespacedName, time.Since(start))
		}
	}()

	err = r.syncPodContainersLivenessProbe(request.Namespace, request.Name)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileEnhancedLivenessProbe2NodeProbe) syncPodContainersLivenessProbe(namespace, name string) error {
	getPod := &v1.Pod{}
	var err error
	if err = r.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, getPod); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get pod %s/%s: %v", namespace, name, err)
	}

	if getPod.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(getPod, FinalizerPodEnhancedLivenessProbe) {
			err = util.UpdateFinalizer(r.Client, getPod, util.AddFinalizerOpType, FinalizerPodEnhancedLivenessProbe)
			if err != nil {
				klog.Errorf("Failed to update pod %s/%s finalizer %v, err: %v", getPod.Namespace, getPod.Name, FinalizerPodEnhancedLivenessProbe, err)
				return err
			}
		}
		err = r.addOrRemoveNodePodProbeConfig(getPod, AddNodeProbeConfigOpType)
		if err != nil {
			klog.Errorf("Failed to add or remove node pod probe config in %s process for pod: %s/%s, err: %v", AddNodeProbeConfigOpType, getPod.Namespace, getPod.Name, err)
			return err
		}
	} else {
		// pod in deleting process
		err = r.addOrRemoveNodePodProbeConfig(getPod, DelNodeProbeConfigOpType)
		if err != nil {
			klog.Errorf("Failed to add or remove node pod probe config in %s process for pod: %s/%s, "+
				"err: %v", DelNodeProbeConfigOpType, getPod.Namespace, getPod.Name, err)
			return err
		}
		if controllerutil.ContainsFinalizer(getPod, FinalizerPodEnhancedLivenessProbe) {
			err = util.UpdateFinalizer(r.Client, getPod, util.RemoveFinalizerOpType, FinalizerPodEnhancedLivenessProbe)
			if err != nil {
				klog.Errorf("Failed to update pod %s/%s finalizer %v, err: %v", getPod.Namespace, getPod.Name, FinalizerPodEnhancedLivenessProbe, err)
				return err
			}
		}
	}
	return nil
}

func (r *ReconcileEnhancedLivenessProbe2NodeProbe) addOrRemoveNodePodProbeConfig(pod *v1.Pod, op string) error {
	if op == DelNodeProbeConfigOpType {
		return r.delNodeProbeConfig(pod)
	}
	if op == AddNodeProbeConfigOpType {
		return r.addNodeProbeConfig(pod)
	}
	return fmt.Errorf("No found op %s(just support %s and %s)", op, AddNodeProbeConfigOpType, DelNodeProbeConfigOpType)
}
