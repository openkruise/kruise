/*
Copyright 2019 The Kruise Authors.
Copyright 2016 The Kubernetes Authors.

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

package statefulset

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller/history"
	"k8s.io/utils/integer"
	utilpointer "k8s.io/utils/pointer"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	imagejobutilfunc "github.com/openkruise/kruise/pkg/util/imagejob/utilfunction"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/lifecycle"
	"github.com/openkruise/kruise/pkg/util/specifieddelete"
)

// Realistic value for maximum in-flight requests when processing in parallel mode.
const MaxBatchSize = 500

const PVCOwnedByStsAnnotationKey = "apps.kruise.io/owned-by-asts"

// StatefulSetControlInterface implements the control logic for updating StatefulSets and their children Pods. It is implemented
// as an interface to allow for extensions that provide different semantics. Currently, there is only one implementation.
type StatefulSetControlInterface interface {
	// UpdateStatefulSet implements the control logic for Pod creation, update, and deletion, and
	// persistent volume creation, update, and deletion.
	// If an implementation returns a non-nil error, the invocation will be retried using a rate-limited strategy.
	// Implementors should sink any errors that they do not wish to trigger a retry, and they may feel free to
	// exit exceptionally at any point provided they wish the update to be re-run at a later point in time.
	UpdateStatefulSet(ctx context.Context, set *appsv1beta1.StatefulSet, pods []*v1.Pod) error
	// ListRevisions returns a array of the ControllerRevisions that represent the revisions of set. If the returned
	// error is nil, the returns slice of ControllerRevisions is valid.
	ListRevisions(set *appsv1beta1.StatefulSet) ([]*apps.ControllerRevision, error)
	// AdoptOrphanRevisions adopts any orphaned ControllerRevisions that match set's Selector. If all adoptions are
	// successful the returned error is nil.
	AdoptOrphanRevisions(set *appsv1beta1.StatefulSet, revisions []*apps.ControllerRevision) error
}

// NewDefaultStatefulSetControl returns a new instance of the default implementation ControlInterface that
// implements the documented semantics for StatefulSets. podControl is the PodControlInterface used to create, update,
// and delete Pods and to create PersistentVolumeClaims. statusUpdater is the StatusUpdaterInterface used
// to update the status of StatefulSets. You should use an instance returned from NewRealStatefulPodControl() for any
// scenario other than testing.
func NewDefaultStatefulSetControl(
	podControl *StatefulPodControl,
	inplaceControl inplaceupdate.Interface,
	lifecycleControl lifecycle.Interface,
	statusUpdater StatusUpdaterInterface,
	controllerHistory history.Interface,
	recorder record.EventRecorder) StatefulSetControlInterface {
	return &defaultStatefulSetControl{
		podControl,
		statusUpdater,
		controllerHistory,
		recorder,
		inplaceControl,
		lifecycleControl,
	}
}

// defaultStatefulSetControl implements ControlInterface
var _ StatefulSetControlInterface = &defaultStatefulSetControl{}

type defaultStatefulSetControl struct {
	podControl        *StatefulPodControl
	statusUpdater     StatusUpdaterInterface
	controllerHistory history.Interface
	recorder          record.EventRecorder
	inplaceControl    inplaceupdate.Interface
	lifecycleControl  lifecycle.Interface
}

// UpdateStatefulSet executes the core logic loop for a stateful set, applying the predictable and
// consistent monotonic update strategy by default - scale up proceeds in ordinal order, no new pod
// is created while any pod is unhealthy, and pods are terminated in descending order. The burst
// strategy allows these constraints to be relaxed - pods will be created and deleted eagerly and
// in no particular order. Clients using the burst strategy should be careful to ensure they
// understand the consistency implications of having unpredictable numbers of pods available.
func (ssc *defaultStatefulSetControl) UpdateStatefulSet(ctx context.Context, set *appsv1beta1.StatefulSet, pods []*v1.Pod) error {
	set = set.DeepCopy()

	// list all revisions and sort them
	revisions, err := ssc.ListRevisions(set)
	if err != nil {
		return err
	}
	history.SortControllerRevisions(revisions)

	currentRevision, updateRevision, err := ssc.performUpdate(ctx, set, pods, revisions)
	if err != nil {
		return utilerrors.NewAggregate([]error{err, ssc.truncateHistory(set, pods, revisions, currentRevision, updateRevision)})
	}

	// maintain the set's revision history limit
	return ssc.truncateHistory(set, pods, revisions, currentRevision, updateRevision)
}

func (ssc *defaultStatefulSetControl) performUpdate(
	ctx context.Context, set *appsv1beta1.StatefulSet, pods []*v1.Pod, revisions []*apps.ControllerRevision) (*apps.ControllerRevision, *apps.ControllerRevision, error) {
	var currentStatus *appsv1beta1.StatefulSetStatus
	// get the current, and update revisions
	currentRevision, updateRevision, collisionCount, err := ssc.getStatefulSetRevisions(set, revisions)
	if err != nil {
		return currentRevision, updateRevision, err
	}

	// Refresh update expectations
	for _, pod := range pods {
		updateExpectations.ObserveUpdated(getStatefulSetKey(set), updateRevision.Name, pod)
	}

	// perform the main update function and get the status
	currentStatus, getStatusErr := ssc.updateStatefulSet(ctx, set, currentRevision, updateRevision, collisionCount, pods, revisions)
	if getStatusErr != nil && currentStatus == nil {
		return currentRevision, updateRevision, getStatusErr
	}

	// make sure to update the latest status even if there is an error with non-nil currentStatus
	updateStatusErr := ssc.updateStatefulSetStatus(ctx, set, currentStatus)
	if updateStatusErr == nil {
		klog.V(4).InfoS("Updated status", "statefulSet", klog.KObj(set),
			"replicas", currentStatus.Replicas,
			"readyReplicas", currentStatus.ReadyReplicas,
			"currentReplicas", currentStatus.CurrentReplicas,
			"updatedReplicas", currentStatus.UpdatedReplicas)
	}

	switch {
	case getStatusErr != nil && updateStatusErr != nil:
		klog.ErrorS(updateStatusErr, "Could not update status", "statefulSet", klog.KObj(set))
		return currentRevision, updateRevision, getStatusErr
	case getStatusErr != nil:
		return currentRevision, updateRevision, getStatusErr
	case updateStatusErr != nil:
		return currentRevision, updateRevision, updateStatusErr
	}

	klog.V(4).InfoS("StatefulSet revisions", "statefulSet", klog.KObj(set),
		"currentRevision", currentStatus.CurrentRevision,
		"updateRevision", currentStatus.UpdateRevision)

	return currentRevision, updateRevision, nil
}

func (ssc *defaultStatefulSetControl) ListRevisions(set *appsv1beta1.StatefulSet) ([]*apps.ControllerRevision, error) {
	selector, err := util.ValidatedLabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	return ssc.controllerHistory.ListControllerRevisions(set, selector)
}

func (ssc *defaultStatefulSetControl) AdoptOrphanRevisions(
	set *appsv1beta1.StatefulSet,
	revisions []*apps.ControllerRevision) error {
	for i := range revisions {
		adopted, err := ssc.controllerHistory.AdoptControllerRevision(set, controllerKind, revisions[i])
		if err != nil {
			return err
		}
		revisions[i] = adopted
	}
	return nil
}

// truncateHistory truncates any non-live ControllerRevisions in revisions from set's history. The UpdateRevision and
// CurrentRevision in set's Status are considered to be live. Any revisions associated with the Pods in pods are also
// considered to be live. Non-live revisions are deleted, starting with the revision with the lowest Revision, until
// only RevisionHistoryLimit revisions remain. If the returned error is nil the operation was successful. This method
// expects that revisions is sorted when supplied.
func (ssc *defaultStatefulSetControl) truncateHistory(
	set *appsv1beta1.StatefulSet,
	pods []*v1.Pod,
	revisions []*apps.ControllerRevision,
	current *apps.ControllerRevision,
	update *apps.ControllerRevision) error {
	history := make([]*apps.ControllerRevision, 0, len(revisions))
	// mark all live revisions
	live := map[string]bool{}
	if current != nil {
		live[current.Name] = true
	}
	if update != nil {
		live[update.Name] = true
	}
	for i := range pods {
		live[getPodRevision(pods[i])] = true
	}
	// collect live revisions and historic revisions
	for i := range revisions {
		if !live[revisions[i].Name] {
			history = append(history, revisions[i])
		}
	}
	historyLen := len(history)
	historyLimit := int(*set.Spec.RevisionHistoryLimit)
	if historyLen <= historyLimit {
		return nil
	}
	// delete any non-live history to maintain the revision limit.
	history = history[:(historyLen - historyLimit)]
	for i := 0; i < len(history); i++ {
		if err := ssc.controllerHistory.DeleteControllerRevision(history[i]); err != nil {
			return err
		}
	}
	return nil
}

// getStatefulSetRevisions returns the current and update ControllerRevisions for set. It also
// returns a collision count that records the number of name collisions set saw when creating
// new ControllerRevisions. This count is incremented on every name collision and is used in
// building the ControllerRevision names for name collision avoidance. This method may create
// a new revision, or modify the Revision of an existing revision if an update to set is detected.
// This method expects that revisions is sorted when supplied.
func (ssc *defaultStatefulSetControl) getStatefulSetRevisions(
	set *appsv1beta1.StatefulSet,
	revisions []*apps.ControllerRevision) (*apps.ControllerRevision, *apps.ControllerRevision, int32, error) {
	var currentRevision, updateRevision *apps.ControllerRevision

	revisionCount := len(revisions)
	history.SortControllerRevisions(revisions)

	// Use a local copy of set.Status.CollisionCount to avoid modifying set.Status directly.
	// This copy is returned so the value gets carried over to set.Status in updateStatefulSet.
	var collisionCount int32
	if set.Status.CollisionCount != nil {
		collisionCount = *set.Status.CollisionCount
	}

	// create a new revision from the current set
	updateRevision, err := newRevision(set, nextRevision(revisions), &collisionCount)
	if err != nil {
		return nil, nil, collisionCount, err
	}

	// find any equivalent revisions
	equalRevisions := history.FindEqualRevisions(revisions, updateRevision)
	equalCount := len(equalRevisions)

	if equalCount > 0 {
		if history.EqualRevision(revisions[revisionCount-1], equalRevisions[equalCount-1]) {
			// if the equivalent revision is immediately prior the update revision has not changed
			updateRevision = revisions[revisionCount-1]
		} else {
			// if the equivalent revision is not immediately prior we will roll back by incrementing the
			// Revision of the equivalent revision
			updateRevision, err = ssc.controllerHistory.UpdateControllerRevision(
				equalRevisions[equalCount-1],
				updateRevision.Revision)
			if err != nil {
				return nil, nil, collisionCount, err
			}
		}
	} else {
		//if there is no equivalent revision we create a new one
		updateRevision, err = ssc.controllerHistory.CreateControllerRevision(set, updateRevision, &collisionCount)
		if err != nil {
			return nil, nil, collisionCount, err
		}
	}

	// attempt to find the revision that corresponds to the current revision
	for i := range revisions {
		if revisions[i].Name == set.Status.CurrentRevision {
			currentRevision = revisions[i]
			break
		}
	}

	// if the current revision is nil we initialize the history by setting it to the update revision
	if currentRevision == nil {
		currentRevision = updateRevision
	}

	return currentRevision, updateRevision, collisionCount, nil
}

func (ssc *defaultStatefulSetControl) doPreDownload(set *appsv1beta1.StatefulSet, currentRevision, updateRevision *apps.ControllerRevision) {
	var err error
	if isPreDownloadDisabled || sigsruntimeClient == nil {
		return
	}
	if currentRevision.Name != updateRevision.Name {
		// get asts pre-download annotation
		minUpdatedReadyPodsCount := 0
		if minUpdatedReadyPods, ok := set.Annotations[appsv1alpha1.ImagePreDownloadMinUpdatedReadyPods]; ok {
			minUpdatedReadyPodsIntStr := intstrutil.Parse(minUpdatedReadyPods)
			minUpdatedReadyPodsCount, err = intstrutil.GetScaledValueFromIntOrPercent(&minUpdatedReadyPodsIntStr, int(*set.Spec.Replicas), true)
			if err != nil {
				klog.ErrorS(err, "Failed to GetScaledValueFromIntOrPercent of minUpdatedReadyPods for statefulSet", "statefulSet", klog.KObj(set))
			}
		}
		updatedReadyReplicas := set.Status.UpdatedReadyReplicas
		if updateRevision.Name != set.Status.UpdateRevision {
			updatedReadyReplicas = 0
		}
		if int32(minUpdatedReadyPodsCount) <= updatedReadyReplicas {
			// pre-download images for new revision
			if err := ssc.createImagePullJobsForInPlaceUpdate(set, currentRevision, updateRevision); err != nil {
				klog.ErrorS(err, "Failed to create ImagePullJobs for statefulSet", "statefulSet", klog.KObj(set))
			}
		}
	} else {
		// delete ImagePullJobs if revisions have been consistent
		if err := imagejobutilfunc.DeleteJobsForWorkload(sigsruntimeClient, set); err != nil {
			klog.ErrorS(err, "Failed to delete ImagePullJobs for statefulSet", "statefulSet", klog.KObj(set))
		}
	}
}

// updateStatefulSet performs the update function for a StatefulSet. This method creates, updates, and deletes Pods in
// the set in order to conform the system to the target state for the set. The target state always contains
// set.Spec.Replicas Pods with a Ready Condition. If the UpdateStrategy.Type for the set is
// RollingUpdateStatefulSetStrategyType then all Pods in the set must be at set.Status.CurrentRevision.
// If the UpdateStrategy.Type for the set is OnDeleteStatefulSetStrategyType, the target state implies nothing about
// the revisions of Pods in the set. If the UpdateStrategy.Type for the set is PartitionStatefulSetStrategyType, then
// all Pods with ordinal less than UpdateStrategy.Partition.Ordinal must be at Status.CurrentRevision and all other
// Pods must be at Status.UpdateRevision. If the returned error is nil, the returned StatefulSetStatus is valid and the
// update must be recorded. If the error is not nil, the method should be retried until successful.
func (ssc *defaultStatefulSetControl) updateStatefulSet(
	ctx context.Context,
	set *appsv1beta1.StatefulSet,
	currentRevision *apps.ControllerRevision,
	updateRevision *apps.ControllerRevision,
	collisionCount int32,
	pods []*v1.Pod,
	revisions []*apps.ControllerRevision) (*appsv1beta1.StatefulSetStatus, error) {
	selector, err := util.ValidatedLabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return set.Status.DeepCopy(), err
	}

	// get the current and update revisions of the set.
	currentSet, err := ApplyRevision(set, currentRevision)
	if err != nil {
		return set.Status.DeepCopy(), err
	}
	updateSet, err := ApplyRevision(set, updateRevision)
	if err != nil {
		return set.Status.DeepCopy(), err
	}

	ssc.doPreDownload(set, currentRevision, updateRevision)

	// set the generation, and revisions in the returned status
	status := appsv1beta1.StatefulSetStatus{}
	status.ObservedGeneration = set.Generation
	status.CurrentRevision = currentRevision.Name
	status.UpdateRevision = updateRevision.Name
	status.CollisionCount = utilpointer.Int32Ptr(collisionCount)
	status.LabelSelector = selector.String()
	minReadySeconds := getMinReadySeconds(set)

	ssc.updatePVCStatus(&status, set, pods)
	updateStatus(&status, minReadySeconds, currentRevision, updateRevision, pods)

	startOrdinal, endOrdinal, reserveOrdinals := getStatefulSetReplicasRange(set)
	// slice that will contain all Pods such that startOrdinal <= getOrdinal(pod) < endOrdinal and not in reserveOrdinals
	replicas := make([]*v1.Pod, endOrdinal-startOrdinal)
	// slice that will contain all Pods such that getOrdinal(pod) < startOrdinal or getOrdinal(pod) >= endOrdinal or in reserveOrdinals
	condemned := make([]*v1.Pod, 0, len(pods))
	unhealthy := 0
	firstUnhealthyOrdinal := math.MaxInt32
	var firstUnhealthyPod *v1.Pod
	monotonic := !allowsBurst(set)

	// First we partition pods into two lists valid replicas and condemned Pods
	for i := range pods {
		if ord := getOrdinal(pods[i]); podInOrdinalRangeWithParams(pods[i], startOrdinal, endOrdinal, reserveOrdinals) {
			// if the ordinal of the pod is within the range of the current number of replicas and not in reserveOrdinals,
			// insert it at the indirection of its ordinal
			replicas[ord-startOrdinal] = pods[i]

		} else if ord >= 0 {
			// if the ordinal is valid, but not within the range or in reserveOrdinals,
			// add it to the condemned list
			condemned = append(condemned, pods[i])
		}
		// If the ordinal could not be parsed (ord < 0), ignore the Pod.
	}

	// for any empty indices in the sequence [0,set.Spec.Replicas) create a new Pod at the correct revision
	for ord := startOrdinal; ord < endOrdinal; ord++ {
		if reserveOrdinals.Has(ord) {
			continue
		}
		replicaIdx := ord - startOrdinal
		if replicas[replicaIdx] == nil {
			replicas[replicaIdx] = newVersionedStatefulSetPod(
				currentSet,
				updateSet,
				currentRevision.Name,
				updateRevision.Name, ord, replicas)
		}
	}

	// sort the condemned Pods by their ordinals
	sort.Sort(descendingOrdinal(condemned))

	// find the first unhealthy Pod
	for i := range replicas {
		if replicas[i] == nil {
			continue
		}
		if !isHealthy(replicas[i]) {
			unhealthy++
			if ord := getOrdinal(replicas[i]); ord < firstUnhealthyOrdinal {
				firstUnhealthyOrdinal = ord
				firstUnhealthyPod = replicas[i]
			}
		}
	}

	// or the first unhealthy condemned Pod (condemned are sorted in descending order for ease of use)
	for i := len(condemned) - 1; i >= 0; i-- {
		if !isHealthy(condemned[i]) {
			unhealthy++
			if ord := getOrdinal(condemned[i]); ord < firstUnhealthyOrdinal {
				firstUnhealthyOrdinal = ord
				firstUnhealthyPod = condemned[i]
			}
		}
	}

	if unhealthy > 0 {
		klog.V(4).InfoS("StatefulSet has unhealthy Pods", "statefulSet", klog.KObj(set), "unhealthyReplicas", unhealthy, "pod", klog.KObj(firstUnhealthyPod))
	}

	// If the StatefulSet is being deleted, don't do anything other than updating
	// status.
	if set.DeletionTimestamp != nil {
		return &status, nil
	}

	// First, process each living replica. Exit if we run into an error or something blocking in monotonic mode.
	scaleMaxUnavailable, err := getScaleMaxUnavailable(set)
	if err != nil {
		return &status, err
	}
	processReplicaFn := func(i int) (bool, bool, error) {
		return ssc.processReplica(ctx, set, updateSet, monotonic, replicas, i, &status, scaleMaxUnavailable)
	}
	if shouldExit, err := runForAllWithBreak(replicas, processReplicaFn); shouldExit || err != nil {
		ssc.updatePVCStatus(&status, set, replicas)
		updateStatus(&status, minReadySeconds, currentRevision, updateRevision, replicas, condemned)
		return &status, err
	}

	if utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetAutoDeletePVC) {
		// Ensure ownerRefs are set correctly for the condemned pods.
		fixPodClaim := func(i int) (bool, error) {
			if matchPolicy, err := ssc.podControl.ClaimsMatchRetentionPolicy(updateSet, condemned[i]); err != nil {
				return true, err
			} else if !matchPolicy {
				if err := ssc.podControl.UpdatePodClaimForRetentionPolicy(updateSet, condemned[i]); err != nil {
					return true, err
				}
			}
			return false, nil
		}
		if shouldExit, err := runForAll(condemned, fixPodClaim, monotonic); shouldExit || err != nil {
			ssc.updatePVCStatus(&status, set, replicas)
			updateStatus(&status, minReadySeconds, currentRevision, updateRevision, replicas, condemned)
			return &status, err
		}
	}

	// At this point, in monotonic mode all of the current Replicas are Running, Ready and Available,
	// and we can consider termination.
	// We will wait for all predecessors to be Running and Ready prior to attempting a deletion.
	// We will terminate Pods in a monotonically decreasing order.
	// Note that we do not resurrect Pods in this interval. Also note that scaling will take precedence over
	// updates.
	processCondemnedFn := func(i int) (bool, error) {
		return ssc.processCondemned(ctx, set, firstUnhealthyPod, monotonic, condemned, i)
	}
	if shouldExit, err := runForAll(condemned, processCondemnedFn, monotonic); shouldExit || err != nil {
		ssc.updatePVCStatus(&status, set, replicas)
		updateStatus(&status, minReadySeconds, currentRevision, updateRevision, replicas, condemned)
		return &status, err
	}
	ssc.updatePVCStatus(&status, set, replicas)
	updateStatus(&status, minReadySeconds, currentRevision, updateRevision, replicas, condemned)

	// for the OnDelete strategy we short circuit. Pods will be updated when they are manually deleted.
	if set.Spec.UpdateStrategy.Type == apps.OnDeleteStatefulSetStrategyType {
		return &status, nil
	}

	return ssc.rollingUpdateStatefulsetPods(
		set, &status, currentRevision, updateRevision, revisions, pods, replicas, minReadySeconds,
	)
}

func (ssc *defaultStatefulSetControl) rollingUpdateStatefulsetPods(
	set *appsv1beta1.StatefulSet,
	status *appsv1beta1.StatefulSetStatus,
	currentRevision *apps.ControllerRevision,
	updateRevision *apps.ControllerRevision,
	revisions []*apps.ControllerRevision,
	pods []*v1.Pod,
	replicas []*v1.Pod,
	minReadySeconds int32,
) (*appsv1beta1.StatefulSetStatus, error) {

	// If update expectations have not satisfied yet, skip updating pods
	if updateSatisfied, _, updateDirtyPods := updateExpectations.SatisfiedExpectations(getStatefulSetKey(set), updateRevision.Name); !updateSatisfied {
		klog.V(4).InfoS("Not satisfied update for statefulSet", "statefulSet", klog.KObj(set), "updateDirtyPods", updateDirtyPods)
		return status, nil
	}

	// refresh states for all pods
	var modified bool
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		refreshed, duration, err := ssc.refreshPodState(set, pod, updateRevision.Name)
		if err != nil {
			return status, err
		} else if duration > 0 {
			durationStore.Push(getStatefulSetKey(set), duration)
		}
		if refreshed {
			modified = true
		}
	}
	if modified {
		return status, nil
	}

	var err error
	// we compute the minimum ordinal of the target sequence for a destructive update based on the strategy.
	maxUnavailable := 1
	if set.Spec.UpdateStrategy.RollingUpdate != nil {
		if set.Spec.UpdateStrategy.RollingUpdate.Paused {
			return status, nil
		}

		maxUnavailable, err = intstrutil.GetValueFromIntOrPercent(intstrutil.ValueOrDefault(set.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable, intstrutil.FromInt(1)), int(*set.Spec.Replicas), false)
		if err != nil {
			return status, err
		}
		// maxUnavailable should not be less than 1
		if maxUnavailable < 1 {
			maxUnavailable = 1
		}
	}

	minWaitTime := appsv1beta1.MaxMinReadySeconds * time.Second
	unavailablePods := sets.NewString()
	opts := &inplaceupdate.UpdateOptions{}
	opts = inplaceupdate.SetOptionsDefaults(opts)
	// counts any targets in the replicas that are unhealthy (terminated / in-place update not-ready / not running and
	// ready for minReadySeconds) for checking if the count satisfied the MaxUnavailable limit.
	for target := range replicas {
		if replicas[target] == nil {
			continue
		}
		if !isHealthy(replicas[target]) {
			// 1. count pod as unavailable if it's unhealthy or terminating
			unavailablePods.Insert(replicas[target].Name)
		} else if completedErr := opts.CheckPodUpdateCompleted(replicas[target]); completedErr != nil {
			// 2. count pod as unavailable if it's in-place updating and not ready
			klog.V(4).ErrorS(completedErr, "StatefulSet found Pod in-place update not-ready",
				"statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[target]))
			unavailablePods.Insert(replicas[target].Name)
		} else if isAvailable, waitTime := isRunningAndAvailable(replicas[target], minReadySeconds); !isAvailable {
			// 3. count pod as unavailable if it's not available yet given the minReadySeconds requirement
			unavailablePods.Insert(replicas[target].Name)
			// make sure that we will wait for the first pod to get available
			if waitTime != 0 && waitTime <= minWaitTime {
				minWaitTime = waitTime
				durationStore.Push(getStatefulSetKey(set), waitTime)
			}
		} else if utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetAutoResizePVCGate) &&
			set.Spec.VolumeClaimUpdateStrategy.Type == appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType {
			// check pvc resize status, if not ready, record pod to unavailablePods
			ready, err := ssc.podControl.IsOwnedPVCsReady(set, replicas[target])
			if err == nil && ready {
				continue
			}
			if err != nil {
				klog.V(4).ErrorS(err, "StatefulSet check owned pvcs unready",
					"statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[target]))
			}
			klog.V(4).InfoS("owned pvcs unready, added in unavailable Pods",
				"statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[target]))
			unavailablePods.Insert(replicas[target].Name)
		}
	}

	// handle specified deleted pod under maxUnavailable constrain
	// NOTE: specified deletion is not constraint by partition setting
	specifiedDeletedPods, err := ssc.handleSpecifiedDeletedPods(set, status, currentRevision, updateRevision, replicas, maxUnavailable, unavailablePods)
	if err != nil {
		return status, err
	}

	updateIndexes := sortPodsToUpdate(set.Spec.UpdateStrategy.RollingUpdate, updateRevision.Name, *set.Spec.Replicas, replicas)
	klog.V(3).InfoS("Prepare to update pods indexes for StatefulSet", "statefulSet", klog.KObj(set), "podIndexes", updateIndexes)
	// update pods in sequence
	for _, target := range updateIndexes {
		var pvcMatched bool = true
		if utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetAutoResizePVCGate) &&
			set.Spec.VolumeClaimUpdateStrategy.Type == appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType {
			if pvcMatched, err = ssc.podControl.IsClaimsCompatible(set, replicas[target]); err != nil {
				return status, err
			}
		}

		// the target is already up-to-date, go to next
		if getPodRevision(replicas[target]) == updateRevision.Name && pvcMatched {
			continue
		}

		// the unavailable pods count exceed the maxUnavailable and the target is available, so we can't process it,
		// wait for unhealthy Pods on update
		if len(unavailablePods) >= maxUnavailable && !unavailablePods.Has(replicas[target].Name) {
			klog.V(4).InfoS("StatefulSet was waiting for unavailable Pods to update, blocked pod",
				"statefulSet", klog.KObj(set), "unavailablePods", unavailablePods.List(), "blockedPod", klog.KObj(replicas[target]))
			return status, nil
		}

		// Kruise currently will not patch pvc size until a pod references the resized volume.
		// online-file-system-expansion: if no pods referencing the volume are running, file system expansion will not happen.
		// refer to https://kubernetes.io/blog/2018/07/12/resizing-persistent-volumes-using-kubernetes/#online-file-system-expansion
		if utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetAutoResizePVCGate) &&
			set.Spec.VolumeClaimUpdateStrategy.Type == appsv1beta1.OnPodRollingUpdateVolumeClaimUpdateStrategyType {
			// resize pvc if necessary and wait for resize completed
			if !pvcMatched {
				err = ssc.podControl.TryPatchPVC(set, replicas[target])
				if err != nil {
					return status, err
				}
			}

			allCompleted, err := ssc.podControl.IsOwnedPVCsCompleted(set, replicas[target])
			if err != nil {
				return status, err
			} else if !allCompleted {
				// mark target as unavailable because pvc's updated
				unavailablePods.Insert(replicas[target].Name)
				// need to wait for pvc resize completed, continue to handle next pod
				continue
			}
		}

		// delete the Pod if it is not already terminating and does not match the update revision.
		if !specifiedDeletedPods.Has(replicas[target].Name) && !isTerminating(replicas[target]) {
			// todo validate in-place for pub
			inplacing, inplaceUpdateErr := ssc.inPlaceUpdatePod(set, replicas[target], updateRevision, revisions)
			if inplaceUpdateErr != nil {
				return status, inplaceUpdateErr
			}
			// if pod is inplacing or actual deleting, decrease revision
			revisionNeedDecrease := inplacing
			if !inplacing {
				klog.V(2).InfoS("StatefulSet terminating Pod for update", "statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[target]))
				if _, actualDeleting, err := ssc.deletePod(set, replicas[target]); err != nil {
					return status, err
				} else {
					revisionNeedDecrease = actualDeleting
				}
			}
			// mark target as unavailable because it's updated
			unavailablePods.Insert(replicas[target].Name)

			if revisionNeedDecrease && getPodRevision(replicas[target]) == currentRevision.Name {
				status.CurrentReplicas--
			}
		}
	}

	return status, nil
}

func (ssc *defaultStatefulSetControl) handleSpecifiedDeletedPods(
	set *appsv1beta1.StatefulSet,
	status *appsv1beta1.StatefulSetStatus,
	currentRevision *apps.ControllerRevision,
	updateRevision *apps.ControllerRevision,
	replicas []*v1.Pod,
	maxUnavailable int,
	unavailablePods sets.String) (sets.String, error) {
	specifiedDeletedPods := sets.NewString()
	for target := len(replicas) - 1; target >= 0; target-- {
		if replicas[target] == nil || !specifieddelete.IsSpecifiedDelete(replicas[target]) {
			continue
		}
		// the unavailable pods count exceed the maxUnavailable and the target is available, so we can't process it,
		// why skip here rather than return?
		// case: pod 0 ready, pod1 unready, pod 2 unready, pod3 ready, pod4 ready
		// when maxUnavailable = 3, pod4 with specified deleted will be deleted but pod3 can't
		// pod 2 and pod 1 can be deleted because they were unavailable
		if len(unavailablePods) >= maxUnavailable && !unavailablePods.Has(replicas[target].Name) {
			klog.V(4).InfoS("StatefulSet was waiting for unavailable Pods to update, blocked pod",
				"statefulSet", klog.KObj(set), "unavailablePods", unavailablePods.List(), "blockedPod", klog.KObj(replicas[target]))
			continue
		}

		specifiedDeletedPods.Insert(replicas[target].Name)
		if _, actualDeleting, err := ssc.deletePod(set, replicas[target]); err != nil {
			return specifiedDeletedPods, err
		} else if actualDeleting {
			// if actual deleted, update revision count in status
			if getPodRevision(replicas[target]) == currentRevision.Name {
				status.CurrentReplicas--
			} else if getPodRevision(replicas[target]) == updateRevision.Name {
				status.UpdatedReplicas--
			}
		}
		// mark target as unavailable because it's deleting or pre-deleting
		unavailablePods.Insert(replicas[target].Name)
	}
	return specifiedDeletedPods, nil
}

func (ssc *defaultStatefulSetControl) deletePod(set *appsv1beta1.StatefulSet, pod *v1.Pod) (modified, actualDeleting bool, err error) {
	if set.Spec.Lifecycle != nil && lifecycle.IsPodHooked(set.Spec.Lifecycle.PreDelete, pod) {
		markPodNotReady := set.Spec.Lifecycle.PreDelete.MarkPodNotReady
		if updated, _, err := ssc.lifecycleControl.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingDelete, markPodNotReady); err != nil {
			return false, false, err
		} else if updated {
			klog.V(3).InfoS("StatefulSet scaling update pod lifecycle to PreparingDelete", "statefulSet", klog.KObj(set), "pod", klog.KObj(pod))
			return true, false, nil
		}
		return false, false, nil
	}
	if err := ssc.podControl.DeleteStatefulPod(set, pod); err != nil {
		ssc.recorder.Eventf(set, v1.EventTypeWarning, "FailedDelete", "failed to delete pod %s: %v", pod.Name, err)
		return false, false, err
	}
	return true, true, nil
}

func (ssc *defaultStatefulSetControl) refreshPodState(set *appsv1beta1.StatefulSet, pod *v1.Pod, updateRevision string) (bool, time.Duration, error) {
	opts := &inplaceupdate.UpdateOptions{}
	if set.Spec.UpdateStrategy.RollingUpdate != nil && set.Spec.UpdateStrategy.RollingUpdate.InPlaceUpdateStrategy != nil {
		opts.GracePeriodSeconds = set.Spec.UpdateStrategy.RollingUpdate.InPlaceUpdateStrategy.GracePeriodSeconds
	}
	opts = inplaceupdate.SetOptionsDefaults(opts)

	res := ssc.inplaceControl.Refresh(pod, opts)
	if res.RefreshErr != nil {
		klog.ErrorS(res.RefreshErr, "AdvancedStatefulSet failed to update pod condition for inplace",
			"statefulSet", klog.KObj(set), "pod", klog.KObj(pod))
		return false, 0, res.RefreshErr
	}

	var state appspub.LifecycleStateType
	switch lifecycle.GetPodLifecycleState(pod) {
	case appspub.LifecycleStatePreparingNormal:
		if set.Spec.Lifecycle == nil ||
			set.Spec.Lifecycle.PreNormal == nil ||
			lifecycle.IsPodAllHooked(set.Spec.Lifecycle.PreNormal, pod) {
			state = appspub.LifecycleStateNormal
		}
	case appspub.LifecycleStatePreparingUpdate:
		// when pod updated to PreparingUpdate state to wait lifecycle blocker to remove,
		// then rollback, do not need update pod inplace since it is the update revision,
		// so just update pod lifecycle state. ref: https://github.com/openkruise/kruise/issues/1156
		if getPodRevision(pod) == updateRevision {
			if set.Spec.Lifecycle != nil && !lifecycle.IsPodAllHooked(set.Spec.Lifecycle.InPlaceUpdate, pod) {
				state = appspub.LifecycleStateUpdated
			} else {
				state = appspub.LifecycleStateNormal
			}
		}
	case appspub.LifecycleStateUpdating:
		if opts.CheckPodUpdateCompleted(pod) == nil {
			if set.Spec.Lifecycle != nil && !lifecycle.IsPodAllHooked(set.Spec.Lifecycle.InPlaceUpdate, pod) {
				state = appspub.LifecycleStateUpdated
			} else {
				state = appspub.LifecycleStateNormal
			}
		}
	case appspub.LifecycleStateUpdated:
		if set.Spec.Lifecycle == nil ||
			set.Spec.Lifecycle.InPlaceUpdate == nil ||
			lifecycle.IsPodAllHooked(set.Spec.Lifecycle.InPlaceUpdate, pod) {
			state = appspub.LifecycleStateNormal
		}
	}

	if state != "" {
		var markPodNotReady bool
		if set.Spec.Lifecycle != nil && set.Spec.Lifecycle.InPlaceUpdate != nil {
			markPodNotReady = set.Spec.Lifecycle.InPlaceUpdate.MarkPodNotReady
		}
		if updated, _, err := ssc.lifecycleControl.UpdatePodLifecycle(pod, state, markPodNotReady); err != nil {
			return false, 0, err
		} else if updated {
			klog.V(3).InfoS("AdvancedStatefulSet updated pod lifecycle", "statefulSet", klog.KObj(set), "pod", klog.KObj(pod), "lifecycleState", state)
			return true, res.DelayDuration, nil
		}
	}

	return false, res.DelayDuration, nil
}

func (ssc *defaultStatefulSetControl) inPlaceUpdatePod(
	set *appsv1beta1.StatefulSet, pod *v1.Pod,
	updateRevision *apps.ControllerRevision, revisions []*apps.ControllerRevision,
) (bool, error) {
	if set.Spec.UpdateStrategy.RollingUpdate == nil {
		return false, nil
	}
	if set.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy != appsv1beta1.InPlaceIfPossiblePodUpdateStrategyType &&
		set.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy != appsv1beta1.InPlaceOnlyPodUpdateStrategyType {
		return false, nil
	}

	var oldRevision *apps.ControllerRevision
	for _, r := range revisions {
		if r.Name == getPodRevision(pod) {
			oldRevision = r
			break
		}
	}

	opts := &inplaceupdate.UpdateOptions{}
	if set.Spec.UpdateStrategy.RollingUpdate.InPlaceUpdateStrategy != nil {
		opts.GracePeriodSeconds = set.Spec.UpdateStrategy.RollingUpdate.InPlaceUpdateStrategy.GracePeriodSeconds
	}

	if ssc.inplaceControl.CanUpdateInPlace(oldRevision, updateRevision, opts) {
		state := lifecycle.GetPodLifecycleState(pod)
		switch state {
		case "", appspub.LifecycleStatePreparingNormal, appspub.LifecycleStateNormal:
			var err error
			var updated bool
			if set.Spec.Lifecycle != nil && lifecycle.IsPodHooked(set.Spec.Lifecycle.InPlaceUpdate, pod) {
				markPodNotReady := set.Spec.Lifecycle.InPlaceUpdate.MarkPodNotReady
				if updated, _, err = ssc.lifecycleControl.UpdatePodLifecycle(pod, appspub.LifecycleStatePreparingUpdate, markPodNotReady); err == nil && updated {
					klog.V(3).InfoS("StatefulSet updated pod lifecycle to PreparingUpdate", "statefulSet", klog.KObj(set), "pod", klog.KObj(pod))
				}
				return true, err
			}
		case appspub.LifecycleStateUpdated:
			var err error
			var updated bool
			var inPlaceUpdateHandler *appspub.LifecycleHook
			if set.Spec.Lifecycle != nil {
				inPlaceUpdateHandler = set.Spec.Lifecycle.InPlaceUpdate
			}
			if updated, _, err = ssc.lifecycleControl.UpdatePodLifecycleWithHandler(pod, appspub.LifecycleStatePreparingUpdate, inPlaceUpdateHandler); err == nil && updated {
				klog.V(3).InfoS("StatefulSet updated pod lifecycle to PreparingUpdate", "statefulSet", klog.KObj(set), "pod", klog.KObj(pod))
			}
			return true, err
		case appspub.LifecycleStatePreparingUpdate:
			if set.Spec.Lifecycle != nil && lifecycle.IsPodHooked(set.Spec.Lifecycle.InPlaceUpdate, pod) {
				return true, nil
			}
		case appspub.LifecycleStateUpdating:
		default:
			return true, fmt.Errorf("not allowed to in-place update pod %s in state %s", pod.Name, state)
		}

		if state != "" {
			opts.AdditionalFuncs = append(opts.AdditionalFuncs, lifecycle.SetPodLifecycle(appspub.LifecycleStateUpdating))
		}
		res := ssc.inplaceControl.Update(pod, oldRevision, updateRevision, opts)
		if res.InPlaceUpdate {
			if res.DelayDuration > 0 {
				durationStore.Push(getStatefulSetKey(set), res.DelayDuration)
			}

			if res.UpdateErr == nil {
				updateExpectations.ExpectUpdated(getStatefulSetKey(set), updateRevision.Name, pod)
				ssc.recorder.Eventf(set, v1.EventTypeNormal, "SuccessfulUpdatePodInPlace", "successfully update pod %s in-place(revision %v)", pod.Name, updateRevision.Name)
				return res.InPlaceUpdate, nil
			}
			ssc.recorder.Eventf(set, v1.EventTypeWarning, "FailedUpdatePodInPlace", "failed to update pod %s in-place(revision %v): %v", pod.Name, updateRevision.Name, res.UpdateErr)
			return res.InPlaceUpdate, res.UpdateErr
		}
	}

	if set.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy == appsv1beta1.InPlaceOnlyPodUpdateStrategyType {
		return false, fmt.Errorf("find strategy is InPlaceOnly but Pod %s can not update in-place", pod.Name)
	}

	return false, nil
}

// updateStatefulSetStatus updates set's Status to be equal to status. If status indicates a complete update, it is
// mutated to indicate completion. If status is semantically equivalent to set's Status no update is performed. If the
// returned error is nil, the update is successful.
func (ssc *defaultStatefulSetControl) updateStatefulSetStatus(
	ctx context.Context,
	set *appsv1beta1.StatefulSet,
	status *appsv1beta1.StatefulSetStatus) error {

	// complete any in progress rolling update if necessary
	completeRollingUpdate(set, status)

	// if the status is not inconsistent do not perform an update
	if !inconsistentStatus(set, status) {
		return nil
	}

	// copy set and update its status
	set = set.DeepCopy()
	if err := ssc.statusUpdater.UpdateStatefulSetStatus(ctx, set, status); err != nil {
		return err
	}

	return nil
}

type replicaStatus struct {
	replicas          int32
	readyReplicas     int32
	availableReplicas int32
	currentReplicas   int32

	updatedReplicas          int32
	updatedReadyReplicas     int32
	updatedAvailableReplicas int32
}

func computeReplicaStatus(pods []*v1.Pod, minReadySeconds int32, currentRevision, updateRevision *apps.ControllerRevision) replicaStatus {
	status := replicaStatus{}
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		if isCreated(pod) {
			status.replicas++
		}

		// count the number of running and ready replicas
		if isRunningAndReady(pod) {
			status.readyReplicas++
			if getPodRevision(pod) == updateRevision.Name {
				status.updatedReadyReplicas++
				if avail, _ := isRunningAndAvailable(pod, minReadySeconds); avail {
					status.updatedAvailableReplicas++
				}
			}
			// count the number of running and available replicas
			ok, _ := isRunningAndAvailable(pod, minReadySeconds)
			if ok {
				status.availableReplicas++
			}
		}

		// count the number of current and update replicas
		if isCreated(pod) && !isTerminating(pod) {
			revision := getPodRevision(pod)
			if revision == currentRevision.Name {
				status.currentReplicas++
			}
			if revision == updateRevision.Name {
				status.updatedReplicas++
			}
		}
	}
	return status
}

func updateStatus(status *appsv1beta1.StatefulSetStatus, minReadySeconds int32, currentRevision, updateRevision *apps.ControllerRevision, podLists ...[]*v1.Pod) {
	status.Replicas = 0
	status.ReadyReplicas = 0
	status.AvailableReplicas = 0
	status.CurrentReplicas = 0
	status.UpdatedReplicas = 0
	status.UpdatedReadyReplicas = 0
	status.UpdatedAvailableReplicas = 0
	for _, list := range podLists {
		replicaStatus := computeReplicaStatus(list, minReadySeconds, currentRevision, updateRevision)
		status.Replicas += replicaStatus.replicas
		status.ReadyReplicas += replicaStatus.readyReplicas
		status.AvailableReplicas += replicaStatus.availableReplicas
		status.UpdatedReadyReplicas += replicaStatus.updatedReadyReplicas
		status.UpdatedAvailableReplicas += replicaStatus.updatedAvailableReplicas
		status.CurrentReplicas += replicaStatus.currentReplicas
		status.UpdatedReplicas += replicaStatus.updatedReplicas
	}
}

// getStartOrdinal gets the first possible ordinal (inclusive).
// Returns spec.ordinals.start if spec.ordinals is set, otherwise returns 0.
func getStartOrdinal(set *appsv1beta1.StatefulSet) int {
	if utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetStartOrdinal) {
		if set.Spec.Ordinals != nil {
			return int(set.Spec.Ordinals.Start)
		}
	}
	return 0
}

func (ssc *defaultStatefulSetControl) processCondemned(ctx context.Context, set *appsv1beta1.StatefulSet, firstUnhealthyPod *v1.Pod, monotonic bool, condemned []*v1.Pod, i int) (bool, error) {
	logger := klog.FromContext(ctx)
	if isTerminating(condemned[i]) {
		// if we are in monotonic mode, block and wait for terminating pods to expire
		if monotonic {
			logger.V(4).Info("StatefulSet is waiting for Pod to Terminate prior to scale down",
				"statefulSet", klog.KObj(set), "pod", klog.KObj(condemned[i]))
			return true, nil
		}
		return false, nil
	}
	// if we are in monotonic mode and the condemned target is not the first unhealthy Pod block
	if !isRunningAndReady(condemned[i]) && monotonic && condemned[i] != firstUnhealthyPod {
		logger.V(4).Info("StatefulSet is waiting for Pod to be Running and Ready prior to scale down",
			"statefulSet", klog.KObj(set), "pod", klog.KObj(firstUnhealthyPod))
		return true, nil
	}
	// if we are in monotonic mode and the condemned target is not the first unhealthy Pod, block.
	if avail, waitTime := isRunningAndAvailable(condemned[i], getMinReadySeconds(set)); !avail && monotonic && condemned[i] != firstUnhealthyPod {
		logger.V(4).Info("StatefulSet is waiting for Pod to be Available prior to scale down",
			"statefulSet", klog.KObj(set), "pod", klog.KObj(firstUnhealthyPod))
		if waitTime > 0 {
			durationStore.Push(getStatefulSetKey(condemned[i]), waitTime)
		}
		return true, nil
	}

	logger.V(2).Info("Pod of StatefulSet is terminating for scale down",
		"statefulSet", klog.KObj(set), "pod", klog.KObj(condemned[i]))

	modified, _, err := ssc.deletePod(set, condemned[i])
	if err != nil || (monotonic && modified) {
		return true, err
	}
	return false, nil
}

// processReplica handles an individual replica within a StatefulSet based on its current state.
// It decides whether to delete, create, update the replica, or await its readiness.
//
// different from stateful set:
//
//		If decreaseAndCheckMaxUnavailable(scaleMaxUnavailable) returns true,
//		break the pod for-loop and proceed with the update logic,
//		which will apply in-place conditions to make the pod ready.
//	 For example: update unhealthy pods or add some conditions
//
// Returns:
// - bool shouldExit: whether to exit.
// - bool shouldBreak: whether to break the pod for-loop and proceed with the update logic.
// - An error if encountered during processing; nil otherwise.
func (ssc *defaultStatefulSetControl) processReplica(
	ctx context.Context,
	set *appsv1beta1.StatefulSet,
	updateSet *appsv1beta1.StatefulSet,
	monotonic bool,
	replicas []*v1.Pod,
	i int,
	status *appsv1beta1.StatefulSetStatus,
	scaleMaxUnavailable *int) (bool, bool, error) {
	minReadySeconds := getMinReadySeconds(set)
	logger := klog.FromContext(ctx)

	if replicas[i] == nil {
		return false, false, nil
	}
	// Note that pods with phase Succeeded will also trigger this event. This is
	// because final pod phase of evicted or otherwise forcibly stopped pods
	// (e.g. terminated on node reboot) is determined by the exit code of the
	// container, not by the reason for pod termination. We should restart the pod
	// regardless of the exit code.
	if isFailed(replicas[i]) || isSucceeded(replicas[i]) {
		if replicas[i].DeletionTimestamp == nil {
			if _, _, err := ssc.deletePod(set, replicas[i]); err != nil {
				return true, false, err
			}
		}
		// New pod should be generated on the next sync after the current pod is removed from etcd.
		return true, false, nil
	}
	// If we find a Pod that has not been created we create the Pod
	if !isCreated(replicas[i]) {
		if utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetAutoDeletePVC) {
			if isStale, err := ssc.podControl.PodClaimIsStale(set, replicas[i]); err != nil {
				return true, false, err
			} else if isStale {
				// If a pod has a stale PVC, no more work can be done this round.
				return true, false, err
			}
		}
		// asts ut invoke updateStatefulset once by once,
		// so we can update pod into normal state to avoid changing so many ut cases
		state := appspub.LifecycleStatePreparingNormal
		if set.Spec.Lifecycle == nil ||
			set.Spec.Lifecycle.PreNormal == nil ||
			lifecycle.IsPodAllHooked(set.Spec.Lifecycle.PreNormal, replicas[i]) {
			state = appspub.LifecycleStateNormal
		}
		lifecycle.SetPodLifecycle(state)(replicas[i])
		if err := ssc.podControl.CreateStatefulPod(ctx, set, replicas[i]); err != nil {
			msg := fmt.Sprintf("StatefulPodControl failed to create Pod error: %s", err)
			condition := NewStatefulsetCondition(appsv1beta1.FailedCreatePod, v1.ConditionTrue, "", msg)
			SetStatefulsetCondition(status, condition)
			return true, false, err
		}
		if monotonic {
			// if the set does not allow bursting, return immediately
			return true, false, nil
		} else if decreaseAndCheckMaxUnavailable(scaleMaxUnavailable) {
			logger.V(4).Info(
				"StatefulSet pod is Creating, and break pods scale",
				"statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[i]))
			return false, true, nil
		}
		// pod created, no more work possible for this round
		return false, false, nil
	}

	// If the Pod is in pending state then trigger PVC creation to create missing PVCs
	if isPending(replicas[i]) {
		logger.V(4).Info(
			"StatefulSet is triggering PVC creation for pending Pod",
			"statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[i]))
		if err := ssc.podControl.createMissingPersistentVolumeClaims(ctx, set, replicas[i]); err != nil {
			return true, false, err
		}
	}

	// If we find a Pod that is currently terminating, we must wait until graceful deletion
	// completes before we continue to make progress.
	if isTerminating(replicas[i]) && monotonic {
		logger.V(4).Info("StatefulSet is waiting for Pod to Terminate",
			"statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[i]))
		return true, false, nil
	} else if isTerminating(replicas[i]) && decreaseAndCheckMaxUnavailable(scaleMaxUnavailable) {
		logger.V(4).Info(
			"StatefulSet pod is Terminating, and break pods scale",
			"statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[i]))
		return false, true, nil
	}

	// Update InPlaceUpdateReady condition for pod
	_, duration, err := ssc.refreshPodState(set, replicas[i], status.UpdateRevision)
	if err != nil {
		logger.Error(err, "StatefulSet failed to update pod condition for inplace",
			"statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[i]))
		return true, false, err
	} else if duration > 0 {
		durationStore.Push(getStatefulSetKey(set), duration)
	}

	// If we have a Pod that has been created but is not running and available we can not make progress.
	// We must ensure that all for each Pod, when we create it, all of its predecessors, with respect to its
	// ordinal, are Running and Available.
	if monotonic || scaleMaxUnavailable != nil {
		isAvailable, waitTime := isRunningAndAvailable(replicas[i], minReadySeconds)
		if !isAvailable && monotonic {
			if waitTime > 0 {
				// make sure we check later
				durationStore.Push(getStatefulSetKey(set), waitTime)
				logger.V(4).Info(
					"StatefulSet needs to wait for the pod to be Running and Available after being"+
						" Ready for minReadySeconds", "statefulSet", klog.KObj(set), "waitTime", waitTime,
					"pod", klog.KObj(replicas[i]), "minReadySeconds", minReadySeconds)
			} else {
				logger.V(4).Info("StatefulSet is waiting for Pod to be Available",
					"statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[i]))
			}
			return true, false, nil
		} else if !isAvailable && decreaseAndCheckMaxUnavailable(scaleMaxUnavailable) {
			logger.V(4).Info(
				"StatefulSet pod is unavailable, and break pods scale",
				"statefulSet", klog.KObj(set), "pod", klog.KObj(replicas[i]))
			if waitTime > 0 {
				// make sure we check later
				durationStore.Push(getStatefulSetKey(set), waitTime)
			}
			return false, true, nil
		}
	}

	// Enforce the StatefulSet invariants
	retentionMatch := true
	if utilfeature.DefaultFeatureGate.Enabled(features.StatefulSetAutoDeletePVC) {
		var err error
		retentionMatch, err = ssc.podControl.ClaimsMatchRetentionPolicy(updateSet, replicas[i])
		// An error is expected if the pod is not yet fully updated, and so return is treated as matching.
		if err != nil {
			retentionMatch = true
		}
	}

	if identityMatches(set, replicas[i]) && storageMatches(set, replicas[i]) && retentionMatch {
		return false, false, nil
	}

	// Make a deep copy so we don't mutate the shared cache
	replica := replicas[i].DeepCopy()
	if err := ssc.podControl.UpdateStatefulPod(updateSet, replica); err != nil {
		msg := fmt.Sprintf("StatefulPodControl failed to update Pod error: %s", err)
		condition := NewStatefulsetCondition(appsv1beta1.FailedUpdatePod, v1.ConditionTrue, "", msg)
		SetStatefulsetCondition(status, condition)
		return true, false, err
	}

	return false, false, nil
}

func slowStartBatch(initialBatchSize int, remaining int, fn func(int) (bool, error)) (int, error) {
	successes := 0
	j := 0
	for batchSize := integer.IntMin(remaining, initialBatchSize); batchSize > 0; batchSize = integer.IntMin(integer.IntMin(2*batchSize, remaining), MaxBatchSize) {
		errCh := make(chan error, batchSize)
		var wg sync.WaitGroup
		wg.Add(batchSize)
		for i := 0; i < batchSize; i++ {
			go func(k int) {
				defer wg.Done()
				// Ignore the first parameter - relevant for monotonic only.
				if _, err := fn(k); err != nil {
					errCh <- err
				}
			}(j)
			j++
		}
		wg.Wait()
		successes += batchSize - len(errCh)
		close(errCh)
		if len(errCh) > 0 {
			errs := make([]error, 0)
			for err := range errCh {
				errs = append(errs, err)
			}
			return successes, utilerrors.NewAggregate(errs)
		}
		remaining -= batchSize
	}
	return successes, nil
}

// runForAllWithBreak iterates through all pod objects, applying the given function until a specified condition is met.
// The function can decide whether to continue, break out of the loop, or return an error.
// Parameters:
// - pods: An array of pointers to Pod objects, representing the collection of pods to be processed.
// - fn: A function that takes an index as a parameter and returns three values:
//  1. A boolean indicating whether to exit the current iteration.
//  2. A boolean indicating whether to break out of the loop.
//  3. An error object, in case an error occurs during function execution.
//
// Returns:
// - A boolean indicating whether an exit condition was met or an error occurred during iteration.
// - An error object, if an error was encountered during the execution of the provided function.
func runForAllWithBreak(pods []*v1.Pod, fn func(i int) (bool, bool, error)) (bool, error) {
	for i := range pods {
		if shouldExit, shouldBreak, err := fn(i); shouldExit || err != nil {
			return true, err
		} else if shouldBreak {
			//Introduce this branch to exit the for-loop while proceeding with subsequent update logic.
			break
		}
	}
	return false, nil
}

func runForAll(pods []*v1.Pod, fn func(i int) (bool, error), monotonic bool) (bool, error) {
	if monotonic {
		for i := range pods {
			if shouldExit, err := fn(i); shouldExit || err != nil {
				return true, err
			}
		}
	} else {
		if _, err := slowStartBatch(1, len(pods), fn); err != nil {
			return true, err
		}
	}
	return false, nil
}

func getScaleMaxUnavailable(set *appsv1beta1.StatefulSet) (*int, error) {
	var scaleMaxUnavailable *int
	if set.Spec.ScaleStrategy != nil && set.Spec.ScaleStrategy.MaxUnavailable != nil {
		maxUnavailable, err := intstrutil.GetValueFromIntOrPercent(set.Spec.ScaleStrategy.MaxUnavailable, int(*set.Spec.Replicas), false)
		if err != nil {
			return scaleMaxUnavailable, err
		}
		// maxUnavailable should not be less than 1
		if maxUnavailable < 1 {
			maxUnavailable = 1
		}
		scaleMaxUnavailable = &maxUnavailable
	}
	return scaleMaxUnavailable, nil
}
