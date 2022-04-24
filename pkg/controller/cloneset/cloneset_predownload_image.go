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

package cloneset

import (
	"context"
	"fmt"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetcore "github.com/openkruise/kruise/pkg/controller/cloneset/core"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
	imagejobutilfunc "github.com/openkruise/kruise/pkg/util/imagejob/utilfunction"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReconcileCloneSet) createImagePullJobsForInPlaceUpdate(cs *appsv1alpha1.CloneSet, currentRevision, updateRevision *apps.ControllerRevision) error {
	if _, ok := updateRevision.Labels[appsv1alpha1.ImagePreDownloadCreatedKey]; ok {
		return nil
	} else if _, ok := updateRevision.Labels[appsv1alpha1.ImagePreDownloadIgnoredKey]; ok {
		return nil
	}

	// ignore if update type is ReCreate
	if cs.Spec.UpdateStrategy.Type == appsv1alpha1.RecreateCloneSetUpdateStrategyType {
		klog.V(4).Infof("CloneSet %s/%s skipped to create ImagePullJob for update type is %s",
			cs.Namespace, cs.Name, cs.Spec.UpdateStrategy.Type)
		return r.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// ignore if replicas <= minimumReplicasToPreDownloadImage
	if *cs.Spec.Replicas <= minimumReplicasToPreDownloadImage {
		klog.V(4).Infof("CloneSet %s/%s skipped to create ImagePullJob for replicas %d <= %d",
			cs.Namespace, cs.Name, *cs.Spec.Replicas, minimumReplicasToPreDownloadImage)
		return r.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// ignore if all Pods update in one batch
	var partition, maxUnavailable int
	if cs.Spec.UpdateStrategy.Partition != nil {
		if pValue, err := util.CalculatePartitionReplicas(cs.Spec.UpdateStrategy.Partition, cs.Spec.Replicas); err != nil {
			klog.Errorf("CloneSet %s/%s partition value is illegal", cs.Namespace, cs.Name)
			return err
		} else {
			partition = pValue
		}
	}
	maxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(cs.Spec.UpdateStrategy.MaxUnavailable, intstrutil.FromString(appsv1alpha1.DefaultCloneSetMaxUnavailable)), int(*cs.Spec.Replicas), false)
	if partition == 0 && maxUnavailable >= int(*cs.Spec.Replicas) {
		klog.V(4).Infof("CloneSet %s/%s skipped to create ImagePullJob for all Pods update in one batch, replicas=%d, partition=%d, maxUnavailable=%d",
			cs.Namespace, cs.Name, *cs.Spec.Replicas, partition, maxUnavailable)
		return r.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// ignore if this revision can not update in-place
	coreControl := clonesetcore.New(cs)
	inplaceControl := inplaceupdate.New(r.Client, clonesetutils.RevisionAdapterImpl)
	if !inplaceControl.CanUpdateInPlace(currentRevision, updateRevision, coreControl.GetUpdateOptions()) {
		klog.V(4).Infof("CloneSet %s/%s skipped to create ImagePullJob for %s -> %s can not update in-place",
			cs.Namespace, cs.Name, currentRevision.Name, updateRevision.Name)
		return r.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// start to create jobs

	var pullSecrets []string
	for _, s := range cs.Spec.Template.Spec.ImagePullSecrets {
		pullSecrets = append(pullSecrets, s.Name)
	}

	selector := cs.Spec.Selector.DeepCopy()
	selector.MatchExpressions = append(selector.MatchExpressions, metav1.LabelSelectorRequirement{
		Key:      apps.ControllerRevisionHashLabelKey,
		Operator: metav1.LabelSelectorOpNotIn,
		Values:   []string{updateRevision.Name, updateRevision.Labels[history.ControllerRevisionHashLabel]},
	})

	// As cloneset is the job's owner, we have the convention that all resources owned by cloneset
	// have to match the selector of cloneset, such as pod, pvc and controllerrevision.
	// So we had better put the labels into jobs.
	labelMap := make(map[string]string)
	for k, v := range cs.Spec.Template.Labels {
		labelMap[k] = v
	}
	labelMap[history.ControllerRevisionHashLabel] = updateRevision.Labels[history.ControllerRevisionHashLabel]

	containerImages := diffImagesBetweenRevisions(currentRevision, updateRevision)
	klog.V(3).Infof("CloneSet %s/%s begin to create ImagePullJobs for revision %s -> %s: %v",
		cs.Namespace, cs.Name, currentRevision.Name, updateRevision.Name, containerImages)
	for name, image := range containerImages {
		// job name is revision name + container name, it can not be more than 255 characters
		jobName := fmt.Sprintf("%s-%s", updateRevision.Name, name)
		err := imagejobutilfunc.CreateJobForWorkload(r.Client, cs, clonesetutils.ControllerKind, jobName, image, labelMap, *selector, pullSecrets)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				klog.Errorf("CloneSet %s/%s failed to create ImagePullJob %s: %v", cs.Namespace, cs.Name, jobName, err)
				r.recorder.Eventf(cs, v1.EventTypeNormal, "FailedCreateImagePullJob", "failed to create ImagePullJob %s: %v", jobName, err)
			}
			continue
		}
		klog.V(3).Infof("CloneSet %s/%s created ImagePullJob %s for image: %s", cs.Namespace, cs.Name, jobName, image)
		r.recorder.Eventf(cs, v1.EventTypeNormal, "CreatedImagePullJob", "created ImagePullJob %s for image: %s", jobName, image)
	}

	return r.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadCreatedKey, "true")
}

func (r *ReconcileCloneSet) patchControllerRevisionLabels(revision *apps.ControllerRevision, key, value string) error {
	oldRevision := revision.ResourceVersion
	body := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, key, value)
	if err := r.Patch(context.TODO(), revision, client.RawPatch(types.StrategicMergePatchType, []byte(body))); err != nil {
		return err
	}
	if oldRevision != revision.ResourceVersion {
		clonesetutils.ResourceVersionExpectations.Expect(revision)
	}
	return nil
}

func diffImagesBetweenRevisions(oldRevision, newRevision *apps.ControllerRevision) map[string]string {
	oldTemp, err := inplaceupdate.GetTemplateFromRevision(oldRevision)
	if err != nil {
		return nil
	}
	newTemp, err := inplaceupdate.GetTemplateFromRevision(newRevision)
	if err != nil {
		return nil
	}

	containerImages := make(map[string]string)
	for i := range newTemp.Spec.Containers {
		name := newTemp.Spec.Containers[i].Name
		newImage := newTemp.Spec.Containers[i].Image

		var found bool
		for j := range oldTemp.Spec.Containers {
			if oldTemp.Spec.Containers[j].Name != name {
				continue
			}
			if oldTemp.Spec.Containers[j].Image != newImage {
				containerImages[name] = newImage
			}
			found = true
			break
		}
		if !found {
			containerImages[name] = newImage
		}
	}
	return containerImages
}
