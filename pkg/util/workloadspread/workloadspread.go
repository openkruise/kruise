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
	"time"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
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

type workload struct {
	Kind   string
	Groups []string
}

var (
	workloads []workload
)

func init() {
	workloads = append(workloads, workload{Kind: controllerKruiseKindCS.Kind, Groups: []string{controllerKruiseKindCS.Group}})
	workloads = append(workloads, workload{Kind: controllerKindRS.Kind, Groups: []string{controllerKindRS.Group}})
	workloads = append(workloads, workload{Kind: controllerKindJob.Kind, Groups: []string{controllerKindJob.Group}})
}

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
	// generate id if the Pod' name is nil.
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

func (h *Handler) HandlePodUpdate(oldPod, newPod *corev1.Pod) error {
	str := newPod.Annotations[MatchedWorkloadSpreadSubsetAnnotations]
	if str == "" {
		return nil
	}

	// Pod has been terminated its lifecycle
	if oldPod.Status.Phase != newPod.Status.Phase &&
		(newPod.Status.Phase == corev1.PodFailed || newPod.Status.Phase == corev1.PodSucceeded) {
		var injectWS *InjectWorkloadSpread
		err := json.Unmarshal([]byte(str), &injectWS)
		if err != nil {
			klog.Errorf("parse Pod (%s/%s) annotations[%s]=%s failed: %s", newPod.Namespace, newPod.Name,
				MatchedWorkloadSpreadSubsetAnnotations, str, err.Error())
			return nil
		}
		if injectWS == nil || metav1.GetControllerOf(newPod) == nil {
			return nil
		}

		matchedWS := &appsv1alpha1.WorkloadSpread{}
		err = h.Client.Get(context.TODO(), client.ObjectKey{Namespace: newPod.Namespace, Name: injectWS.Name}, matchedWS)
		if err != nil {
			if errors.IsNotFound(err) {
				klog.Warningf("Pod(%s/%s) matched WorkloadSpread(%s) Not Found", newPod.Namespace, newPod.Name, injectWS.Name)
				return nil
			}
			klog.Errorf("get Pod(%s/%s) matched workloadSpread(%s) failed: %s", newPod.Namespace, newPod.Name, injectWS.Name, err.Error())
			return err
		}

		return h.mutatingPod(matchedWS, newPod, injectWS, admissionv1beta1.Update)
	}

	return nil
}

func (h *Handler) HandlePodCreation(pod *corev1.Pod) error {
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
		if ws.Spec.TargetReference == nil {
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
		return nil
	}

	return h.mutatingPod(matchedWS, pod, nil, admissionv1beta1.Create)
}

func (h *Handler) HandlePodDeletion(pod *corev1.Pod) error {
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
	// 1. Deletion pod
	// 2. Pod.Status.Phase = Succeeded or Failed
	// 3. Pod.OwnerReference is nil
	ref := metav1.GetControllerOf(pod)
	if injectWS == nil || !kubecontroller.IsPodActive(pod) || ref == nil {
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

	return h.mutatingPod(matchedWS, pod, injectWS, admissionv1beta1.Delete)
}

func (h *Handler) mutatingPod(matchedWS *appsv1alpha1.WorkloadSpread,
	pod *corev1.Pod,
	injectWS *InjectWorkloadSpread,
	operation admissionv1beta1.Operation) error {
	podName := pod.Name
	if podName == "" {
		podName = pod.GetGenerateName()
	}

	klog.V(3).Infof("Operation(%s) Pod(%s/%s) matched WorkloadSpread(%s/%s)", operation, pod.Namespace, podName, matchedWS.Namespace, matchedWS.Name)

	wsClone := matchedWS.DeepCopy()
	var refresh, changed, isInject bool
	var suitableSubset *appsv1alpha1.WorkloadSpreadSubsetStatus
	var generatedUID string
	var injectErr error

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var err error
		if refresh {
			if err = h.Client.Get(context.TODO(), client.ObjectKey{Namespace: matchedWS.Namespace, Name: matchedWS.Name}, wsClone); err != nil {
				klog.Errorf("error getting updated WorkloadSpread(%s/%s) from client", matchedWS.Namespace, matchedWS.Name)
				return err
			}
		}

		// check whether WorkloadSpread has suitable subset for the pod
		// 1. changed indicates whether workloadSpread status changed
		// 2. isInject indicates whether to inject workloadSpread subset in pod
		// 3. suitableSubset is matched subset for the pod
		changed, isInject, suitableSubset, generatedUID, injectErr = h.checkDone(wsClone, pod, injectWS, operation)
		if injectErr != nil || !changed {
			return nil
		}

		// update WorkloadSpread status
		if err = h.Client.Status().Update(context.TODO(), wsClone); err != nil {
			refresh = true
		} else {
			klog.V(3).Infof("update workloadSpread(%s/%s) SubsetStatus(%s) missingReplicas(%d) creatingPods(%d) deletingPods(%d) success",
				wsClone.Namespace, wsClone.Name, suitableSubset.Name,
				suitableSubset.MissingReplicas, len(suitableSubset.CreatingPods), len(suitableSubset.DeletingPods))
		}
		return err
	}); err != nil {
		klog.Errorf("update WorkloadSpread(%s/%s) error %s", matchedWS.Namespace, matchedWS.Name, err.Error())
		return err
	}

	// if create pod, inject affinity、toleration、metadata in pod object
	if isInject {
		if _, injectErr = injectWorkloadSpreadIntoPod(matchedWS, pod, suitableSubset.Name, generatedUID); injectErr != nil {
			klog.Errorf("failed to inject Pod(%s/%s) subset(%s) data for workloadSpread(%s/%s)",
				pod.Namespace, pod.Name, suitableSubset.Name, matchedWS.Namespace, matchedWS.Name)
			return injectErr
		}
		klog.V(3).Infof("inject Pod(%s/%s) subset(%s) data for workloadSpread(%s/%s)",
			pod.Namespace, pod.Name, suitableSubset.Name, matchedWS.Namespace, matchedWS.Name)
	}

	klog.V(3).Infof("handler Pod(%s/%s) generatedUID(%s) operation(%s) for workloadSpread(%s/%s) done",
		pod.Namespace, podName, generatedUID, operation, matchedWS.Namespace, matchedWS.Name)

	return injectErr
}

// return four parameters:
// 1. changed(bool) indicates if workloadSpread.Status has changed
// 2. inject(bool) indicates whether to inject workloadSpread subset
// 3. suitableSubset(*struct{}) indicates which workloadSpread.Subset does this pod match
// 4. generatedUID(types.UID) indicates which workloadSpread generate a UID for identifying Pod without a full name.
func (h *Handler) checkDone(ws *appsv1alpha1.WorkloadSpread,
	pod *corev1.Pod, injectWS *InjectWorkloadSpread, operation admissionv1beta1.Operation) (
	bool, bool, *appsv1alpha1.WorkloadSpreadSubsetStatus, string, error) {
	var suitableSubset *appsv1alpha1.WorkloadSpreadSubsetStatus
	var generatedUID string
	var err error

	switch operation {
	case admissionv1beta1.Create:
		if pod.Name != "" {
			// pod is already in CreatingPods/DeletingPods List, then return
			if isRecord, subset := isPodRecordedInSubset(ws, pod.Name); isRecord {
				return false, true, subset, "", nil
			}
		}

		suitableSubset, err = h.getSuitableSubset(ws, pod)
		if err != nil {
			return false, false, nil, "", err
		}
		if suitableSubset == nil {
			klog.V(5).Infof("WorkloadSpread (%s/%s) don't have suitable subset for Pod (%s)",
				ws.Namespace, ws.Name, pod.Name)
			return false, false, nil, "", nil
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
	case admissionv1beta1.Delete, admissionv1beta1.Update:
		// pod is already in DeletingPods/CreatingPods List, then return
		if isRecord, _ := isPodRecordedInSubset(ws, pod.Name); isRecord {
			return false, false, nil, "", nil
		}

		suitableSubset = getSpecificSubset(ws, injectWS.Subset)
		if suitableSubset == nil {
			klog.V(5).Infof("Pod (%s/%s) matched WorkloadSpread (%s) not found Subset(%s)", ws.Namespace, pod.Name, ws.Name, injectWS.Subset)
			return false, false, nil, "", nil
		}
		if suitableSubset.MissingReplicas >= 0 {
			suitableSubset.MissingReplicas++
		}
		if suitableSubset.DeletingPods == nil {
			suitableSubset.DeletingPods = map[string]metav1.Time{}
		}
		suitableSubset.DeletingPods[pod.Name] = metav1.Time{Time: time.Now()}
	default:
		return false, false, nil, "", nil
	}

	// update subset status
	for i := range ws.Status.SubsetStatuses {
		if ws.Status.SubsetStatuses[i].Name == suitableSubset.Name {
			ws.Status.SubsetStatuses[i] = *suitableSubset
			break
		}
	}

	return true, operation == admissionv1beta1.Create, suitableSubset, generatedUID, nil
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

	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
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
				{
					MatchExpressions: subset.RequiredNodeSelectorTerm.MatchExpressions,
					MatchFields:      subset.RequiredNodeSelectorTerm.MatchFields,
				},
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
		if err = json.Unmarshal(modified, pod); err != nil {
			klog.Errorf("failed to unmarshal %s to Pod", modified)
			return false, err
		}
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

func (h *Handler) getSuitableSubset(ws *appsv1alpha1.WorkloadSpread, pod *corev1.Pod) (
	*appsv1alpha1.WorkloadSpreadSubsetStatus, error) {
	simulationSchedule := false
	if ws.Spec.ScheduleStrategy.Type == appsv1alpha1.AdaptiveWorkloadSpreadScheduleStrategyType &&
		ws.Spec.ScheduleStrategy.Adaptive != nil && !ws.Spec.ScheduleStrategy.Adaptive.DisableSimulationSchedule {
		simulationSchedule = true
	}

	for _, subset := range ws.Status.SubsetStatuses {
		canSchedule := true
		for _, condition := range subset.Conditions {
			if condition.Type == appsv1alpha1.SubsetSchedulable && condition.Status == corev1.ConditionFalse {
				canSchedule = false
				break
			}
		}

		if canSchedule && (subset.MissingReplicas > 0 || subset.MissingReplicas == -1) {
			// scheduleStrategy.Type = Adaptive
			// Webhook will simulate a schedule in order to check whether Pod can run in this subset,
			// which does a generic predicates by the cache of nodes and pods in kruise manager.
			// There may be some errors between simulation schedule and kubernetes scheduler with small probability.
			if simulationSchedule {
				schedulable, err := h.canScheduleOnSubset(ws, pod, subset.Name)
				if err != nil {
					return nil, err
				}
				// If the simulated schedule failed we will consider the next subset to be scheduled.
				if !schedulable {
					klog.V(3).Infof("can't schedule Pod (%s/%s) into Subset (%s) of WorkloadSpread (%s/%s)",
						pod.Namespace, pod.Name, subset.Name, ws.Namespace, ws.Name)
					continue
				}
			}

			return &subset, nil
		}
	}

	return nil, nil
}

func (h Handler) isReferenceEqual(target *appsv1alpha1.TargetReference, owner *metav1.OwnerReference, namespace string) bool {
	targetGv, err := schema.ParseGroupVersion(target.APIVersion)
	if err != nil {
		klog.Errorf("parse TargetReference apiVersion (%s) failed: %s", target.APIVersion, err.Error())
		return false
	}

	var ownerGv schema.GroupVersion
	if target.Kind == controllerKindDep.Kind {
		rs := &appsv1.ReplicaSet{}
		err = h.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: owner.Name}, rs)
		if err != nil {
			return false
		}
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
	ownerGv, err = schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		klog.Errorf("parse OwnerReference apiVersion (%s) failed: %s", owner.APIVersion, err.Error())
		return false
	}

	return targetGv.Group == ownerGv.Group && target.Kind == owner.Kind && target.Name == owner.Name
}
