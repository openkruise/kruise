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
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	utils "github.com/openkruise/kruise/pkg/webhook/resourcedistribution/validating"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	types2 "k8s.io/apimachinery/pkg/types"
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

//+kubebuilder:rbac:groups=apps.kruise.io,resources=resourcedistributions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps.kruise.io,resources=resourcedistributions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

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
		klog.Errorf("ResourceDistribution update status error, err: %v, name: %s, new status %v", err, distributor.Name, newStatus.DistributedResources)
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, err
}

func (r *ReconcileResourceDistribution) doReconcile(distributor *appsv1alpha1.ResourceDistribution) (*appsv1alpha1.ResourceDistributionStatus, error) {
	// 1. init new distributor status
	failedNamespaces := make(map[string]struct{})
	newStatus := &appsv1alpha1.ResourceDistributionStatus{}
	newStatus.DistributedResources = make(map[string]string)

	// 2. prepare namespaces
	isInTargets, allNamespaces, err := prepareNamespaces(r.Client, distributor)
	if err != nil {
		klog.Errorf("ResourceDistribution parse namespaces error, err: %v, name: %s", err, distributor.Name)
		return updateNewStatus(newStatus, failedNamespaces), err
	}

	// deep copy target namespaces
	for namespace := range isInTargets {
		failedNamespaces[namespace] = struct{}{}
	}

	// 3. hash resource yaml as its version ID
	newResourceHashCode := hashResource(distributor.Spec.Resource)

	// 4. deserialize new resource as UnifiedResource type
	newResource, errs := utils.DeserializeResource(&distributor.Spec.Resource, field.NewPath("resource"))
	if len(errs) != 0 {
		klog.Errorf("ResourceDistribution deserialize resource error, err: %v, name: %s", errs.ToAggregate(), distributor.Name)
		return updateNewStatus(newStatus, failedNamespaces), errs.ToAggregate()
	}

	// 5. begin to distribute and update resource
	for _, namespace := range allNamespaces {
		// 5.1 try to fetch existing resource
		existedResource := newResource.GetObjectDeepCopy()
		err := r.Client.Get(context.TODO(), types2.NamespacedName{Namespace: namespace, Name: newResource.GetName()}, existedResource)
		if err != nil && !errors.IsNotFound(err) {
			klog.Errorf("ResourceDistribution get resource error, err: %v, name: %s", err, distributor.Name)
			return updateNewStatus(newStatus, failedNamespaces), err
		}

		// 5.2 resource doesn't exist
		//	(1).if the namespace is in targets, distribute resource;
		//  (2).else the namespace is NOT in targets, do nothing;
		if err != nil && errors.IsNotFound(err) {
			if _, ok := isInTargets[namespace]; ok {
				if err := r.distributeResource(distributor, namespace, newResource, newResourceHashCode); err != nil {
					klog.Errorf("ResourceDistribution create resource error, err: %v, name: %s", err, distributor.Name)
					return updateNewStatus(newStatus, failedNamespaces), err
				}
				delete(failedNamespaces, namespace) // delete this namespace from this map if successful
				newStatus.DistributedResources[namespace] = newResourceHashCode
				klog.V(3).Infof("ResourceDistribution(%s) create (%s/%s) for namespaces %s", distributor.Name, newResource.GetGroupVersionKind().Kind, newResource.GetName(), namespace)
			}
			continue
		}

		// 5.3 convert existedResource to UnifiedResource type for doing something
		oldResource, errs := utils.MakeUnifiedResourceFromObject(existedResource, field.NewPath(""))
		if len(errs) != 0 {
			klog.Errorf("ResourceDistribution convert resource to unifiedResource error, err: %v, name: %s", errs.ToAggregate(), distributor.Name)
			return updateNewStatus(newStatus, failedNamespaces), errs.ToAggregate()
		}

		// 5.4 resource exits
		//	(1).if the namespace is in targets, update the resource;
		//  (2).else if the namespace is NOT in targets and it was distributed by this distributor, delete the resource;
		annotations := oldResource.GetObjectMeta().Annotations
		_, ok := isInTargets[namespace]
		if ok && (annotations != nil && newResourceHashCode != annotations[utils.ResourceHashCodeAnnotation]) {
			if err := r.updateResource(distributor, namespace, newResource, newResourceHashCode); err != nil {
				klog.Errorf("ResourceDistribution update resource error, err: %v, name: %s", err, distributor.Name)
				return updateNewStatus(newStatus, failedNamespaces), err
			}
			delete(failedNamespaces, namespace) // delete this namespace from this map if successful
			newStatus.DistributedResources[namespace] = newResourceHashCode
			klog.V(3).Infof("ResourceDistribution(%s) update (%s/%s) for namespaces %s", distributor.Name, newResource.GetGroupVersionKind().Kind, newResource.GetName(), namespace)
		} else if !ok && (annotations != nil && annotations[utils.SourceResourceDistributionOfResource] == distributor.Name) {
			if err := r.deleteResource(namespace, oldResource); err != nil {
				klog.Errorf("ResourceDistribution delete resource error, err: %v, name: %s", err, distributor.Name)
				return updateNewStatus(newStatus, failedNamespaces), err
			}
			delete(failedNamespaces, namespace) // delete this namespace from this map if successful
			delete(distributor.Status.DistributedResources, namespace)
			klog.V(3).Infof("ResourceDistribution(%s) delete (%s/%s) for namespaces %s", distributor.Name, oldResource.GetGroupVersionKind().Kind, oldResource.GetName(), namespace)
		}
	}

	return newStatus, nil
}

// updateNewStatus update Description field of status
func updateNewStatus(status *appsv1alpha1.ResourceDistributionStatus, failedNamespaces map[string]struct{}) *appsv1alpha1.ResourceDistributionStatus {
	var namespaceList []string
	for namespace := range failedNamespaces {
		namespaceList = append(namespaceList, namespace)
	}
	if len(namespaceList) == 0 {
		status.Description = ResourceDistributionSucceed
	} else {
		status.Description = fmt.Sprintf("%s: %v", ResourceDistributionFailed, namespaceList)
	}
	return status
}

// getNamespaces return:
// (1) a map that contains all target namespace names
// (2) all namespace names, including both those fetched from cluster and newly-created namespaces
func prepareNamespaces(handlerClient client.Client, distributor *appsv1alpha1.ResourceDistribution) (isInTargets map[string]struct{}, allNamespaces []string, err error) {
	// 1. parsing all targets
	targetNamespaces, errs := utils.GetTargetNamespaces(handlerClient, &distributor.Spec.Targets)
	if len(errs) != 0 {
		klog.Errorf("ResourceDistribution get target namespaces error, err: %v, name: %s", errs.ToAggregate(), distributor.Name)
		return nil, nil, errs.ToAggregate()
	}
	klog.V(3).Infof("ResourceDistribution %s target namespaces %v\n\n", distributor.Name, targetNamespaces)

	// 2. construct a query map to check whether namespace is in targets quickly
	isInTargets = make(map[string]struct{})
	for _, namespace := range targetNamespaces {
		isInTargets[namespace] = struct{}{}
	}

	// 3. fetch all namespaces from cluster
	Namespaces := &corev1.NamespaceList{}
	if err := handlerClient.List(context.TODO(), Namespaces, &client.ListOptions{}); err != nil {
		klog.Errorf("ResourceDistribution get all namespaces error, err: %v, name: %s", err, distributor.Name)
		return nil, nil, err
	}

	// 4. record all cluster-in namespaces to check how many target namespaces have not been created
	readyNamespace := make(map[string]struct{})
	for _, namespace := range Namespaces.Items {
		readyNamespace[namespace.Name] = struct{}{}
		allNamespaces = append(allNamespaces, namespace.Name)
	}

	// 5. prepare un-created namespaces
	for _, namespace := range targetNamespaces {
		if _, ok := readyNamespace[namespace]; ok {
			continue
		}
		if err := createNamespace(handlerClient, namespace); err != nil {
			return nil, nil, err
		}
		allNamespaces = append(allNamespaces, namespace)
	}

	return
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

	klog.V(3).Infof("RD(%s) update status success", distributor.Name)
	return nil
}

// hashResource hash resource yaml using MD5
func hashResource(resourceYaml runtime.RawExtension) string {
	md5Hash := md5.Sum(resourceYaml.Raw)
	return hex.EncodeToString(md5Hash[:])
}

// makeResourceObject set some necessary information for resource before updating and creating
func makeResourceObject(distributor *appsv1alpha1.ResourceDistribution, namespace string, resource utils.UnifiedResource, hashCode string) runtime.Object {
	// hold resource Metadata pointer
	metaData := resource.GetObjectMeta()
	// 1. set namespace
	metaData.Namespace = namespace
	// 2. set ownerReference for cascading deletion
	metaData.OwnerReferences = []v1.OwnerReference{
		{
			APIVersion: distributor.APIVersion,
			Kind:       distributor.Kind,
			Name:       distributor.Name,
			UID:        distributor.UID,
		},
	}
	// 3. set resource annotations
	if metaData.Annotations == nil {
		metaData.Annotations = make(map[string]string)
	}
	metaData.Annotations[utils.ResourceHashCodeAnnotation] = hashCode
	metaData.Annotations[utils.SourceResourceDistributionOfResource] = distributor.Name

	return resource.GetObject()
}

// createNamespace try to create namespace
func createNamespace(handlerClient client.Client, name string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
	}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		createErr := handlerClient.Create(context.TODO(), namespace, &client.CreateOptions{})
		if createErr == nil {
			return nil
		}
		if err := handlerClient.Get(context.TODO(), types.NamespacedName{Namespace: name, Name: name}, namespace); err != nil {
			klog.Errorf("error create namespace %s from client", name)
		}
		return createErr
	}); err != nil {
		return err
	}

	return nil
}

// updateResource try to update resource
func (r *ReconcileResourceDistribution) updateResource(distributor *appsv1alpha1.ResourceDistribution, namespace string, resource utils.UnifiedResource, hashCode string) error {
	resource, errs := utils.MakeUnifiedResourceFromObject(resource.GetObjectDeepCopy(), field.NewPath(""))
	if len(errs) != 0 {
		klog.Errorf("error making resource from object in namespace %s", namespace)
		return errs.ToAggregate()
	}

	resourceObject := makeResourceObject(distributor, namespace, resource, hashCode)
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		updateErr := r.Client.Update(context.TODO(), resourceObject, &client.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: resource.GetName()}, resourceObject); err != nil {
			klog.Errorf("error getting updated resource in namespace %s from client", namespace)
		}
		return updateErr
	}); err != nil {
		return err
	}

	return nil
}

// updateResource try to create resource
func (r *ReconcileResourceDistribution) distributeResource(distributor *appsv1alpha1.ResourceDistribution, namespace string, resource utils.UnifiedResource, hashCode string) error {
	resource, errs := utils.MakeUnifiedResourceFromObject(resource.GetObjectDeepCopy(), field.NewPath(""))
	if len(errs) != 0 {
		klog.Errorf("error making resource from object in namespace %s", namespace)
		return errs.ToAggregate()
	}

	resourceObject := makeResourceObject(distributor, namespace, resource, hashCode)
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		createErr := r.Client.Create(context.TODO(), resourceObject, &client.CreateOptions{})
		if createErr == nil {
			return nil
		}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: resource.GetName()}, resourceObject); err != nil {
			klog.Errorf("error getting distribute resource in namespace %s from client", namespace)
		}
		return createErr
	}); err != nil {
		return err
	}

	return nil
}

// deleteResource try to delete resource
func (r *ReconcileResourceDistribution) deleteResource(namespace string, resource utils.UnifiedResource) error {
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		deleteErr := r.Client.Delete(context.TODO(), resource.GetObject(), &client.DeleteOptions{})
		if deleteErr == nil {
			return nil
		}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: resource.GetName()}, resource.GetObject()); err == nil || !errors.IsNotFound(err) {
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
