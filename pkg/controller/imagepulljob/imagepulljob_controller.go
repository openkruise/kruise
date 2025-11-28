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

package imagepulljob

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openkruise/kruise/apis/apps/defaults"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	utilimagejob "github.com/openkruise/kruise/pkg/util/imagejob"
)

func init() {
	flag.IntVar(&concurrentReconciles, "imagepulljob-workers", concurrentReconciles, "Max concurrent workers for ImagePullJob controller.")
}

var (
	concurrentReconciles        = 3
	controllerKind              = appsv1beta1.SchemeGroupVersion.WithKind("ImagePullJob")
	resourceVersionExpectations = expectations.NewResourceVersionExpectation()
	scaleExpectations           = expectations.NewScaleExpectations()
)

const (
	defaultParallelism = 1
	minRequeueTime     = time.Second

	// SourceSecretKeyAnno is an annotations instead of label
	// because the length of key may be more than 64.
	SourceSecretKeyAnno = "imagepulljobs.kruise.io/source-key"
	// SourceSecretUIDLabelKey is designed to select target via source secret.
	SourceSecretUIDLabelKey = "imagepulljobs.kruise.io/source-uid"
	// TargetOwnerReferencesAnno records the keys of imagePullJobs that refers
	// the target secret. If TargetOwnerReferencesAnno is empty, means the target
	// secret should be deleted.
	TargetOwnerReferencesAnno = "imagepulljobs.kruise.io/references"
)

// Add creates a new ImagePullJob Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) || !utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) ||
		!utilfeature.DefaultFeatureGate.Enabled(features.ImagePullJobGate) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileImagePullJob {
	return &ReconcileImagePullJob{
		Client: utilclient.NewClientFromManager(mgr, "imagepulljob-controller"),
		scheme: mgr.GetScheme(),
		clock:  clock.RealClock{},
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileImagePullJob) error {
	// Create a new controller
	c, err := controller.New("imagepulljob-controller", mgr, controller.Options{Reconciler: r,
		MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout()})
	if err != nil {
		return err
	}

	// Watch for changes to ImagePullJob
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1beta1.ImagePullJob{}, &handler.TypedEnqueueRequestForObject[*appsv1beta1.ImagePullJob]{}))
	if err != nil {
		return err
	}

	// Watch for nodeimage update to get image pull status
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1beta1.NodeImage{}, &nodeImageEventHandler{Reader: mgr.GetCache()}))
	if err != nil {
		return err
	}

	// Watch for pod for jobs that have pod selector
	err = c.Watch(source.Kind(mgr.GetCache(), &v1.Pod{}, &podEventHandler{Reader: mgr.GetCache()}))
	if err != nil {
		return err
	}

	// Watch for secret for jobs that have pullSecrets
	err = c.Watch(source.Kind(mgr.GetCache(), &v1.Secret{}, &secretEventHandler{Reader: mgr.GetCache()}))
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileImagePullJob{}

// ReconcileImagePullJob reconciles a ImagePullJob object
type ReconcileImagePullJob struct {
	client.Client
	scheme *runtime.Scheme
	clock  clock.Clock
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=imagepulljobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=imagepulljobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=imagepulljobs/finalizers,verbs=update

// Reconcile reads that state of the cluster for a ImagePullJob object and makes changes based on the state read
// and what is in the ImagePullJob.Spec
func (r *ReconcileImagePullJob) Reconcile(_ context.Context, request reconcile.Request) (res reconcile.Result, err error) {
	start := time.Now()
	klog.V(5).InfoS("Starting to process ImagePullJob", "imagePullJob", request)
	defer func() {
		if err != nil {
			klog.ErrorS(err, "Failed to process ImagePullJob", "imagePullJob", request, "elapsedTime", time.Since(start))
		} else if res.RequeueAfter > 0 {
			klog.InfoS("Finish to process ImagePullJob with scheduled retry", "imagePullJob", request, "elapsedTime", time.Since(start), "RetryAfter", res.RequeueAfter)
		} else {
			klog.InfoS("Finish to process ImagePullJob", "imagePullJob", request, "elapsedTime", time.Since(start))
		}
	}()

	// Fetch the ImagePullJob instance
	job := &appsv1beta1.ImagePullJob{}
	err = r.Get(context.TODO(), request.NamespacedName, job)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// If scale expectations have not satisfied yet, just skip this reconcile
	if scaleSatisfied, unsatisfiedDuration, dirtyData := scaleExpectations.SatisfiedExpectations(request.String()); !scaleSatisfied {
		if unsatisfiedDuration >= expectations.ExpectationTimeout {
			klog.InfoS("ImagePullJob: expectation unsatisfied overtime", "imagePullJob", request, "dirtyData", dirtyData, "overTime", unsatisfiedDuration)
			return reconcile.Result{}, nil
		}
		klog.V(4).InfoS("ImagePullJob: not satisfied scale", "imagePullJob", request, "dirtyData", dirtyData)
		return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
	}

	// If resourceVersion expectations have not satisfied yet, just skip this reconcile
	resourceVersionExpectations.Observe(job)
	if isSatisfied, unsatisfiedDuration := resourceVersionExpectations.IsSatisfied(job); !isSatisfied {
		if unsatisfiedDuration >= expectations.ExpectationTimeout {
			klog.InfoS("Expectation unsatisfied overtime", "imagePullJob", request, "timeout", unsatisfiedDuration)
			return reconcile.Result{}, nil
		}
		klog.V(4).InfoS("Not satisfied resourceVersion", "imagePullJob", request)
		return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
	}

	// The Job has been finished
	if job.DeletionTimestamp == nil && job.Status.CompletionTime != nil {
		var leftTime time.Duration
		if job.Spec.CompletionPolicy.TTLSecondsAfterFinished != nil {
			leftTime = time.Duration(*job.Spec.CompletionPolicy.TTLSecondsAfterFinished)*time.Second - time.Since(job.Status.CompletionTime.Time)
			if leftTime <= 0 {
				klog.InfoS("Deleting ImagePullJob for ttlSecondsAfterFinished", "imagePullJob", klog.KObj(job))
				if err = r.Delete(context.TODO(), job); err != nil {
					return reconcile.Result{}, fmt.Errorf("delete job error: %v", err)
				}
				return reconcile.Result{}, nil
			}
		}
		return reconcile.Result{RequeueAfter: leftTime}, nil
	} else if job.DeletionTimestamp == nil {
		// add protection finalizer to ensure the GC of secrets
		if err = r.addProtectionFinalizer(job); err != nil {
			return reconcile.Result{}, err
		}
	}

	// Get all NodeImage related to this ImagePullJob
	nodeImages, err := utilimagejob.GetNodeImagesForJob(r.Client, job)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get NodeImages: %v", err)
	}

	// If resourceVersion expectations have not satisfied yet, just skip this reconcile
	for _, nodeImage := range nodeImages {
		resourceVersionExpectations.Observe(nodeImage)
		if isSatisfied, unsatisfiedDuration := resourceVersionExpectations.IsSatisfied(nodeImage); !isSatisfied {
			if unsatisfiedDuration >= expectations.ExpectationTimeout {
				klog.InfoS("Expectation unsatisfied overtime, waiting for NodeImage updating", "imagePullJob", request, "nodeImageName", nodeImage.Name, "timeout", unsatisfiedDuration)
				return reconcile.Result{}, nil
			}
			klog.V(4).InfoS("Not satisfied resourceVersion, waiting for NodeImage updating", "imagePullJob", request, "nodeImageName", nodeImage.Name)
			// fix issue https://github.com/openkruise/kruise/issues/1528
			return reconcile.Result{RequeueAfter: time.Second * 5}, nil
		}
	}

	// sync secret to kruise-daemon-config namespace before pulling
	secrets, secretsDeleted, err := r.syncSecrets(job)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync secrets: %v", err)
	}

	// Calculate the new status for this job
	newStatus, err := r.calculateStatus(job, nodeImages)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to calculate status: %v", err)
	}

	// Sync image to more NodeImages
	if err = r.syncNodeImages(job, newStatus, nodeImages, secrets, secretsDeleted); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync NodeImages: %v", err)
	}

	if job.DeletionTimestamp != nil {
		// ensure the GC of secrets and remove protection finalizer
		return reconcile.Result{}, r.finalize(job)
	}

	if !util.IsJSONObjectEqual(&job.Status, newStatus) {
		job.Status = *newStatus
		if err = r.Status().Update(context.TODO(), job); err != nil {
			return reconcile.Result{}, fmt.Errorf("update ImagePullJob status error: %v", err)
		}
		resourceVersionExpectations.Expect(job)
	}

	if job.Spec.CompletionPolicy.Type != appsv1beta1.Never && job.Spec.CompletionPolicy.ActiveDeadlineSeconds != nil {
		leftTime := time.Duration(*job.Spec.CompletionPolicy.ActiveDeadlineSeconds)*time.Second - time.Since(newStatus.StartTime.Time)
		if leftTime < minRequeueTime {
			leftTime = minRequeueTime
		}
		return reconcile.Result{RequeueAfter: leftTime}, nil
	}
	return reconcile.Result{}, nil
}

// syncSecrets synchronizes secrets for the given ImagePullJob
// It manages secrets in the kruise-daemon-config namespace by:
// 1. Getting target secret mappings
// 2. Releasing secrets that are no longer needed
// 3. Syncing current secrets to target namespace
//
// Parameters:
//   - job: The ImagePullJob to sync secrets for
//
// Returns:
//   - []appsv1beta1.ReferenceObject: Updated secrets references
//   - []appsv1beta1.ReferenceObject: Deleted secrets references
//   - error: Any error that occurred during sync
func (r *ReconcileImagePullJob) syncSecrets(job *appsv1beta1.ImagePullJob) ([]appsv1beta1.ReferenceObject, []appsv1beta1.ReferenceObject, error) {
	if job.Namespace == util.GetKruiseDaemonConfigNamespace() {
		return getSecrets(job), nil, nil // Ignore this special case.
	}

	targetMap, deleteMap, err := r.getTargetSecretMap(job)
	if err != nil {
		return nil, nil, err
	}
	secretsDeleted, err := r.releaseTargetSecrets(deleteMap, job)
	if err != nil {
		return nil, nil, err
	}
	if job.DeletionTimestamp != nil || job.Status.CompletionTime != nil {
		secretsDeleted, err := r.releaseTargetSecrets(targetMap, job)
		return nil, secretsDeleted, err
	}
	if err = r.checkNamespaceExists(util.GetKruiseDaemonConfigNamespace()); err != nil {
		return nil, nil, fmt.Errorf("failed to check kruise-daemon-config namespace: %v", err)
	}
	secretsUpdated, err := r.syncTargetSecrets(job, targetMap)
	return secretsUpdated, secretsDeleted, err
}

// syncNodeImages synchronizes NodeImage resources for the given ImagePullJob
// It ensures image pull tasks on nodes are consistent with the current job configuration
//
// Parameters:
//   - job: The ImagePullJob being processed
//   - newStatus: The new status of the ImagePullJob tracking active pulls
//   - nodeImages: List of all related NodeImage objects
//   - secrets: PullSecret references to be synchronized to NodeImages
//   - secretsDeleted: PullSecret references that should be removed from NodeImages
//
// Returns:
//   - error: Any error that occurred during synchronization, or nil if successful
func (r *ReconcileImagePullJob) syncNodeImages(job *appsv1beta1.ImagePullJob, newStatus *appsv1beta1.ImagePullJobStatus, nodeImages []*appsv1beta1.NodeImage, secrets []appsv1beta1.ReferenceObject, secretsDeleted []appsv1beta1.ReferenceObject) error {

	imageName, imageTag, _ := daemonutil.NormalizeImageRefToNameTag(job.Spec.Image)
	klog.V(3).InfoS("Start to sync NodeImages", "imagePullJob", klog.KObj(job), "activePullingCount", newStatus.Active)

	var notSyncedNodeImages []string

nodeloop:
	for _, nodeImage := range nodeImages {
		imageSpec, ok := nodeImage.Spec.Images[imageName]
		if !ok {
			notSyncedNodeImages = append(notSyncedNodeImages, nodeImage.Name)
			continue
		}
		for _, secret := range secrets {
			if !containsObject(imageSpec.PullSecrets, secret) {
				notSyncedNodeImages = append(notSyncedNodeImages, nodeImage.Name)
				continue nodeloop
			}
		}

		tagVersion := searchImage(job, imageSpec, imageTag)
		if tagVersion < 0 || job.DeletionTimestamp != nil {
			notSyncedNodeImages = append(notSyncedNodeImages, nodeImage.Name)
			continue
		}

		for _, secret := range secretsDeleted {
			if containsObject(imageSpec.PullSecrets, secret) {
				notSyncedNodeImages = append(notSyncedNodeImages, nodeImage.Name)
				break
			}
		}
	}

	if len(notSyncedNodeImages) == 0 {
		return nil
	}
	parallelismLimit := defaultParallelism
	if job.Spec.Parallelism != nil {
		parallelismLimit = job.Spec.Parallelism.IntValue()
	}
	parallelism := parallelismLimit - int(newStatus.Active)
	if parallelism <= 0 {
		klog.V(3).InfoS("Found ImagePullJob have active pulling more than parallelism, so skip to sync the left NodeImages",
			"imagePullJob", klog.KObj(job), "activePullingCount", newStatus.Active, "parallelismLimit", parallelismLimit, "notSyncedNodeImagesCount", len(notSyncedNodeImages))
		return nil
	}

	if len(notSyncedNodeImages) < parallelism {
		parallelism = len(notSyncedNodeImages)
	}

	ownerRef := getOwnerRef(job)
	pullPolicy := getImagePullPolicy(job)
	now := metav1.NewTime(r.clock.Now())
	for i := 0; i < parallelism; i++ {
		var skip bool
		updateErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			nodeImage := appsv1beta1.NodeImage{}
			if err := r.Get(context.TODO(), types.NamespacedName{Name: notSyncedNodeImages[i]}, &nodeImage); err != nil {
				return err
			}
			if nodeImage.Spec.Images == nil {
				nodeImage.Spec.Images = make(map[string]appsv1beta1.ImageSpec, 1)
			}
			imageSpec := nodeImage.Spec.Images[imageName]
			imageSpec.SandboxConfig = job.Spec.SandboxConfig

			for _, secret := range secrets {
				if !containsObject(imageSpec.PullSecrets, secret) {
					imageSpec.PullSecrets = append(imageSpec.PullSecrets, secret)
				}
			}
			for _, secret := range secretsDeleted {
				imageSpec.PullSecrets = util.Filter(imageSpec.PullSecrets, func(o appsv1beta1.ReferenceObject) bool {
					return o.Namespace == secret.Namespace && o.Name == secret.Name
				})
			}

			var tags2Delete []*appsv1beta1.ImageTagSpec
			needAddTag := true
			for i := range imageSpec.Tags {
				tagSpec := &imageSpec.Tags[i]
				if tagSpec.Tag != imageTag {
					continue
				}
				needAddTag = false
				if util.ContainsObjectRef(tagSpec.OwnerReferences, *ownerRef) {
					if job.DeletionTimestamp != nil {
						tagSpec.OwnerReferences = util.Filter(tagSpec.OwnerReferences, func(o v1.ObjectReference) bool {
							return o.UID == ownerRef.UID
						})
						if len(tagSpec.OwnerReferences) == 0 {
							tags2Delete = append(tags2Delete, tagSpec)
						}
					}
					skip = true
					continue
				}
				// increase version to start a new round of image downloads
				tagSpec.Version++
				// merge owner reference
				tagSpec.OwnerReferences = append(tagSpec.OwnerReferences, *ownerRef)
				tagSpec.CreatedAt = &now
				tagSpec.ImagePullPolicy = job.Spec.ImagePullPolicy
				break
			}
			if needAddTag {
				var foundVersion int64 = -1
				if imageStatus, ok := nodeImage.Status.ImageStatuses[imageName]; ok {
					for _, tagStatus := range imageStatus.Tags {
						if tagStatus.Tag == imageTag {
							foundVersion = tagStatus.Version
							break
						}
					}
				}

				imageSpec.Tags = append(imageSpec.Tags, appsv1beta1.ImageTagSpec{
					Tag:             imageTag,
					Version:         foundVersion + 1,
					PullPolicy:      pullPolicy,
					OwnerReferences: []v1.ObjectReference{*ownerRef},
					CreatedAt:       &now,
					ImagePullPolicy: job.Spec.ImagePullPolicy,
				})
			}

			for _, imageTag := range tags2Delete {
				imageSpec.Tags = util.Filter(imageSpec.Tags, func(t appsv1beta1.ImageTagSpec) bool {
					return t.Tag == imageTag.Tag && t.Version == imageTag.Version
				})
			}

			if len(imageSpec.Tags) == 0 {
				delete(nodeImage.Spec.Images, imageName)
			} else {
				utilimagejob.SortSpecImageTagsV1beta1(&imageSpec)
				nodeImage.Spec.Images[imageName] = imageSpec
			}

			oldResourceVersion := nodeImage.ResourceVersion
			err := r.Update(context.TODO(), &nodeImage)
			if err != nil {
				return err
			}
			if oldResourceVersion != nodeImage.ResourceVersion {
				resourceVersionExpectations.Expect(&nodeImage)
			}
			return nil
		})
		if updateErr != nil {
			return fmt.Errorf("update NodeImage %s error: %v", notSyncedNodeImages[i], updateErr)
		} else if skip {
			klog.V(4).InfoS("ImagePullJob found image already synced in NodeImage", "imagePullJob", klog.KObj(job), "image", job.Spec.Image, "nodeImage", notSyncedNodeImages[i])
			continue
		}
		klog.V(3).InfoS("ImagePullJob had synced image into NodeImage", "imagePullJob", klog.KObj(job), "image", job.Spec.Image, "nodeImage", notSyncedNodeImages[i])
	}
	return nil
}

// getTargetSecretMap retrieves and categorizes secrets related to the given ImagePullJob.
// It returns two maps: one for active target secrets and another for secrets that should be deleted,
// along with any error encountered during the process.
//
// Parameters:
//   - job: The ImagePullJob for which to retrieve related secrets
//
// Returns:
//   - map[string]*v1.Secret: Map of active target secrets keyed by source secret UID
//   - map[string]*v1.Secret: Map of secrets to be deleted keyed by source secret UID
//   - error: Any error that occurred during the operation
func (r *ReconcileImagePullJob) getTargetSecretMap(job *appsv1beta1.ImagePullJob) (map[string]*v1.Secret, map[string]*v1.Secret, error) {
	options := client.ListOptions{
		Namespace: util.GetKruiseDaemonConfigNamespace(),
	}
	targetLister := &v1.SecretList{}
	if err := r.List(context.TODO(), targetLister, &options, utilclient.DisableDeepCopy); err != nil {
		return nil, nil, err
	}

	jobKey := keyFromObject(job)
	sourceReferences := getSecrets(job)
	deleteMap := make(map[string]*v1.Secret)
	targetMap := make(map[string]*v1.Secret, len(targetLister.Items))
	for i := range targetLister.Items {
		target := &targetLister.Items[i]
		if target.DeletionTimestamp != nil {
			continue
		}
		keySet := referenceSetFromTarget(target)
		if !keySet.Contains(jobKey) {
			continue
		}
		sourceNs, sourceName, err := cache.SplitMetaNamespaceKey(target.Annotations[SourceSecretKeyAnno])
		if err != nil {
			klog.ErrorS(err, "Failed to parse source key from annotations in Secret", "secret", klog.KObj(target))
			continue
		}
		if containsObject(sourceReferences, appsv1beta1.ReferenceObject{Namespace: sourceNs, Name: sourceName}) {
			targetMap[target.Labels[SourceSecretUIDLabelKey]] = target
		} else {
			deleteMap[target.Labels[SourceSecretUIDLabelKey]] = target
		}
	}
	return targetMap, deleteMap, nil
}

// releaseTargetSecrets releases the target secrets that are no longer referenced by any ImagePullJob.
// It removes the reference to the given job from each target secret's annotation, and deletes the secret
// if no other jobs are referencing it.
//
// Parameters:
//   - targetMap: A map of target secrets that need to be released, keyed by their identifiers
//   - job: The ImagePullJob that is being reconciled
//
// Returns:
//   - []appsv1beta1.ReferenceObject: A slice of ReferenceObject representing the secrets that were deleted
//   - error: An error if any operation fails, nil otherwise
func (r *ReconcileImagePullJob) releaseTargetSecrets(targetMap map[string]*v1.Secret, job *appsv1beta1.ImagePullJob) ([]appsv1beta1.ReferenceObject, error) {
	if len(targetMap) == 0 {
		return nil, nil
	}

	secretsDeleted := make([]appsv1beta1.ReferenceObject, 0, len(targetMap))
	jobKey := keyFromObject(job)
	for _, secret := range targetMap {
		if secret == nil {
			continue
		}

		keySet := referenceSetFromTarget(secret)
		// Remove the reference to this job from target, we use Update instead of
		// Patch to make sure we do not delete any targets that is still referred,
		// because a target may be newly referred in this reconcile round.
		if keySet.Contains(keyFromObject(job)) {
			keySet.Delete(jobKey)
			secret = secret.DeepCopy()
			secret.Annotations[TargetOwnerReferencesAnno] = keySet.String()
			if err := r.Update(context.TODO(), secret); err != nil {
				return nil, err
			}
			resourceVersionExpectations.Expect(secret)
		}

		// The target is still referred by other jobs, do not delete it.
		if !keySet.IsEmpty() {
			return nil, nil
		}

		// Just delete it if no one refers it anymore.
		if err := r.Delete(context.TODO(), secret); err != nil && !errors.IsNotFound(err) {
			return nil, err
		}

		secretsDeleted = append(secretsDeleted, appsv1beta1.ReferenceObject{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		})
	}
	return secretsDeleted, nil
}

func (r *ReconcileImagePullJob) syncTargetSecrets(job *appsv1beta1.ImagePullJob, targetMap map[string]*v1.Secret) ([]appsv1beta1.ReferenceObject, error) {
	sourceReferences := getSecrets(job)
	targetReferences := make([]appsv1beta1.ReferenceObject, 0, len(sourceReferences))
	for _, sourceRef := range sourceReferences {
		source := &v1.Secret{}
		if err := r.Get(context.TODO(), keyFromRef(sourceRef), source); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return nil, err
		}

		target := targetMap[string(source.UID)]
		switch action := computeTargetSyncAction(source, target, job); action {
		case create:
			referenceKeys := makeReferenceSet(keyFromObject(job))
			target = targetFromSource(source, referenceKeys)
			scaleExpectations.ExpectScale(keyFromObject(job).String(), expectations.Create, string(source.UID))
			if err := r.Create(context.TODO(), target); err != nil {
				scaleExpectations.ObserveScale(keyFromObject(job).String(), expectations.Create, string(source.UID))
				return nil, err
			}

		case update:
			referenceKeys := referenceSetFromTarget(target).Insert(keyFromObject(job))
			target = updateTarget(target, source, referenceKeys)
			if err := r.Update(context.TODO(), target); err != nil {
				return nil, err
			}
			resourceVersionExpectations.Expect(target)
		}
		targetReferences = append(targetReferences, appsv1beta1.ReferenceObject{Namespace: target.Namespace, Name: target.Name})
	}
	return targetReferences, nil
}

func (r *ReconcileImagePullJob) calculateStatus(job *appsv1beta1.ImagePullJob, nodeImages []*appsv1beta1.NodeImage) (*appsv1beta1.ImagePullJobStatus, error) {
	newStatus := appsv1beta1.ImagePullJobStatus{
		StartTime: job.Status.StartTime,
		Desired:   int32(len(nodeImages)),
	}
	now := metav1.NewTime(r.clock.Now())
	if newStatus.StartTime == nil {
		newStatus.StartTime = &now
	}

	imageName, imageTag, err := daemonutil.NormalizeImageRefToNameTag(job.Spec.Image)
	if err != nil {
		return nil, fmt.Errorf("invalid image %s: %v", job.Spec.Image, err)
	}

	var notSynced, pulling, succeeded, failed []string
	for _, nodeImage := range nodeImages {
		var tagVersion int64 = -1
		if imageSpec, ok := nodeImage.Spec.Images[imageName]; ok {
			tagVersion = searchImage(job, imageSpec, imageTag)
		}

		if tagVersion < 0 {
			notSynced = append(notSynced, nodeImage.Name)
			continue
		}

		imageStatus, _ := nodeImage.Status.ImageStatuses[imageName]
		foundTag := false
		for _, tagStatus := range imageStatus.Tags {
			if tagStatus.Tag != imageTag {
				continue
			}
			if tagStatus.Version != tagVersion {
				break
			}
			switch tagStatus.Phase {
			case appsv1beta1.ImagePhaseSucceeded:
				succeeded = append(succeeded, nodeImage.Name)
			case appsv1beta1.ImagePhaseFailed:
				failed = append(failed, nodeImage.Name)
			default:
				pulling = append(pulling, nodeImage.Name)
			}
			foundTag = true
			break
		}
		if !foundTag {
			pulling = append(pulling, nodeImage.Name)
		}
	}

	if job.Spec.CompletionPolicy.Type != appsv1beta1.Never && job.Spec.CompletionPolicy.ActiveDeadlineSeconds != nil && int(newStatus.Desired) != len(succeeded)+len(failed) {
		if time.Duration(*job.Spec.CompletionPolicy.ActiveDeadlineSeconds)*time.Second <= time.Since(newStatus.StartTime.Time) {
			newStatus.CompletionTime = &now
			newStatus.Succeeded = int32(len(succeeded))
			failed = append(failed, pulling...)
			failed = append(failed, notSynced...)
			newStatus.Failed = int32(len(failed))
			newStatus.FailedNodes = failed
			newStatus.Message = "job exceeds activeDeadlineSeconds"
			return &newStatus, nil
		}
	}

	newStatus.Active = int32(len(pulling))
	newStatus.Succeeded = int32(len(succeeded))
	newStatus.Failed = int32(len(failed))
	newStatus.FailedNodes = failed
	if job.Spec.CompletionPolicy.Type != appsv1beta1.Never && (newStatus.Desired-newStatus.Succeeded-newStatus.Failed) == 0 {
		newStatus.CompletionTime = &now
	}

	newStatus.Message = formatStatusMessage(&newStatus)
	sort.Strings(newStatus.FailedNodes)
	return &newStatus, nil
}

func searchImage(job *appsv1beta1.ImagePullJob, imageSpec appsv1beta1.ImageSpec, imageTag string) int64 {
	for _, tagSpec := range imageSpec.Tags {
		if tagSpec.Tag != imageTag {
			continue
		}
		var foundOwner bool
		for _, ref := range tagSpec.OwnerReferences {
			if ref.UID == job.UID {
				foundOwner = true
				break
			}
		}
		if !foundOwner {
			break
		}
		return tagSpec.Version
	}
	return -1
}

func (r *ReconcileImagePullJob) checkNamespaceExists(nsName string) error {
	namespace := v1.Namespace{}
	return r.Get(context.TODO(), types.NamespacedName{Name: nsName}, &namespace)
}

// addProtectionFinalizer ensure the GC of secrets in kruise-daemon-config ns
func (r *ReconcileImagePullJob) addProtectionFinalizer(job *appsv1beta1.ImagePullJob) error {
	if job.DeletionTimestamp != nil {
		return nil
	}
	if controllerutil.ContainsFinalizer(job, defaults.ProtectionFinalizer) {
		return nil
	}
	job.Finalizers = append(job.Finalizers, defaults.ProtectionFinalizer)
	return r.Update(context.TODO(), job)
}

// finalize also ensure the GC of secrets in kruise-daemon-config ns
func (r *ReconcileImagePullJob) finalize(job *appsv1beta1.ImagePullJob) error {
	if !controllerutil.ContainsFinalizer(job, defaults.ProtectionFinalizer) {
		return nil
	}
	controllerutil.RemoveFinalizer(job, defaults.ProtectionFinalizer)
	return r.Update(context.TODO(), job)
}
