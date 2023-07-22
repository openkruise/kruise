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

package sidecarset

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
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/util"
	imagejobutilfunc "github.com/openkruise/kruise/pkg/util/imagejob/utilfunction"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
)

func (p *Processor) createImagePullJobsForInPlaceUpdate(sidecarset *appsv1alpha1.SidecarSet, pods []*v1.Pod, currentRevision, updateRevision *apps.ControllerRevision) error {
	if _, ok := updateRevision.Labels[appsv1alpha1.ImagePreDownloadCreatedKey]; ok {
		return nil
	} else if _, ok := updateRevision.Labels[appsv1alpha1.ImagePreDownloadIgnoredKey]; ok {
		return nil
	}

	// ignore if update type is NotUpdate
	if sidecarset.Spec.UpdateStrategy.Type == appsv1alpha1.NotUpdateSidecarSetStrategyType {
		klog.V(4).Infof("SidecarSet %s/%s skipped to create ImagePullJob for update type is %s",
			sidecarset.Namespace, sidecarset.Name, sidecarset.Spec.UpdateStrategy.Type)
		return p.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// ignore if all Pods update in one batch
	var partition, maxUnavailable int
	managedPodNum := len(pods)
	if sidecarset.Spec.UpdateStrategy.Partition != nil {
		pValue, err := util.CalculatePartitionReplicas(sidecarset.Spec.UpdateStrategy.Partition, pointer.Int32(int32(managedPodNum)))
		if err != nil {
			klog.Errorf("SidecarSet %s/%s partition value is illegal", sidecarset.Namespace, sidecarset.Name)
			return err
		}
		partition = pValue
	}
	maxUnavailable, _ = intstrutil.GetValueFromIntOrPercent(sidecarset.Spec.UpdateStrategy.MaxUnavailable, managedPodNum, false)
	if partition == 0 && maxUnavailable >= managedPodNum {
		klog.V(4).Infof("SidecarSet %s/%s skipped to create ImagePullJob for all Pods update in one batch, podNum=%d, partition=%d, maxUnavailable=%d",
			sidecarset.Namespace, sidecarset.Name, managedPodNum, partition, maxUnavailable)
		return p.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadIgnoredKey, "true")
	}

	// start to create jobs

	var pullSecrets []string
	for _, s := range sidecarset.Spec.ImagePullSecrets {
		pullSecrets = append(pullSecrets, s.Name)
	}

	selector := sidecarset.Spec.Selector.DeepCopy()
	selector.MatchExpressions = append(selector.MatchExpressions, metav1.LabelSelectorRequirement{
		Key:      apps.ControllerRevisionHashLabelKey,
		Operator: metav1.LabelSelectorOpNotIn,
		Values:   []string{updateRevision.Name, updateRevision.Labels[history.ControllerRevisionHashLabel]},
	})

	// As sidecarset is the job's owner, we have the convention that all resources owned by sidecarset
	// have to match the selector of sidecarset, such as pod, pvc and controllerrevision.
	// So we had better put the labels into jobs.
	labelMap := make(map[string]string)
	for k, v := range sidecarset.Spec.Selector.MatchLabels {
		labelMap[k] = v
	}
	labelMap[history.ControllerRevisionHashLabel] = updateRevision.Labels[history.ControllerRevisionHashLabel]

	annotationMap := make(map[string]string)
	for k, v := range sidecarset.Annotations {
		annotationMap[k] = v
	}

	containerImages := diffImagesBetweenRevisions(currentRevision, updateRevision)
	klog.V(3).Infof("SidecarSet %s/%s begin to create ImagePullJobs for revision %s -> %s: %v",
		sidecarset.Namespace, sidecarset.Name, currentRevision.Name, updateRevision.Name, containerImages)
	for name, image := range containerImages {
		// job name is revision name + container name, it can not be more than 255 characters
		jobName := fmt.Sprintf("%s-%s", updateRevision.Name, name)
		err := imagejobutilfunc.CreateJobForWorkload(p.Client, sidecarset, controllerKind, jobName, image, labelMap, annotationMap, *selector, pullSecrets)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				klog.Errorf("SidecarSet %s/%s failed to create ImagePullJob %s: %v", sidecarset.Namespace, sidecarset.Name, jobName, err)
				p.recorder.Eventf(sidecarset, v1.EventTypeNormal, "FailedCreateImagePullJob", "failed to create ImagePullJob %s: %v", jobName, err)
			}
			continue
		}
		klog.V(3).Infof("SidecarSet %s/%s created ImagePullJob %s for image: %s", sidecarset.Namespace, sidecarset.Name, jobName, image)
		p.recorder.Eventf(sidecarset, v1.EventTypeNormal, "CreatedImagePullJob", "created ImagePullJob %s for image: %s", jobName, image)
	}

	return p.patchControllerRevisionLabels(updateRevision, appsv1alpha1.ImagePreDownloadCreatedKey, "true")
}

func (p *Processor) patchControllerRevisionLabels(revision *apps.ControllerRevision, key, value string) error {
	oldRevision := revision.ResourceVersion
	newRevision := &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revision.Name,
			Namespace: revision.Namespace,
		},
	}
	body := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`, key, value)
	if err := p.Client.Patch(context.TODO(), newRevision, client.RawPatch(types.StrategicMergePatchType, []byte(body))); err != nil {
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
