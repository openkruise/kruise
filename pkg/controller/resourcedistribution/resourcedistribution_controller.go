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

package resourcedistribution

import (
	"context"
	"flag"
	"fmt"
	"reflect"
	"sync"

	"go.uber.org/atomic"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	utils "github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func init() {
	flag.IntVar(&concurrentReconciles, "resourcedistribution-workers", concurrentReconciles, "Max concurrent workers for ResourceDistribution controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("ResourceDistribution")
)

// Add creates a new ResourceDistribution Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cli := util.NewClientFromManager(mgr, "resourcedistribution-controller")
	return &ReconcileResourceDistribution{
		Client: cli,
		scheme: mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("resourcedistribution-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to ResourceDistribution
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.ResourceDistribution{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj := e.ObjectOld.(*appsv1alpha1.ResourceDistribution)
			newObj := e.ObjectNew.(*appsv1alpha1.ResourceDistribution)
			if !reflect.DeepEqual(oldObj.Spec, newObj.Spec) {
				klog.V(3).Infof("Observed updated Spec for ResourceDistribution: %s/%s", oldObj.Namespace, newObj.Name)
				return true
			}
			return false
		},
	})
	if err != nil {
		return err
	}

	// Watch for changes to all namespaces
	if err = c.Watch(&source.Kind{Type: &corev1.Namespace{}}, &enqueueRequestForNamespace{reader: mgr.GetCache()}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileResourceDistribution{}

// ReconcileResourceDistribution reconciles a ResourceDistribution object
type ReconcileResourceDistribution struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=apps.kruise.io,resources=resourcedistributions,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=apps.kruise.io,resources=resourcedistributions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ResourceDistribution object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *ReconcileResourceDistribution) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	klog.V(3).Infof("ResourceDistribution(%s) begin to reconcile\n\n", req.NamespacedName.Name)

	// fetch resourcedistribution instance as distributor
	distributor := &appsv1alpha1.ResourceDistribution{}
	if err := r.Client.Get(context.TODO(), req.NamespacedName, distributor); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// do reconcile
	newStatus, err := r.doReconcile(distributor)

	// update distributor status
	if reflect.DeepEqual(distributor.Status, newStatus) {
		return ctrl.Result{}, nil
	}
	distributor.Status = *newStatus
	if err := r.updateDistributorStatus(distributor); err != nil {
		klog.Errorf("ResourceDistribution update status error, err: %v, name: %s, new status %v", err, distributor.Name, newStatus)
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, err
}

// doReconcile distribute and sync the resource
func (r *ReconcileResourceDistribution) doReconcile(distributor *appsv1alpha1.ResourceDistribution) (*appsv1alpha1.ResourceDistributionStatus, error) {
	// 1. init new distributor status
	newStatus := &appsv1alpha1.ResourceDistributionStatus{}
	conditions := make([]appsv1alpha1.ResourceDistributionCondition, NumberOfConditionTypes)

	// 2. prepare namespaces
	isInTargets, allNamespaces, err := prepareNamespaces(r.Client, distributor)
	if err != nil {
		klog.Errorf("ResourceDistribution parse namespaces error, err: %v, name: %s", err, distributor.Name)
		return makeNewStatus(newStatus, distributor, conditions, 0), err
	}
	newStatus.Desired = int32(len(isInTargets)) // set newStatus.Desired to the number of all target namespaces

	// 3. hash resource yaml as its version ID
	newResourceHashCode := hashResource(distributor.Spec.Resource)

	// 4. deserialize new resource as UnifiedResource type for setting its metadata
	resource, errs := utils.DeserializeResource(&distributor.Spec.Resource, field.NewPath("resource"))
	if len(errs) != 0 {
		klog.Errorf("ResourceDistribution deserialize resource error, err: %v, name: %s", errs.ToAggregate(), distributor.Name)
		return makeNewStatus(newStatus, distributor, conditions, 0), errs.ToAggregate()
	}

	// 5. begin to distribute and update resource
	var succeeded atomic.Int32 // for setting .status.succeeded
	wait := sync.WaitGroup{}
	wait.Add(len(allNamespaces))
	errCh := make(chan *UnexpectedError, len(allNamespaces))
	for _, namespaceName := range allNamespaces {
		go func(namespace string) {
			defer wait.Done()

			newResource := resource.DeepCopyObject()
			oldResource := resource.DeepCopyObject()
			// 5.1 try to fetch existing old resource
			resourceName := utils.ConvertToUnstructured(oldResource).GetName()
			err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: resourceName}, oldResource)
			if err != nil && !errors.IsNotFound(err) {
				errCh <- &UnexpectedError{
					err:           err,
					namespace:     namespace,
					conditionID:   GetConditionID,
					conditionType: appsv1alpha1.ResourceDistributionResourceGetSuccessfully,
				}
				klog.Errorf("error getting resource in namespace %s from client, err： %v, name: %s", namespace, err, distributor.Name)
				return
			}

			// 5.2 resource doesn't exist
			//  (1).if the namespace is in targets, distribute resource;
			//  (2).else the namespace is NOT in targets, do nothing;
			if err != nil && errors.IsNotFound(err) {
				if _, ok := isInTargets[namespace]; ok {
					if err := r.distributeResource(distributor, namespace, resourceName, newResource, newResourceHashCode); err != nil {
						errCh <- &UnexpectedError{
							err:           err,
							namespace:     namespace,
							conditionID:   CreateConditionID,
							conditionType: appsv1alpha1.ResourceDistributionResourceCreateSuccessfully,
						}
						klog.Errorf("error creating resource in namespace %s from client, err： %v, name: %s", namespace, err, distributor.Name)
						return
					}
					succeeded.Inc()
					klog.V(3).Infof("ResourceDistribution(%s) create (%s/%s) in namespaces %s", distributor.Name, newResource.GetObjectKind(), resourceName, namespace)
				}
				return
			}

			// 5.3 resource exits
			// 	(1). if conflict occurs, record condition and return
			annotations := utils.ConvertToUnstructured(oldResource).GetAnnotations()
			if annotations == nil || annotations[utils.SourceResourceDistributionOfResource] != distributor.Name {
				errCh <- &UnexpectedError{
					err:           fmt.Errorf("conflict with existing resource because of the same namespace and name"),
					namespace:     namespace,
					conditionID:   ConflictConditionID,
					conditionType: appsv1alpha1.ResourceDistributionNoConflictOccurred,
				}
				klog.Errorf("conflict with existing resource(%s/%s) in namespaces %s, name: %s", newResource.GetObjectKind(), resourceName, namespace, distributor.Name)
				return
			}

			// 5.4 resource exits
			// 	(1).if the namespace is in targets and the old resource needs to update, update the resource;
			// 	(2).else if the namespace is NOT in targets and it was distributed by this distributor, delete the resource;
			_, ok := isInTargets[namespace]
			if ok && newResourceHashCode != annotations[utils.ResourceHashCodeAnnotation] {
				if err := r.updateResource(distributor, namespace, resourceName, newResource, newResourceHashCode); err != nil {
					errCh <- &UnexpectedError{
						err:           err,
						namespace:     namespace,
						conditionID:   UpdateConditionID,
						conditionType: appsv1alpha1.ResourceDistributionResourceUpdateSuccessfully,
					}
					klog.Errorf("error updating resource in namespace %s from client, err： %v, name: %s", namespace, err, distributor.Name)
					return
				}
				klog.V(3).Infof("ResourceDistribution(%s) update (%s/%s) for namespaces %s", distributor.Name, newResource.GetObjectKind(), resourceName, namespace)
			} else if !ok {
				if err := r.deleteResource(namespace, resourceName, oldResource); err != nil && !errors.IsNotFound(err) {
					errCh <- &UnexpectedError{
						err:           err,
						namespace:     namespace,
						conditionID:   DeleteConditionID,
						conditionType: appsv1alpha1.ResourceDistributionResourceDeleteSuccessfully,
					}
					klog.Errorf("error deleting resource in namespace %s from client, err： %v, name: %s", namespace, err, distributor.Name)
					return
				}
				klog.V(3).Infof("ResourceDistribution(%s) delete (%s/%s) in namespaces %s", distributor.Name, oldResource.GetObjectKind(), resourceName, namespace)
			}

			if ok { // here means resource is distributed successfully
				succeeded.Inc()
			}
		}(namespaceName)
	}
	wait.Wait() // wait for all distribution done.

	// process all unexpected errors
	numErrs := len(errCh)
	if numErrs != 0 {
		err = fmt.Errorf("some unexpected errors occurred, error message will be recorded in .status.conditions, name: %s", distributor.Name)
	}
	for i := 0; i < numErrs; i++ {
		unexpected := <-errCh
		setCondition(&conditions[unexpected.conditionID], unexpected.conditionType, unexpected.namespace, unexpected.err)
	}

	return makeNewStatus(newStatus, distributor, conditions, succeeded.Load()), err
}

// updateDistributorStatus update distributor status after reconcile
func (r *ReconcileResourceDistribution) updateDistributorStatus(distributor *appsv1alpha1.ResourceDistribution) error {
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		updateErr := r.Client.Status().Update(context.TODO(), distributor)
		if updateErr == nil {
			return nil
		}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: distributor.Name}, distributor); err != nil {
			klog.Errorf("error getting updated distributor %s from client", distributor.Name)
		}
		return updateErr
	}); err != nil {
		return err
	}

	klog.V(3).Infof("ResourceDistribution(%s) update status success", distributor.Name)
	return nil
}

// updateResource try to update resource
func (r *ReconcileResourceDistribution) updateResource(distributor *appsv1alpha1.ResourceDistribution, namespace, name string, resource runtime.Object, hashCode string) error {
	resourceObject := makeResourceObject(distributor, namespace, resource, hashCode)
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		updateErr := r.Client.Update(context.TODO(), resourceObject, &client.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, resourceObject); err != nil {
			klog.Errorf("error getting updated resource in namespace %s from client", namespace)
		}
		return updateErr
	}); err != nil {
		return err
	}

	return nil
}

// distributeResource try to create resource
func (r *ReconcileResourceDistribution) distributeResource(distributor *appsv1alpha1.ResourceDistribution, namespace, name string, resource runtime.Object, hashCode string) error {
	resourceObject := makeResourceObject(distributor, namespace, resource, hashCode)
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		createErr := r.Client.Create(context.TODO(), resourceObject, &client.CreateOptions{})
		if createErr == nil {
			return nil
		}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, resourceObject); err != nil {
			klog.Errorf("error getting distributed resource in namespace %s from client", namespace)
		}
		return createErr
	}); err != nil {
		return err
	}

	return nil
}

// deleteResource try to delete resource
func (r *ReconcileResourceDistribution) deleteResource(namespace, name string, resource runtime.Object) error {
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		deleteErr := r.Client.Delete(context.TODO(), resource, &client.DeleteOptions{})
		if deleteErr == nil {
			return nil
		}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name}, resource); err == nil || !errors.IsNotFound(err) {
			klog.Errorf("error getting delete resource in namespace %s from client", namespace)
		}
		return deleteErr
	}); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReconcileResourceDistribution) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		Complete(r)
}
