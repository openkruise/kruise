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
	"regexp"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kubeClient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/configuration"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	// MatchedWorkloadSpreadSubsetAnnotations matched pod workloadSpread
	MatchedWorkloadSpreadSubsetAnnotations = "apps.kruise.io/matched-workloadspread"

	PodDeletionCostAnnotation = "controller.kubernetes.io/pod-deletion-cost"

	PodDeletionCostPositive = 100
	PodDeletionCostNegative = -100
)

var (
	controllerKruiseKindCS       = appsv1alpha1.SchemeGroupVersion.WithKind("CloneSet")
	controllerKruiseKindAlphaSts = appsv1alpha1.SchemeGroupVersion.WithKind("StatefulSet")
	controllerKruiseKindBetaSts  = appsv1beta1.SchemeGroupVersion.WithKind("StatefulSet")
	controllerKindJob            = batchv1.SchemeGroupVersion.WithKind("Job")
	controllerKindRS             = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	controllerKindDep            = appsv1.SchemeGroupVersion.WithKind("Deployment")
	controllerKindSts            = appsv1.SchemeGroupVersion.WithKind("StatefulSet")
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
		{Kind: controllerKindDep.Kind, Groups: []string{controllerKindDep.Group}},
		{Kind: controllerKruiseKindCS.Kind, Groups: []string{controllerKruiseKindCS.Group}},
		{Kind: controllerKindRS.Kind, Groups: []string{controllerKindRS.Group}},
		{Kind: controllerKindJob.Kind, Groups: []string{controllerKindJob.Group}},
		{Kind: controllerKindSts.Kind, Groups: []string{controllerKindSts.Group, controllerKruiseKindAlphaSts.Group, controllerKruiseKindBetaSts.Group}},
	}
	workloadsInWhiteListInitialized = false
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

func (h *Handler) HandlePodCreation(pod *corev1.Pod) (skip bool, err error) {
	start := time.Now()

	// filter out pods, include the following:
	// 1. Deletion pod
	// 2. Pod.Status.Phase = Succeeded or Failed
	// 3. Pod.OwnerReference is nil
	// 4. Pod.OwnerReference is not one of workloads, such as CloneSet, Deployment, ReplicaSet.
	if !kubecontroller.IsPodActive(pod) {
		return true, nil
	}
	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return true, nil
	}

	initializeWorkloadsInWhiteList(h.Client)
	matched, err := matchReference(ref)
	if err != nil || !matched {
		return true, nil
	}

	var matchedWS *appsv1alpha1.WorkloadSpread
	workloadSpreadList := &appsv1alpha1.WorkloadSpreadList{}
	if err = h.Client.List(context.TODO(), workloadSpreadList, &client.ListOptions{Namespace: pod.Namespace}); err != nil {
		return false, err
	}
	for _, ws := range workloadSpreadList.Items {
		if ws.Spec.TargetReference == nil || !ws.DeletionTimestamp.IsZero() {
			continue
		}
		// determine if the reference of workloadSpread and pod is equal
		if h.isReferenceEqual(ws.Spec.TargetReference, ref, pod.Namespace) {
			matchedWS = &ws
			// pod has at most one matched workloadSpread
			break
		}
	}
	// not found matched workloadSpread
	if matchedWS == nil {
		return true, nil
	}

	defer func() {
		klog.V(3).Infof("Cost of handling pod creation by WorkloadSpread (%s/%s) is %v",
			matchedWS.Namespace, matchedWS.Name, time.Since(start))
	}()

	return false, h.mutatingPod(matchedWS, pod, nil, CreateOperation)
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

	klog.V(3).Infof("handler operation[%s] Pod(%s/%s) generatedUID(%s) for WorkloadSpread(%s/%s) done",
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

	switch matchedWS.Spec.TargetReference.Kind {
	case controllerKindSts.Kind:
		// StatefulSet has special logic about pod assignment for subsets.
		// For example, suppose that we have the following sub sets config:
		//	- name: subset-a
		//	  maxReplicas: 5
		//	- name: subset-b
		//	  maxReplicas: 5
		//	- name: subset-c
		// the pods with order within [0, 5) will be assigned to subset-a;
		// the pods with order within [5, 10) will be assigned to subset-b;
		// the pods with order within [10, inf) will be assigned to subset-c.
		currentThresholdID := int64(0)
		for _, subset := range matchedWS.Spec.Subsets {
			cond := getSubsetCondition(matchedWS, subset.Name, appsv1alpha1.SubsetSchedulable)
			if cond != nil && cond.Status == corev1.ConditionFalse {
				continue
			}
			subsetReplicasLimit := math.MaxInt32
			if subset.MaxReplicas != nil {
				subsetReplicasLimit = subset.MaxReplicas.IntValue()
			}
			// currently, we do not support reserveOrdinals feature for advanced statefulSet
			currentThresholdID += int64(subsetReplicasLimit)
			_, orderID := getParentNameAndOrdinal(pod)
			if int64(orderID) < currentThresholdID {
				suitableSubsetName = subset.Name
				break
			}
		}

	default:
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
			changed, suitableSubset, generatedUID = h.updateSubsetForPod(wsClone, pod, injectWS, operation)
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

// return three parameters:
// 1. changed(bool) indicates if workloadSpread.Status has changed
// 2. suitableSubset(*struct{}) indicates which workloadSpread.Subset does this pod match
// 3. generatedUID(types.UID) indicates which workloadSpread generate a UID for identifying Pod without a full name.
func (h *Handler) updateSubsetForPod(ws *appsv1alpha1.WorkloadSpread,
	pod *corev1.Pod, injectWS *InjectWorkloadSpread, operation Operation) (
	bool, *appsv1alpha1.WorkloadSpreadSubsetStatus, string) {
	var suitableSubset *appsv1alpha1.WorkloadSpreadSubsetStatus
	var generatedUID string

	switch operation {
	case CreateOperation:
		if pod.Name != "" {
			// pod is already in CreatingPods/DeletingPods List, then return
			if isRecord, subset := isPodRecordedInSubset(ws, pod.Name); isRecord {
				return false, subset, ""
			}
		}

		suitableSubset = h.getSuitableSubset(ws)
		if suitableSubset == nil {
			klog.V(5).Infof("WorkloadSpread (%s/%s) don't have a suitable subset for Pod (%s)",
				ws.Namespace, ws.Name, pod.Name)
			return false, nil, ""
		}
		// no need to update WorkloadSpread status if MaxReplicas == nil
		if suitableSubset.MissingReplicas == -1 {
			return false, suitableSubset, ""
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
			return false, nil, ""
		}

		suitableSubset = getSpecificSubset(ws, injectWS.Subset)
		if suitableSubset == nil {
			klog.V(5).Infof("Pod (%s/%s) matched WorkloadSpread (%s) not found Subset(%s)", ws.Namespace, pod.Name, ws.Name, injectWS.Subset)
			return false, nil, ""
		}
		if suitableSubset.MissingReplicas == -1 {
			return false, suitableSubset, ""
		}
		if suitableSubset.DeletingPods == nil {
			suitableSubset.DeletingPods = map[string]metav1.Time{}
		}
		suitableSubset.DeletingPods[pod.Name] = metav1.Time{Time: time.Now()}
		if suitableSubset.MissingReplicas >= 0 {
			suitableSubset.MissingReplicas++
		}
	default:
		return false, nil, ""
	}

	// update subset status
	for i := range ws.Status.SubsetStatuses {
		if ws.Status.SubsetStatuses[i].Name == suitableSubset.Name {
			ws.Status.SubsetStatuses[i] = *suitableSubset
			break
		}
	}

	return true, suitableSubset, generatedUID
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

func (h *Handler) getSuitableSubset(ws *appsv1alpha1.WorkloadSpread) *appsv1alpha1.WorkloadSpreadSubsetStatus {
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

func (h Handler) isReferenceEqual(target *appsv1alpha1.TargetReference, owner *metav1.OwnerReference, namespace string) bool {
	if owner == nil {
		return false
	}

	targetGv, err := schema.ParseGroupVersion(target.APIVersion)
	if err != nil {
		klog.Errorf("parse TargetReference apiVersion (%s) failed: %s", target.APIVersion, err.Error())
		return false
	}

	ownerGv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		klog.Errorf("parse OwnerReference apiVersion (%s) failed: %s", owner.APIVersion, err.Error())
		return false
	}

	if targetGv.Group == ownerGv.Group && target.Kind == owner.Kind && target.Name == owner.Name {
		return true
	}

	if match, err := matchReference(owner); err != nil || !match {
		return false
	}

	ownerObject, err := h.getObjectOf(owner, namespace)
	if err != nil {
		klog.Errorf("Failed to get owner object %v: %v", owner, err)
		return false
	}

	return h.isReferenceEqual(target, metav1.GetControllerOfNoCopy(ownerObject), namespace)
}

// statefulPodRegex is a regular expression that extracts the parent StatefulSet and ordinal from the Name of a Pod
var statefulPodRegex = regexp.MustCompile("(.*)-([0-9]+)$")

// getParentNameAndOrdinal gets the name of pod's parent StatefulSet and pod's ordinal as extracted from its Name. If
// the Pod was not created by a StatefulSet, its parent is considered to be empty string, and its ordinal is considered
// to be -1.
func getParentNameAndOrdinal(pod *corev1.Pod) (string, int) {
	parent := ""
	ordinal := -1
	subMatches := statefulPodRegex.FindStringSubmatch(pod.Name)
	if len(subMatches) < 3 {
		return parent, ordinal
	}
	parent = subMatches[1]
	if i, err := strconv.ParseInt(subMatches[2], 10, 32); err == nil {
		ordinal = int(i)
	}
	return parent, ordinal
}

func getSubsetCondition(ws *appsv1alpha1.WorkloadSpread, subsetName string, condType appsv1alpha1.WorkloadSpreadSubsetConditionType) *appsv1alpha1.WorkloadSpreadSubsetCondition {
	for i := range ws.Status.SubsetStatuses {
		subset := &ws.Status.SubsetStatuses[i]
		if subset.Name != subsetName {
			continue
		}
		for _, condition := range subset.Conditions {
			if condition.Type == condType {
				return &condition
			}
		}
	}
	return nil
}

func (h Handler) getObjectOf(owner *metav1.OwnerReference, namespace string) (client.Object, error) {
	var object client.Object
	objectKey := types.NamespacedName{Namespace: namespace, Name: owner.Name}
	objectGvk := schema.FromAPIVersionAndKind(owner.APIVersion, owner.Kind)
	switch objectGvk {
	case controllerKindRS:
		object = &appsv1.ReplicaSet{}
	case controllerKindDep:
		object = &appsv1.Deployment{}
	case controllerKindSts:
		object = &appsv1.StatefulSet{}
	case controllerKruiseKindBetaSts, controllerKruiseKindAlphaSts:
		object = &appsv1beta1.StatefulSet{}
	case controllerKruiseKindCS:
		object = &appsv1alpha1.CloneSet{}
	default:
		o := unstructured.Unstructured{}
		o.SetGroupVersionKind(objectGvk)
		object = &o
	}
	if err := h.Get(context.TODO(), objectKey, object); err != nil {
		return nil, err
	}
	return object, nil
}

func initializeWorkloadsInWhiteList(c client.Client) {
	if workloadsInWhiteListInitialized {
		return
	}
	whiteList, err := configuration.GetWSWatchCustomWorkloadWhiteList(c)
	if err != nil {
		return
	}
	for _, wl := range whiteList.Workloads {
		workloads = append(workloads, workload{
			Groups: []string{wl.Group},
			Kind:   wl.Kind,
		})
		for _, subWl := range wl.SubResources {
			workloads = append(workloads, workload{
				Groups: []string{subWl.Group},
				Kind:   subWl.Kind,
			})
		}
	}
	workloadsInWhiteListInitialized = true
}
