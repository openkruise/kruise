/*
Copyright 2020 The Kruise Authors.

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
	"k8s.io/klog/v2"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util/refmanager"
)

// DeploymentAdapter implements the Adapter interface for Deployment objects
type DeploymentAdapter struct {
	client.Client

	Scheme *runtime.Scheme
}

// NewResourceObject creates a empty Deployment object.
func (a *DeploymentAdapter) NewResourceObject() client.Object {
	return &appsv1.Deployment{}
}

// NewResourceListObject creates a empty DeploymentList object.
func (a *DeploymentAdapter) NewResourceListObject() client.ObjectList {
	return &appsv1.DeploymentList{}
}

// GetStatusObservedGeneration returns the observed generation of the subset.
func (a *DeploymentAdapter) GetStatusObservedGeneration(obj metav1.Object) int64 {
	return obj.(*appsv1.Deployment).Status.ObservedGeneration
}

func (a *DeploymentAdapter) GetSubsetPods(obj metav1.Object) ([]*corev1.Pod, error) {
	set := obj.(*appsv1.Deployment)
	return a.getDeploymentPods(set)
}

func (a *DeploymentAdapter) GetSpecReplicas(obj metav1.Object) *int32 {
	set := obj.(*appsv1.Deployment)
	return set.Spec.Replicas
}

func (a *DeploymentAdapter) GetSpecPartition(_ metav1.Object, _ []*corev1.Pod) *int32 {
	return nil
}

func (a *DeploymentAdapter) GetStatusReplicas(obj metav1.Object) int32 {
	set := obj.(*appsv1.Deployment)
	return set.Status.Replicas
}

func (a *DeploymentAdapter) GetStatusReadyReplicas(obj metav1.Object) int32 {
	set := obj.(*appsv1.Deployment)
	return set.Status.ReadyReplicas
}

// GetSubsetFailure returns the failure information of the subset.
// Deployment has no condition.
func (a *DeploymentAdapter) GetSubsetFailure() *string {
	return nil
}

// ApplySubsetTemplate updates the subset to the latest revision, depending on the DeploymentTemplate.
func (a *DeploymentAdapter) ApplySubsetTemplate(ud *alpha1.UnitedDeployment, subsetName, revision string, replicas, _ int32, obj runtime.Object) error {
	// Convert to Deployment Object
	set := obj.(*appsv1.Deployment)

	// Retrieve subset configuration based on UnitedDeployment spec
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

	// Set correct labels
	if set.Labels == nil {
		set.Labels = map[string]string{}
	}
	for k, v := range ud.Spec.Template.DeploymentTemplate.Labels {
		set.Labels[k] = v
	}
	for k, v := range ud.Spec.Selector.MatchLabels {
		set.Labels[k] = v
	}
	set.Labels[alpha1.ControllerRevisionHashLabelKey] = revision
	// record the subset name as a label
	set.Labels[alpha1.SubSetNameLabelKey] = subsetName

	// Set correct annotations
	if set.Annotations == nil {
		set.Annotations = map[string]string{}
	}
	for k, v := range ud.Spec.Template.DeploymentTemplate.Annotations {
		set.Annotations[k] = v
	}

	// Generate unique name for deployment
	set.GenerateName = getSubsetPrefix(ud.Name, subsetName)

	// Set correct selectors
	selectors := ud.Spec.Selector.DeepCopy()
	selectors.MatchLabels[alpha1.SubSetNameLabelKey] = subsetName

	// Set Deployment object's owner reference to UnitedDeployment object
	if err := controllerutil.SetControllerReference(ud, set, a.Scheme); err != nil {
		return err
	}

	set.Spec = *ud.Spec.Template.DeploymentTemplate.Spec.DeepCopy()
	set.Spec.Selector = selectors
	set.Spec.Replicas = &replicas
	if set.Spec.Template.Labels == nil {
		set.Spec.Template.Labels = map[string]string{}
	}
	set.Spec.Template.Labels[alpha1.SubSetNameLabelKey] = subsetName
	set.Spec.Template.Labels[alpha1.ControllerRevisionHashLabelKey] = revision

	attachNodeAffinity(&set.Spec.Template.Spec, subSetConfig)
	attachTolerations(&set.Spec.Template.Spec, subSetConfig)

	if subSetConfig.Patch.Raw != nil {
		TemplateSpecBytes, _ := json.Marshal(set.Spec.Template)
		modified, err := strategicpatch.StrategicMergePatch(TemplateSpecBytes, subSetConfig.Patch.Raw, &corev1.PodTemplateSpec{})
		if err != nil {
			klog.ErrorS(err, "Failed to merge patch raw", "patch", subSetConfig.Patch.Raw)
			return err
		}
		patchedTemplateSpec := corev1.PodTemplateSpec{}
		if err = json.Unmarshal(modified, &patchedTemplateSpec); err != nil {
			klog.ErrorS(err, "Failed to unmarshal modified JSON to podTemplateSpec", "JSON", modified)
			return err
		}

		set.Spec.Template = patchedTemplateSpec
		klog.V(2).InfoS("Deployment was patched successfully", "deployment", klog.KRef(set.Namespace, set.GenerateName), "patch", subSetConfig.Patch.Raw)
	}
	if set.Annotations == nil {
		set.Annotations = make(map[string]string)
	}
	set.Annotations[alpha1.AnnotationSubsetPatchKey] = string(subSetConfig.Patch.Raw)

	return nil
}

// PostUpdate does some works after subset updated. Deployments typically don't have post update operations.
func (a *DeploymentAdapter) PostUpdate(_ *alpha1.UnitedDeployment, _ runtime.Object, _ string, _ int32) error {
	return nil
}

// getDeploymentPods gets all Pods under a Deployment object
func (a *DeploymentAdapter) getDeploymentPods(set *appsv1.Deployment) ([]*corev1.Pod, error) {
	deploymentReplicaSets, err := a.getDeploymentReplicaSets(set)
	if err != nil {
		return nil, err
	}

	var deploymentPods []*corev1.Pod
	for _, replicaSet := range deploymentReplicaSets {
		replicaSetPods, err := a.getReplicaSetPods(replicaSet)
		if err != nil {
			return nil, err
		}

		deploymentPods = append(deploymentPods, replicaSetPods...)
	}

	return deploymentPods, nil
}

// getDeploymentReplicaSets gets all ReplicaSets under a Deployment object
func (a *DeploymentAdapter) getDeploymentReplicaSets(set *appsv1.Deployment) ([]*appsv1.ReplicaSet, error) {
	// Retrieve correct selectors to use
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}

	// Retrieve ReplicaSets based on selectors
	replicaSetList := &appsv1.ReplicaSetList{}
	err = a.Client.List(context.TODO(), replicaSetList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}

	// The remainder of the function retrieves ReplicaSets owned by the set Deployment argument
	manager, err := refmanager.New(a.Client, set.Spec.Selector, set, a.Scheme)
	if err != nil {
		return nil, err
	}

	selected := make([]metav1.Object, len(replicaSetList.Items))
	for i, replicaSet := range replicaSetList.Items {
		selected[i] = replicaSet.DeepCopy()
	}

	claimed, err := manager.ClaimOwnedObjects(selected)
	if err != nil {
		return nil, err
	}

	claimedReplicaSets := make([]*appsv1.ReplicaSet, len(claimed))
	for i, replicaSet := range claimed {
		claimedReplicaSets[i] = replicaSet.(*appsv1.ReplicaSet)
	}

	return claimedReplicaSets, nil
}

// getReplicaSetPods gets all pods under a ReplicaSet object
func (a *DeploymentAdapter) getReplicaSetPods(set *appsv1.ReplicaSet) ([]*corev1.Pod, error) {
	// Retrieve correct selectors to use
	selector, err := metav1.LabelSelectorAsSelector(set.Spec.Selector)
	if err != nil {
		return nil, err
	}

	// Retrieve all pods using selector
	podList := &corev1.PodList{}
	err = a.Client.List(context.TODO(), podList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}

	// The remainder of this function retrieves Pods owned by the set ReplicaSet argument
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
