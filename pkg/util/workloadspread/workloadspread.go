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

package workloadspread

import (
	"context"
	"encoding/json"
	"math"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kubeClient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/util"
)

const (
	// MatchedWorkloadSpreadSubsetAnnotations matched pod workloadSpread
	MatchedWorkloadSpreadSubsetAnnotations = "apps.kruise.io/matched-workloadspread"

	PodDeletionCostAnnotation = "controller.kubernetes.io/pod-deletion-cost"

	PodDeletionCostPositive = 100
	PodDeletionCostNegative = -100
)

var (
	controllerKruiseKindCS = appsv1alpha1.SchemeGroupVersion.WithKind("CloneSet")
	controllerKindRS       = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	controllerKindDep      = appsv1.SchemeGroupVersion.WithKind("Deployment")
	controllerKindJob      = batchv1.SchemeGroupVersion.WithKind("Job")
)

type Operation string

const (
	CreateOperation   Operation = "Create"
	DeleteOperation   Operation = "Delete"
	EvictionOperation Operation = "Eviction"
)

type workload struct {
	Kind   string
	Groups []string
}

var (
	workloads = []workload{
		{Kind: controllerKruiseKindCS.Kind, Groups: []string{controllerKruiseKindCS.Group}},
		{Kind: controllerKindRS.Kind, Groups: []string{controllerKindRS.Group}},
		{Kind: controllerKindJob.Kind, Groups: []string{controllerKindJob.Group}},
	}
)

type Handler struct {
	client.Client
}

func NewWorkloadSpreadHandler(c client.Client) *Handler {
	return &Handler{Client: c}
}

type InjectWorkloadSpread struct {
	// matched WorkloadSpread.Name
	Name string `json:"name"`
	// Subset.Name
	Subset string `json:"subset"`
	// generate id if the Pod's name is nil.
	UID string `json:"uid,omitempty"`
}

func VerifyGroupKind(ref interface{}, expectedKind string, expectedGroups []string) (bool, error) {
	var gv schema.GroupVersion
	var kind string
	var err error

	switch ref.(type) {
	case *appsv1alpha1.TargetReference:
		gv, err = schema.ParseGroupVersion(ref.(*appsv1alpha1.TargetReference).APIVersion)
		if err != nil {
			klog.Errorf("failed to parse GroupVersion for apiVersion (%s): %s", ref.(*appsv1alpha1.TargetReference).APIVersion, err.Error())
			return false, err
		}
		kind = ref.(*appsv1alpha1.TargetReference).Kind
	case *metav1.OwnerReference:
		gv, err = schema.ParseGroupVersion(ref.(*metav1.OwnerReference).APIVersion)
		if err != nil {
			klog.Errorf("failed to parse GroupVersion for apiVersion (%s): %s", ref.(*metav1.OwnerReference).APIVersion, err.Error())
			return false, err
		}
		kind = ref.(*metav1.OwnerReference).Kind
	default:
		return false, nil
	}

	if kind != expectedKind {
		return false, nil
	}

	for _, group := range expectedGroups {
		if group == gv.Group {
			return true, nil
		}
	}

	return false, nil
}

// matchReference return true if Pod has ownerReference matched workloads.
func matchReference(ref *metav1.OwnerReference) (bool, error) {
	if ref == nil {
		return false, nil
	}
	for _, wl := range workloads {
		matched, err := VerifyGroupKind(ref, wl.Kind, wl.Groups)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

// TODO consider pod/status update operation

func (h *Handler) HandlePodCreation(pod *corev1.Pod) error {
	start := time.Now()

	// filter out pods, include the following:
	// 1. Deletion pod
	// 2. Pod.Status.Phase = Succeeded or Failed
	// 3. Pod.OwnerReference is nil
	// 4. Pod.OwnerReference is not one of workloads, such as CloneSet, Deployment, ReplicaSet.
	if !kubecontroller.IsPodActive(pod) {
		return nil
	}
	ref := metav1.GetControllerOf(pod)
	matched, err := matchReference(ref)
	if err != nil || !matched {
		return nil
	}

	var matchedWS *appsv1alpha1.WorkloadSpread
	workloadSpreadList := &appsv1alpha1.WorkloadSpreadList{}
	if err = h.Client.List(context.TODO(), workloadSpreadList, &client.ListOptions{Namespace: pod.Namespace}); err != nil {
		return err
	}
	for _, ws := range workloadSpreadList.Items {
		if ws.Spec.TargetReference == nil || !ws.DeletionTimestamp.IsZero() {
			continue
		}
		// determine if the reference of workloadSpread and pod is equal
		if h.isReferenceEqual(ws.Spec.TargetReference, pod) {
			matchedWS = &ws
			// pod has at most one matched workloadSpread
			break
		}
	}
	// not found matched workloadSpread
	if matchedWS == nil {
		return nil
	}

	defer func() {
		klog.V(3).Infof("Cost of handling Pod(%v/%v) creation by WorkloadSpread (%s/%s) is %v",
			pod.Namespace, pod.Name, matchedWS.Namespace, matchedWS.Name, time.Since(start))
	}()

	return h.mutatingPod(matchedWS, pod, nil, CreateOperation)
}

func (h *Handler) HandlePodDeletion(pod *corev1.Pod, operation Operation) error {
	start := time.Now()

	var injectWS *InjectWorkloadSpread
	str, ok := pod.Annotations[MatchedWorkloadSpreadSubsetAnnotations]
	if !ok || str == "" {
		return nil
	}
	err := json.Unmarshal([]byte(str), &injectWS)
	if err != nil {
		klog.Errorf("parse Pod (%s/%s) annotations[%s]=%s failed: %s", pod.Namespace, pod.Name,
			MatchedWorkloadSpreadSubsetAnnotations, str, err.Error())
		return nil
	}

	// filter out pods, include the following:
	// 1. DeletionTimestamp is not nil
	// 2. Pod.Status.Phase = Succeeded or Failed
	// 3. Pod.OwnerReference is nil
	if injectWS == nil || !kubecontroller.IsPodActive(pod) || metav1.GetControllerOf(pod) == nil {
		return nil
	}

	matchedWS := &appsv1alpha1.WorkloadSpread{}
	err = h.Client.Get(context.TODO(), client.ObjectKey{Namespace: pod.Namespace, Name: injectWS.Name}, matchedWS)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Warningf("Pod(%s/%s) matched WorkloadSpread(%s) Not Found", pod.Namespace, pod.Name, injectWS.Name)
			return nil
		}
		klog.Errorf("get pod(%s/%s) matched workloadSpread(%s) failed: %s", pod.Namespace, pod.Name, injectWS.Name, err.Error())
		return err
	}

	defer func() {
		klog.V(3).Infof("Cost of handling pod deletion by WorkloadSpread (%s/%s) is %v",
			matchedWS.Namespace, matchedWS.Name, time.Since(start))
	}()

	return h.mutatingPod(matchedWS, pod, injectWS, operation)
}

func (h *Handler) mutatingPod(matchedWS *appsv1alpha1.WorkloadSpread,
	pod *corev1.Pod,
	injectWS *InjectWorkloadSpread,
	operation Operation) error {
	podName := pod.Name
	if podName == "" {
		podName = pod.GetGenerateName()
	}

	klog.V(3).Infof("Operation[%s] Pod(%s/%s) matched WorkloadSpread(%s/%s)", operation, pod.Namespace, podName, matchedWS.Namespace, matchedWS.Name)

	suitableSubsetName, generatedUID, err := h.acquireSuitableSubset(matchedWS, pod, injectWS, operation)
	if err != nil {
		return err
	}

	var injectErr error
	// if create pod, inject affinity、toleration、metadata in pod object
	if operation == CreateOperation && len(suitableSubsetName) > 0 {
		if _, injectErr = injectWorkloadSpreadIntoPod(matchedWS, pod, suitableSubsetName, generatedUID); injectErr != nil {
			klog.Errorf("failed to inject Pod(%s/%s) subset(%s) data for WorkloadSpread(%s/%s)",
				pod.Namespace, podName, suitableSubsetName, matchedWS.Namespace, matchedWS.Name)
			return injectErr
		}
		klog.V(3).Infof("inject Pod(%s/%s) subset(%s) data for WorkloadSpread(%s/%s)",
			pod.Namespace, podName, suitableSubsetName, matchedWS.Namespace, matchedWS.Name)
	}

	klog.V(3).Infof("handle operation[%s] Pod(%s/%s) generatedUID(%s) for WorkloadSpread(%s/%s) done",
		operation, pod.Namespace, podName, generatedUID, matchedWS.Namespace, matchedWS.Name)

	return injectErr
}

func (h *Handler) acquireSuitableSubset(matchedWS *appsv1alpha1.WorkloadSpread,
	pod *corev1.Pod,
	injectWS *InjectWorkloadSpread,
	operation Operation) (string, string, error) {
	if len(matchedWS.Spec.Subsets) == 1 &&
		matchedWS.Spec.Subsets[0].MaxReplicas == nil {
		return matchedWS.Spec.Subsets[0].Name, "", nil
	}

	var refresh, changed bool
	var wsClone *appsv1alpha1.WorkloadSpread
	var suitableSubset *appsv1alpha1.WorkloadSpreadSubsetStatus
	var generatedUID, suitableSubsetName string

	// for debug
	var conflictTimes int
	var costOfGet, costOfUpdate time.Duration

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		unlock := util.GlobalKeyedMutex.Lock(string(matchedWS.GetUID()))
		defer unlock()

		var err error
		startGet := time.Now()
		// try best to get the latest revision of matchedWS in a low cost:
		// 1. get two matchedWS, one is cached by this webhook process,
		// another is cached by informer, compare and get the newer one;
		// 2. if 1. failed, directly fetch matchedWS from APIServer;
		wsClone, err = h.tryToGetTheLatestMatchedWS(matchedWS, refresh)
		costOfGet += time.Since(startGet)
		if err != nil {
			return err
		} else if wsClone == nil {
			return nil
		}

		// check whether WorkloadSpread has suitable subset for the pod
		// 1. changed indicates whether workloadSpread status changed
		// 2. suitableSubset is matched subset for the pod
		changed, suitableSubset, generatedUID, err = h.updateSubsetForPod(wsClone, pod, injectWS, operation)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}

		// update WorkloadSpread status
		start := time.Now()
		if err = h.Client.Status().Update(context.TODO(), wsClone); err != nil {
			refresh = true
			conflictTimes++
		} else {
			klog.V(3).Infof("update workloadSpread(%s/%s) SubsetStatus(%s) missingReplicas(%d) creatingPods(%d) deletingPods(%d) success",
				wsClone.Namespace, wsClone.Name, suitableSubset.Name,
				suitableSubset.MissingReplicas, len(suitableSubset.CreatingPods), len(suitableSubset.DeletingPods))
			if cacheErr := util.GlobalCache.Add(wsClone); cacheErr != nil {
				klog.Warningf("Failed to update workloadSpread(%s/%s) cache after update status, err: %v", wsClone.Namespace, wsClone.Name, cacheErr)
			}
		}
		costOfUpdate += time.Since(start)
		return err
	}); err != nil {
		klog.Errorf("update WorkloadSpread(%s/%s) error %s", matchedWS.Namespace, matchedWS.Name, err.Error())
		return "", "", err
	}

	if suitableSubset != nil {
		suitableSubsetName = suitableSubset.Name
	}

	klog.V(5).Infof("Cost of assigning suitable subset of WorkloadSpread (%s %s) for pod is: conflict times: %v, cost of Get %v, cost of Update %v",
		matchedWS.Namespace, matchedWS.Name, conflictTimes, costOfGet, costOfUpdate)

	return suitableSubsetName, generatedUID, nil
}

func (h *Handler) tryToGetTheLatestMatchedWS(matchedWS *appsv1alpha1.WorkloadSpread, refresh bool) (
	*appsv1alpha1.WorkloadSpread, error) {
	var err error
	var wsClone *appsv1alpha1.WorkloadSpread

	if refresh {
		// TODO: shall we set metav1.GetOptions{resourceVersion="0"} so that we get the cached object in apiServer memory instead of etcd?
		wsClone, err = kubeClient.GetGenericClient().KruiseClient.AppsV1alpha1().
			WorkloadSpreads(matchedWS.Namespace).Get(context.TODO(), matchedWS.Name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, nil
			}
			klog.Errorf("error getting updated WorkloadSpread(%s/%s) from APIServer, err: %v", matchedWS.Namespace, matchedWS.Name, err)
			return nil, err
		}
	} else {
		item, _, cacheErr := util.GlobalCache.Get(matchedWS)
		if cacheErr != nil {
			klog.Errorf("Failed to get cached WorkloadSpread(%s/%s) from GlobalCache, err: %v", matchedWS.Namespace, matchedWS.Name, cacheErr)
		}
		if localCachedWS, ok := item.(*appsv1alpha1.WorkloadSpread); ok {
			wsClone = localCachedWS.DeepCopy()
		} else {
			wsClone = matchedWS.DeepCopy()
		}

		// compare and use the newer version
		informerCachedWS := &appsv1alpha1.WorkloadSpread{}
		if err = h.Get(context.TODO(), types.NamespacedName{Namespace: matchedWS.Namespace,
			Name: matchedWS.Name}, informerCachedWS); err == nil {
			// TODO: shall we process the case of that the ResourceVersion exceeds MaxInt64?
			var localRV, informerRV int64
			_ = runtime.Convert_string_To_int64(&wsClone.ResourceVersion, &localRV, nil)
			_ = runtime.Convert_string_To_int64(&informerCachedWS.ResourceVersion, &informerRV, nil)
			if localRV < informerRV {
				wsClone = informerCachedWS
			}
		}
	}

	return wsClone, nil
}

// handleWorkloadChange handle the update event of workload in case that webhook cannot wait for controller to be
// done. In this function, if some errors occurred, we will regard pod as the latest, and continue to follow the
// MissingReplicas(original) logic.
func (h *Handler) handleWorkloadChange(ws *appsv1alpha1.WorkloadSpread, pod *corev1.Pod, operation Operation) (bool, int32, error) {
	switch operation {
	case CreateOperation, EvictionOperation, DeleteOperation:
	default:
		return false, -1, nil
	}

	var workloadReplicas int32 = 0
	switch ws.Spec.TargetReference.Kind {
	// Deployment Handler
	case controllerKindDep.Kind:
		// If the pod is managed by Deployment, 'pod-template-hash' label should not be empty.
		podTemplateHash, ok := pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey]
		if !ok {
			return false, -1, nil
		}

		// try to get the deployment from informer cache
		// TODO: we should consider how to solve informer latency problem
		deployment := &appsv1.Deployment{}
		depKey := types.NamespacedName{Name: ws.Spec.TargetReference.Name, Namespace: ws.Namespace}
		if err := h.Client.Get(context.TODO(), depKey, deployment); client.IgnoreNotFound(err) != nil {
			return false, -1, err
		} else if errors.IsNotFound(err) {
			// In creation case, this pod most likely is the latest revision, because this deployment cache is not synced by our informer, how fresh is this pod!
			// In deletion/eviction case, if this deployment is not found, the pod most likely is dangling pod, we can ignore this case, because the workload has gone!
			// Actually, in most cases, this condition will not be true, because we have returned in HandlePodCreation func if deployment is not found.
			klog.Warningf("WorkloadSpread(%v/%v) cannot find Deployment when handle Pod(%v) create", ws.Namespace, ws.Name, pod.Name)
		}

		// deployment is not deleted and is cached by this webhook informer
		if deployment != nil {
			if deployment.Spec.Replicas != nil {
				workloadReplicas = *deployment.Spec.Replicas
			} else {
				// deployment default replicas is 1
				workloadReplicas = 1
			}

			replicaset := h.GetOwnerReplicaSetFor(pod)
			if replicaset != nil &&
				// make sure it is not the case that the replicaset is cached before deployment
				IsReplicaSetRevisionLessThanOrEqualToDeployment(replicaset, deployment) &&
				!util.EqualIgnoreHash(&deployment.Spec.Template, &replicaset.Spec.Template) {
				return false, workloadReplicas, nil
			}

			// we check the existence of replicaset to address informer cache latency problem, because if the
			// replicaset is not found when creating, the pod most likely is the latest revision.
			if replicaset == nil && operation != CreateOperation {
				return false, workloadReplicas, nil
			}
		}

		// If workloadSpread doesn't observe this updatedRevision yet, then refresh the workloadSpread status.
		// But, we are only interested in creation event to refresh the status.
		if ws.Status.ObservedWorkloadRevision != podTemplateHash && operation == CreateOperation {
			// TODO: In case of workload not found, if use percentages as MaxSurge, we cannot reset the status
			newStatus := resetStatus(ws, int(workloadReplicas), podTemplateHash)
			if newStatus == nil {
				return true, workloadReplicas, nil
			}
			klog.Warningf("WorkloadSpread(%v/%v) observed deployment revision changed: %v --> %v, and reset status %+v -> %+v",
				ws.Namespace, ws.Name, ws.Status.ObservedWorkloadRevision, podTemplateHash, ws.Status, *newStatus)
			ws.Status = *newStatus
		}

	case controllerKruiseKindCS.Kind:
		// TODO: support CloneSet RollingUpdate & MaxSurge
	}

	return true, workloadReplicas, nil
}

func (h *Handler) GetOwnerReplicaSetFor(pod *corev1.Pod) *appsv1.ReplicaSet {
	owner := metav1.GetControllerOfNoCopy(pod)
	if owner == nil || owner.Kind != controllerKindRS.Kind {
		return nil
	}
	ownerKey := types.NamespacedName{
		Name:      owner.Name,
		Namespace: pod.GetNamespace(),
	}

	replicaset := &appsv1.ReplicaSet{}
	if err := h.Get(context.TODO(), ownerKey, replicaset); err == nil {
		return replicaset
	}
	return nil
}

// resetStatus will reset some fields of status, including:
// 1. ObservedWorkloadRevision: latestRevision,
// 2. Each SubsetStatus: MissingReplicas: MaxReplicas, CreatingPods: nil, DeletingPods: nil
func resetStatus(ws *appsv1alpha1.WorkloadSpread, replicas int, latestRevision string) *appsv1alpha1.WorkloadSpreadStatus {
	newStatus := &appsv1alpha1.WorkloadSpreadStatus{}
	newStatus.ObservedWorkloadRevision = latestRevision
	newStatus.ObservedGeneration = ws.Status.ObservedGeneration

	for i := range ws.Spec.Subsets {
		subset := &ws.Spec.Subsets[i]

		// get the current subset status, make new one if the subset status is not initialized
		var subsetStatus *appsv1alpha1.WorkloadSpreadSubsetStatus
		if len(ws.Status.SubsetStatuses) < i+1 || ws.Status.SubsetStatuses[i].Name != subset.Name {
			subsetStatus = &appsv1alpha1.WorkloadSpreadSubsetStatus{
				Name:            subset.Name,
				MissingReplicas: -1,
			}
		} else {
			subsetStatus = ws.Status.SubsetStatuses[i].DeepCopy()
		}

		// just ignore if subset max replicas is nil, because reset an unlimited subset is meaningless
		if subset.MaxReplicas == nil {
			newStatus.SubsetStatuses = append(newStatus.SubsetStatuses, *subsetStatus)
			continue
		}

		subsetMaxReplicas, err := intstr.GetValueFromIntOrPercent(subset.MaxReplicas, replicas, true)
		if err != nil || subsetMaxReplicas < 0 {
			klog.Errorf("Failed to parse maxReplicas value from subset (%s) of WorkloadSpread (%s/%s)",
				subset.Name, ws.Namespace, ws.Name)
			return nil
		}

		subsetStatus.CreatingPods = nil
		subsetStatus.DeletingPods = nil
		subsetStatus.MissingReplicas = int32(subsetMaxReplicas)
		newStatus.SubsetStatuses = append(newStatus.SubsetStatuses, *subsetStatus)
	}

	return newStatus
}

// return three parameters:
// 1. changed(bool) indicates if workloadSpread.Status has changed
// 2. suitableSubset(*struct{}) indicates which workloadSpread.Subset does this pod match
// 3. generatedUID(types.UID) indicates which workloadSpread generate a UID for identifying Pod without a full name.
func (h *Handler) updateSubsetForPod(ws *appsv1alpha1.WorkloadSpread,
	pod *corev1.Pod, injectWS *InjectWorkloadSpread, operation Operation) (
	bool, *appsv1alpha1.WorkloadSpreadSubsetStatus, string, error) {
	var suitableSubset *appsv1alpha1.WorkloadSpreadSubsetStatus
	var generatedUID string

	// This is for solving rollingUpdate problems in Deployment.
	// Here, if Deployment changed, we will only care about the
	// pods with the latest revision, otherwise we will fail to
	// count the subset missingReplicas because the latest pods
	// may be created before the deletion of the old pods.
	// When Deployment revision changed, webhook cannot wait for
	// controller to reconcile, thus, we have to reset some fields
	// for subsetStatuses in webhook instead of controller.
	// Due to the guarantees of the lock and etcd, we can ensure
	// subsetStatus cannot be reset repeatedly in the concurrent
	// scenarios. The following codes will be retried if conflict
	// occurred.
	isPodNewRevision, replicas, err := h.handleWorkloadChange(ws, pod, operation)
	if err != nil {
		klog.Errorf("Failed to handle the workload change at webhook, WorkloadSpread(%s/%s), error: %v",
			ws.Namespace, ws.Name, err)
		return false, nil, "", err
	}

	switch operation {
	case CreateOperation:
		if pod.Name != "" {
			// pod is already in CreatingPods/DeletingPods List, then return
			if isRecord, subset := isPodRecordedInSubset(ws, pod.Name); isRecord {
				return false, subset, "", nil
			}
		}

		if isPodNewRevision {
			// Use MissingReplicas to decide the suitable subsets.
			suitableSubset = h.getSuitableSubsetForNewRevision(ws)
		} else {
			// Use MaxReplicas-Replicas to decide the suitable subset.
			// Actually, we do not care about the pod if it is not the
			// latest, because it is not the final-state pod. But,this
			// function can make the pod distribution more uniform and
			// reasonable.
			// At present, only Deployment pod with old revisions will
			// follow this logic.
			suitableSubset = h.getSuitableSubsetForOldRevision(ws, replicas)
		}

		if suitableSubset == nil {
			klog.V(5).Infof("WorkloadSpread (%s/%s) don't have a suitable subset for Pod (%s)",
				ws.Namespace, ws.Name, pod.Name)
			return false, nil, "", nil
		}

		// subsetStatus will be reset if `latest` is not true,
		// so just ignore the following calculation logic.
		if !isPodNewRevision {
			return true, suitableSubset, generatedUID, nil
		}

		if suitableSubset.CreatingPods == nil {
			suitableSubset.CreatingPods = map[string]metav1.Time{}
		}
		if pod.Name != "" {
			suitableSubset.CreatingPods[pod.Name] = metav1.Time{Time: time.Now()}
		} else {
			// pod.Name is "" means that the Pod does not have a full name, but has a generated name during the mutating phase.
			// We generate a uid to identify this Pod.
			generatedUID = string(uuid.NewUUID())
			suitableSubset.CreatingPods[generatedUID] = metav1.Time{Time: time.Now()}
		}
		if suitableSubset.MissingReplicas > 0 {
			suitableSubset.MissingReplicas--
		}

	case DeleteOperation, EvictionOperation:
		// pod is already in DeletingPods/CreatingPods List, then return
		if isRecord, _ := isPodRecordedInSubset(ws, pod.Name); isRecord {
			return false, nil, "", nil
		}

		suitableSubset = getSpecificSubset(ws, injectWS.Subset)
		if suitableSubset == nil {
			klog.V(5).Infof("Pod (%s/%s) matched WorkloadSpread (%s) not found Subset(%s)", ws.Namespace, pod.Name, ws.Name, injectWS.Subset)
			return false, nil, "", nil
		}

		// subsetStatus will be reset if `latest` is not true,
		// so just ignore the following calculation logic.
		if !isPodNewRevision {
			return false, suitableSubset, generatedUID, nil
		}

		if suitableSubset.DeletingPods == nil {
			suitableSubset.DeletingPods = map[string]metav1.Time{}
		}
		suitableSubset.DeletingPods[pod.Name] = metav1.Time{Time: time.Now()}
		if suitableSubset.MissingReplicas >= 0 {
			suitableSubset.MissingReplicas++
		}

	default:
		return false, nil, "", nil
	}

	// update subset status
	for i := range ws.Status.SubsetStatuses {
		if ws.Status.SubsetStatuses[i].Name == suitableSubset.Name {
			ws.Status.SubsetStatuses[i] = *suitableSubset
			break
		}
	}

	return true, suitableSubset, generatedUID, nil
}

// return two parameters
// 1. isRecord(bool) 2. SubsetStatus
func isPodRecordedInSubset(ws *appsv1alpha1.WorkloadSpread, podName string) (bool, *appsv1alpha1.WorkloadSpreadSubsetStatus) {
	for _, subset := range ws.Status.SubsetStatuses {
		if _, ok := subset.CreatingPods[podName]; ok {
			return true, &subset
		}
		if _, ok := subset.DeletingPods[podName]; ok {
			return true, &subset
		}
	}
	return false, nil
}

func injectWorkloadSpreadIntoPod(ws *appsv1alpha1.WorkloadSpread, pod *corev1.Pod, subsetName string, generatedUID string) (bool, error) {
	var subset *appsv1alpha1.WorkloadSpreadSubset
	for _, object := range ws.Spec.Subsets {
		if subsetName == object.Name {
			subset = &object
			break
		}
	}
	if subset == nil {
		return false, nil
	}

	// inject toleration
	if len(subset.Tolerations) > 0 {
		pod.Spec.Tolerations = append(pod.Spec.Tolerations, subset.Tolerations...)
	}
	if pod.Spec.Affinity == nil {
		pod.Spec.Affinity = &corev1.Affinity{}
	}
	if pod.Spec.Affinity.NodeAffinity == nil {
		pod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	if len(subset.PreferredNodeSelectorTerms) > 0 {
		pod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution = append(pod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution,
			subset.PreferredNodeSelectorTerms...)
	}
	if subset.RequiredNodeSelectorTerm != nil {
		if pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
		}
		if len(pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
			pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = []corev1.NodeSelectorTerm{
				*subset.RequiredNodeSelectorTerm,
			}
		} else {
			for i := range pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
				selectorTerm := &pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[i]
				selectorTerm.MatchExpressions = append(selectorTerm.MatchExpressions, subset.RequiredNodeSelectorTerm.MatchExpressions...)
				selectorTerm.MatchFields = append(selectorTerm.MatchFields, subset.RequiredNodeSelectorTerm.MatchFields...)
			}
		}
	}
	if subset.Patch.Raw != nil {
		cloneBytes, _ := json.Marshal(pod)
		modified, err := strategicpatch.StrategicMergePatch(cloneBytes, subset.Patch.Raw, &corev1.Pod{})
		if err != nil {
			klog.Errorf("failed to merge patch raw %s", subset.Patch.Raw)
			return false, err
		}
		newPod := &corev1.Pod{}
		if err = json.Unmarshal(modified, newPod); err != nil {
			klog.Errorf("failed to unmarshal %s to Pod", modified)
			return false, err
		}
		*pod = *newPod
	}

	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}

	injectWS := &InjectWorkloadSpread{
		Name:   ws.Name,
		Subset: subsetName,
		UID:    generatedUID,
	}
	by, _ := json.Marshal(injectWS)
	pod.Annotations[MatchedWorkloadSpreadSubsetAnnotations] = string(by)
	return true, nil
}

func getSpecificSubset(ws *appsv1alpha1.WorkloadSpread, specifySubset string) *appsv1alpha1.WorkloadSpreadSubsetStatus {
	for _, subset := range ws.Status.SubsetStatuses {
		if specifySubset == subset.Name {
			return &subset
		}
	}
	return nil
}

func getSubsetMaxReplicas(ws *appsv1alpha1.WorkloadSpread, subsetName string) (*intstr.IntOrString, bool) {
	for i := range ws.Spec.Subsets {
		subset := &ws.Spec.Subsets[i]
		if subset.Name == subsetName {
			return subset.MaxReplicas, true
		}
	}
	return nil, false
}

func (h *Handler) getSuitableSubsetForNewRevision(ws *appsv1alpha1.WorkloadSpread) *appsv1alpha1.WorkloadSpreadSubsetStatus {
	for i := range ws.Status.SubsetStatuses {
		subset := &ws.Status.SubsetStatuses[i]
		canSchedule := true
		for _, condition := range subset.Conditions {
			if condition.Type == appsv1alpha1.SubsetSchedulable && condition.Status == corev1.ConditionFalse {
				canSchedule = false
				break
			}
		}

		if canSchedule && (subset.MissingReplicas > 0 || subset.MissingReplicas == -1) {
			// TODO simulation schedule
			// scheduleStrategy.Type = Adaptive
			// Webhook will simulate a schedule in order to check whether Pod can run in this subset,
			// which does a generic predicates by the cache of nodes and pods in kruise manager.
			// There may be some errors between simulation schedule and kubernetes scheduler with small probability.

			return subset
		}
	}

	return nil
}

func (h *Handler) getSuitableSubsetForOldRevision(ws *appsv1alpha1.WorkloadSpread, replicas int32) *appsv1alpha1.WorkloadSpreadSubsetStatus {
	subsetPriority := make([]int32, len(ws.Status.SubsetStatuses))

	for i := range ws.Status.SubsetStatuses {
		subsetStatus := &ws.Status.SubsetStatuses[i]
		usable := true
		for _, condition := range subsetStatus.Conditions {
			if condition.Type == appsv1alpha1.SubsetSchedulable && condition.Status == corev1.ConditionFalse {
				usable = false
				break
			}
		}

		var err error
		var subsetMaxReplicas int
		if usable {
			maxReplicas, found := getSubsetMaxReplicas(ws, subsetStatus.Name)
			if !found {
				usable = false
			}

			subsetMaxReplicas, err = intstr.GetValueFromIntOrPercent(
				intstr.ValueOrDefault(maxReplicas, intstr.FromInt(math.MaxInt32)), int(replicas), true)
			if err != nil {
				usable = false
				klog.Warningf("WorkloadSpread(%v %v) subset(%v) MaxReplicas cannot be parsed, error: %v",
					ws.Namespace, ws.Name, subsetStatus.Name, err)
			}
		}

		// calculate subset priority
		if usable {
			subsetPriority[i] = int32(subsetMaxReplicas) - subsetStatus.Replicas
		} else {
			subsetPriority[i] = math.MinInt32
		}
	}

	// select the subset with max priority
	suitableSubsetIndex := 0
	for i := range ws.Status.SubsetStatuses {
		if subsetPriority[suitableSubsetIndex] > subsetPriority[i] {
			suitableSubsetIndex = i
		}
	}

	// in case that all the subsets are not usable
	if subsetPriority[suitableSubsetIndex] == math.MinInt32 {
		return nil
	}

	return &ws.Status.SubsetStatuses[suitableSubsetIndex]
}

func (h Handler) isReferenceEqual(target *appsv1alpha1.TargetReference, pod *corev1.Pod) bool {
	namespace := pod.GetNamespace()
	owner := metav1.GetControllerOfNoCopy(pod)
	targetGv, err := schema.ParseGroupVersion(target.APIVersion)
	if err != nil {
		klog.Errorf("parse TargetReference apiVersion (%s) failed: %s", target.APIVersion, err.Error())
		return false
	}

	var ownerGv schema.GroupVersion
	if target.Kind == controllerKindDep.Kind {
		rs := &appsv1.ReplicaSet{}
		err = h.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: owner.Name}, rs)
		switch {
		case client.IgnoreNotFound(err) != nil:
			return false

		// address informer cache latency problem, we directly fetch the deployment instead of replicaSet.
		// TODO: hanle the case that the deployment is not found due to informer cache latency. In fact, all types of workloads face this problem too.
		case errors.IsNotFound(err):
			deploymentName := ParseDeploymentNameFrom(pod)
			if deploymentName != "" {
				deployment := &appsv1.Deployment{}
				deploymentKey := types.NamespacedName{Name: deploymentName, Namespace: namespace}
				return h.Get(context.TODO(), deploymentKey, deployment) == nil
			}
			return false

		default:
			if rs.UID != owner.UID {
				return false
			}

			owner = metav1.GetControllerOf(rs)
			if owner == nil {
				return false
			}
			ok, err := VerifyGroupKind(owner, controllerKindDep.Kind, []string{controllerKindDep.Group})
			if !ok || err != nil {
				return false
			}
		}
	}
	ownerGv, err = schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		klog.Errorf("parse OwnerReference apiVersion (%s) failed: %s", owner.APIVersion, err.Error())
		return false
	}

	return targetGv.Group == ownerGv.Group && target.Kind == owner.Kind && target.Name == owner.Name
}

func ParseDeploymentNameFrom(pod *corev1.Pod) string {
	owner := metav1.GetControllerOfNoCopy(pod)
	if owner == nil || owner.Kind != controllerKindRS.Kind {
		return ""
	}

	podTemplateHash := pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey]
	if podTemplateHash == "" {
		return ""
	}

	deploymentName := owner.Name[:len(owner.Name)-len(podTemplateHash)-1]
	klog.V(5).Infof("parsed deployment-name: %v, replicaset-name: %v, pod-template-hash: %v.",
		deploymentName, owner.Name, podTemplateHash)

	return deploymentName
}

func IsReplicaSetRevisionLessThanOrEqualToDeployment(replicaset *appsv1.ReplicaSet, deployment *appsv1.Deployment) bool {
	var rsRevision, dmRevision int64
	rsRevisionStr := replicaset.Annotations[appsv1.ControllerRevisionHashLabelKey]
	dmRevisionStr := deployment.Annotations[appsv1.ControllerRevisionHashLabelKey]
	_ = runtime.Convert_string_To_int64(&rsRevisionStr, &rsRevision, nil)
	_ = runtime.Convert_string_To_int64(&dmRevisionStr, &dmRevision, nil)
	return rsRevision <= dmRevision
}
