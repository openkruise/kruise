/*
Copyright 2016 The Kubernetes Authors.
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

package broadcastjob

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	v1affinityhelper "k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	daemonsetutil "k8s.io/kubernetes/pkg/controller/daemon/util"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeaffinity"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodename"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/noderesources"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeunschedulable"
	"k8s.io/utils/integer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/expectations"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
)

func init() {
	flag.BoolVar(&scheduleBroadcastJobPods, "assign-bcj-pods-by-scheduler", true, "Use scheduler to assign broadcastJob pod to node.")
	flag.IntVar(&concurrentReconciles, "broadcastjob-workers", concurrentReconciles, "Max concurrent workers for BroadCastJob controller.")
}

const (
	JobNameLabelKey       = "broadcastjob-name"
	ControllerUIDLabelKey = "broadcastjob-controller-uid"
)

var (
	concurrentReconciles     = 3
	scheduleBroadcastJobPods bool
	controllerKind           = appsv1alpha1.SchemeGroupVersion.WithKind("BroadcastJob")
	scaleExpectations        = expectations.NewScaleExpectations()
)

// Add creates a new BroadcastJob Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor("broadcastjob-controller")
	return &ReconcileBroadcastJob{
		Client:   utilclient.NewClientFromManager(mgr, "broadcastjob-controller"),
		scheme:   mgr.GetScheme(),
		recorder: recorder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {

	// todo: maybe we can change to NewControllerManagedBy, example
	//return ctrl.NewControllerManagedBy(mgr).
	//	Named("broadcastjob-controller").
	//	WithOptions(controller.Options{
	//		MaxConcurrentReconciles: concurrentReconciles,
	//		CacheSyncTimeout:        util.GetControllerCacheSyncTimeout(),
	//		RateLimiter:             ratelimiter.DefaultControllerRateLimiter(),
	//	}).
	//	For(&appsv1alpha1.BroadcastJob{}).
	//	Owns(&corev1.Pod{}).
	//	Watches(&corev1.Node{}, &enqueueBroadcastJobForNode{reader: mgr.GetCache()}).
	//	Complete(r)

	// Create a new controller
	c, err := controller.New("broadcastjob-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles, CacheSyncTimeout: util.GetControllerCacheSyncTimeout(),
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to BroadcastJob
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1alpha1.BroadcastJob{}, &handler.TypedEnqueueRequestForObject[*appsv1alpha1.BroadcastJob]{}))
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, &podEventHandler{
		enqueueHandler: handler.TypedEnqueueRequestForOwner[*corev1.Pod](mgr.GetScheme(), mgr.GetRESTMapper(), &appsv1alpha1.BroadcastJob{}, handler.OnlyControllerOwner()),
	}))
	if err != nil {
		return err
	}

	// Watch for changes to Node
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Node{}, &enqueueBroadcastJobForNode{reader: mgr.GetCache()}))
}

var _ reconcile.Reconciler = &ReconcileBroadcastJob{}

// ReconcileBroadcastJob reconciles a BroadcastJob object
type ReconcileBroadcastJob struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	// podModifier is only for testing to set the pod.Name, if pod.GenerateName is used
	podModifier func(pod *corev1.Pod)
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=broadcastjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=broadcastjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=broadcastjobs/finalizers,verbs=update

// Reconcile reads that state of the cluster for a BroadcastJob object and makes changes based on the state read
// and what is in the BroadcastJob.Spec
func (r *ReconcileBroadcastJob) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Fetch the BroadcastJob instance
	job := &appsv1alpha1.BroadcastJob{}
	err := r.Get(context.TODO(), request.NamespacedName, job)

	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			scaleExpectations.DeleteExpectations(request.String())
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		klog.ErrorS(err, "Failed to get BroadcastJob", "broadcastJob", klog.KObj(job))
		return reconcile.Result{}, err
	}

	if scaleSatisfied, unsatisfiedDuration, scaleDirtyPods := scaleExpectations.SatisfiedExpectations(request.String()); !scaleSatisfied {
		if unsatisfiedDuration >= expectations.ExpectationTimeout {
			klog.InfoS("Expectation unsatisfied overtime for BroadcastJob", "broadcastJob", request, "scaleDirtyPods", scaleDirtyPods, "overtime", unsatisfiedDuration)
			return reconcile.Result{}, nil
		}
		klog.V(4).InfoS("Not satisfied scale for BroadcastJob", "broadcastJob", request, "scaleDirtyPods", scaleDirtyPods)
		return reconcile.Result{RequeueAfter: expectations.ExpectationTimeout - unsatisfiedDuration}, nil
	}

	// Add pre-defined labels to pod template
	addLabelToPodTemplate(job)

	if IsJobFinished(job) {
		isPast, leftTime := pastTTLDeadline(job)
		if isPast {
			klog.InfoS("Deleting BroadcastJob", "broadcastJob", klog.KObj(job))
			err = r.Delete(context.TODO(), job)
			if err != nil {
				klog.ErrorS(err, "Failed to delete BroadcastJob", "broadcastJob", klog.KObj(job))
			}
		} else if leftTime > 0 {
			return reconcile.Result{RequeueAfter: leftTime}, nil
		}
		return reconcile.Result{}, nil
	}
	// requeueAfter is zero, meaning no requeue
	requeueAfter := time.Duration(0)
	// set the job startTime
	if job.Status.StartTime == nil {
		now := metav1.Now()
		job.Status.StartTime = &now
		if job.Spec.CompletionPolicy.Type == appsv1alpha1.Always &&
			job.Spec.CompletionPolicy.ActiveDeadlineSeconds != nil {
			klog.InfoS("BroadcastJob has ActiveDeadlineSeconds, will resync after the deadline", "broadcastJob", klog.KObj(job),
				"activeDeadlineSeconds", *job.Spec.CompletionPolicy.ActiveDeadlineSeconds)
			requeueAfter = time.Duration(*job.Spec.CompletionPolicy.ActiveDeadlineSeconds) * time.Second
		}
	}

	if job.Status.Phase == "" {
		job.Status.Phase = appsv1alpha1.PhaseRunning
	}

	// list pods for this job
	podList := &corev1.PodList{}
	listOptions := &client.ListOptions{
		Namespace:     request.Namespace,
		LabelSelector: labels.SelectorFromSet(labelsAsMap(job)),
	}
	err = r.List(context.TODO(), podList, listOptions, utilclient.DisableDeepCopy)
	if err != nil {
		klog.ErrorS(err, "Failed to get podList for BroadcastJob", "broadcastJob", klog.KObj(job))
		return reconcile.Result{}, err
	}

	// convert pod list to a slice of pointers
	var pods []*corev1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		controllerRef := metav1.GetControllerOf(pod)
		if controllerRef != nil && controllerRef.Kind == job.Kind && controllerRef.UID == job.UID {
			pods = append(pods, pod)
		}
	}

	// Get the map (nodeName -> Pod) for pods with node assigned
	existingNodeToPodMap := r.getNodeToPodMap(pods, job)
	// list all nodes in cluster
	nodes := &corev1.NodeList{}
	err = r.List(context.TODO(), nodes)
	if err != nil {
		klog.ErrorS(err, "Failed to get nodeList for BroadcastJob", "broadcastJob", klog.KObj(job))
		return reconcile.Result{}, err
	}

	// Get active, failed, succeeded pods
	activePods, failedPods, succeededPods := filterPods(job.Spec.FailurePolicy.RestartLimit, pods)
	active := int32(len(activePods))
	failed := int32(len(failedPods))
	succeeded := int32(len(succeededPods))

	desiredNodes, restNodesToRunPod, podsToDelete := getNodesToRunPod(nodes, job, existingNodeToPodMap)
	desired := int32(len(desiredNodes))
	klog.InfoS("BroadcastJob has some nodes remaining to schedule pods", "broadcastJob", klog.KObj(job), "restNodeCount", len(restNodesToRunPod), "desiredNodeCount", desired)
	klog.InfoS("Before BroadcastJob reconcile, with desired, active and failed counts",
		"broadcastJob", klog.KObj(job), "desiredCount", desired, "activeCount", active, "failedCount", failed)
	job.Status.Active = active
	job.Status.Failed = failed
	job.Status.Succeeded = succeeded
	job.Status.Desired = desired

	if job.Status.Phase == appsv1alpha1.PhaseFailed {
		return reconcile.Result{RequeueAfter: requeueAfter}, r.updateJobStatus(request, job)
	}

	if job.Spec.Paused && (job.Status.Phase == appsv1alpha1.PhaseRunning || job.Status.Phase == appsv1alpha1.PhasePaused) {
		job.Status.Phase = appsv1alpha1.PhasePaused
		return reconcile.Result{RequeueAfter: requeueAfter}, r.updateJobStatus(request, job)
	}
	if !job.Spec.Paused && job.Status.Phase == appsv1alpha1.PhasePaused {
		job.Status.Phase = appsv1alpha1.PhaseRunning
		r.recorder.Event(job, corev1.EventTypeNormal, "Continue", "continue to process job")
	}

	jobFailed := false
	var failureReason, failureMessage string
	if failed > 0 {
		switch job.Spec.FailurePolicy.Type {
		case appsv1alpha1.FailurePolicyTypePause:
			r.recorder.Event(job, corev1.EventTypeWarning, "Paused", "job is paused, due to failed pod")
			job.Spec.Paused = true
			job.Status.Phase = appsv1alpha1.PhasePaused
			return reconcile.Result{RequeueAfter: requeueAfter}, r.updateJobStatus(request, job)
		case appsv1alpha1.FailurePolicyTypeFailFast:
			// mark the job is failed
			jobFailed, failureReason, failureMessage = true, "failed pod is found", "failure policy is FailurePolicyTypeFailFast and failed pod is found"
			r.recorder.Event(job, corev1.EventTypeWarning, failureReason, fmt.Sprintf("%s: %d pods succeeded, %d pods failed", failureMessage, succeeded, failed))
		case appsv1alpha1.FailurePolicyTypeContinue:
		}
	}

	if !jobFailed {
		jobFailed, failureReason, failureMessage = isJobFailed(job, pods)
	}
	// Job is failed. For keepAlive type, the job will never fail.
	if jobFailed {
		// Handle Job failures, delete all active pods
		failed, active, err = r.deleteJobPods(job, activePods, failed, active)
		if err != nil {
			klog.ErrorS(err, "Failed to deleteJobPods for job", "broadcastJob", klog.KObj(job))
		}
		job.Status.Phase = appsv1alpha1.PhaseFailed
		requeueAfter = finishJob(job, appsv1alpha1.JobFailed, failureMessage)
		r.recorder.Event(job, corev1.EventTypeWarning, failureReason,
			fmt.Sprintf("%s: %d pods succeeded, %d pods failed", failureMessage, succeeded, failed))
	} else {
		// Job is still active
		if len(podsToDelete) > 0 {
			// should we remove the pods without nodes associated, the podgc controller will do this if enabled
			failed, active, err = r.deleteJobPods(job, podsToDelete, failed, active)
			if err != nil {
				klog.ErrorS(err, "Failed to delete BroadcastJob Pods", "broadcastJob", klog.KObj(job))
			}
		}

		// DeletionTimestamp is not set and more nodes to run pod
		if job.DeletionTimestamp == nil && len(restNodesToRunPod) > 0 {
			active, err = r.reconcilePods(job, restNodesToRunPod, active, desired)
			if err != nil {
				klog.ErrorS(err, "Failed to reconcile Pods for BroadcastJob", "broadcastJob", klog.KObj(job))
			}
		}

		if isJobComplete(job, desiredNodes) {
			message := fmt.Sprintf("Job completed, %d pods succeeded, %d pods failed", succeeded, failed)
			job.Status.Phase = appsv1alpha1.PhaseCompleted
			requeueAfter = finishJob(job, appsv1alpha1.JobComplete, message)
			r.recorder.Event(job, corev1.EventTypeNormal, "JobComplete",
				fmt.Sprintf("Job %s/%s is completed, %d pods succeeded, %d pods failed", job.Namespace, job.Name, succeeded, failed))
		}
	}
	klog.InfoS("After broadcastjob reconcile, with desired, active and failed counts",
		"broadcastJob", klog.KObj(job), "desiredCount", desired, "activeCount", active, "failedCount", failed)

	// update the status
	job.Status.Failed = failed
	job.Status.Active = active
	if err := r.updateJobStatus(request, job); err != nil {
		klog.ErrorS(err, "Failed to update BroadcastJob", "broadcastJob", klog.KObj(job))
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, err
}

func (r *ReconcileBroadcastJob) updateJobStatus(request reconcile.Request, job *appsv1alpha1.BroadcastJob) error {
	klog.InfoS("Updating BroadcastJob status", "broadcastJob", klog.KObj(job), "status", job.Status)
	jobCopy := job.DeepCopy()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := r.Status().Update(context.TODO(), jobCopy)
		if err == nil {
			return nil
		}

		updated := &appsv1alpha1.BroadcastJob{}
		err = r.Get(context.TODO(), request.NamespacedName, updated)
		if err == nil {
			jobCopy = updated
			jobCopy.Status = job.Status
		} else {
			utilruntime.HandleError(fmt.Errorf("error getting updated broadcastjob %s/%s from lister: %v", job.Namespace, job.Name, err))
		}
		return err
	})
}

// finishJob appends the condition to JobStatus, and sets ttl if needed
func finishJob(job *appsv1alpha1.BroadcastJob, conditionType appsv1alpha1.JobConditionType, message string) time.Duration {
	job.Status.Conditions = append(job.Status.Conditions, newCondition(conditionType, string(conditionType), message))
	klog.InfoS("BroadcastJob conditions updated", "broadcastJob", klog.KObj(job), "conditionType", string(conditionType), "message", message)

	now := metav1.Now()
	job.Status.CompletionTime = &now

	var requeueAfter time.Duration
	if job.Spec.CompletionPolicy.TTLSecondsAfterFinished != nil {
		klog.InfoS("BroadcastJob will be deleted after a certain seconds", "broadcastJob", klog.KObj(job), "conditionType", string(conditionType),
			"TTLSecondsAfterFinished", *job.Spec.CompletionPolicy.TTLSecondsAfterFinished)
		// a bit more than the TTLSecondsAfterFinished to ensure it exceeds the TTLSecondsAfterFinished when being reconciled
		requeueAfter = time.Duration(*job.Spec.CompletionPolicy.TTLSecondsAfterFinished+1) * time.Second
	}
	return requeueAfter
}

// addLabelToPodTemplate will add the pre-defined labels to the pod template so that the pods created will
// have these labels associated. The labels can be used for querying pods for a specific job
func addLabelToPodTemplate(job *appsv1alpha1.BroadcastJob) {
	if job.Spec.Template.Labels == nil {
		job.Spec.Template.Labels = make(map[string]string)
	}
	for k, v := range labelsAsMap(job) {
		job.Spec.Template.Labels[k] = v
	}
}

func (r *ReconcileBroadcastJob) reconcilePods(job *appsv1alpha1.BroadcastJob,
	restNodesToRunPod []*corev1.Node, active, desired int32) (int32, error) {

	// max concurrent running pods
	parallelismInt, err := intstr.GetValueFromIntOrPercent(intstr.ValueOrDefault(job.Spec.Parallelism, intstr.FromInt(1<<31-1)), int(desired), true)
	if err != nil {
		return active, err
	}
	parallelism := int32(parallelismInt)

	// The rest pods to run
	rest := int32(len(restNodesToRunPod))
	var errCh chan error
	if active > parallelism {
		// exceed parallelism limit
		r.recorder.Eventf(job, corev1.EventTypeWarning, "TooManyActivePods", "Number of active pods exceed parallelism limit")
		// TODO should we remove the extra pods ? it may just finish by its own.

	} else if active < parallelism {
		// diff is the current number of pods to run in this reconcile loop
		diff := integer.Int32Min(parallelism-active, rest)

		// Batch the pod creates. Batch sizes start at SlowStartInitialBatchSize
		// and double with each successful iteration in a kind of "slow start".
		// This handles attempts to start large numbers of pods that would
		// likely all fail with the same error. For example a project with a
		// low quota that attempts to create a large number of pods will be
		// prevented from spamming the API service with the pod create requests
		// after one of its pods fails.  Conveniently, this also prevents the
		// event spam that those failures would generate.
		var activeLock sync.Mutex
		errCh = make(chan error, diff)
		wait := sync.WaitGroup{}
		startIndex := int32(0)
		for batchSize := integer.Int32Min(diff, kubecontroller.SlowStartInitialBatchSize); diff > 0; batchSize = integer.Int32Min(2*batchSize, diff) {
			// count of errors in current error channel
			errorCount := len(errCh)
			wait.Add(int(batchSize))

			// create pod concurrently in each batch by go routine
			curBatchNodes := restNodesToRunPod[startIndex : startIndex+batchSize]
			for _, node := range curBatchNodes {
				go func(nodeName string) {
					defer wait.Done()
					// parallelize pod creation
					klog.InfoS("Creating pod on node", "nodeName", nodeName)
					err := r.createPodOnNode(nodeName, job.Namespace, &job.Spec.Template, job, asOwner(job))
					if err != nil && errors.IsTimeout(err) {
						// Pod is created but its initialization has timed out.
						// If the initialization is successful eventually, the
						// controller will observe the creation via the informer.
						// If the initialization fails, or if the pod keeps
						// uninitialized for a long time, the informer will not
						// receive any update, and the controller will create a new
						// pod when the expectation expires.
						return
					}
					if err != nil {
						defer utilruntime.HandleError(err)
						errCh <- err
					}
					// If succeed, increase active counter
					activeLock.Lock()
					active++
					activeLock.Unlock()
				}(node.Name)
			}
			// wait for all pods created
			wait.Wait()
			// If there are error occurs and there are still pods remaining to be created
			skippedPods := diff - batchSize
			if errorCount < len(errCh) && skippedPods > 0 {
				// The skipped pods will be retried later. The next controller resync will
				// retry the slow start process.
				break
			}
			diff -= batchSize
			startIndex += batchSize
		}
	}
	select {
	case err = <-errCh:
	default:
	}
	return active, err
}

// isJobComplete returns true if all pods on all desiredNodes are either succeeded or failed or deletionTimestamp !=nil.
func isJobComplete(job *appsv1alpha1.BroadcastJob, desiredNodes map[string]*corev1.Pod) bool {
	if job.Spec.CompletionPolicy.Type == appsv1alpha1.Never {
		// the job will not terminate, if the the completion policy is never
		return false
	}
	// if no desiredNodes, job pending
	if len(desiredNodes) == 0 {
		klog.InfoS("Num desiredNodes is 0")
		return false
	}
	for _, pod := range desiredNodes {
		if pod == nil || kubecontroller.IsPodActive(pod) {
			// the job is incomplete if there exits any pod not yet created OR  still active
			return false
		}
	}
	return true
}

// isJobFailed checks if the job CompletionPolicy is not Never, and it has past ActiveDeadlineSeconds.
func isJobFailed(job *appsv1alpha1.BroadcastJob, pods []*corev1.Pod) (bool, string, string) {
	if job.Spec.CompletionPolicy.Type == appsv1alpha1.Never {
		return false, "", ""
	}
	jobFailed := false
	var failureReason string
	var failureMessage string
	if pastActiveDeadline(job) {
		jobFailed = true
		failureReason = "DeadlineExceeded"
		failureMessage = fmt.Sprintf("Job %s/%s was active longer than specified deadline", job.Namespace, job.Name)
	}
	return jobFailed, failureReason, failureMessage
}

// getNodesToRunPod returns
// * desiredNodes : the nodes desired to run pods including node with or without running pods
// * restNodesToRunPod:  the nodes do not have pods running yet, excluding the nodes not satisfying constraints such as affinity, taints
// * podsToDelete: the pods that do not satisfy the node constraint any more
func getNodesToRunPod(nodes *corev1.NodeList, job *appsv1alpha1.BroadcastJob,
	existingNodeToPodMap map[string]*corev1.Pod) (map[string]*corev1.Pod, []*corev1.Node, []*corev1.Pod) {

	var podsToDelete []*corev1.Pod
	var restNodesToRunPod []*corev1.Node
	desiredNodes := make(map[string]*corev1.Pod)
	for i, node := range nodes.Items {

		var canFit bool
		var err error
		// there's pod existing on the node
		if pod, ok := existingNodeToPodMap[node.Name]; ok {
			canFit, err = checkNodeFitness(pod, &node)
			if !canFit && pod.DeletionTimestamp == nil {
				klog.ErrorS(err, "Pod did not fit on node", "pod", klog.KObj(pod), "nodeName", node.Name)
				podsToDelete = append(podsToDelete, pod)
				continue
			}
			desiredNodes[node.Name] = pod
		} else {
			// no pod exists, mock a pod to check if the pod can fit on the node,
			// considering nodeName, label affinity and taints
			mockPod := NewMockPod(job, node.Name)
			canFit, err = checkNodeFitness(mockPod, &node)
			if !canFit {
				klog.InfoS("Pod did not fit on node", "nodeName", node.Name, "err", err)
				continue
			}
			restNodesToRunPod = append(restNodesToRunPod, &nodes.Items[i])
			desiredNodes[node.Name] = nil
		}
	}
	return desiredNodes, restNodesToRunPod, podsToDelete
}

// getNodeToPodMap scans the pods and construct a map : nodeName -> pod.
// Ideally, each node should have only 1 pod. Else, something is wrong.
func (r *ReconcileBroadcastJob) getNodeToPodMap(pods []*corev1.Pod, job *appsv1alpha1.BroadcastJob) map[string]*corev1.Pod {
	nodeToPodMap := make(map[string]*corev1.Pod)
	for i, pod := range pods {
		nodeName := getAssignedNode(pod)
		if _, ok := nodeToPodMap[nodeName]; ok {
			// should not happen
			klog.InfoS("Duplicated pod run on the same node. This should not happen", "pod", klog.KObj(pod), "nodeName", nodeName)
			r.recorder.Eventf(job, corev1.EventTypeWarning, "DuplicatePodCreatedOnSameNode",
				"Duplicated pod %s found on same node %s", pod.Name, nodeName)
		}
		nodeToPodMap[nodeName] = pods[i]
	}
	return nodeToPodMap
}

func labelsAsMap(job *appsv1alpha1.BroadcastJob) map[string]string {
	return map[string]string{
		JobNameLabelKey:       job.Name,
		ControllerUIDLabelKey: string(job.UID),
	}
}

// checkNodeFitness runs a set of predicates that select candidate nodes for the job pod;
// the predicates include:
//   - PodFitsHost: checks pod's NodeName against node
//   - PodMatchNodeSelector: checks pod's ImagePullJobNodeSelector and NodeAffinity against node
//   - PodToleratesNodeTaints: exclude tainted node unless pod has specific toleration
//   - CheckNodeUnschedulablePredicate: check if the pod can tolerate node unschedulable
//   - PodFitsResources: checks if a node has sufficient resources, such as cpu, memory, gpu, opaque int resources etc to run a pod.
func checkNodeFitness(pod *corev1.Pod, node *corev1.Node) (bool, error) {
	nodeInfo := framework.NewNodeInfo()
	nodeInfo.SetNode(node)

	if len(pod.Spec.NodeName) != 0 && pod.Spec.NodeName != node.Name {
		return logPredicateFailedReason(node, framework.NewStatus(framework.UnschedulableAndUnresolvable, nodename.ErrReason))
	}

	if fitsNodeAffinity, _ := v1affinityhelper.GetRequiredNodeAffinity(pod).Match(node); !fitsNodeAffinity {
		return logPredicateFailedReason(node, framework.NewStatus(framework.UnschedulableAndUnresolvable, nodeaffinity.ErrReasonPod))
	}

	filterPredicate := func(t *corev1.Taint) bool {
		// PodToleratesNodeTaints is only interested in NoSchedule and NoExecute taints.
		return t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute
	}
	taint, isUntolerated := v1helper.FindMatchingUntoleratedTaint(node.Spec.Taints, pod.Spec.Tolerations, filterPredicate)
	if isUntolerated {
		errReason := fmt.Sprintf("node(s) had taint {%s: %s}, that the pod didn't tolerate",
			taint.Key, taint.Value)
		return logPredicateFailedReason(node, framework.NewStatus(framework.UnschedulableAndUnresolvable, errReason))
	}

	// If pod tolerate unschedulable taint, it's also tolerate `node.Spec.Unschedulable`.
	podToleratesUnschedulable := v1helper.TolerationsTolerateTaint(pod.Spec.Tolerations, &corev1.Taint{
		Key:    corev1.TaintNodeUnschedulable,
		Effect: corev1.TaintEffectNoSchedule,
	})
	if nodeInfo.Node().Spec.Unschedulable && !podToleratesUnschedulable {
		return logPredicateFailedReason(node, framework.NewStatus(framework.UnschedulableAndUnresolvable, nodeunschedulable.ErrReasonUnschedulable))
	}

	insufficientResources := noderesources.Fits(pod, nodeInfo)
	if len(insufficientResources) != 0 {
		// We will keep all failure reasons.
		failureReasons := make([]string, 0, len(insufficientResources))
		for _, r := range insufficientResources {
			failureReasons = append(failureReasons, r.Reason)
		}
		return logPredicateFailedReason(node, framework.NewStatus(framework.Unschedulable, failureReasons...))
	}

	return true, nil
}

func logPredicateFailedReason(node *corev1.Node, status *framework.Status) (bool, error) {
	if status.IsSuccess() {
		return true, nil
	}
	for _, reason := range status.Reasons() {
		klog.ErrorS(fmt.Errorf(reason), "Failed to predicate on node", "nodeName", node.Name)
	}
	return status.IsSuccess(), status.AsError()
}

// NewMockPod creates a new mock pod
func NewMockPod(job *appsv1alpha1.BroadcastJob, nodeName string) *corev1.Pod {
	newPod := &corev1.Pod{Spec: job.Spec.Template.Spec, ObjectMeta: job.Spec.Template.ObjectMeta}
	newPod.Namespace = job.Namespace
	newPod.Spec.NodeName = nodeName
	return newPod
}

// deleteJobPods delete the pods concurrently and wait for them to be done
func (r *ReconcileBroadcastJob) deleteJobPods(job *appsv1alpha1.BroadcastJob, pods []*corev1.Pod, failed, active int32) (int32, int32, error) {
	errCh := make(chan error, len(pods))
	wait := sync.WaitGroup{}
	nbPods := len(pods)
	var failedLock sync.Mutex
	wait.Add(nbPods)
	for i := int32(0); i < int32(nbPods); i++ {
		go func(ix int32) {
			defer wait.Done()
			key := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}.String()
			scaleExpectations.ExpectScale(key, expectations.Delete, getAssignedNode(pods[ix]))
			if err := r.Delete(context.TODO(), pods[ix]); err != nil {
				scaleExpectations.ObserveScale(key, expectations.Delete, getAssignedNode(pods[ix]))
				defer utilruntime.HandleError(err)
				klog.InfoS("Failed to delete BroadcastJob Pod", "pod", klog.KObj(pods[ix]), "broadcastJob", klog.KObj(job))
				errCh <- err
			} else {
				failedLock.Lock()
				failed++
				active--
				r.recorder.Eventf(job, corev1.EventTypeNormal, kubecontroller.SuccessfulDeletePodReason, "Delete pod: %v", pods[ix].Name)
				failedLock.Unlock()
			}
		}(i)
	}
	wait.Wait()
	var manageJobErr error
	select {
	case manageJobErr = <-errCh:
	default:
	}
	return failed, active, manageJobErr
}

func (r *ReconcileBroadcastJob) createPodOnNode(nodeName, namespace string, template *corev1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	if err := validateControllerRef(controllerRef); err != nil {
		return err
	}
	return r.createPod(nodeName, namespace, template, object, controllerRef)
}

func (r *ReconcileBroadcastJob) createPod(nodeName, namespace string, template *corev1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	pod, err := kubecontroller.GetPodFromTemplate(template, object, controllerRef)
	if err != nil {
		return err
	}
	pod.Namespace = namespace
	if scheduleBroadcastJobPods {
		// The pod's NodeAffinity will be updated to make sure the Pod is bound
		// to the target node by default scheduler. It is safe to do so because there
		// should be no conflicting node affinity with the target node.
		pod.Spec.Affinity = daemonsetutil.ReplaceDaemonSetPodNodeNameNodeAffinity(pod.Spec.Affinity, nodeName)
	} else {
		if len(nodeName) != 0 {
			pod.Spec.NodeName = nodeName
		}
	}
	if labels.Set(pod.Labels).AsSelectorPreValidated().Empty() {
		return fmt.Errorf("unable to create pods, no labels")
	}

	// podModifier is only used for testing
	if r.podModifier != nil {
		r.podModifier(pod)
	}

	key := types.NamespacedName{Namespace: namespace, Name: controllerRef.Name}.String()
	// Pod.Name is empty since the Pod uses generated name. We use nodeName as the unique identity
	// since each node should only contain one job Pod.
	scaleExpectations.ExpectScale(key, expectations.Create, nodeName)
	if err := r.Client.Create(context.TODO(), pod); err != nil {
		scaleExpectations.ObserveScale(key, expectations.Create, nodeName)
		r.recorder.Eventf(object, corev1.EventTypeWarning, kubecontroller.FailedCreatePodReason, "Error creating: %v", err)
		return err
	}

	accessor, err := meta.Accessor(object)
	if err != nil {
		klog.ErrorS(err, "ParentObject did not have ObjectMeta")
		return nil
	}
	klog.InfoS("Controller created pod", "controllerName", accessor.GetName(), "pod", klog.KObj(pod))
	r.recorder.Eventf(object, corev1.EventTypeNormal, kubecontroller.SuccessfulCreatePodReason, "Created pod: %v", pod.Name)

	return nil
}
