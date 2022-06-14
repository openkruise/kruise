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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

func (r *ReconcileUnitedDeployment) manageSubsets(ud *appsv1alpha1.UnitedDeployment, nameToSubset *map[string]*Subset, nextUpdate map[string]SubsetUpdate, currentRevision, updatedRevision *appsv1.ControllerRevision, subsetType subSetType) (newStatus *appsv1alpha1.UnitedDeploymentStatus, updateErr error) {
	newStatus = ud.Status.DeepCopy()
	exists, provisioned, err := r.manageSubsetProvision(ud, nameToSubset, nextUpdate, currentRevision, updatedRevision, subsetType)
	if err != nil {
		SetUnitedDeploymentCondition(newStatus, NewUnitedDeploymentCondition(appsv1alpha1.SubsetProvisioned, corev1.ConditionFalse, "Error", err.Error()))
		return newStatus, fmt.Errorf("fail to manage Subset provision: %s", err)
	}

	if provisioned {
		SetUnitedDeploymentCondition(newStatus, NewUnitedDeploymentCondition(appsv1alpha1.SubsetProvisioned, corev1.ConditionTrue, "", ""))
	}

	expectedRevision := currentRevision
	if updatedRevision != nil {
		expectedRevision = updatedRevision
	}

	var needUpdate []string
	for _, name := range exists.List() {
		subset := (*nameToSubset)[name]
		if r.subSetControls[subsetType].IsExpected(subset, expectedRevision.Name) ||
			subset.Spec.Replicas != nextUpdate[name].Replicas ||
			subset.Spec.UpdateStrategy.Partition != nextUpdate[name].Partition ||
			subset.GetAnnotations()[appsv1alpha1.AnnotationSubsetPatchKey] != nextUpdate[name].Patch {
			needUpdate = append(needUpdate, name)
		}
	}

	if len(needUpdate) > 0 {
		_, updateErr = util.SlowStartBatch(len(needUpdate), slowStartInitialBatchSize, func(index int) error {
			cell := needUpdate[index]
			subset := (*nameToSubset)[cell]
			replicas := nextUpdate[cell].Replicas
			partition := nextUpdate[cell].Partition

			klog.V(0).Infof("UnitedDeployment %s/%s needs to update Subset (%s) %s/%s with revision %s, replicas %d, partition %d", ud.Namespace, ud.Name, subsetType, subset.Namespace, subset.Name, expectedRevision.Name, replicas, partition)
			updateSubsetErr := r.subSetControls[subsetType].UpdateSubset(subset, ud, expectedRevision.Name, replicas, partition)
			if updateSubsetErr != nil {
				r.recorder.Event(ud.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("Failed%s", eventTypeSubsetsUpdate), fmt.Sprintf("Error updating PodSet (%s) %s when updating: %s", subsetType, subset.Name, updateSubsetErr))
			}
			return updateSubsetErr
		})
	}

	if updateErr == nil {
		SetUnitedDeploymentCondition(newStatus, NewUnitedDeploymentCondition(appsv1alpha1.SubsetUpdated, corev1.ConditionTrue, "", ""))
	} else {
		SetUnitedDeploymentCondition(newStatus, NewUnitedDeploymentCondition(appsv1alpha1.SubsetUpdated, corev1.ConditionFalse, "Error", updateErr.Error()))
	}
	return
}

func (r *ReconcileUnitedDeployment) manageSubsetProvision(ud *appsv1alpha1.UnitedDeployment, nameToSubset *map[string]*Subset, nextUpdate map[string]SubsetUpdate, currentRevision, updatedRevision *appsv1.ControllerRevision, subsetType subSetType) (sets.String, bool, error) {
	expectedSubsets := sets.String{}
	gotSubsets := sets.String{}

	for _, subset := range ud.Spec.Topology.Subsets {
		expectedSubsets.Insert(subset.Name)
	}

	for subsetName := range *nameToSubset {
		gotSubsets.Insert(subsetName)
	}
	klog.V(4).Infof("UnitedDeployment %s/%s has subsets %v, expects subsets %v", ud.Namespace, ud.Name, gotSubsets.List(), expectedSubsets.List())

	creates := expectedSubsets.Difference(gotSubsets).List()
	deletes := gotSubsets.Difference(expectedSubsets).List()

	revision := currentRevision.Name
	if updatedRevision != nil {
		revision = updatedRevision.Name
	}

	var errs []error
	// manage creating
	if len(creates) > 0 {
		// do not consider deletion
		klog.V(0).Infof("UnitedDeployment %s/%s needs creating subset (%s) with name: %v", ud.Namespace, ud.Name, subsetType, creates)
		createdSubsets := make([]string, len(creates))
		for i, subset := range creates {
			createdSubsets[i] = subset
		}

		var createdNum int
		var createdErr error
		createdNum, createdErr = util.SlowStartBatch(len(creates), slowStartInitialBatchSize, func(idx int) error {
			subsetName := createdSubsets[idx]

			replicas := nextUpdate[subsetName].Replicas
			partition := nextUpdate[subsetName].Partition
			err := r.subSetControls[subsetType].CreateSubset(ud, subsetName, revision, replicas, partition)
			if err != nil {
				if !errors.IsTimeout(err) {
					return fmt.Errorf("fail to create Subset (%s) %s: %s", subsetType, subsetName, err.Error())
				}
			}

			return nil
		})
		if createdErr == nil {
			r.recorder.Eventf(ud.DeepCopy(), corev1.EventTypeNormal, fmt.Sprintf("Successful%s", eventTypeSubsetsUpdate), "Create %d Subset (%s)", createdNum, subsetType)
		} else {
			errs = append(errs, createdErr)
		}
	}

	// manage deleting
	if len(deletes) > 0 {
		klog.V(0).Infof("UnitedDeployment %s/%s needs deleting subset (%s) with name: [%v]", ud.Namespace, ud.Name, subsetType, deletes)
		var deleteErrs []error
		for _, subsetName := range deletes {
			subset := (*nameToSubset)[subsetName]
			if err := r.subSetControls[subsetType].DeleteSubset(subset); err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("fail to delete Subset (%s) %s/%s for %s: %s", subsetType, subset.Namespace, subset.Name, subsetName, err))
			}
		}

		if len(deleteErrs) > 0 {
			errs = append(errs, deleteErrs...)
		} else {
			r.recorder.Eventf(ud.DeepCopy(), corev1.EventTypeNormal, fmt.Sprintf("Successful%s", eventTypeSubsetsUpdate), "Delete %d Subset (%s)", len(deletes), subsetType)
		}
	}

	// clean the other kind of subsets
	cleaned := false
	for t, control := range r.subSetControls {
		if t == subsetType {
			continue
		}

		subsets, err := control.GetAllSubsets(ud, revision)
		if err != nil {
			errs = append(errs, fmt.Errorf("fail to list Subset of other type %s for UnitedDeployment %s/%s: %s", t, ud.Namespace, ud.Name, err))
			continue
		}

		for _, subset := range subsets {
			cleaned = true
			if err := control.DeleteSubset(subset); err != nil {
				errs = append(errs, fmt.Errorf("fail to delete Subset %s of other type %s for UnitedDeployment %s/%s: %s", subset.Name, t, ud.Namespace, ud.Name, err))
				continue
			}
		}
	}

	return expectedSubsets.Intersection(gotSubsets), len(creates) > 0 || len(deletes) > 0 || cleaned, utilerrors.NewAggregate(errs)
}
