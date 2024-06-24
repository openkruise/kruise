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

package daemonset

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
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	imagejobutilfunc "github.com/openkruise/kruise/pkg/util/imagejob/utilfunction"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
)

func (dsc *ReconcileDaemonSet) createImagePullJobsForInPlaceUpdate(ds *appsv1alpha1.DaemonSet, oldRevisions []*apps.ControllerRevision, updateRevision *apps.ControllerRevision) error {
	if _, ok := updateRevision.Labels[appsv1alpha1.ImagePreDownloadCreatedKey]; ok {
		return nil
	} else if _, ok := updateRevision.Labels[appsv1alpha1.ImagePreDownloadIgnoredKey]; ok {
		return nil
	}

	// ignore if all Pods update in one batch
	var partition, maxUnavailable int
	var dsPodsNumber = int(ds.Status.DesiredNumberScheduled)
	if ds.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		partition = int(*ds.Spec.UpdateStrategy.RollingUpdate.Partition)
	}
	maxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(
		intstrutil.ValueOrDefault(ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable, intstrutil.FromInt(1)), dsPodsNumber, false)
	if partition == 0 && maxUnavailable >= dsPodsNumber {
		klog.V(4).InfoS("DaemonSet skipped to create ImagePullJob for all Pods update in one batch",
			"daemonSet", klog.KObj(ds), "replicas", dsPodsNumber, "partition", partition, "maxUnavailable", maxUnavailable)
		return dsc.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// start to create jobs
	var pullSecrets []string
	for _, s := range ds.Spec.Template.Spec.ImagePullSecrets {
		pullSecrets = append(pullSecrets, s.Name)
	}

	selector := ds.Spec.Selector.DeepCopy()
	selector.MatchExpressions = append(selector.MatchExpressions, metav1.LabelSelectorRequirement{
		Key:      apps.ControllerRevisionHashLabelKey,
		Operator: metav1.LabelSelectorOpNotIn,
		Values:   []string{updateRevision.Name, updateRevision.Labels[history.ControllerRevisionHashLabel]},
	})

	// As deamonset is the job's owner, we have the convention that all resources owned by deamonset
	// have to match the selector of deamonset, such as pod, pvc and controllerrevision.
	// So we had better put the labels into jobs.
	labelMap := make(map[string]string)
	for k, v := range ds.Spec.Template.Labels {
		labelMap[k] = v
	}
	labelMap[history.ControllerRevisionHashLabel] = updateRevision.Labels[history.ControllerRevisionHashLabel]

	annotationMap := make(map[string]string)
	for k, v := range ds.Spec.Template.Annotations {
		annotationMap[k] = v
	}

	containerImages := diffImagesBetweenRevisions(oldRevisions, updateRevision)
	klog.V(3).InfoS("DaemonSet begin to create ImagePullJobs for revision",
		"daemonSet", klog.KObj(ds), "revision", klog.KObj(updateRevision), "containerImageNames", containerImages)
	for name, image := range containerImages {
		// job name is revision name + container name, it can not be more than 255 characters
		jobName := fmt.Sprintf("%s-%s", updateRevision.Name, name)
		err := imagejobutilfunc.CreateJobForWorkload(dsc.Client, ds, controllerKind, jobName, image, labelMap, annotationMap, *selector, pullSecrets)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				klog.ErrorS(err, "DaemonSet failed to create ImagePullJob", "daemonSet", klog.KObj(ds), "jobName", jobName)
				dsc.eventRecorder.Eventf(ds, v1.EventTypeNormal, "FailedCreateImagePullJob", "failed to create ImagePullJob %s: %v", jobName, err)
			}
			continue
		}
		klog.V(3).InfoS("DaemonSet created ImagePullJob for image", "daemonSet", klog.KObj(ds), "jobName", jobName, "image", image)
		dsc.eventRecorder.Eventf(ds, v1.EventTypeNormal, "CreatedImagePullJob", "created ImagePullJob %s for image: %s", jobName, image)
	}

	return dsc.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadCreatedKey, "true")
}

func (dsc *ReconcileDaemonSet) patchControllerRevisionLabels(revision *apps.ControllerRevision, key, value string) error {
	oldRevision := revision.ResourceVersion
	newRevision := &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revision.Name,
			Namespace: revision.Namespace,
		},
	}
	body := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, key, value)
	if err := dsc.Patch(context.TODO(), newRevision, client.RawPatch(types.StrategicMergePatchType, []byte(body))); err != nil {
		return err
	}
	if oldRevision != newRevision.ResourceVersion {
		clonesetutils.ResourceVersionExpectations.Expect(newRevision)
	}
	return nil
}

func diffImagesBetweenRevisions(oldRevisions []*apps.ControllerRevision, newRevision *apps.ControllerRevision) map[string]string {
	var oldTemps []*v1.PodTemplateSpec
	for _, oldRevision := range oldRevisions {
		oldTemp, err := inplaceupdate.GetTemplateFromRevision(oldRevision)
		if err != nil {
			return nil
		}
		oldTemps = append(oldTemps, oldTemp)
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
		for _, oldTemp := range oldTemps {
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
		}
		if !found {
			containerImages[name] = newImage
		}
	}
	return containerImages
}
