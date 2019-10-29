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

package uniteddeployment

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/refmanager"
)

var statefulPodRegex = regexp.MustCompile("(.*)-([0-9]+)$")

// StatefulSetControl provides subset operations of StatefulSet.
type StatefulSetControl struct {
	client.Client

	scheme *runtime.Scheme
}

// GetAllSubsets returns all of subsets owned by the UnitedDeployment.
func (m *StatefulSetControl) GetAllSubsets(ud *alpha1.UnitedDeployment) (podSets []*Subset, err error) {
	selector, err := metav1.LabelSelectorAsSelector(ud.Spec.Selector)
	if err != nil {
		return nil, err
	}

	setList := &appsv1.StatefulSetList{}
	err = m.Client.List(context.TODO(), &client.ListOptions{LabelSelector: selector}, setList)
	if err != nil {
		return nil, err
	}

	manager, err := refmanager.New(m.Client, ud.Spec.Selector, ud, m.scheme)
	if err != nil {
		return nil, err
	}
	selected := make([]metav1.Object, len(setList.Items))
	for i, set := range setList.Items {
		selected[i] = set.DeepCopy()
	}
	claimedSets, err := manager.ClaimOwnedObjects(selected)
	if err != nil {
		return nil, err
	}

	for _, claimedSet := range claimedSets {
		podSet, err := m.convertToSubset(claimedSet.(*appsv1.StatefulSet))
		if err != nil {
			return nil, err
		}
		podSets = append(podSets, podSet)
	}
	return podSets, nil
}

// CreateSubset creates the StatefulSet depending on the inputs.
func (m *StatefulSetControl) CreateSubset(ud *alpha1.UnitedDeployment, subsetName string, revision string, replicas, partition int32) error {
	set, err := createStatefulSet(ud, subsetName, m.scheme, revision, replicas, partition)
	if err != nil {
		klog.Errorf("Fail to create StatefulSet: %s", err)
		return err
	}

	klog.V(4).Infof("Have %d replicas when creating StatefulSet %s/%s", *set.Spec.Replicas, set.Namespace, set.Name)
	err = m.Create(context.TODO(), set)
	return err
}

func createStatefulSet(ud *alpha1.UnitedDeployment, subsetName string, scheme *runtime.Scheme, revision string, replicas, partition int32) (*appsv1.StatefulSet, error) {
	set := &appsv1.StatefulSet{}
	set.Spec = *ud.Spec.Template.StatefulSetTemplate.Spec.DeepCopy()
	applyStatefulSetTemplate(ud, subsetName, revision, scheme, replicas, partition, set)
	return set, nil
}

func applyStatefulSetTemplate(ud *alpha1.UnitedDeployment, subsetName string, revision string, scheme *runtime.Scheme, replicas, partition int32, set *appsv1.StatefulSet) error {
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
	set.Labels[alpha1.SubSetNameLabelKey] = subsetName

	if set.Annotations == nil {
		set.Annotations = map[string]string{}
	}
	for k, v := range ud.Spec.Template.StatefulSetTemplate.Annotations {
		set.Annotations[k] = v
	}

	set.GenerateName = getPodsPrefix(ud.Name)

	selectors := ud.Spec.Selector.DeepCopy()
	selectors.MatchLabels[alpha1.SubSetNameLabelKey] = subsetName

	if err := controllerutil.SetControllerReference(ud, set, scheme); err != nil {
		return err
	}

	set.Spec.Selector = selectors
	set.Spec.Replicas = &replicas
	if set.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType {
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
	attachNodeAffinity(&set.Spec.Template.Spec, subSetConfig)

	return nil
}

func (m *StatefulSetControl) applyRollingUpdateStrategyPartition(set *appsv1.StatefulSet, partition int32) {
	if set.Spec.UpdateStrategy.RollingUpdate == nil {
		set.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
	}
	set.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
}

// UpdateSubset is used to update the subset. The target StatefulSet can be found with the input subset.
func (m *StatefulSetControl) UpdateSubset(subset *Subset, ud *alpha1.UnitedDeployment, revision string, replicas, partition int32) error {
	set := &appsv1.StatefulSet{}
	var updateError error
	for i := 0; i < updateRetries; i++ {
		getError := m.Client.Get(context.TODO(), m.objectKey(&subset.ObjectMeta), set)
		if getError != nil {
			return getError
		}

		if err := applyStatefulSetTemplate(ud, subset.Spec.SubsetName, revision, m.scheme, replicas, partition, set); err != nil {
			return err
		}

		updateError = m.Client.Update(context.TODO(), set)
		if updateError == nil {
			break
		}
	}

	if updateError != nil {
		return updateError
	}

	if set.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		return m.applyOnDeleteUpdateStrategyPartition(set, revision, *set.Spec.UpdateStrategy.RollingUpdate.Partition)
	}

	// If RollingUpdate, work around for issue https://github.com/kubernetes/kubernetes/issues/67250
	return m.deleteStuckPods(set, revision, partition)
}

func (m *StatefulSetControl) applyOnDeleteUpdateStrategyPartition(set *appsv1.StatefulSet, revision string, partition int32) error {
	pods, err := m.getStatefulSetPods(set)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		ordinal := getOrdinal(pod)
		if ordinal >= partition && getRevision(&pod.ObjectMeta) != revision {
			if err = m.deletePod(pod); err != nil {
				return err
			}
		}
	}

	return nil
}

func convertToNextPartition(partition int32, replicas int32) int32 {
	if replicas > partition {
		return replicas - partition
	}

	return 0
}

// DeleteSubset is called to delete the subset. The target StatefulSet can be found with the input subset.
func (m *StatefulSetControl) DeleteSubset(podSet *Subset) error {
	set := podSet.Spec.SubsetRef.Resources[0].(*appsv1.StatefulSet)
	return m.Delete(context.TODO(), set, client.PropagationPolicy(metav1.DeletePropagationBackground))
}

func (m *StatefulSetControl) convertToSubset(set *appsv1.StatefulSet) (*Subset, error) {
	subSetName, err := getSubsetNameFrom(set)
	if err != nil {
		return nil, err
	}

	subset := &Subset{}
	subset.ObjectMeta = *set.ObjectMeta.DeepCopy()

	subset.Spec.SubsetName = subSetName
	if set.Spec.Replicas != nil {
		subset.Spec.Replicas = *set.Spec.Replicas
	}

	pods, err := m.getStatefulSetPods(set)
	if err != nil {
		return subset, err
	}

	subset.Spec.Strategy.Partition = 0
	if set.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		revision := getRevision(&set.ObjectMeta)
		subset.Spec.Strategy.Partition = getCurrentPartition(subset.Spec.Replicas, pods, revision)
	} else if set.Spec.UpdateStrategy.RollingUpdate != nil &&
		set.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		subset.Spec.Strategy.Partition = *set.Spec.UpdateStrategy.RollingUpdate.Partition
	}

	subset.Spec.SubsetRef.Resources = append(subset.Spec.SubsetRef.Resources, set)
	subset.Status.ObservedGeneration = set.Status.ObservedGeneration
	subset.Status.Replicas = set.Status.Replicas
	subset.Status.ReadyReplicas = set.Status.ReadyReplicas
	subset.Status.RevisionReplicas = calculateStatus(pods, set)

	return subset, nil
}

func getCurrentPartition(replicas int32, pods []*corev1.Pod, revision string) int32 {
	ordinalMarks := make([]bool, replicas)
	for _, pod := range pods {
		ordinal := getOrdinal(pod)
		if ordinal >= replicas {
			// unexpected
			continue
		}
		if getRevision(&pod.ObjectMeta) == revision {
			ordinalMarks[ordinal] = true
		}
	}

	partition := replicas
	for i := replicas - 1; i >= 0; i-- {
		if ordinalMarks[i] {
			partition = i
		} else {
			break
		}
	}

	return partition
}

func (m *StatefulSetControl) deleteStuckPods(set *appsv1.StatefulSet, revision string, partition int32) error {
	pods, err := m.getStatefulSetPods(set)
	if err != nil {
		return err
	}

	for i := range pods {
		pod := pods[i]
		if isPodStuck(pod, revision, partition) {
			klog.V(2).Infof("Delete pod %s/%s at stuck state", pod.Namespace, pod.Name)
			err = m.deletePod(pod)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *StatefulSetControl) deletePod(pod *corev1.Pod) error {
	return m.Delete(context.TODO(), pod, client.PropagationPolicy(metav1.DeletePropagationBackground))
}

func (m *StatefulSetControl) getStatefulSetPods(set *appsv1.StatefulSet) ([]*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}
	podList := &corev1.PodList{}
	err = m.Client.List(context.TODO(), &client.ListOptions{LabelSelector: selector}, podList)
	if err != nil {
		return nil, err
	}

	manager, err := refmanager.New(m.Client, set.Spec.Selector, set, m.scheme)
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

func calculateStatus(podList []*corev1.Pod, set *appsv1.StatefulSet) (revisionReplicas map[string]*SubsetReplicaStatus) {
	revisionReplicas = map[string]*SubsetReplicaStatus{}
	for _, pod := range podList {
		revision := getRevision(&pod.ObjectMeta)
		status, exist := revisionReplicas[revision]
		if !exist {
			status = &SubsetReplicaStatus{}
			revisionReplicas[revision] = status
		}
		status.Replicas++
		if podutil.IsPodReady(pod) {
			status.ReadyReplicas++
		}
	}
	return
}

func isPodUpgradeComplete(pod *corev1.Pod, revision string) bool {
	if getRevision(&pod.ObjectMeta) == revision {
		return podutil.IsPodReadyConditionTrue(pod.Status)
	}
	return false
}

func isPodStuck(pod *corev1.Pod, revision string, partition int32) bool {
	return !isPodUpgradeComplete(pod, revision) && getOrdinal(pod) >= partition
}

func getOrdinal(pod *corev1.Pod) int32 {
	_, ordinal := getParentNameAndOrdinal(pod)
	return ordinal
}

func getParentNameAndOrdinal(pod *corev1.Pod) (string, int32) {
	parent := ""
	var ordinal int32 = -1
	subMatches := statefulPodRegex.FindStringSubmatch(pod.Name)
	if len(subMatches) < 3 {
		return parent, ordinal
	}
	parent = subMatches[1]
	if i, err := strconv.ParseInt(subMatches[2], 10, 32); err == nil {
		ordinal = int32(i)
	}
	return parent, ordinal
}

func (m *StatefulSetControl) objectKey(objMeta *metav1.ObjectMeta) client.ObjectKey {
	return types.NamespacedName{
		Namespace: objMeta.Namespace,
		Name:      objMeta.Name,
	}
}
