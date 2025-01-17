/*
Copyright 2023 The Kruise Authors.

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

package imagelistpulljob

import (
	"context"
	"fmt"
	"hash/fnv"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	hashutil "k8s.io/kubernetes/pkg/util/hash"
	"k8s.io/kubernetes/pkg/util/slice"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
)

var (
	concurrentReconciles        = 3
	controllerKind              = appsv1alpha1.SchemeGroupVersion.WithKind("ImageListPullJob")
	slowStartInitialBatchSize   = 1
	controllerName              = "imagelistpulljob-controller"
	resourceVersionExpectations = expectations.NewResourceVersionExpectation()
	scaleExpectations           = expectations.NewScaleExpectations()
)

// Add creates a new ImageListPullJob Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) || !utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) ||
		!utilfeature.DefaultFeatureGate.Enabled(features.ImagePullJobGate) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileImageListPullJob {
	return &ReconcileImageListPullJob{
		Client:   utilclient.NewClientFromManager(mgr, controllerName),
		scheme:   mgr.GetScheme(),
		clock:    clock.RealClock{},
		recorder: mgr.GetEventRecorderFor(controllerName),
	}
}

// add a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileImageListPullJob) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r,
		MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout()})
	if err != nil {
		return err
	}

	// Watch for changes to ImageListPullJob
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.ImageListPullJob{}, &handler.TypedEnqueueRequestForObject[*appsv1alpha1.ImageListPullJob]{}))
	if err != nil {
		return err
	}
	// Watch for changes to ImagePullJob
	// todo the imagelistpulljob(status) will not change if  the pull job status does not change significantly (ex. number of failed nodeimage changes from 1 to 2)
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.ImagePullJob{},
		&imagePullJobEventHandler{
			enqueueHandler: handler.TypedEnqueueRequestForOwner[*appsv1alpha1.ImagePullJob](mgr.GetScheme(), mgr.GetRESTMapper(),
				&appsv1alpha1.ImageListPullJob{}, handler.OnlyControllerOwner()),
		}))
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileImageListPullJob{}

// ReconcileImageListPullJob reconciles a ImageListPullJob object
type ReconcileImageListPullJob struct {
	client.Client
	scheme   *runtime.Scheme
	clock    clock.Clock
	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=imagelistpulljobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=imagelistpulljobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=imagelistpulljobs/finalizers,verbs=update

// Reconcile reads that state of the cluster for a ImageListPullJob object and makes changes based on the state read
// and what is in the ImageListPullJob.Spec
// Automatically generate RBAC rules to allow the Controller to read and write ImageListPullJob
func (r *ReconcileImageListPullJob) Reconcile(_ context.Context, request reconcile.Request) (res reconcile.Result, err error) {
	klog.V(5).InfoS("Starting to process ImageListPullJob", "imageListPullJob", request)

	// 1.Fetch the ImageListPullJob instance
	job := &appsv1alpha1.ImageListPullJob{}
	err = r.Get(context.TODO(), request.NamespacedName, job)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	hash, err := r.refreshJobTemplateHash(job)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("refresh job template hash error: %v", err)
	}

	// The Job has been finished
	if job.Status.CompletionTime != nil {
		var leftTime time.Duration
		if job.Spec.CompletionPolicy.TTLSecondsAfterFinished != nil {
			leftTime = time.Duration(*job.Spec.CompletionPolicy.TTLSecondsAfterFinished)*time.Second - time.Since(job.Status.CompletionTime.Time)
			if leftTime <= 0 {
				klog.InfoS("Deleting ImageListPullJob for ttlSecondsAfterFinished", "imageListPullJob", klog.KObj(job))
				if err = r.Delete(context.TODO(), job); err != nil {
					return reconcile.Result{}, fmt.Errorf("delete ImageListPullJob error: %v", err)
				}
				return reconcile.Result{}, nil
			}
		}
		return reconcile.Result{RequeueAfter: leftTime}, nil
	}

	if scaleSatisfied, unsatisfiedDuration, scaleDirtyImagePullJobs := scaleExpectations.SatisfiedExpectations(request.String()); !scaleSatisfied {
		if unsatisfiedDuration >= expectations.ExpectationTimeout {
			klog.InfoS("Expectation unsatisfied overtime for ImageListPullJob", "imageListPullJob", request, "scaleDirtyImagePullJobs", scaleDirtyImagePullJobs, "overtime", unsatisfiedDuration)
			return reconcile.Result{}, nil
		}
		klog.V(4).InfoS("Not satisfied scale for ImageListPullJob", "imageListPullJob", request, "scaleDirtyImagePullJobs", scaleDirtyImagePullJobs)
		return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
	}

	// 2. Get ImagePullJob owned by this job
	imagePullJobsMap, err := r.getOwnedImagePullJob(job)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get imagePullJob: %v", err)
	}

	// If resourceVersion expectations have not satisfied yet, just skip this reconcile
	for _, imagePullJob := range imagePullJobsMap {
		resourceVersionExpectations.Observe(imagePullJob)
		if isSatisfied, unsatisfiedDuration := resourceVersionExpectations.IsSatisfied(imagePullJob); !isSatisfied {
			if unsatisfiedDuration >= expectations.ExpectationTimeout {
				klog.InfoS("Expectation unsatisfied overtime for ImageListPullJob", "imageListPullJob", request, "timeout", unsatisfiedDuration)
				return reconcile.Result{}, nil
			}
			klog.V(4).InfoS("Not satisfied resourceVersion for ImageListPullJob", "imageListPullJob", request)
			return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
		}
	}

	// 3. Calculate the new status for this job
	newStatus := r.calculateStatus(job, imagePullJobsMap)

	// 4. Compute ImagePullJobActions
	needToCreate, needToDelete := r.computeImagePullJobActions(job, imagePullJobsMap, hash)

	// 5. Sync ImagePullJob
	err = r.syncImagePullJob(job, needToCreate, needToDelete)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 6. Update status
	if !util.IsJSONObjectEqual(&job.Status, newStatus) {
		if err = r.updateStatus(job, newStatus); err != nil {
			return reconcile.Result{}, fmt.Errorf("update ImageListPullJob status error: %v", err)
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileImageListPullJob) refreshJobTemplateHash(job *appsv1alpha1.ImageListPullJob) (string, error) {
	newHash := func(job *appsv1alpha1.ImageListPullJob) string {
		jobTemplateHasher := fnv.New32a()
		hashutil.DeepHashObject(jobTemplateHasher, job.Spec.ImagePullJobTemplate)
		return rand.SafeEncodeString(fmt.Sprint(jobTemplateHasher.Sum32()))
	}(job)

	oldHash := job.Labels[appsv1.ControllerRevisionHashLabelKey]
	if newHash == oldHash {
		return newHash, nil
	}

	emptyJob := &appsv1alpha1.ImageListPullJob{}
	emptyJob.SetName(job.Name)
	emptyJob.SetNamespace(job.Namespace)
	body := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, appsv1.ControllerRevisionHashLabelKey, newHash)
	return newHash, r.Patch(context.TODO(), emptyJob, client.RawPatch(types.MergePatchType, []byte(body)))
}

func (r *ReconcileImageListPullJob) updateStatus(job *appsv1alpha1.ImageListPullJob, newStatus *appsv1alpha1.ImageListPullJobStatus) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		imageListPullJob := &appsv1alpha1.ImageListPullJob{}
		if err := r.Get(context.TODO(), types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, imageListPullJob); err != nil {
			return err
		}
		imageListPullJob.Status = *newStatus
		return r.Status().Update(context.TODO(), imageListPullJob)
	})
}

func (r *ReconcileImageListPullJob) computeImagePullJobActions(job *appsv1alpha1.ImageListPullJob, imagePullJobs map[string]*appsv1alpha1.ImagePullJob, hash string) ([]*appsv1alpha1.ImagePullJob, []*appsv1alpha1.ImagePullJob) {
	var needToDelete, needToCreate []*appsv1alpha1.ImagePullJob
	//1. need to create
	images, needToDelete := r.filterImagesAndImagePullJobs(job, imagePullJobs, hash)
	needToCreate = r.newImagePullJobs(job, images, hash)
	// some images delete from ImageListPullJob.Spec.Images
	for image, imagePullJob := range imagePullJobs {
		if !slice.ContainsString(job.Spec.Images, image, nil) {
			needToDelete = append(needToDelete, imagePullJob)
		}
	}
	return needToCreate, needToDelete
}

func (r *ReconcileImageListPullJob) calculateStatus(job *appsv1alpha1.ImageListPullJob, imagePullJobs map[string]*appsv1alpha1.ImagePullJob) *appsv1alpha1.ImageListPullJobStatus {
	var active, completed, succeeded int32
	// record the failed image status
	var failedImageStatuses []*appsv1alpha1.FailedImageStatus

	for _, imagePullJob := range imagePullJobs {
		if imagePullJob.Status.StartTime == nil {
			continue
		}

		if imagePullJob.Status.Active > 0 {
			active = active + 1
		}

		if imagePullJob.Status.Failed > 0 {
			failedImagePullJobStatus := &appsv1alpha1.FailedImageStatus{
				ImagePullJob: imagePullJob.Name,
				Name:         imagePullJob.Spec.Image,
				Message:      fmt.Sprintf("Please check for details which nodes failed by 'kubectl get ImagePullJob %s'.", imagePullJob.Name),
			}
			failedImageStatuses = append(failedImageStatuses, failedImagePullJobStatus)
		}

		if imagePullJob.Status.Desired == (imagePullJob.Status.Failed + imagePullJob.Status.Succeeded) {
			completed = completed + 1
		}

		if imagePullJob.Status.Desired == imagePullJob.Status.Succeeded {
			succeeded = succeeded + 1
		}
	}

	// 4. status
	newStatus := &appsv1alpha1.ImageListPullJobStatus{
		Desired:             int32(len(job.Spec.Images)),
		Active:              active,
		Completed:           completed,
		Succeeded:           succeeded,
		StartTime:           job.Status.StartTime,
		FailedImageStatuses: failedImageStatuses,
	}

	now := metav1.NewTime(r.clock.Now())
	if newStatus.StartTime == nil {
		newStatus.StartTime = &now
	}

	if job.Spec.CompletionPolicy.Type != appsv1alpha1.Never && newStatus.Desired == newStatus.Completed {
		newStatus.CompletionTime = &now
	}
	return newStatus
}

func (r *ReconcileImageListPullJob) syncImagePullJob(job *appsv1alpha1.ImageListPullJob, needToCreate, needToDelete []*appsv1alpha1.ImagePullJob) error {

	//1. manage creating
	var errs []error
	if len(needToCreate) > 0 {
		var createdNum int
		var createdErr error

		createdNum, createdErr = util.SlowStartBatch(len(needToCreate), slowStartInitialBatchSize, func(idx int) error {
			imagePullJob := needToCreate[idx]
			key := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}.String()
			scaleExpectations.ExpectScale(key, expectations.Create, imagePullJob.Spec.Image)
			err := r.Create(context.TODO(), imagePullJob)
			if err != nil {
				scaleExpectations.ObserveScale(key, expectations.Create, imagePullJob.Spec.Image)
			}
			return err
		})

		if createdErr == nil {
			r.recorder.Eventf(job, corev1.EventTypeNormal, "Successful create ImagePullJob", "Create %d ImagePullJob", createdNum)
		} else {
			errs = append(errs, createdErr)
		}
	}

	//2. manage deleting
	if len(needToDelete) > 0 {
		var deleteErrs []error
		for _, imagePullJob := range needToDelete {
			key := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}.String()
			scaleExpectations.ExpectScale(key, expectations.Delete, imagePullJob.Spec.Image)
			if err := r.Delete(context.TODO(), imagePullJob); err != nil {
				scaleExpectations.ObserveScale(key, expectations.Delete, imagePullJob.Spec.Image)
				deleteErrs = append(deleteErrs, fmt.Errorf("fail to delete ImagePullJob (%s/%s) for : %s", imagePullJob.Namespace, imagePullJob.Name, err))
			}
		}

		if len(deleteErrs) > 0 {
			errs = append(errs, deleteErrs...)
		} else {
			r.recorder.Eventf(job, corev1.EventTypeNormal, "Successful delete ImagePullJob", "Delete %d ImagePullJob", len(needToDelete))
		}
	}

	return utilerrors.NewAggregate(errs)
}

func (r *ReconcileImageListPullJob) filterImagesAndImagePullJobs(job *appsv1alpha1.ImageListPullJob, imagePullJobs map[string]*appsv1alpha1.ImagePullJob, hash string) ([]string, []*appsv1alpha1.ImagePullJob) {
	var images, imagesInCurrentImagePullJob []string
	var needToDelete []*appsv1alpha1.ImagePullJob

	for image := range imagePullJobs {
		imagesInCurrentImagePullJob = append(imagesInCurrentImagePullJob, image)
	}

	for _, image := range job.Spec.Images {

		// should create imagePullJob for new image
		if len(imagePullJobs) <= 0 || !slice.ContainsString(imagesInCurrentImagePullJob, image, nil) {
			images = append(images, image)
			continue
		}
		imagePullJob, ok := imagePullJobs[image]
		if !ok {
			klog.InfoS("Could not found imagePullJob for image name", "imageName", image)
			continue
		}
		// should create new imagePullJob if the template is changed.
		if !isConsistentVersion(imagePullJob, &job.Spec.ImagePullJobTemplate, hash) {
			images = append(images, image)
			// should delete old imagepulljob
			needToDelete = append(needToDelete, imagePullJob)
		}
	}

	return images, needToDelete
}

func (r *ReconcileImageListPullJob) newImagePullJobs(job *appsv1alpha1.ImageListPullJob, images []string, hash string) []*appsv1alpha1.ImagePullJob {
	var needToCreate []*appsv1alpha1.ImagePullJob
	if len(images) <= 0 {
		return needToCreate
	}
	for _, image := range images {
		imagePullJob := &appsv1alpha1.ImagePullJob{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    job.Namespace,
				GenerateName: fmt.Sprintf("%s-", job.Name),
				Labels: map[string]string{
					appsv1.ControllerRevisionHashLabelKey: hash,
				},
				Annotations: make(map[string]string),
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(job, controllerKind),
				},
			},
			Spec: appsv1alpha1.ImagePullJobSpec{
				Image:                image,
				ImagePullJobTemplate: job.Spec.ImagePullJobTemplate,
			},
		}
		needToCreate = append(needToCreate, imagePullJob)
	}

	return needToCreate
}

func (r *ReconcileImageListPullJob) getOwnedImagePullJob(job *appsv1alpha1.ImageListPullJob) (map[string]*appsv1alpha1.ImagePullJob, error) {

	opts := &client.ListOptions{
		Namespace:     job.Namespace,
		FieldSelector: fields.SelectorFromSet(fields.Set{fieldindex.IndexNameForOwnerRefUID: string(job.UID)}),
	}
	imagePullJobList := &appsv1alpha1.ImagePullJobList{}
	err := r.List(context.TODO(), imagePullJobList, opts, utilclient.DisableDeepCopy)
	if err != nil {
		return nil, err
	}
	imagePullJobsMap := make(map[string]*appsv1alpha1.ImagePullJob)
	for i := range imagePullJobList.Items {
		imagePullJob := imagePullJobList.Items[i]
		if imagePullJob.DeletionTimestamp.IsZero() {
			imagePullJobsMap[imagePullJob.Spec.Image] = &imagePullJob
		}
	}
	return imagePullJobsMap, nil
}

func isConsistentVersion(oldImagePullJob *appsv1alpha1.ImagePullJob, newTemplate *appsv1alpha1.ImagePullJobTemplate, hash string) bool {
	oldHash, exists := oldImagePullJob.Labels[appsv1.ControllerRevisionHashLabelKey]
	if oldHash == hash {
		return true
	}
	if !exists && reflect.DeepEqual(oldImagePullJob.Spec.ImagePullJobTemplate, *newTemplate) {
		return true
	}

	klog.V(4).InfoS("ImagePullJob specification changed", "imagePullJob", klog.KObj(oldImagePullJob))
	return false
}
