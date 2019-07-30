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
	"fmt"
	"sync"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/scheduler/algorithm"
	"k8s.io/kubernetes/pkg/scheduler/algorithm/predicates"
	schedulercache "k8s.io/kubernetes/pkg/scheduler/cache"
	"k8s.io/utils/integer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var controllerKind = appsv1alpha1.SchemeGroupVersion.WithKind("BroadcastJob")

// Add creates a new BroadcastJob Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetRecorder("broadcastjob-controller")
	return &ReconcileBroadcastJob{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: recorder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("broadcastjob-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to BroadcastJob
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.BroadcastJob{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appsv1alpha1.BroadcastJob{},
	})

	if err != nil {
		return err
	}

	// Watch for changes to Pod
	if err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &enqueueBroadcastJobForNode{client: mgr.GetClient()}); err != nil {
		return err
	}
	return nil
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

// Reconcile reads that state of the cluster for a BroadcastJob object and makes changes based on the state read
// and what is in the BroadcastJob.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=broadcastjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=broadcastjobs/status,verbs=get;update;patch
func (r *ReconcileBroadcastJob) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the BroadcastJob instance
	job := &appsv1alpha1.BroadcastJob{}
	err := r.Get(context.TODO(), request.NamespacedName, job)

	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		klog.Errorf("failed to get job %s,", job.Name)
		return reconcile.Result{}, err
	}
	// Add pre-defined labels to pod template
	addLabelToPodTemplate(job)

	if IsJobFinished(job) {
		if pastTTLDeadline(job) {
			klog.Infof("deleting the job %s", job.Name)
			err = r.Delete(context.TODO(), job)
			if err != nil {
				klog.Errorf("failed to delete job %s", job.Name)
			}
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
			klog.Infof("Job %s has ActiveDeadlineSeconds, will resync after %d seconds",
				job.Name, *job.Spec.CompletionPolicy.ActiveDeadlineSeconds)
			requeueAfter = time.Duration(*job.Spec.CompletionPolicy.ActiveDeadlineSeconds) * time.Second
		}
	}

	// list pods for this job
	podList := &corev1.PodList{}
	listOptions := client.InNamespace(request.Namespace)
	listOptions.MatchingLabels(labelsAsMap(job))
	err = r.List(context.TODO(), listOptions, podList)
	if err != nil {
		klog.Errorf("failed to get podList for job %s,", job.Name)
		return reconcile.Result{}, err
	}

	// convert pod list to a slice of pointers
	var pods []*corev1.Pod
	for i := range podList.Items {
		pods = append(pods, &podList.Items[i])
	}

	// Get the map (nodeName -> Pod) for pods with node assigned
	existingNodeToPodMap := r.getNodeToPodMap(pods, job)
	// list all nodes in cluster
	nodes := &corev1.NodeList{}
	err = r.List(context.TODO(), &client.ListOptions{}, nodes)
	if err != nil {
		klog.Errorf("failed to get nodeList for job %s,", job.Name)
		return reconcile.Result{}, err
	}

	// Get active, failed, succeeded pods
	activePods, failedPods, succeededPods := filterPods(pods)
	active := int32(len(activePods))
	failed := int32(len(failedPods))
	succeeded := int32(len(succeededPods))

	oldNumConditions := len(job.Status.Conditions)
	var desired int32
	desiredNodes, restNodesToRunPod, podsToDelete := getNodesToRunPod(nodes, job, existingNodeToPodMap)
	desired = int32(len(desiredNodes))
	klog.Infof("%s/%s has %d/%d nodes remaining to schedule pods", job.Namespace, job.Name, len(restNodesToRunPod), desired)
	klog.Infof("Before broadcastjob reconcile %s/%s, desired=%d, active=%d, failed=%d", job.Namespace, job.Name, desired, active, failed)

	jobFailed, failureReason, failureMessage := isJobFailed(job, pods)
	// Job is failed. For keepAlive type, the job will never fail.
	if jobFailed {
		// Handle Job failures, delete all active pods
		failed, active, err = r.deleteJobPods(job, activePods, failed, active)
		if err != nil {
			klog.Errorf("failed to deleteJobPods for job %s,", job.Name)
		}
		requeueAfter = finishJob(job, appsv1alpha1.JobFailed, failureMessage)
		r.recorder.Event(job, corev1.EventTypeWarning, failureReason,
			fmt.Sprintf("%s: %d pods succeeded, %d pods failed", failureMessage, succeeded, failed))
	} else {
		// Job is still active
		if len(podsToDelete) > 0 {
			//should we remove the pods without nodes associated, the podgc controller will do this if enabled
			failed, active, err = r.deleteJobPods(job, podsToDelete, failed, active)
			if err != nil {
				klog.Errorf("failed to deleteJobPods for job %s,", job.Name)
			}
		}

		// DeletionTimestamp is not set and more nodes to run pod
		if job.DeletionTimestamp == nil && len(restNodesToRunPod) > 0 {
			active, err = r.reconcilePods(job, restNodesToRunPod, active)
			if err != nil {
				klog.Errorf("failed to reconcilePods for job %s,", job.Name)
			}
		}

		if isJobComplete(job, desiredNodes) {
			message := fmt.Sprintf("Job completed, %d pods succeeded, %d pods failed", succeeded, failed)
			requeueAfter = finishJob(job, appsv1alpha1.JobComplete, message)
			r.recorder.Event(job, corev1.EventTypeNormal, "JobComplete",
				fmt.Sprintf("Job %s/%s is completed, %d pods succeeded, %d pods failed", job.Namespace, job.Name, succeeded, failed))
		}
	}
	klog.Infof("After broadcastjob reconcile %s/%s, desired=%d, active=%d, failed=%d", job.Namespace, job.Name, desired, active, failed)

	// no need to update the job if the status hasn't changed since last time
	if job.Status.Active != active || job.Status.Succeeded != succeeded ||
		job.Status.Desired != desired || job.Status.Failed != failed ||
		len(job.Status.Conditions) != oldNumConditions {

		job.Status.Active = active
		job.Status.Succeeded = succeeded
		job.Status.Failed = failed
		job.Status.Desired = desired

		klog.Infof("Updating job %s status", job.Name)
		// update the status
		jobCopy := job.DeepCopy()
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
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
		if err != nil {
			klog.Errorf("failed to update job %s, %v", job.Name, err)
		}

	}
	return reconcile.Result{RequeueAfter: requeueAfter}, err
}

// finishJob appends the condition to JobStatus, and sets ttl if needed
func finishJob(job *appsv1alpha1.BroadcastJob, conditionType appsv1alpha1.JobConditionType, message string) time.Duration {
	job.Status.Conditions = append(job.Status.Conditions, newCondition(conditionType, string(conditionType), message))
	klog.Infof("job %s/%s is %s: %s", job.Namespace, job.Name, string(conditionType), message)

	now := metav1.Now()
	job.Status.CompletionTime = &now

	var requeueAfter time.Duration
	if job.Spec.CompletionPolicy.TTLSecondsAfterFinished != nil {
		klog.Infof("Job %s is %s, will be deleted after %d seconds", job.Name, string(conditionType),
			*job.Spec.CompletionPolicy.TTLSecondsAfterFinished)
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
	restNodesToRunPod []*corev1.Node, active int32) (int32, error) {

	// max concurrent running pods
	var parallelism int32
	if job.Spec.Parallelism == nil {
		// not specify,take the max int
		parallelism = int32(1<<31 - 1)
	} else {
		parallelism = *job.Spec.Parallelism
	}

	// The rest pods to run
	rest := int32(len(restNodesToRunPod))
	var err error
	var errCh chan error
	if active > parallelism {
		// exceed parallelism limit
		r.recorder.Eventf(job, corev1.EventTypeWarning, "TooManyActivePods", "Number of active pods exceed parallelism limit")
		//TODO should we remove the extra pods ? it may just finish by its own.

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
		for batchSize := int32(integer.Int32Min(diff, kubecontroller.SlowStartInitialBatchSize)); diff > 0; batchSize = integer.Int32Min(2*batchSize, diff) {
			// count of errors in current error channel
			errorCount := len(errCh)
			wait.Add(int(batchSize))

			// create pod concurrently in each batch by go routine
			curBatchNodes := restNodesToRunPod[startIndex : startIndex+batchSize]
			for _, node := range curBatchNodes {
				go func(nodeName string) {
					defer wait.Done()
					// parallelize pod creation
					klog.Infof("creating pod on node %s", nodeName)
					err := r.createPodsOnNode(nodeName, job.Namespace, &job.Spec.Template, job, asOwner(job))
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
		klog.Info("Num desiredNodes is 0")
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

// isJobFailed checks if the job CompletionPolicy is not Never, and it has past the backofflimit or ActiveDeadlineSeconds.
func isJobFailed(job *appsv1alpha1.BroadcastJob, pods []*corev1.Pod) (bool, string, string) {
	if job.Spec.CompletionPolicy.Type == appsv1alpha1.Never {
		return false, "", ""
	}
	jobFailed := false
	var failureReason string
	var failureMessage string
	if pastBackoffLimitOnFailure(job, pods) {
		jobFailed = true
		failureReason = "BackoffLimitExceeded"
		failureMessage = fmt.Sprintf("Job %s/%s has reached the specified backoff limit", job.Namespace, job.Name)
	} else if pastActiveDeadline(job) {
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
			if err != nil {
				klog.Errorf("pod %s failed to checkNodeFitness for node %s, %v", pod.Name, node.Name, err)
				continue
			}
			if !canFit {
				if pod.DeletionTimestamp == nil {
					podsToDelete = append(podsToDelete, pod)
				}
				continue
			}
			desiredNodes[node.Name] = pod
		} else {
			// no pod exists, mock a pod to check if the pod can fit on the node,
			// considering nodeName, label affinity and taints
			mockPod := NewPod(job, node.Name)
			canFit, err = checkNodeFitness(mockPod, &node)
			if err != nil {
				klog.Errorf("failed to checkNodeFitness for node %s, %v", node.Name, err)
				continue
			}
			if !canFit {
				klog.Infof("Pod does not fit on node %s", node.Name)
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
		nodeName := pod.Spec.NodeName
		if _, ok := nodeToPodMap[nodeName]; ok {
			// should not happen
			klog.Warningf("Duplicated pod %s run on the same node %s. this should not happen.", pod.Name, nodeName)
			r.recorder.Eventf(job, corev1.EventTypeWarning, "DuplicatePodCreatedOnSameNode",
				"Duplicated pod %s found on same node %s", pod.Name, nodeName)
		}
		nodeToPodMap[nodeName] = pods[i]
	}
	return nodeToPodMap
}

func labelsAsMap(job *appsv1alpha1.BroadcastJob) map[string]string {
	return map[string]string{
		"job-name":       job.Name,
		"controller-uid": string(job.UID),
	}
}

// checkNodeFitness runs a set of predicates that select candidate nodes for the job pod;
// the predicates include:
//   - PodFitsHost: checks pod's NodeName against node
//   - PodMatchNodeSelector: checks pod's NodeSelector and NodeAffinity against node
//   - PodToleratesNodeTaints: exclude tainted node unless pod has specific toleration
//   - CheckNodeUnschedulablePredicate: check if the pod can tolerate node unschedulable
func checkNodeFitness(pod *corev1.Pod, node *corev1.Node) (bool, error) {
	nodeInfo := schedulercache.NewNodeInfo()
	_ = nodeInfo.SetNode(node)

	fit, reasons, err := predicates.PodFitsHost(pod, nil, nodeInfo)
	if err != nil || !fit {
		logPredicateFailedReason(reasons, node)
		return false, err
	}

	fit, reasons, err = predicates.PodMatchNodeSelector(pod, nil, nodeInfo)
	if err != nil || !fit {
		logPredicateFailedReason(reasons, node)
		return false, err
	}

	fit, reasons, err = predicates.PodToleratesNodeTaints(pod, nil, nodeInfo)
	if err != nil || !fit {
		logPredicateFailedReason(reasons, node)
		return false, err
	}

	fit, reasons, err = predicates.CheckNodeUnschedulablePredicate(pod, nil, nodeInfo)
	if err != nil || !fit {
		logPredicateFailedReason(reasons, node)
		return false, err
	}
	return true, nil
}

func logPredicateFailedReason(reasons []algorithm.PredicateFailureReason, node *corev1.Node) {
	if len(reasons) == 0 {
		return
	}
	for _, reason := range reasons {
		klog.Errorf("Failed predicate on node %s : %s ", node.Name, reason.GetReason())
	}
}

// NewPod creates a new pod
func NewPod(job *appsv1alpha1.BroadcastJob, nodeName string) *corev1.Pod {
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
			if err := r.Delete(context.TODO(), pods[ix]); err != nil {
				defer utilruntime.HandleError(err)
				klog.Infof("Failed to delete %v, job %q/%q", pods[ix].Name, job.Namespace, job.Name)
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

func (r *ReconcileBroadcastJob) createPodsOnNode(nodeName, namespace string, template *corev1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	if err := validateControllerRef(controllerRef); err != nil {
		return err
	}
	return r.createPods(nodeName, namespace, template, object, controllerRef)
}

func (r *ReconcileBroadcastJob) createPods(nodeName, namespace string, template *corev1.PodTemplateSpec, object runtime.Object, controllerRef *metav1.OwnerReference) error {
	pod, err := kubecontroller.GetPodFromTemplate(template, object, controllerRef)
	if err != nil {
		return err
	}
	pod.Namespace = namespace
	if len(nodeName) != 0 {
		pod.Spec.NodeName = nodeName
	}
	if labels.Set(pod.Labels).AsSelectorPreValidated().Empty() {
		return fmt.Errorf("unable to create pods, no labels")
	}

	// podModifier is only used for testing
	if r.podModifier != nil {
		r.podModifier(pod)
	}
	if err := r.Client.Create(context.TODO(), pod); err != nil {
		r.recorder.Eventf(object, corev1.EventTypeWarning, kubecontroller.FailedCreatePodReason, "Error creating: %v", err)
		return err
	}

	accessor, err := meta.Accessor(object)
	if err != nil {
		klog.Errorf("parentObject does not have ObjectMeta, %v", err)
		return nil
	}
	klog.Infof("Controller %v created pod %v", accessor.GetName(), pod.Name)
	r.recorder.Eventf(object, corev1.EventTypeNormal, kubecontroller.SuccessfulCreatePodReason, "Created pod: %v", pod.Name)

	return nil
}
