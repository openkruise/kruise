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
		klog.V(4).InfoS("CloneSet skipped to create ImagePullJob for update type is ReCreate", "cloneSet", klog.KObj(cs))
		return r.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// ignore if replicas <= minimumReplicasToPreDownloadImage
	if *cs.Spec.Replicas <= minimumReplicasToPreDownloadImage {
		klog.V(4).InfoS("CloneSet skipped to create ImagePullJob because replicas less than or equal to the minimum threshold",
			"cloneSet", klog.KObj(cs), "replicas", *cs.Spec.Replicas, "minimumReplicasToPreDownloadImage", minimumReplicasToPreDownloadImage)
		return r.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// ignore if all Pods update in one batch
	var partition, maxUnavailable int
	if cs.Spec.UpdateStrategy.Partition != nil {
		pValue, err := util.CalculatePartitionReplicas(cs.Spec.UpdateStrategy.Partition, cs.Spec.Replicas)
		if err != nil {
			klog.ErrorS(err, "CloneSet partition value was illegal", "cloneSet", klog.KObj(cs))
			return err
		}
		partition = pValue
	}
	maxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(cs.Spec.UpdateStrategy.MaxUnavailable, intstrutil.FromString(appsv1alpha1.DefaultCloneSetMaxUnavailable)), int(*cs.Spec.Replicas), false)
	if partition == 0 && maxUnavailable >= int(*cs.Spec.Replicas) {
		klog.V(4).InfoS("CloneSet skipped to create ImagePullJob for all Pods update in one batch",
			"cloneSet", klog.KObj(cs), "replicas", *cs.Spec.Replicas, "partition", partition, "maxUnavailable", maxUnavailable)
		return r.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// ignore if this revision can not update in-place
	coreControl := clonesetcore.New(cs)
	inplaceControl := inplaceupdate.New(r.Client, clonesetutils.RevisionAdapterImpl)
	if !inplaceControl.CanUpdateInPlace(currentRevision, updateRevision, coreControl.GetUpdateOptions()) {
		klog.V(4).InfoS("CloneSet skipped to create ImagePullJob because in-place update was not possible",
			"cloneSet", klog.KObj(cs), "currentRevision", klog.KObj(currentRevision), "updateRevision", klog.KObj(updateRevision))
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

	annotationMap := make(map[string]string)
	for k, v := range cs.Spec.Template.Annotations {
		annotationMap[k] = v
	}

	containerImages := diffImagesBetweenRevisions(currentRevision, updateRevision)
	klog.V(3).InfoS("CloneSet began to create ImagePullJobs for the revision changes",
		"cloneSet", klog.KObj(cs), "currentRevision", klog.KObj(currentRevision), "updateRevision", klog.KObj(updateRevision), "containerImages", containerImages)
	for name, image := range containerImages {
		// job name is revision name + container name, it can not be more than 255 characters
		jobName := fmt.Sprintf("%s-%s", updateRevision.Name, name)
		err := imagejobutilfunc.CreateJobForWorkload(r.Client, cs, clonesetutils.ControllerKind, jobName, image, labelMap, annotationMap, *selector, pullSecrets)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				klog.ErrorS(err, "CloneSet failed to create ImagePullJob", "cloneSet", klog.KObj(cs), "jobName", jobName)
				r.recorder.Eventf(cs, v1.EventTypeNormal, "FailedCreateImagePullJob", "failed to create ImagePullJob %s: %v", jobName, err)
			}
			continue
		}
		klog.V(3).InfoS("CloneSet created ImagePullJob for the image", "cloneSet", klog.KObj(cs), "jobName", jobName, "image", image)
		r.recorder.Eventf(cs, v1.EventTypeNormal, "CreatedImagePullJob", "created ImagePullJob %s for image: %s", jobName, image)
	}

	return r.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadCreatedKey, "true")
}

func (r *ReconcileCloneSet) patchControllerRevisionLabels(revision *apps.ControllerRevision, key, value string) error {
	oldRevision := revision.ResourceVersion
	newRevision := &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revision.Name,
			Namespace: revision.Namespace,
		},
	}
	body := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, key, value)
	if err := r.Patch(context.TODO(), newRevision, client.RawPatch(types.StrategicMergePatchType, []byte(body))); err != nil {
		return err
	}
	if oldRevision != newRevision.ResourceVersion {
		clonesetutils.ResourceVersionExpectations.Expect(newRevision)
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
