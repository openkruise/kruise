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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/refmanager"
)

type AdvancedStatefulSetAdapter struct {
	client.Client

	Scheme *runtime.Scheme
}

// NewResourceObject creates a empty AdvancedStatefulSet object.
func (a *AdvancedStatefulSetAdapter) NewResourceObject() runtime.Object {
	return &alpha1.StatefulSet{}
}

// NewResourceListObject creates a empty AdvancedStatefulSet object.
func (a *AdvancedStatefulSetAdapter) NewResourceListObject() runtime.Object {
	return &alpha1.StatefulSetList{}
}

// GetObjectMeta returns the ObjectMeta of the subset of AdvancedStatefulSet.
func (a *AdvancedStatefulSetAdapter) GetObjectMeta(obj metav1.Object) *metav1.ObjectMeta {
	return &obj.(*alpha1.StatefulSet).ObjectMeta
}

// GetStatusObservedGeneration returns the observed generation of the subset.
func (a *AdvancedStatefulSetAdapter) GetStatusObservedGeneration(obj metav1.Object) int64 {
	return obj.(*alpha1.StatefulSet).Status.ObservedGeneration
}

// GetReplicaDetails returns the replicas detail the subset needs.
func (a *AdvancedStatefulSetAdapter) GetReplicaDetails(obj metav1.Object, updatedRevision string) (specReplicas, specPartition *int32, statusReplicas, statusReadyReplicas, statusUpdatedReplicas, statusUpdatedReadyReplicas int32, err error) {
	set := obj.(*alpha1.StatefulSet)
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
// AdvancedStatefulSet has no condition.
func (a *AdvancedStatefulSetAdapter) GetSubsetFailure() *string {
	return nil
}

// ConvertToResourceList converts AdvancedStatefulSetList object to AdvancedStatefulSet array.
func (a *AdvancedStatefulSetAdapter) ConvertToResourceList(obj runtime.Object) []metav1.Object {
	stsList := obj.(*alpha1.StatefulSetList)
	objList := make([]metav1.Object, len(stsList.Items))
	for i, set := range stsList.Items {
		objList[i] = set.DeepCopy()
	}

	return objList
}

// ApplySubsetTemplate updates the subset to the latest revision, depending on the AdvancedStatefulSetTemplate.
func (a *AdvancedStatefulSetAdapter) ApplySubsetTemplate(ud *alpha1.UnitedDeployment, subsetName, revision string, replicas, partition int32, obj runtime.Object) error {
	set := obj.(*alpha1.StatefulSet)

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
	for k, v := range ud.Spec.Template.AdvancedStatefulSetTemplate.Labels {
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
	for k, v := range ud.Spec.Template.AdvancedStatefulSetTemplate.Annotations {
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
	if ud.Spec.Template.AdvancedStatefulSetTemplate.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		set.Spec.UpdateStrategy.Type = appsv1.OnDeleteStatefulSetStrategyType
	} else {
		set.Spec.UpdateStrategy.RollingUpdate = ud.Spec.Template.AdvancedStatefulSetTemplate.Spec.UpdateStrategy.RollingUpdate
		if set.Spec.UpdateStrategy.RollingUpdate == nil {
			set.Spec.UpdateStrategy.RollingUpdate = &alpha1.RollingUpdateStatefulSetStrategy{}
		}
		set.Spec.UpdateStrategy.RollingUpdate.Partition = &partition
	}

	set.Spec.Template = *ud.Spec.Template.AdvancedStatefulSetTemplate.Spec.Template.DeepCopy()
	if set.Spec.Template.Labels == nil {
		set.Spec.Template.Labels = map[string]string{}
	}
	set.Spec.Template.Labels[alpha1.SubSetNameLabelKey] = subsetName
	set.Spec.Template.Labels[alpha1.ControllerRevisionHashLabelKey] = revision

	set.Spec.RevisionHistoryLimit = ud.Spec.Template.AdvancedStatefulSetTemplate.Spec.RevisionHistoryLimit
	set.Spec.PodManagementPolicy = ud.Spec.Template.AdvancedStatefulSetTemplate.Spec.PodManagementPolicy
	set.Spec.ServiceName = ud.Spec.Template.AdvancedStatefulSetTemplate.Spec.ServiceName
	set.Spec.VolumeClaimTemplates = ud.Spec.Template.AdvancedStatefulSetTemplate.Spec.VolumeClaimTemplates

	attachNodeAffinity(&set.Spec.Template.Spec, subSetConfig)
	attachTolerations(&set.Spec.Template.Spec, subSetConfig)

	return nil
}

// PostUpdate does some works after subset updated.
func (a *AdvancedStatefulSetAdapter) PostUpdate(ud *alpha1.UnitedDeployment, obj runtime.Object, revision string, partition int32) error {
	return nil
}

// IsExpected checks the subset is the expected revision or not.
// The revision label can tell the current subset revision.
func (a *AdvancedStatefulSetAdapter) IsExpected(obj metav1.Object, revision string) bool {
	return obj.GetLabels()[alpha1.ControllerRevisionHashLabelKey] != revision
}

func (a *AdvancedStatefulSetAdapter) getStatefulSetPods(set *alpha1.StatefulSet) ([]*corev1.Pod, error) {
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
