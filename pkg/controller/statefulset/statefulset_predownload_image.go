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

package statefulset

import (
	"context"
	"fmt"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/controller/history"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util/expectations"
	imagejobutilfunc "github.com/openkruise/kruise/pkg/util/imagejob/utilfunction"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
	"github.com/openkruise/kruise/pkg/util/revisionadapter"
)

func (dss *defaultStatefulSetControl) createImagePullJobsForInPlaceUpdate(sts *appsv1beta1.StatefulSet, currentRevision, updateRevision *apps.ControllerRevision) error {
	if _, ok := updateRevision.Labels[appsv1alpha1.ImagePreDownloadCreatedKey]; ok {
		return nil
	} else if _, ok := updateRevision.Labels[appsv1alpha1.ImagePreDownloadIgnoredKey]; ok {
		return nil
	}

	// ignore if update type is OnDelete or pod update policy is ReCreate
	if string(sts.Spec.UpdateStrategy.Type) == string(apps.OnDeleteStatefulSetStrategyType) || sts.Spec.UpdateStrategy.RollingUpdate == nil ||
		string(sts.Spec.UpdateStrategy.RollingUpdate.PodUpdatePolicy) == string(appsv1beta1.RecreatePodUpdateStrategyType) {
		klog.V(4).InfoS("Statefulset skipped to create ImagePullJob for certain update type",
			"statefulSet", klog.KObj(sts), "updateType", sts.Spec.UpdateStrategy.Type)
		return dss.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// ignore if replicas <= minimumReplicasToPreDownloadImage
	if *sts.Spec.Replicas <= minimumReplicasToPreDownloadImage {
		klog.V(4).InfoS("Statefulset skipped to create ImagePullJob for replicas less than or equal to minimumReplicasToPreDownloadImage",
			"statefulSet", klog.KObj(sts), "replicas", *sts.Spec.Replicas, "minimumReplicasToPreDownloadImage", minimumReplicasToPreDownloadImage)
		return dss.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// ignore if all Pods update in one batch
	var partition, maxUnavailable int
	if sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		// partition, _ = intstrutil.GetValueFromIntOrPercent(sts.Spec.UpdateStrategy.Partition, int(*sts.Spec.Replicas), true)
		partition = int(*sts.Spec.UpdateStrategy.RollingUpdate.Partition)
	}
	maxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(sts.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable, intstrutil.FromString("1")), int(*sts.Spec.Replicas), false)
	if partition == 0 && maxUnavailable >= int(*sts.Spec.Replicas) {
		klog.V(4).InfoS("Statefulset skipped to create ImagePullJob for all Pods update in one batch",
			"statefulSet", klog.KObj(sts), "replicas", *sts.Spec.Replicas, "partition", partition, "maxUnavailable", maxUnavailable)
		return dss.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// opt is update option, this section is to get update option
	opts := &inplaceupdate.UpdateOptions{}
	if sts.Spec.UpdateStrategy.RollingUpdate.InPlaceUpdateStrategy != nil {
		opts.GracePeriodSeconds = sts.Spec.UpdateStrategy.RollingUpdate.InPlaceUpdateStrategy.GracePeriodSeconds
	}

	// ignore if this revision can not update in-place
	inplaceControl := inplaceupdate.New(sigsruntimeClient, revisionadapter.NewDefaultImpl())
	if !inplaceControl.CanUpdateInPlace(currentRevision, updateRevision, opts) {
		klog.V(4).InfoS("Statefulset skipped to create ImagePullJob because it could not update in-place",
			"statefulSet", klog.KObj(sts), "currentRevisionName", currentRevision.Name, "updateRevisionName", updateRevision.Name)
		return dss.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// start to create jobs
	var pullSecrets []string
	for _, s := range sts.Spec.Template.Spec.ImagePullSecrets {
		pullSecrets = append(pullSecrets, s.Name)
	}

	selector := sts.Spec.Selector.DeepCopy()
	selector.MatchExpressions = append(selector.MatchExpressions, metav1.LabelSelectorRequirement{
		Key:      apps.ControllerRevisionHashLabelKey,
		Operator: metav1.LabelSelectorOpNotIn,
		Values:   []string{updateRevision.Name, updateRevision.Labels[history.ControllerRevisionHashLabel]},
	})

	// As statefulset is the job's owner, we have the convention that all resources owned by statefulset
	// have to match the selector of statefulset, such as pod, pvc and controllerrevision.
	// So we had better put the labels into jobs.
	labelMap := make(map[string]string)
	for k, v := range sts.Spec.Template.Labels {
		labelMap[k] = v
	}
	labelMap[history.ControllerRevisionHashLabel] = updateRevision.Labels[history.ControllerRevisionHashLabel]

	annotationMap := make(map[string]string)
	for k, v := range sts.Spec.Template.Annotations {
		annotationMap[k] = v
	}

	containerImages := diffImagesBetweenRevisions(currentRevision, updateRevision)
	klog.V(3).InfoS("Statefulset began to create ImagePullJobs", "statefulSet", klog.KObj(sts), "currentRevisionName", currentRevision.Name,
		"updateRevisionName", updateRevision.Name, "containerImages", containerImages)
	for name, image := range containerImages {
		// job name is revision name + container name, it can not be more than 255 characters
		jobName := fmt.Sprintf("%s-%s", updateRevision.Name, name)
		err := imagejobutilfunc.CreateJobForWorkload(sigsruntimeClient, sts, controllerKind, jobName, image, labelMap, annotationMap, *selector, pullSecrets)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				klog.ErrorS(err, "Statefulset failed to create ImagePullJob", "statefulSet", klog.KObj(sts), "jobName", jobName)
				dss.recorder.Eventf(sts, v1.EventTypeNormal, "FailedCreateImagePullJob", "failed to create ImagePullJob %s: %v", jobName, err)
			}
			continue
		}
		klog.V(3).InfoS("Statefulset created ImagePullJob for image", "statefulSet", klog.KObj(sts), "jobName", jobName, "image", image)
		dss.recorder.Eventf(sts, v1.EventTypeNormal, "CreatedImagePullJob", "created ImagePullJob %s for image: %s", jobName, image)
	}

	return dss.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadCreatedKey, "true")
}

func (dss *defaultStatefulSetControl) patchControllerRevisionLabels(revision *apps.ControllerRevision, key, value string) error {
	oldRevision := revision.ResourceVersion
	newRevision := &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revision.Name,
			Namespace: revision.Namespace,
		},
	}
	body := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, key, value)
	if err := sigsruntimeClient.Patch(context.TODO(), newRevision, client.RawPatch(types.StrategicMergePatchType, []byte(body))); err != nil {
		return err
	}
	if oldRevision != newRevision.ResourceVersion {
		expectations.NewResourceVersionExpectation()
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
