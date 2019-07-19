/*
Copyright 2019 The Kruise Authors.

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

package sidecarset

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	controllerutil "k8s.io/kubernetes/pkg/controller"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new SidecarSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSidecarSet{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("sidecarset-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to SidecarSet
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.SidecarSet{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	if err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &enqueueRequestForPod{client: mgr.GetClient()}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSidecarSet{}

// ReconcileSidecarSet reconciles a SidecarSet object
type ReconcileSidecarSet struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a SidecarSet object and makes changes based on the state read
// and what is in the SidecarSet.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps.kruise.io,resources=sidecarsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=sidecarsets/status,verbs=get;update;patch
func (r *ReconcileSidecarSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the SidecarSet instance
	sidecarSet := &appsv1alpha1.SidecarSet{}
	err := r.Get(context.TODO(), request.NamespacedName, sidecarSet)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	klog.V(3).Infof("begin to process sidecarset %v", sidecarSet.Name)

	selector, err := metav1.LabelSelectorAsSelector(sidecarSet.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}
	matchedPods := &corev1.PodList{}
	if err := r.List(context.TODO(), &client.ListOptions{LabelSelector: selector}, matchedPods); err != nil {
		return reconcile.Result{}, err
	}

	// ignore inactive pods and pods are created before sidecarset creates
	var filteredPods []*corev1.Pod
	for i := range matchedPods.Items {
		pod := &matchedPods.Items[i]
		podCreateBeforeSidecarSet, err := isPodCreatedBeforeSidecarSet(sidecarSet, pod)
		if err != nil {
			return reconcile.Result{}, err
		}
		if controllerutil.IsPodActive(pod) && !isIgnoredPod(pod) && !podCreateBeforeSidecarSet {
			filteredPods = append(filteredPods, pod)
		}
	}

	status, err := calculateStatus(sidecarSet, filteredPods)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.updateSidecarSetStatus(sidecarSet, status)
	if err != nil {
		return reconcile.Result{}, err
	}

	// update procedure:
	// 1. check if sidecarset paused, if so, then quit
	// 2. check if fields other than image in sidecarset had changed, if so, then quit
	// 3. check unavailable pod number, if > 0, then quit(maxUnavailable=1)
	// 4. find out pods need update
	// 5. update one pod(maxUnavailable=1)
	if sidecarSet.Spec.Paused {
		klog.V(3).Infof("sidecarset %v is paused, skip update", sidecarSet.Name)
		return reconcile.Result{}, nil
	}

	if len(filteredPods) == 0 {
		return reconcile.Result{}, nil
	}
	otherFieldsChanged, err := otherFieldsInSidecarChanged(sidecarSet, filteredPods[0])
	if err != nil {
		return reconcile.Result{}, err
	}
	if otherFieldsChanged {
		klog.V(3).Infof("fields other than image in sidecarset %v had changed, skip update", sidecarSet.Name)
		return reconcile.Result{}, nil
	}

	unavailableNum, err := getUnavailableNumber(sidecarSet, filteredPods)
	if err != nil {
		return reconcile.Result{}, err
	}
	// we only support sequential update currently, equals to maxUnavailable = 1
	if unavailableNum != 0 {
		klog.V(3).Infof("current unavailable pod number: %v, skip update", unavailableNum)
		return reconcile.Result{}, nil
	}

	var podsNeedUpdate []*corev1.Pod
	for _, pod := range filteredPods {
		isUpdated, err := isPodSidecarUpdated(sidecarSet, pod)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !isUpdated {
			podsNeedUpdate = append(podsNeedUpdate, pod)
		}
	}
	return reconcile.Result{}, r.updateSidecarImageAndHash(sidecarSet, podsNeedUpdate)
}
