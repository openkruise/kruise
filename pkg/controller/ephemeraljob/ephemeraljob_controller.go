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

package ephemeraljob

import (
	"context"
	"fmt"
	"sort"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/controller/ephemeraljob/econtainer"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/expectations"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	concurrentReconciles = 10
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("EphemeralJob")
	defaultParallelism   = 1
	scaleExpectations    = expectations.NewScaleExpectations()
)

const EphemeralContainerFinalizer = "apps.kruise.io/ephemeralcontainers-cleanup"

// Add creates a new ImagePullJob Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}

	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileEphemeralJob {
	return &ReconcileEphemeralJob{
		Client:        utilclient.NewClientFromManager(mgr, "ephemeraljob-controller"),
		scheme:        mgr.GetScheme(),
		eventRecorder: mgr.GetEventRecorderFor("ephemeraljob-controller"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileEphemeralJob) error {
	// Create a new controller
	c, err := controller.New("ephemeraljob-controller", mgr, controller.Options{Reconciler: r,
		MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout()})
	if err != nil {
		return err
	}

	// Watch for changes to EphemeralJob
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.EphemeralJob{}}, &ejobHandler{mgr.GetCache()})
	if err != nil {
		return err
	}
	// Watch for changes to Pod
	err = c.Watch(&source.Kind{Type: &v1.Pod{}}, &podHandler{mgr.GetCache()})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileEphemeralJob{}

// ReconcileEphemeralJob reconciles a ImagePullJob object
type ReconcileEphemeralJob struct {
	client.Client
	scheme        *runtime.Scheme
	eventRecorder record.EventRecorder
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=ephemeraljobs,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=ephemeraljobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/ephemeralcontainers,verbs=get;update;patch

// Reconcile reads that state of the cluster for a EphemeralJob object and makes changes based on the state read
// and what is in the EphemeralJob.Spec
func (r *ReconcileEphemeralJob) Reconcile(context context.Context, request reconcile.Request) (res reconcile.Result, err error) {
	start := time.Now()
	klog.V(5).Infof("Starting to process EphemeralJob %v", request.Name)
	defer func() {
		if err != nil {
			klog.Warningf("Failed to process EphemeralJob %v err %v, elapsedTime %v", request.Name, time.Since(start), err)
		} else if res.RequeueAfter > 0 {
			klog.Infof("Finish to process EphemeralJob %v, elapsedTime %v, RetryAfter %v", request.Name, time.Since(start), res.RequeueAfter)
		} else {
			klog.Infof("Finish to process EphemeralJob %v, elapsedTime %v", request.Name, time.Since(start))
		}
	}()

	job := &appsv1alpha1.EphemeralJob{}
	err = r.Get(context, request.NamespacedName, job)

	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			scaleExpectations.DeleteExpectations(request.String())
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		klog.Errorf("Failed to get ephemeral job %s,", job.Name)
		return reconcile.Result{}, err
	}

	if job.DeletionTimestamp != nil {
		if err := r.removeEphemeralContainers(job); err != nil {
			return reconcile.Result{}, err
		}
		job.Finalizers = deleteEphemeralContainerFinalizer(job.Finalizers, EphemeralContainerFinalizer)
		return reconcile.Result{}, r.Update(context, job)
	}

	if !hasEphemeralContainerFinalizer(job.Finalizers) {
		job.Finalizers = append(job.Finalizers, EphemeralContainerFinalizer)
		if err := r.Update(context, job); err != nil {
			return reconcile.Result{}, err
		}
	}

	// The Job has been finished
	if job.Status.CompletionTime != nil {
		var leftTime time.Duration
		if job.Spec.TTLSecondsAfterFinished == nil {
			defaultTTl := int32(1800)
			job.Spec.TTLSecondsAfterFinished = &defaultTTl
		}

		leftTime = time.Duration(*job.Spec.TTLSecondsAfterFinished)*time.Second - time.Since(job.Status.CompletionTime.Time)
		if leftTime <= 0 {
			klog.Infof("Deleting EphemeralJob %s/%s for ttlSecondsAfterFinished", job.Namespace, job.Name)
			if err = r.Delete(context, job); err != nil {
				return reconcile.Result{}, fmt.Errorf("Delete ephemeral job error: %v. ", err)
			}
			scaleExpectations.DeleteExpectations(request.String())
			return reconcile.Result{}, nil
		}

		return reconcile.Result{RequeueAfter: leftTime}, nil
	}

	// requeueAfter is zero, meaning no requeue
	requeueAfter := time.Duration(0)
	// set the job startTime
	if job.Status.StartTime == nil {
		now := metav1.Now()
		job.Status.StartTime = &now
	}
	if job.Spec.ActiveDeadlineSeconds != nil {
		requeueAfter = time.Duration(*job.Spec.ActiveDeadlineSeconds)*time.Second - time.Since(job.Status.StartTime.Time)
		if requeueAfter < 0 {
			requeueAfter = 0
		}
		klog.Infof("Job %s has ActiveDeadlineSeconds, will resync after %d seconds", job.Name, requeueAfter)
	}

	if scaleSatisfied, unsatisfiedDuration, scaleDirtyPods := scaleExpectations.SatisfiedExpectations(request.String()); !scaleSatisfied {
		if unsatisfiedDuration >= expectations.ExpectationTimeout {
			klog.Warningf("Expectation unsatisfied overtime for ejob %v, scaleDirtyPods=%v, overtime=%v", request.String(), scaleDirtyPods, unsatisfiedDuration)
			return reconcile.Result{}, nil
		}
		klog.Infof("Not satisfied scale for ejob %v, scaleDirtyPods=%v", request.String(), scaleDirtyPods)
		return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
	}

	targetPods, err := r.filterPods(job)
	if err != nil {
		klog.Errorf("Failed to get ephemeral job %s/%s related target pods: %v", job.Namespace, job.Name, err)
		return reconcile.Result{RequeueAfter: requeueAfter}, err
	}

	klog.V(5).Infof("filter target pods %d", len(targetPods))
	// calculate status
	if err := r.calculateStatus(job, targetPods); err != nil {
		klog.Errorf("Error calculate ephemeral job %s/%s status: %v", job.Namespace, job.Name, err)
		return reconcile.Result{}, err
	}
	klog.Infof("Sync calculate job %s status: (match: %d, success: %d, failed: %d, running: %d, waiting: %d)",
		job.Name, job.Status.Matches, job.Status.Succeeded, job.Status.Failed, job.Status.Running, job.Status.Waiting)

	if job.Status.Phase == appsv1alpha1.EphemeralJobPause {
		return reconcile.Result{RequeueAfter: requeueAfter}, r.updateJobStatus(job)
	}

	if err := r.syncTargetPods(job, targetPods); err != nil {
		return reconcile.Result{RequeueAfter: requeueAfter}, err
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, r.updateJobStatus(job)
}

func (r *ReconcileEphemeralJob) filterPods(job *appsv1alpha1.EphemeralJob) ([]*v1.Pod, error) {
	selector, err := util.ValidatedLabelSelectorAsSelector(job.Spec.Selector)
	if err != nil {
		return nil, err
	}

	opts := &client.ListOptions{
		Namespace:     job.Namespace,
		LabelSelector: selector,
	}

	podList := &v1.PodList{}
	if err := r.List(context.TODO(), podList, opts); err != nil {
		return nil, err
	}

	sort.Slice(podList.Items, func(i, j int) bool {
		if !podList.Items[i].CreationTimestamp.Equal(&podList.Items[j].CreationTimestamp) {
			return podList.Items[i].CreationTimestamp.Before(&podList.Items[j].CreationTimestamp)
		}
		return podList.Items[i].Name < podList.Items[j].Name
	})

	// Ignore inactive pods
	var targetPods []*v1.Pod
	for i := range podList.Items {
		if !kubecontroller.IsPodActive(&podList.Items[i]) {
			continue
		}

		if existDuplicatedEphemeralContainer(job, &podList.Items[i]) {
			continue
		}

		if job.Spec.Replicas == nil || len(targetPods) < int(*job.Spec.Replicas) {
			targetPods = append(targetPods, &podList.Items[i])
		}
	}

	return targetPods, nil
}

// filterInjectedPods will return pods which has injected ephemeral containers
func (r *ReconcileEphemeralJob) filterInjectedPods(job *appsv1alpha1.EphemeralJob) ([]*v1.Pod, error) {
	selector, err := util.ValidatedLabelSelectorAsSelector(job.Spec.Selector)
	if err != nil {
		return nil, err
	}

	opts := &client.ListOptions{
		Namespace:     job.Namespace,
		LabelSelector: selector,
	}

	podList := &v1.PodList{}
	if err := r.List(context.TODO(), podList, opts); err != nil {
		return nil, err
	}

	// Ignore inactive pods
	var targetPods []*v1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if !kubecontroller.IsPodActive(pod) {
			continue
		}
		if exists, owned := existEphemeralContainer(job, pod); exists {
			if owned {
				targetPods = append(targetPods, pod)
			} else {
				klog.Warningf("EphemeralJob %s/%s ignores Pod %s for it exists conflict ephemeral containers", job.Namespace, job.Name, pod)
			}
		}
	}

	return targetPods, nil
}

func (r *ReconcileEphemeralJob) syncTargetPods(job *appsv1alpha1.EphemeralJob, targetPods []*v1.Pod) error {
	toCreatePods, _, _ := getSyncPods(job, targetPods)
	if len(toCreatePods) == 0 {
		klog.Infoln("there is no target pod to attach")
		return nil
	}

	klog.Infof("Ready to create ephemeral containers in %d pods.", len(toCreatePods))
	parallelism := defaultParallelism
	if job.Spec.Parallelism != nil {
		parallelism = int(*job.Spec.Parallelism)
	}

	if len(toCreatePods) < parallelism {
		parallelism = len(toCreatePods)
	}

	diff := parallelism - int(job.Status.Running)
	if diff < 0 {
		klog.Infof("error sync phemeraljob %s/%s for parallisem %d less than running pod %d", job.Namespace, job.Name, parallelism, job.Status.Running)
		return nil
	}

	toCreatePods = toCreatePods[:diff]

	podsCreationChan := make(chan *v1.Pod, len(toCreatePods))
	for _, p := range toCreatePods {
		podsCreationChan <- p
	}

	control := econtainer.New(job)
	key := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}.String()
	_, err := clonesetutils.DoItSlowly(len(toCreatePods), kubecontroller.SlowStartInitialBatchSize, func() error {
		pod := <-podsCreationChan

		if exists, _ := existEphemeralContainer(job, pod); exists {
			return nil
		}

		klog.Infof("Creating ephemeral container in pod %s/%s", pod.Namespace, pod.Name)

		for _, podEphemeralContainerName := range getPodEphemeralContainers(pod, job) {
			scaleExpectations.ExpectScale(key, expectations.Create, podEphemeralContainerName)
		}
		if err := control.CreateEphemeralContainer(pod); err != nil {
			r.eventRecorder.Event(job, v1.EventTypeWarning, "CreateFailed",
				fmt.Sprintf("Failed to create ephemeral container for pod %s: %v", pod.Name, err))
			for _, podEphemeralContainerName := range getPodEphemeralContainers(pod, job) {
				scaleExpectations.ObserveScale(key, expectations.Create, podEphemeralContainerName)
			}
			return err
		}
		r.eventRecorder.Event(job, v1.EventTypeNormal, "CreateSuccessfully",
			fmt.Sprintf("create ephemeral container for pod %s successfully", pod.Name))
		return nil
	})

	return err
}

func (r *ReconcileEphemeralJob) calculateStatus(job *appsv1alpha1.EphemeralJob, targetPods []*v1.Pod) error {
	if job.Status.Conditions == nil {
		job.Status.Conditions = make([]appsv1alpha1.EphemeralJobCondition, 0)
		job.Status.Conditions = append(job.Status.Conditions, appsv1alpha1.EphemeralJobCondition{
			Type:               appsv1alpha1.EJobInitialized,
			Status:             v1.ConditionTrue,
			LastProbeTime:      metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             string(appsv1alpha1.EJobInitialized),
			Message:            "",
		})
	}
	job.Status.Matches = int32(len(targetPods))

	err := calculateEphemeralContainerStatus(job, targetPods)
	if err != nil {
		return err
	}

	var replicas int32
	if job.Spec.Replicas == nil {
		replicas = job.Status.Matches
	} else {
		replicas = *job.Spec.Replicas
	}

	if job.Status.Matches == 0 {
		job.Status.Phase = appsv1alpha1.EphemeralJobWaiting
		job.Status.Conditions = addConditions(job.Status.Conditions, appsv1alpha1.EJobMatchedEmpty, "MatchEmpty", "job match no pods")
	} else if job.Status.Succeeded == replicas && job.Status.Succeeded > 0 {
		job.Status.CompletionTime = timeNow()
		job.Status.Phase = appsv1alpha1.EphemeralJobSucceeded
		job.Status.Conditions = addConditions(job.Status.Conditions, appsv1alpha1.EJobSucceeded, "JobSucceeded", "job success to run all tasks")
	} else if job.Status.Running > 0 {
		job.Status.Phase = appsv1alpha1.EphemeralJobRunning
	} else if job.Status.Failed == replicas {
		job.Status.CompletionTime = timeNow()
		job.Status.Phase = appsv1alpha1.EphemeralJobFailed
		job.Status.Conditions = addConditions(job.Status.Conditions, appsv1alpha1.EJobFailed, "JobFailed", "job failed to run all tasks")
	} else if job.Status.Waiting == replicas {
		job.Status.Phase = appsv1alpha1.EphemeralJobWaiting
	} else {
		job.Status.Phase = appsv1alpha1.EphemeralJobUnknown
	}

	if job.Spec.Paused {
		job.Status.Phase = appsv1alpha1.EphemeralJobPause
	}

	if job.Status.Failed > 0 {
		job.Status.Conditions = addConditions(job.Status.Conditions,
			appsv1alpha1.EJobFailed, "CreateFailed",
			fmt.Sprintf("EphemeralJob %s/%s failed to create ephemeral container", job.Namespace, job.Name))
	}

	if (job.Status.Phase == appsv1alpha1.EphemeralJobWaiting || job.Status.Phase == appsv1alpha1.EphemeralJobUnknown ||
		job.Status.Phase == appsv1alpha1.EphemeralJobRunning) && pastActiveDeadline(job) {
		job.Status.CompletionTime = timeNow()
		job.Status.Phase = appsv1alpha1.EphemeralJobFailed
		job.Status.Conditions = addConditions(job.Status.Conditions, appsv1alpha1.EJobFailed, "DeadlineExceeded",
			fmt.Sprintf("EphemeralJob %s/%s was active longer than specified deadline", job.Namespace, job.Name))
	}

	if _, empty := getEphemeralContainersMaps(job.Spec.Template.EphemeralContainers); empty {
		job.Status.Phase = appsv1alpha1.EphemeralJobError
		job.Status.Conditions = addConditions(job.Status.Conditions, appsv1alpha1.EJobError, "Error", "job spec invalid fields.")
	}

	return nil
}

func (r *ReconcileEphemeralJob) updateJobStatus(job *appsv1alpha1.EphemeralJob) error {
	klog.V(5).Infof("Updating job %s status %#v", job.Name, job.Status)
	return r.Status().Update(context.TODO(), job)
}

func (r *ReconcileEphemeralJob) removeEphemeralContainers(job *appsv1alpha1.EphemeralJob) error {
	targetPods, err := r.filterInjectedPods(job)
	if err != nil {
		klog.Errorf("Failed to get ephemeral job %s/%s related target pods: %v", job.Namespace, job.Name, err)
		return err
	}

	control := econtainer.New(job)
	for _, pod := range targetPods {
		if err = control.RemoveEphemeralContainer(pod); err != nil {
			r.eventRecorder.Event(job, v1.EventTypeWarning, "RemoveFailed",
				fmt.Sprintf("Failed to remove ephemeral container for pod %s: %v", pod.Name, err))
			return err
		}
	}

	return nil
}
