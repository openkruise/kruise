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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	utils "github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"
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
	if !utildiscovery.DiscoverGVK(controllerKind) || !utilfeature.DefaultFeatureGate.Enabled(features.ResourceDistributionGate) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cli := utilclient.NewClientFromManager(mgr, "resourcedistribution-controller")
	return &ReconcileResourceDistribution{
		Client: cli,
		scheme: mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("resourcedistribution-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to ResourceDistribution
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.ResourceDistribution{},
		&handler.TypedEnqueueRequestForObject[*appsv1alpha1.ResourceDistribution]{},
		predicate.TypedFuncs[*appsv1alpha1.ResourceDistribution]{
			UpdateFunc: func(e event.TypedUpdateEvent[*appsv1alpha1.ResourceDistribution]) bool {
				oldObj := e.ObjectOld
				newObj := e.ObjectNew
				if !reflect.DeepEqual(oldObj.Spec, newObj.Spec) {
					klog.V(3).InfoS("Observed updated Spec for ResourceDistribution", "resourceDistribution", klog.KObj(oldObj))
					return true
				}
				return false
			},
		}))
	if err != nil {
		return err
	}

	// Watch for changes to all namespaces
	err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Namespace{}, &enqueueRequestForNamespace{reader: mgr.GetCache()}))
	if err != nil {
		return err
	}

	// Watch for changes to Secrets
	secret := unstructured.Unstructured{}
	secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	err = c.Watch(source.Kind(mgr.GetCache(), client.Object(&secret), handler.EnqueueRequestForOwner(
		mgr.GetScheme(), mgr.GetRESTMapper(), &appsv1alpha1.ResourceDistribution{}, handler.OnlyControllerOwner()),
		predicate.Funcs{
			CreateFunc: func(createEvent event.CreateEvent) bool {
				return false
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				return false
			},
		}))
	if err != nil {
		return err
	}

	// Watch for changes to ConfigMap
	configMap := unstructured.Unstructured{}
	configMap.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	err = c.Watch(source.Kind(mgr.GetCache(), client.Object(&configMap), handler.EnqueueRequestForOwner(
		mgr.GetScheme(), mgr.GetRESTMapper(), &appsv1alpha1.ResourceDistribution{}, handler.OnlyControllerOwner()),
		predicate.Funcs{
			CreateFunc: func(createEvent event.CreateEvent) bool {
				return false
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				return false
			},
		}))
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileResourceDistribution{}

// ReconcileResourceDistribution reconciles a ResourceDistribution object
type ReconcileResourceDistribution struct {
	client.Client
	scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=apps.kruise.io,resources=resourcedistributions,verbs=get;list;watch;
//+kubebuilder:rbac:groups=apps.kruise.io,resources=resourcedistributions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps.kruise.io,resources=resourcedistributions/finalizers,verbs=update
//+kubebuilder:rbac:groups="core",resources=namespaces,verbs=get;list;watch;
//+kubebuilder:rbac:groups="core",resources=configmaps,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="core",resources=secrets,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ResourceDistribution object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *ReconcileResourceDistribution) Reconcile(_ context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.V(3).InfoS("ResourceDistribution begin to reconcile", "resourceDistribution", req)
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

	return r.doReconcile(distributor)
}

// doReconcile distribute and clean resource
func (r *ReconcileResourceDistribution) doReconcile(distributor *appsv1alpha1.ResourceDistribution) (ctrl.Result, error) {
	resource, errs := utils.DeserializeResource(&distributor.Spec.Resource, field.NewPath("resource"))
	if len(errs) != 0 || resource == nil {
		klog.ErrorS(errs.ToAggregate(), "DeserializeResource error", "resourceDistribution", klog.KObj(distributor))
		return reconcile.Result{}, nil // no need to retry
	}

	matchedNamespaces, unmatchedNamespaces, err := listNamespacesForDistributor(r.Client, &distributor.Spec.Targets)
	if err != nil {
		klog.ErrorS(err, "Failed to list namespace for ResourceDistributor", "resourceDistribution", klog.KObj(distributor))
		return reconcile.Result{}, err
	}

	// 1. distribute resource to matched namespaces
	succeeded, distributeErrList := r.distributeResource(distributor, matchedNamespaces, resource)

	// 2. clean its owned resources in unmatched namespaces
	_, cleanErrList := r.cleanResource(distributor, unmatchedNamespaces, resource)

	// 3. process all errors about resource distribution and cleanup
	conditions, errList := r.handleErrors(distributeErrList, cleanErrList)

	// 4. update distributor status
	newStatus := calculateNewStatus(distributor, conditions, int32(len(matchedNamespaces)), succeeded)
	if err := r.updateDistributorStatus(distributor, newStatus); err != nil {
		errList = append(errList, field.InternalError(field.NewPath("updateStatus"), err))
	}
	return ctrl.Result{}, errList.ToAggregate()
}

func (r *ReconcileResourceDistribution) distributeResource(distributor *appsv1alpha1.ResourceDistribution,
	matchedNamespaces []string, resource runtime.Object) (int32, []*UnexpectedError) {

	resourceName := utils.ConvertToUnstructured(resource).GetName()
	resourceKind := resource.GetObjectKind().GroupVersionKind().Kind
	resourceHashCode := hashResource(distributor.Spec.Resource)
	return syncItSlowly(matchedNamespaces, 1, func(namespace string) *UnexpectedError {
		ns := &corev1.Namespace{}
		getNSErr := r.Client.Get(context.TODO(), types.NamespacedName{Name: namespace}, ns)
		if errors.IsNotFound(getNSErr) || (getNSErr == nil && ns.DeletionTimestamp != nil) {
			return &UnexpectedError{
				err:         fmt.Errorf("namespace not found or is terminating"),
				namespace:   namespace,
				conditionID: NotExistConditionID,
			}
		}

		// 1. try to fetch existing old resource
		oldResource := &unstructured.Unstructured{}
		oldResource.SetGroupVersionKind(resource.GetObjectKind().GroupVersionKind())
		getErr := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: resourceName}, oldResource)
		if getErr != nil && !errors.IsNotFound(getErr) {
			klog.ErrorS(getErr, "Error occurred when getting resource in namespace", "namespace", namespace, "resourceDistribution", klog.KObj(distributor))
			return &UnexpectedError{
				err:         getErr,
				namespace:   namespace,
				conditionID: GetConditionID,
			}
		}

		// 2. if resource doesn't exist, create resource;
		if getErr != nil && errors.IsNotFound(getErr) {
			newResource := makeResourceObject(distributor, namespace, resource, resourceHashCode, nil)
			if createErr := r.Client.Create(context.TODO(), newResource.(client.Object)); createErr != nil {
				klog.ErrorS(createErr, "Error occurred when creating resource in namespace", "namespace", namespace, "resourceDistribution", klog.KObj(distributor))
				return &UnexpectedError{
					err:         createErr,
					namespace:   namespace,
					conditionID: CreateConditionID,
				}
			}
			klog.V(3).InfoS("ResourceDistribution created resource in namespace", "resourceDistribution", klog.KObj(distributor), "resourceKind", resourceKind, "resourceName", resourceName, "namespace", namespace)
			return nil
		}

		// 3. check conflict
		if !isControlledByDistributor(oldResource, distributor) {
			klog.InfoS("Conflict with existing resource in namespace", "resourceKind", resourceKind, "resourceName", resourceName, "namespace", namespace, "resourceDistribution", klog.KObj(distributor))
			return &UnexpectedError{
				err:         fmt.Errorf("conflict with existing resources because of the same namespace, group, version, kind and name"),
				namespace:   namespace,
				conditionID: ConflictConditionID,
			}
		}

		// 4. check whether resource need to update
		if needToUpdate(oldResource, utils.ConvertToUnstructured(resource)) {
			newResource := makeResourceObject(distributor, namespace, resource, resourceHashCode, oldResource)
			if updateErr := r.Client.Update(context.TODO(), newResource.(client.Object)); updateErr != nil {
				klog.ErrorS(updateErr, "Error occurred when updating resource in namespace", "namespace", namespace, "resourceDistribution", klog.KObj(distributor))
				return &UnexpectedError{
					err:         updateErr,
					namespace:   namespace,
					conditionID: UpdateConditionID,
				}
			}
			klog.V(3).InfoS("ResourceDistribution updated for namespaces", "resourceDistribution", klog.KObj(distributor), "resourceKind", resourceKind, "resourceName", resourceName, "namespace", namespace)
		}
		return nil
	})
}

func (r *ReconcileResourceDistribution) cleanResource(distributor *appsv1alpha1.ResourceDistribution,
	unmatchedNamespaces []string, resource runtime.Object) (int32, []*UnexpectedError) {

	resourceName := utils.ConvertToUnstructured(resource).GetName()
	resourceKind := resource.GetObjectKind().GroupVersionKind().Kind
	return syncItSlowly(unmatchedNamespaces, 1, func(namespace string) *UnexpectedError {
		ns := &corev1.Namespace{}
		getNSErr := r.Client.Get(context.TODO(), types.NamespacedName{Name: namespace}, ns)
		if errors.IsNotFound(getNSErr) || (getNSErr == nil && ns.DeletionTimestamp != nil) {
			return nil
		}

		// 1. try to fetch existing old resource
		oldResource := &unstructured.Unstructured{}
		oldResource.SetGroupVersionKind(resource.GetObjectKind().GroupVersionKind())
		if getErr := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: resourceName}, oldResource); getErr != nil {
			if errors.IsNotFound(getErr) {
				return nil
			}
			klog.ErrorS(getErr, "Error occurred when getting resource in namespace", "namespace", namespace, "resourceDistribution", klog.KObj(distributor))
			return &UnexpectedError{
				err:         getErr,
				namespace:   namespace,
				conditionID: GetConditionID,
			}
		}

		// 2. if the owner of the oldResource is not this distributor, just return
		if !isControlledByDistributor(oldResource, distributor) {
			return nil
		}

		// 3. else clean the resource
		if deleteErr := r.Client.Delete(context.TODO(), oldResource); deleteErr != nil && !errors.IsNotFound(deleteErr) {
			klog.ErrorS(deleteErr, "Error occurred when deleting resource in namespace from client", "namespace", namespace, "resourceDistribution", klog.KObj(distributor))
			return &UnexpectedError{
				err:         deleteErr,
				namespace:   namespace,
				conditionID: DeleteConditionID,
			}
		}
		klog.V(3).InfoS("ResourceDistribution deleted in namespace", "resourceDistribution", klog.KObj(distributor), "resourceKind", resourceKind, "resourceName", resourceName, "namespace", namespace)
		return nil
	})
}

// handlerErrors process all errors about resource distribution and clean, and record them to conditions
func (r *ReconcileResourceDistribution) handleErrors(errLists ...[]*UnexpectedError) ([]appsv1alpha1.ResourceDistributionCondition, field.ErrorList) {
	// init a status.conditions
	conditions := make([]appsv1alpha1.ResourceDistributionCondition, NumberOfConditionTypes)
	initConditionType(conditions)

	// 1. build status.conditions
	numberOfErr := 0
	for i := range errLists {
		numberOfErr += len(errLists[i])
		for _, unexpected := range errLists[i] {
			setCondition(&conditions[unexpected.conditionID], unexpected.err, unexpected.namespace)
		}
	}

	// 2. build error list
	errList := field.ErrorList{}
	for i := range conditions {
		if len(conditions[i].FailedNamespaces) == 0 {
			continue
		}
		switch conditions[i].Type {
		case appsv1alpha1.ResourceDistributionConflictOccurred, appsv1alpha1.ResourceDistributionNamespaceNotExists:
		default:
			errList = append(errList, field.InternalError(field.NewPath(string(conditions[i].Type)), fmt.Errorf(conditions[i].Reason)))
		}
	}
	return conditions, errList
}

// updateDistributorStatus update distributor status after reconcile
func (r *ReconcileResourceDistribution) updateDistributorStatus(distributor *appsv1alpha1.ResourceDistribution, newStatus *appsv1alpha1.ResourceDistributionStatus) error {
	if reflect.DeepEqual(distributor.Status, *newStatus) {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		object := &appsv1alpha1.ResourceDistribution{}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: distributor.Name}, object); err != nil {
			return err
		}
		object.Status = *newStatus
		return r.Client.Status().Update(context.TODO(), object)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReconcileResourceDistribution) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		Complete(r)
}
