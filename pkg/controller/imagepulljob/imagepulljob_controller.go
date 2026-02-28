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
	"reflect"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
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

	// SecretAnnotationSourceSecretKey stores the reference (namespace/name) to the source secret
	// that was used to create the corresponding secret in kruise-daemon-config namespace.
	SecretAnnotationSourceSecretKey = "imagepulljobs.kruise.io/source-key"

	// Deprecated: New code should not include this annotation for secrets
	SecretLabelSourceSecretUID = "imagepulljobs.kruise.io/source-uid"

	// SecretAnnotationReferenceJobs records the namespace/name identifiers of ImagePullJobs that reference
	// this secret. When this annotation is empty, the secret should be deleted.
	SecretAnnotationReferenceJobs = "imagepulljobs.kruise.io/references"

	// newly created Secrets will have imagepulljobs.kruise.io/mode=new added for subsequent nodeImage cleanup logic.
	SecretAnnotationMode = "imagepulljobs.kruise.io/mode"
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
		Client:                   utilclient.NewClientFromManager(mgr, "imagepulljob-controller"),
		scheme:                   mgr.GetScheme(),
		clock:                    clock.RealClock{},
		generateRandomStringFunc: defaultGenerateRandomString,
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
	scheme                   *runtime.Scheme
	clock                    clock.Clock
	generateRandomStringFunc func() string
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=imagepulljobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=imagepulljobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=imagepulljobs/finalizers,verbs=update

// Reconcile reads that state of the cluster for a ImagePullJob object and makes changes based on the state read
// and what is in the ImagePullJob.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
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

	if job.DeletionTimestamp != nil {
		// ensure the GC of secrets and remove protection finalizer
		return reconcile.Result{}, r.finalize(job)
	}

	// The Job has been finished
	if job.Status.CompletionTime != nil {
		// ensure the GC of secrets and remove protection finalizer
		if err = r.finalize(job); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to remove finalizer: %v", err)
		}

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
	}

	// add protection finalizer to ensure the GC of secrets
	if err = r.addProtectionFinalizer(job); err != nil {
		return reconcile.Result{}, err
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
	secrets, err := r.syncJobPullSecrets(job)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync secrets: %v", err)
	}

	// Calculate the new status for this job
	newStatus, notSyncedNodeImages, err := r.calculateStatus(job, nodeImages, secrets)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to calculate status: %v", err)
	}

	// Sync image to more NodeImages
	if err = r.syncNodeImages(job, newStatus, notSyncedNodeImages, secrets); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync NodeImages: %v", err)
	}

	if !util.IsJSONObjectEqual(&job.Status, newStatus) {
		job.Status = *newStatus
		if err = r.Status().Update(context.TODO(), job); err != nil {
			return reconcile.Result{}, fmt.Errorf("update ImagePullJob status error: %v", err)
		}
		klog.V(3).InfoS("update imagepulljob status success", "imagepulljob", klog.KObj(job), "status", util.DumpJSON(job.Status))
		resourceVersionExpectations.Expect(job)
		return reconcile.Result{}, nil
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

// For security reasons, kruise-daemon does not have cluster-level secret permissions,
// so it is necessary to synchronize the pullsecrets of imagepulljob to the kruise-daemon-config namespace
// to facilitate kruise-daemon to fetch them.
func (r *ReconcileImagePullJob) syncJobPullSecrets(job *appsv1beta1.ImagePullJob) ([]appsv1beta1.ReferenceObject, error) {
	var secretRefs []appsv1beta1.ReferenceObject
	// If it's in kruise-daemon-config namespace, no need to go through the sync logic, return directly
	if job.Namespace == util.GetKruiseDaemonConfigNamespace() {
		for _, name := range job.Spec.PullSecrets {
			secretRefs = append(secretRefs, appsv1beta1.ReferenceObject{Namespace: job.Namespace, Name: name})
		}
		return secretRefs, nil
	}
	// namespace kruise-daemon-config must be active
	if err := r.namespaceIsActive(util.GetKruiseDaemonConfigNamespace()); err != nil {
		return nil, err
	}
	pullSecrets, releasedSecrets, err := r.classifyPullSecretsForJob(job)
	if err != nil {
		return nil, err
	}
	if err = r.releaseImagePullJobSecrets(releasedSecrets, job); err != nil {
		return nil, err
	}
	return r.claimImagePullJobSecrets(job, pullSecrets)
}

func (r *ReconcileImagePullJob) syncNodeImages(job *appsv1beta1.ImagePullJob, newStatus *appsv1beta1.ImagePullJobStatus, notSyncedNodeImages []string, secrets []appsv1beta1.ReferenceObject) error {
	if len(notSyncedNodeImages) == 0 {
		return nil
	}

	klog.V(3).InfoS("Start to sync NodeImages", "imagePullJob", klog.KObj(job), "activePullingCount", newStatus.Active)
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
	imageName, imageTag, _ := daemonutil.NormalizeImageRefToNameTag(job.Spec.Image)
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

			var found bool
			for i := range imageSpec.Tags {
				tagSpec := &imageSpec.Tags[i]
				if tagSpec.Tag != imageTag {
					continue
				}
				if util.ContainsObjectRef(tagSpec.OwnerReferences, *ownerRef) {
					skip = true
					return nil
				}
				// increase version to start a new round of image downloads
				tagSpec.Version++
				// merge owner reference
				tagSpec.OwnerReferences = append(tagSpec.OwnerReferences, *ownerRef)
				tagSpec.CreatedAt = &now
				tagSpec.ImagePullPolicy = job.Spec.ImagePullPolicy
				found = true
				break
			}
			if !found {
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
			utilimagejob.SortSpecImageTagsV1beta1(&imageSpec)
			nodeImage.Spec.Images[imageName] = imageSpec

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

// classifyPullSecretsForJob categorizes secrets based on the requirements of the ImagePullJob
// This function iterates through all secrets in the kruise-daemon-config namespace and classifies them into secrets to be used and secrets to be released
//
// Parameters:
//
//	job - The ImagePullJob object to be processed
//
// Return values:
//
//	map[appsv1beta1.ReferenceObject]*v1.Secret - First return value is a pull secrets mapping table, where the key is the reference object (namespace/name),
//	                                         and the value is the corresponding secret object (nil if it doesn't exist)
//	[]*v1.Secret - Second return value is a list of secrets to be released, these are secrets that were previously referenced by this job but are no longer needed
func (r *ReconcileImagePullJob) classifyPullSecretsForJob(job *appsv1beta1.ImagePullJob) (map[appsv1beta1.ReferenceObject]*v1.Secret, []*v1.Secret, error) {
	// list all kruise-daemon-config ns secrets
	options := client.ListOptions{
		Namespace: util.GetKruiseDaemonConfigNamespace(),
	}
	// all kruise-daemon-config ns secrets
	secretList := &v1.SecretList{}
	if err := r.List(context.TODO(), secretList, &options, utilclient.DisableDeepCopy); err != nil {
		klog.ErrorS(err, "list kruise-daemon-config ns secrets failed", "imagePullJob", klog.KObj(job))
		return nil, nil, err
	}

	// Import SecretList Items into a new array and sort by name
	syncedSecrets := make([]v1.Secret, len(secretList.Items))
	copy(syncedSecrets, secretList.Items)
	sort.Slice(syncedSecrets, func(i, j int) bool {
		return syncedSecrets[i].Name < syncedSecrets[j].Name
	})

	// jobKey is imagePullJob namespace/name
	jobKey := jobAsReferenceObject(job)
	// pull secret name -> v1.secret
	pullSecrets := map[appsv1beta1.ReferenceObject]*v1.Secret{}

	// Only sync pullSecrets when the job is in running state
	if job.DeletionTimestamp.IsZero() && job.Status.CompletionTime == nil {
		for _, name := range job.Spec.PullSecrets {
			pullSecrets[appsv1beta1.ReferenceObject{Namespace: job.Namespace, Name: name}] = nil
		}
	}
	// imagePullJob previously configured pullSecret, but later removed it during updates, so it needs to be released
	var releasedSecrets []*v1.Secret
	for i := range syncedSecrets {
		secret := &syncedSecrets[i]
		if secret.DeletionTimestamp != nil {
			continue
		}
		// get secrets's reference imagePullJobs
		referJobs := getReferencingJobsFromSecret(secret)
		// If the job has completed or is being deleted, need to release secrets
		if job.DeletionTimestamp != nil || job.Status.CompletionTime != nil {
			if referJobs.Has(jobKey) {
				releasedSecrets = append(releasedSecrets, secret)
			}
			continue
		}

		sourceSecretRef := getSourceSecret(secret)
		// There are three scenarios for kruise-daemon-config secrets:
		// 1. No kruise-daemon-config secrets match pullSecrets, thus requiring creation of a new kruise-daemon-config secret
		// 2. A kruise-daemon-config secret exists that matches pullSecrets and has matching referJobs. This has the highest priority and should be used preferentially
		// 3. A kruise-daemon-config secret exists that matches pullSecrets but has non-matching referJobs. No new kruise-daemon-config secret needs to be created,
		//    but the secret annotations referJobs need to be updated

		// If the source secret matches the pullSecrets, it indicates that the imagePullJob can also use the secret to pull images
		// Therefore, there is no need to duplicate the creation of a new kruise-daemon-config secret.
		if obj, ok := pullSecrets[sourceSecretRef]; ok {
			if obj == nil {
				pullSecrets[sourceSecretRef] = secret
			} else if referJobs.Has(jobKey) {
				pullSecrets[sourceSecretRef] = secret
			}
			continue
		}
		// Code execution reaches here indicates: secret does not match pullSecrets, but secret referenceJobs includes this imagePullJob
		// This usually occurs when updating imagePullJob to remove the pullSecret configuration, so it needs to be released
		if referJobs.Has(jobKey) {
			releasedSecrets = append(releasedSecrets, secret)
		}
	}
	return pullSecrets, releasedSecrets, nil
}

func (r *ReconcileImagePullJob) releaseImagePullJobSecrets(secrets []*v1.Secret, job *appsv1beta1.ImagePullJob) error {
	if len(secrets) == 0 {
		return nil
	}

	jobKey := jobAsReferenceObject(job)
	for _, secret := range secrets {
		// If the secret is already being deleted, no need to process it
		if secret == nil || !secret.DeletionTimestamp.IsZero() {
			continue
		}
		referJobs := getReferencingJobsFromSecret(secret)
		referJobs.Delete(jobKey)
		// if reference jobs is empty, directly delete secret
		if referJobs.Len() == 0 {
			if err := client.IgnoreNotFound(r.Delete(context.TODO(), secret)); err != nil {
				klog.ErrorS(err, "delete secret failed", "secret", klog.KObj(secret))
				return err
			}
			klog.InfoS("delete secret success", "imagePullJob", klog.KObj(job), "secret", klog.KObj(secret))
			continue
		}

		// Remove the reference to this job from the secret. We use Update instead of
		// Patch to ensure we don't delete secrets that are still being referenced,
		// since a secret might be newly referenced in the current reconciliation loop.
		clone := secret.DeepCopy()
		if clone.Annotations == nil {
			clone.Annotations = map[string]string{}
		}
		var referJob []string
		for _, ref := range referJobs.UnsortedList() {
			referJob = append(referJob, ref.String())
		}
		sort.Strings(referJob)
		value := strings.Join(referJob, ",")
		clone.Annotations[SecretAnnotationReferenceJobs] = value
		if err := client.IgnoreNotFound(r.Update(context.TODO(), clone)); err != nil {
			klog.ErrorS(err, "update kruise-daemon-config secret failed", "secret", klog.KObj(clone))
			return err
		}
		klog.InfoS("update kruise-daemon-config secret success", "imagePullJob", klog.KObj(job),
			"secret", klog.KObj(clone), "annotation value", value)
	}
	return nil
}

func (r *ReconcileImagePullJob) claimImagePullJobSecrets(job *appsv1beta1.ImagePullJob, pullSecrets map[appsv1beta1.ReferenceObject]*v1.Secret) ([]appsv1beta1.ReferenceObject, error) {
	var secretRefs []appsv1beta1.ReferenceObject
	jobKey := jobAsReferenceObject(job)

	for sourceRef, syncedSecret := range pullSecrets {
		sourceSecret := &v1.Secret{}
		if err := r.Get(context.TODO(), client.ObjectKey{Namespace: sourceRef.Namespace, Name: sourceRef.Name}, sourceSecret); err != nil {
			if errors.IsNotFound(err) {
				klog.Warningf("imagePullJob(%s/%s) pullSecret(%s) not found", job.Namespace, job.Name, sourceRef.Name)
				continue
			}
			return nil, err
		}
		// if secret is nil, then create
		if syncedSecret == nil {
			syncedSecret = r.generateSyncedSecret(sourceSecret, jobKey)
			scaleExpectations.ExpectScale(jobKey.String(), expectations.Create, syncedSecret.Name)
			if err := r.Create(context.TODO(), syncedSecret); err != nil {
				klog.ErrorS(err, "create kruise-daemon-config secret failed", "imagePullJob", klog.KObj(job), "secret", klog.KObj(syncedSecret))
				scaleExpectations.ObserveScale(jobKey.String(), expectations.Create, syncedSecret.Name)
				return nil, err
			}
			klog.InfoS("create kruise-daemon-config secret success", "imagePullJob", klog.KObj(job), "secret", klog.KObj(syncedSecret))
			secretRefs = append(secretRefs, appsv1beta1.ReferenceObject{
				Namespace: syncedSecret.Namespace,
				Name:      syncedSecret.Name,
				// Add this flag to newly created secrets to distinguish them from old secrets.
				// In NodeImage, pullSecrets will only clean up the old secrets.
				// The new secrets will be cleaned up along with the imageSpec.
				Mode: appsv1beta1.ReferenceObjectModeBatch,
			})
			continue
		}

		secretRefs = append(secretRefs, appsv1beta1.ReferenceObject{
			Namespace: syncedSecret.Namespace,
			Name:      syncedSecret.Name,
			Mode:      syncedSecret.Annotations[SecretAnnotationMode],
		})
		clone := syncedSecret.DeepCopy()
		clone.Data = sourceSecret.Data
		clone.StringData = sourceSecret.StringData
		referJobs := getReferencingJobsFromSecret(syncedSecret)
		referJobs.Insert(jobKey)
		var referJob []string
		for _, ref := range referJobs.UnsortedList() {
			referJob = append(referJob, ref.String())
		}
		sort.Strings(referJob)
		value := strings.Join(referJob, ",")
		clone.Annotations[SecretAnnotationReferenceJobs] = value
		if reflect.DeepEqual(clone.Data, syncedSecret.Data) && reflect.DeepEqual(clone.StringData, syncedSecret.StringData) &&
			reflect.DeepEqual(clone.Annotations, syncedSecret.Annotations) {
			klog.V(5).InfoS("kruise-daemon-config secret configuration has not changed, no update needed", "imagePullJob", klog.KObj(job), "secret", klog.KObj(syncedSecret))
			continue
		}
		if err := r.Update(context.TODO(), clone); err != nil {
			klog.ErrorS(err, "update kruise-daemon-config secret failed", "imagePullJob", klog.KObj(job), "secret", klog.KObj(syncedSecret))
			return nil, err
		}
		klog.InfoS("update kruise-daemon-config secret success", "imagePullJob", klog.KObj(job), "secret", klog.KObj(syncedSecret), "reference annotation value", value)
	}
	return secretRefs, nil
}

func (r *ReconcileImagePullJob) calculateStatus(job *appsv1beta1.ImagePullJob, nodeImages []*appsv1beta1.NodeImage, secrets []appsv1beta1.ReferenceObject) (*appsv1beta1.ImagePullJobStatus, []string, error) {
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
		return nil, nil, fmt.Errorf("invalid image %s: %v", job.Spec.Image, err)
	}

	var pulling, succeeded, failed []string
	notSynced := sets.NewString()
	for _, nodeImage := range nodeImages {
		var tagVersion int64 = -1
		var secretSynced bool = true
		if imageSpec, ok := nodeImage.Spec.Images[imageName]; ok {
			for _, secret := range secrets {
				if !containsObject(imageSpec.PullSecrets, secret) {
					secretSynced = false
					break
				}
			}
			// In scenarios where secrets are not synchronized, do not skip the calculation of succeeded counts directly
			if !secretSynced {
				notSynced.Insert(nodeImage.Name)
			}

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
				tagVersion = tagSpec.Version
			}
		}

		if tagVersion < 0 {
			notSynced.Insert(nodeImage.Name)
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
			failed = append(failed, notSynced.List()...)
			newStatus.Failed = int32(len(failed))
			newStatus.FailedNodes = failed
			newStatus.Message = "job exceeds activeDeadlineSeconds"
			return &newStatus, nil, nil
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
	return &newStatus, notSynced.List(), nil
}

func (r *ReconcileImagePullJob) namespaceIsActive(name string) error {
	namespace := v1.Namespace{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: name}, &namespace)
	if err != nil {
		return err
	}
	if !namespace.DeletionTimestamp.IsZero() {
		return fmt.Errorf("namespace(%s) is being deleted", name)
	}
	return nil
}

// addProtectionFinalizer ensure the GC of secrets in kruise-daemon-config ns
func (r *ReconcileImagePullJob) addProtectionFinalizer(job *appsv1beta1.ImagePullJob) error {
	if controllerutil.ContainsFinalizer(job, defaults.ProtectionFinalizer) {
		return nil
	}
	job.Finalizers = append(job.Finalizers, defaults.ProtectionFinalizer)
	if err := r.Update(context.TODO(), job); err != nil {
		klog.ErrorS(err, "add protection finalizer failed", "imagepulljob", klog.KObj(job))
		return err
	}
	klog.InfoS("add protection finalizer success", "imagepulljob", klog.KObj(job))
	return nil
}

// finalize also ensure the GC of secrets in kruise-daemon-config ns
func (r *ReconcileImagePullJob) finalize(job *appsv1beta1.ImagePullJob) error {
	if !controllerutil.ContainsFinalizer(job, defaults.ProtectionFinalizer) {
		return nil
	}
	if _, err := r.syncJobPullSecrets(job); err != nil {
		return err
	}
	controllerutil.RemoveFinalizer(job, defaults.ProtectionFinalizer)
	if err := r.Update(context.TODO(), job); err != nil {
		klog.ErrorS(err, "remove protection finalizer failed", "imagepulljob", klog.KObj(job))
		return err
	}
	klog.InfoS("remove protection finalizer success", "imagepulljob", klog.KObj(job))
	return nil
}

func (r *ReconcileImagePullJob) generateSyncedSecret(secret *v1.Secret, ref appsv1beta1.ReferenceObject) *v1.Secret {
	syncedSecret := secret.DeepCopy()
	syncedSecret.ObjectMeta = metav1.ObjectMeta{
		Namespace:   util.GetKruiseDaemonConfigNamespace(),
		Name:        fmt.Sprintf("%s-%s", secret.Name, r.generateRandomStringFunc()),
		Labels:      secret.Labels,
		Annotations: secret.Annotations,
	}
	if syncedSecret.Annotations == nil {
		syncedSecret.Annotations = map[string]string{}
	}
	syncedSecret.Annotations[SecretAnnotationReferenceJobs] = ref.String()
	syncedSecret.Annotations[SecretAnnotationSourceSecretKey] = fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
	syncedSecret.Annotations[SecretAnnotationMode] = appsv1beta1.ReferenceObjectModeBatch
	return syncedSecret
}
