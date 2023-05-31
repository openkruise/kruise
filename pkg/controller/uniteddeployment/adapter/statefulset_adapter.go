/*
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

package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/util/strategicpatch"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruisectlutil "github.com/openkruise/kruise/pkg/controller/util"
	"github.com/openkruise/kruise/pkg/util/refmanager"
)

type StatefulSetAdapter struct {
	client.Client

	Scheme *runtime.Scheme
}

// NewResourceObject creates a empty StatefulSet object.
func (a *StatefulSetAdapter) NewResourceObject() client.Object {
	return &appsv1.StatefulSet{}
}

// NewResourceListObject creates a empty StatefulSetList object.
func (a *StatefulSetAdapter) NewResourceListObject() client.ObjectList {
	return &appsv1.StatefulSetList{}
}

// GetStatusObservedGeneration returns the observed generation of the subset.
func (a *StatefulSetAdapter) GetStatusObservedGeneration(obj metav1.Object) int64 {
	return obj.(*appsv1.StatefulSet).Status.ObservedGeneration
}

// GetReplicaDetails returns the replicas detail the subset needs.
func (a *StatefulSetAdapter) GetReplicaDetails(obj metav1.Object, updatedRevision string) (specReplicas, specPartition *int32, statusReplicas, statusReadyReplicas, statusUpdatedReplicas, statusUpdatedReadyReplicas int32, err error) {
	set := obj.(*appsv1.StatefulSet)
	var pods []*corev1.Pod
	pods, err = a.getStatefulSetPods(set)
	if err != nil {
		return
	}

	specReplicas = set.Spec.Replicas
	if set.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		revision := getRevision(&set.ObjectMeta)
		specPartition = getCurrentPartition(pods, revision)
	} else if set.Spec.UpdateStrategy.RollingUpdate != nil &&
		set.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		specPartition = set.Spec.UpdateStrategy.RollingUpdate.Partition
	}

	statusReplicas = set.Status.Replicas
	statusReadyReplicas = set.Status.ReadyReplicas
	statusUpdatedReplicas, statusUpdatedReadyReplicas = calculateUpdatedReplicas(pods, updatedRevision)

	return
}

// GetSubsetFailure returns the failure information of the subset.
// StatefulSet has no condition.
func (a *StatefulSetAdapter) GetSubsetFailure() *string {
	return nil
}

// ApplySubsetTemplate updates the subset to the latest revision, depending on the StatefulSetTemplate.
func (a *StatefulSetAdapter) ApplySubsetTemplate(ud *alpha1.UnitedDeployment, subsetName, revision string, replicas, partition int32, obj runtime.Object) error {
	set := obj.(*appsv1.StatefulSet)

	var subSetConfig *alpha1.Subset
	for _, subset := range ud.Spec.Topology.Subsets {
		if subset.Name == subsetName {
			subSetConfig = &subset
			break
		}
	}
	if subSetConfig == nil {
		return fmt.Errorf("fail to find subset config %s", subsetName)
	}

	set.Namespace = ud.Namespace

	if set.Labels == nil {
		set.Labels = map[string]string{}
	}
	for k, v := range ud.Spec.Template.StatefulSetTemplate.Labels {
		set.Labels[k] = v
	}
	for k, v := range ud.Spec.Selector.MatchLabels {
		set.Labels[k] = v
	}
	set.Labels[alpha1.ControllerRevisionHashLabelKey] = revision
	// record the subset name as a label
	set.Labels[alpha1.SubSetNameLabelKey] = subsetName

	if set.Annotations == nil {
		set.Annotations = map[string]string{}
	}
	for k, v := range ud.Spec.Template.StatefulSetTemplate.Annotations {
		set.Annotations[k] = v
	}

	set.GenerateName = getSubsetPrefix(ud.Name, subsetName)

	selectors := ud.Spec.Selector.DeepCopy()
	selectors.MatchLabels[alpha1.SubSetNameLabelKey] = subsetName

	if err := controllerutil.SetControllerReference(ud, set, a.Scheme); err != nil {
		return err
	}

	set.Spec.Selector = selectors
	set.Spec.Replicas = &replicas
	if ud.Spec.Template.StatefulSetTemplate.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		set.Spec.UpdateStrategy.Type = appsv1.OnDeleteStatefulSetStrategyType
	} else {
		if set.Spec.UpdateStrategy.RollingUpdate == nil {
			set.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
		}
		set.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
	}

	set.Spec.Template = *ud.Spec.Template.StatefulSetTemplate.Spec.Template.DeepCopy()
	if set.Spec.Template.Labels == nil {
		set.Spec.Template.Labels = map[string]string{}
	}
	set.Spec.Template.Labels[alpha1.SubSetNameLabelKey] = subsetName
	set.Spec.Template.Labels[alpha1.ControllerRevisionHashLabelKey] = revision

	set.Spec.RevisionHistoryLimit = ud.Spec.Template.StatefulSetTemplate.Spec.RevisionHistoryLimit
	set.Spec.PodManagementPolicy = ud.Spec.Template.StatefulSetTemplate.Spec.PodManagementPolicy
	set.Spec.ServiceName = ud.Spec.Template.StatefulSetTemplate.Spec.ServiceName
	set.Spec.VolumeClaimTemplates = ud.Spec.Template.StatefulSetTemplate.Spec.VolumeClaimTemplates

	attachNodeAffinity(&set.Spec.Template.Spec, subSetConfig)
	attachTolerations(&set.Spec.Template.Spec, subSetConfig)
	if subSetConfig.Patch.Raw != nil {
		TemplateSpecBytes, _ := json.Marshal(set.Spec.Template)
		modified, err := strategicpatch.StrategicMergePatch(TemplateSpecBytes, subSetConfig.Patch.Raw, &corev1.PodTemplateSpec{})
		if err != nil {
			klog.Errorf("failed to merge patch raw %s", subSetConfig.Patch.Raw)
			return err
		}
		patchedTemplateSpec := corev1.PodTemplateSpec{}
		if err = json.Unmarshal(modified, &patchedTemplateSpec); err != nil {
			klog.Errorf("failed to unmarshal %s to podTemplateSpec", modified)
			return err
		}

		set.Spec.Template = patchedTemplateSpec
		klog.V(2).Infof("StatefulSet [%s/%s] was patched successfully: %s", set.Namespace, set.GenerateName, subSetConfig.Patch.Raw)
	}
	if set.Annotations == nil {
		set.Annotations = make(map[string]string)
	}
	set.Annotations[alpha1.AnnotationSubsetPatchKey] = string(subSetConfig.Patch.Raw)

	return nil
}

// PostUpdate does some works after subset updated. StatefulSet will implement this method to clean stuck pods.
func (a *StatefulSetAdapter) PostUpdate(ud *alpha1.UnitedDeployment, obj runtime.Object, revision string, partition int32) error {
	set := obj.(*appsv1.StatefulSet)
	if set.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		return nil
	}

	// If RollingUpdate, work around for issue https://github.com/kubernetes/kubernetes/issues/67250
	return a.deleteStuckPods(set, revision, partition)
}

// IsExpected checks the subset is the expected revision or not.
// The revision label can tell the current subset revision.
func (a *StatefulSetAdapter) IsExpected(obj metav1.Object, revision string) bool {
	return obj.GetLabels()[alpha1.ControllerRevisionHashLabelKey] != revision
}

func (a *StatefulSetAdapter) getStatefulSetPods(set *appsv1.StatefulSet) ([]*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	podList := &corev1.PodList{}
	err = a.Client.List(context.TODO(), podList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}

	manager, err := refmanager.New(a.Client, set.Spec.Selector, set, a.Scheme)
	if err != nil {
		return nil, err
	}
	selected := make([]metav1.Object, len(podList.Items))
	for i, pod := range podList.Items {
		selected[i] = pod.DeepCopy()
	}
	claimed, err := manager.ClaimOwnedObjects(selected)
	if err != nil {
		return nil, err
	}

	claimedPods := make([]*corev1.Pod, len(claimed))
	for i, pod := range claimed {
		claimedPods[i] = pod.(*corev1.Pod)
	}
	return claimedPods, nil
}

func calculateUpdatedReplicas(podList []*corev1.Pod, updatedRevision string) (updatedReplicas, updatedReadyReplicas int32) {
	for _, pod := range podList {
		revision := getRevision(&pod.ObjectMeta)

		// Only count pods that are updated and are not terminating
		if revision == updatedRevision && pod.GetDeletionTimestamp() == nil {
			updatedReplicas++
			if podutil.IsPodReady(pod) {
				updatedReadyReplicas++
			}
		}
	}

	return
}

// deleteStuckPods tries to work around the blocking issue https://github.com/kubernetes/kubernetes/issues/67250
func (a *StatefulSetAdapter) deleteStuckPods(set *appsv1.StatefulSet, revision string, partition int32) error {
	pods, err := a.getStatefulSetPods(set)
	if err != nil {
		return err
	}

	for i := range pods {
		pod := pods[i]
		// If the pod is considered as stuck, delete it.
		if isPodStuckForRollingUpdate(pod, revision, partition) {
			klog.V(2).Infof("Delete pod %s/%s at stuck state", pod.Namespace, pod.Name)
			err = a.Delete(context.TODO(), pod, client.PropagationPolicy(metav1.DeletePropagationBackground))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// isPodStuckForRollingUpdate checks whether the pod is stuck under strategy RollingUpdate.
// If a pod needs to upgrade (pod_ordinal >= partition && pod_revision != sts_revision)
// and its readiness is false, or worse status like Pending, ImagePullBackOff, it will be blocked.
func isPodStuckForRollingUpdate(pod *corev1.Pod, revision string, partition int32) bool {
	if kruisectlutil.GetOrdinal(pod) < partition {
		return false
	}

	if getRevision(pod) == revision {
		return false
	}

	return !podutil.IsPodReadyConditionTrue(pod.Status)
}
