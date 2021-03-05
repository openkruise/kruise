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
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	nodeimagesutil "github.com/openkruise/kruise/pkg/util/nodeimages"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func init() {
	flag.IntVar(&concurrentReconciles, "imagepulljob-workers", concurrentReconciles, "Max concurrent workers for ImagePullJob controller.")
}

var (
	concurrentReconciles        = 3
	controllerKind              = appsv1alpha1.SchemeGroupVersion.WithKind("ImagePullJob")
	resourceVersionExpectations = expectations.NewResourceVersionExpectation()
)

const (
	defaultParallelism = 1
	minRequeueTime     = time.Second
)

// Add creates a new ImagePullJob Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) || !utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileImagePullJob {
	return &ReconcileImagePullJob{
		Client: util.NewClientFromManager(mgr, "imagepulljob-controller"),
		scheme: mgr.GetScheme(),
		clock:  clock.RealClock{},
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileImagePullJob) error {
	// Create a new controller
	c, err := controller.New("imagepulljob-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	// Watch for changes to ImagePullJob
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.ImagePullJob{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for nodeimage update to get image pull status
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.NodeImage{}}, &nodeImageEventHandler{Reader: mgr.GetCache()})
	if err != nil {
		return err
	}

	// Watch for pod for jobs that have pod selector
	err = c.Watch(&source.Kind{Type: &v1.Pod{}}, &podEventHandler{Reader: mgr.GetCache()})
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

// Reconcile reads that state of the cluster for a ImagePullJob object and makes changes based on the state read
// and what is in the ImagePullJob.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
func (r *ReconcileImagePullJob) Reconcile(request reconcile.Request) (res reconcile.Result, err error) {
	start := time.Now()
	klog.V(5).Infof("Starting to process ImagePullJob %v", request.NamespacedName)
	defer func() {
		if err != nil {
			klog.Warningf("Failed to process ImagePullJob %v, elapsedTime %v, error: %v", request.NamespacedName, time.Since(start), err)
		} else if res.RequeueAfter > 0 {
			klog.Infof("Finish to process ImagePullJob %v, elapsedTime %v, RetryAfter %v", request.NamespacedName, time.Since(start), res.RequeueAfter)
		} else {
			klog.Infof("Finish to process ImagePullJob %v, elapsedTime %v", request.NamespacedName, time.Since(start))
		}
	}()

	// Fetch the ImagePullJob instance
	job := &appsv1alpha1.ImagePullJob{}
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

	if job.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// The Job has been finished
	if job.Status.CompletionTime != nil {
		var leftTime time.Duration
		if job.Spec.CompletionPolicy.TTLSecondsAfterFinished != nil {
			leftTime = time.Duration(*job.Spec.CompletionPolicy.TTLSecondsAfterFinished)*time.Second - time.Since(job.Status.CompletionTime.Time)
			if leftTime <= 0 {
				klog.Infof("Deleting ImagePullJob %s/%s for ttlSecondsAfterFinished", job.Namespace, job.Name)
				if err = r.Delete(context.TODO(), job); err != nil {
					return reconcile.Result{}, fmt.Errorf("delete job error: %v", err)
				}
				return reconcile.Result{}, nil
			}
		}
		return reconcile.Result{RequeueAfter: leftTime}, nil
	}

	// Get all NodeImage related to this ImagePullJob
	nodeImages, err := nodeimagesutil.GetNodeImagesForJob(r.Client, job)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get NodeImages: %v", err)
	}

	// If resourceVersion expectations have not satisfied yet, just skip this reconcile
	for _, nodeImage := range nodeImages {
		if isSatisfied, unsatisfiedDuration := resourceVersionExpectations.IsSatisfied(nodeImage); !isSatisfied {
			if unsatisfiedDuration >= expectations.ExpectationTimeout {
				klog.Warningf("Expectation unsatisfied overtime for %v, wait for NodeImage %v updating, timeout=%v", request.String(), nodeImage.Name, unsatisfiedDuration)
				return reconcile.Result{}, nil
			}
			klog.V(4).Infof("Not satisfied resourceVersion for %v, wait for NodeImage %v updating", request.String(), nodeImage.Name)
			return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
		}
	}

	// Calculate the new status for this job
	newStatus, notSyncedNodeImages, err := r.calculateStatus(job, nodeImages)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to calculate status: %v", err)
	}

	// Sync image to more NodeImages
	if err = r.syncNodeImages(job, newStatus, notSyncedNodeImages); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to sync NodeImages: %v", err)
	}

	if !reflect.DeepEqual(job.Status, newStatus) {
		job.Status = *newStatus
		if err = r.Status().Update(context.TODO(), job); err != nil {
			return reconcile.Result{}, fmt.Errorf("update ImagePullJob status error: %v", err)
		}
		return reconcile.Result{}, nil
	}

	if job.Spec.CompletionPolicy.Type != appsv1alpha1.Never && job.Spec.CompletionPolicy.ActiveDeadlineSeconds != nil {
		leftTime := time.Duration(*job.Spec.CompletionPolicy.ActiveDeadlineSeconds)*time.Second - time.Since(newStatus.StartTime.Time)
		if leftTime < minRequeueTime {
			leftTime = minRequeueTime
		}
		return reconcile.Result{RequeueAfter: leftTime}, nil
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileImagePullJob) syncNodeImages(job *appsv1alpha1.ImagePullJob, newStatus *appsv1alpha1.ImagePullJobStatus, notSyncedNodeImages []string) error {
	if len(notSyncedNodeImages) == 0 {
		return nil
	}

	parallelismLimit := defaultParallelism
	if job.Spec.Parallelism != nil {
		parallelismLimit = job.Spec.Parallelism.IntValue()
	}
	parallelism := parallelismLimit - int(newStatus.Active)
	if parallelism <= 0 {
		klog.V(3).Infof("Find ImagePullJob %s/%s have active pulling %d >= parallelism %d, so skip to sync the left %d NodeImages",
			job.Namespace, job.Name, newStatus.Active, parallelismLimit, len(notSyncedNodeImages))
		return nil
	}
	if len(notSyncedNodeImages) < parallelism {
		parallelism = len(notSyncedNodeImages)
	}

	ownerRef := &v1.ObjectReference{
		APIVersion: controllerKind.GroupVersion().String(),
		Kind:       controllerKind.Kind,
		Name:       job.Name,
		Namespace:  job.Namespace,
		UID:        job.UID,
	}
	var secrets []appsv1alpha1.ReferenceObject
	for _, secret := range job.Spec.PullSecrets {
		secrets = append(secrets,
			appsv1alpha1.ReferenceObject{
				Namespace: job.Namespace,
				Name:      secret,
			})
	}

	pullPolicy := appsv1alpha1.ImageTagPullPolicy{}
	if job.Spec.PullPolicy != nil {
		pullPolicy.BackoffLimit = job.Spec.PullPolicy.BackoffLimit
		pullPolicy.TimeoutSeconds = job.Spec.PullPolicy.TimeoutSeconds
	}
	if job.Spec.CompletionPolicy.Type == appsv1alpha1.Never {
		pullPolicy.TTLSecondsAfterFinished = getTTLSecondsForNever()
		pullPolicy.ActiveDeadlineSeconds = getActiveDeadlineSecondsForNever()
	} else {
		pullPolicy.TTLSecondsAfterFinished = getTTLSecondsForAlways(job)
		pullPolicy.ActiveDeadlineSeconds = job.Spec.CompletionPolicy.ActiveDeadlineSeconds
	}

	imageName, imageTag, _ := daemonutil.NormalizeImageRefToNameTag(job.Spec.Image)
	for i := 0; i < parallelism; i++ {
		var skip bool
		updateErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			nodeImage := appsv1alpha1.NodeImage{}
			if err := r.Get(context.TODO(), types.NamespacedName{Name: notSyncedNodeImages[i]}, &nodeImage); err != nil {
				return err
			}
			if nodeImage.Spec.Images == nil {
				nodeImage.Spec.Images = make(map[string]appsv1alpha1.ImageSpec, 1)
			}
			imageSpec := nodeImage.Spec.Images[imageName]

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
				if containsObjectRef(tagSpec.OwnerReferences, *ownerRef) {
					skip = true
					return nil
				}
				// increase version to start a new round of image downloads
				tagSpec.Version++
				// merge owner reference
				tagSpec.OwnerReferences = append(tagSpec.OwnerReferences, *ownerRef)
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

				imageSpec.Tags = append(imageSpec.Tags, appsv1alpha1.ImageTagSpec{
					Tag:             imageTag,
					Version:         foundVersion + 1,
					PullPolicy:      &pullPolicy,
					OwnerReferences: []v1.ObjectReference{*ownerRef},
				})
			}
			nodeimagesutil.SortSpecImageTags(&imageSpec)
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
			klog.V(4).Infof("ImagePullJob %s/%s find %s already synced in NodeImage %s", job.Namespace, job.Name, job.Spec.Image, notSyncedNodeImages[i])
			continue
		}
		klog.V(3).Infof("ImagePullJob %s/%s has synced %s into NodeImage %s", job.Namespace, job.Name, job.Spec.Image, notSyncedNodeImages[i])
	}
	return nil
}

func (r *ReconcileImagePullJob) calculateStatus(job *appsv1alpha1.ImagePullJob, nodeImages []*appsv1alpha1.NodeImage) (*appsv1alpha1.ImagePullJobStatus, []string, error) {
	newStatus := appsv1alpha1.ImagePullJobStatus{
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

	var notSynced, pulling, succeeded, failed []string
	for _, nodeImage := range nodeImages {
		var tagVersion int64 = -1
		if imageSpec, ok := nodeImage.Spec.Images[imageName]; ok {
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
			notSynced = append(notSynced, nodeImage.Name)
			continue
		}

		imageStatus, ok := nodeImage.Status.ImageStatuses[imageName]
		if !ok {
			pulling = append(pulling, nodeImage.Name)
		}

		for _, tagStatus := range imageStatus.Tags {
			if tagStatus.Tag != imageTag {
				continue
			}
			if tagStatus.Version != tagVersion {
				pulling = append(pulling, nodeImage.Name)
				break
			}
			switch tagStatus.Phase {
			case appsv1alpha1.ImagePhaseSucceeded:
				succeeded = append(succeeded, nodeImage.Name)
			case appsv1alpha1.ImagePhaseFailed:
				failed = append(failed, nodeImage.Name)
			default:
				pulling = append(pulling, nodeImage.Name)
			}
			break
		}
	}

	if job.Spec.CompletionPolicy.Type != appsv1alpha1.Never && job.Spec.CompletionPolicy.ActiveDeadlineSeconds != nil && int(newStatus.Desired) != len(succeeded)+len(failed) {
		if time.Duration(*job.Spec.CompletionPolicy.ActiveDeadlineSeconds)*time.Second <= time.Since(newStatus.StartTime.Time) {
			newStatus.CompletionTime = &now
			newStatus.Succeeded = int32(len(succeeded))
			failed = append(failed, pulling...)
			failed = append(failed, notSynced...)
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
	if job.Spec.CompletionPolicy.Type != appsv1alpha1.Never && (newStatus.Desired-newStatus.Succeeded-newStatus.Failed) == 0 {
		newStatus.CompletionTime = &now
	}

	newStatus.Message = formatStatusMessage(&newStatus)
	sort.Strings(newStatus.FailedNodes)
	return &newStatus, notSynced, nil
}
