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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

// DeploymentAdapter implements the Adapter interface for Deployment objects
type DeploymentAdapter struct {
	client.Client

	Scheme *runtime.Scheme
}

// NewResourceObject creates a empty Deployment object.
func (a *DeploymentAdapter) NewResourceObject() runtime.Object {
	return &appsv1.Deployment{}
}

// NewResourceListObject creates a empty DeploymentList object.
func (a *DeploymentAdapter) NewResourceListObject() runtime.Object {
	return &appsv1.DeploymentList{}
}

// GetStatusObservedGeneration returns the observed generation of the subset.
func (a *DeploymentAdapter) GetStatusObservedGeneration(obj metav1.Object) int64 {
	return obj.(*appsv1.Deployment).Status.ObservedGeneration
}

// GetReplicaDetails returns the replicas detail the subset needs.
func (a *DeploymentAdapter) GetReplicaDetails(obj metav1.Object, updatedRevision string) (specReplicas, specPartition *int32, statusReplicas, statusReadyReplicas, statusUpdatedReplicas, statusUpdatedReadyReplicas int32, err error) {
	// Convert to Deployment Object
	set := obj.(*appsv1.Deployment)

	// Set according replica counts
	specReplicas = set.Spec.Replicas
	statusReplicas = set.Status.Replicas
	statusReadyReplicas = set.Status.ReadyReplicas
	statusUpdatedReplicas = set.Status.UpdatedReplicas
	statusUpdatedReadyReplicas = set.Status.UpdatedReplicas

	return
}

// GetSubsetFailure returns the failure information of the subset.
// Deployment has no condition.
func (a *DeploymentAdapter) GetSubsetFailure() *string {
	return nil
}

// ApplySubsetTemplate updates the subset to the latest revision, depending on the DeploymentTemplate.
func (a *DeploymentAdapter) ApplySubsetTemplate(ud *alpha1.UnitedDeployment, subsetName, revision string, replicas, partition int32, obj runtime.Object) error {
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

	set.Spec.Selector = selectors
	set.Spec.Replicas = &replicas
	set.Spec.Template = *ud.Spec.Template.DeploymentTemplate.Spec.Template.DeepCopy()
	if set.Spec.Template.Labels == nil {
		set.Spec.Template.Labels = map[string]string{}
	}
	set.Spec.Template.Labels[alpha1.SubSetNameLabelKey] = subsetName
	set.Spec.Template.Labels[alpha1.ControllerRevisionHashLabelKey] = revision
	set.Spec.RevisionHistoryLimit = ud.Spec.Template.DeploymentTemplate.Spec.RevisionHistoryLimit

	attachNodeAffinity(&set.Spec.Template.Spec, subSetConfig)
	attachTolerations(&set.Spec.Template.Spec, subSetConfig)

	return nil
}

// PostUpdate does some works after subset updated. StatefulSets will implement this method to clean stuck pods.
func (a *DeploymentAdapter) PostUpdate(ud *alpha1.UnitedDeployment, obj runtime.Object, revision string, partition int32) error {
	return nil
}

// IsExpected checks the subset is the expected revision or not.
// The revision label can tell the current subset revision.
func (a *DeploymentAdapter) IsExpected(obj metav1.Object, revision string) bool {
	return obj.GetLabels()[appsv1.ControllerRevisionHashLabelKey] != revision
}
