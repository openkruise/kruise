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

package imagejob

import (
	"context"
	"fmt"
	"sort"
	"sync"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	sortingcontrol "github.com/openkruise/kruise/pkg/control/sorting"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/util/slice"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	cacheLock        sync.Mutex
	cachedNodeImages = make(map[string][]string)
)

func PopCachedNodeImagesForJob(job *appsv1alpha1.ImagePullJob) []string {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	names, ok := cachedNodeImages[string(job.UID)]
	if ok {
		delete(cachedNodeImages, string(job.UID))
	}
	return names
}

func writeCache(job *appsv1alpha1.ImagePullJob, nodeImages []*appsv1alpha1.NodeImage) {
	names := make([]string, 0, len(nodeImages))
	for _, n := range nodeImages {
		names = append(names, n.Name)
	}
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cachedNodeImages[string(job.UID)] = names
}

func GetNodeImagesForJob(reader client.Reader, job *appsv1alpha1.ImagePullJob) (nodeImages []*appsv1alpha1.NodeImage, err error) {
	defer func() {
		if err == nil {
			writeCache(job, nodeImages)
		}
	}()

	if job.Spec.PodSelector != nil {
		selector, err := util.ValidatedLabelSelectorAsSelector(&job.Spec.PodSelector.LabelSelector)
		if err != nil {
			return nil, fmt.Errorf("parse podSelector error: %v", err)
		}

		podList := &v1.PodList{}
		if err := reader.List(context.TODO(), podList, client.InNamespace(job.Namespace), client.MatchingLabelsSelector{Selector: selector}, utilclient.DisableDeepCopy); err != nil {
			return nil, err
		}

		pods := make([]*v1.Pod, 0, len(podList.Items))
		for i := range podList.Items {
			pod := &podList.Items[i]
			if !kubecontroller.IsPodActive(pod) || pod.Spec.NodeName == "" {
				continue
			}
			pods = append(pods, pod)
		}

		owner := metav1.GetControllerOf(job)
		if owner != nil {
			newPods, err := sortingcontrol.SortPods(reader, job.Namespace, *owner, pods)
			if err != nil {
				klog.ErrorS(err, "ImagePullJob failed to sort Pods", "namespace", job.Namespace, "name", job.Name)
			} else {
				pods = newPods
			}
		}

		nodeImageNames := sets.NewString()
		for _, pod := range pods {
			if nodeImageNames.Has(pod.Spec.NodeName) {
				continue
			}
			nodeImageNames.Insert(pod.Spec.NodeName)
			nodeImage := &appsv1alpha1.NodeImage{}
			if err := reader.Get(context.TODO(), types.NamespacedName{Name: pod.Spec.NodeName}, nodeImage); err != nil {
				if errors.IsNotFound(err) {
					klog.InfoS("Get NodeImages for ImagePullJob, find Pod on Node but NodeImage not found",
						"namespace", job.Namespace, "name", job.Name, "pod", pod.Name, "node", pod.Spec.NodeName)
					continue
				}
				return nil, err
			}
			nodeImages = append(nodeImages, nodeImage)
		}
		return nodeImages, nil
	}

	nodeImageList := &appsv1alpha1.NodeImageList{}
	if job.Spec.Selector == nil {
		if err := reader.List(context.TODO(), nodeImageList, utilclient.DisableDeepCopy); err != nil {
			return nil, err
		}
		return convertNodeImages(nodeImageList), err
	}

	if job.Spec.Selector.Names != nil {
		for _, name := range job.Spec.Selector.Names {
			var nodeImage appsv1alpha1.NodeImage
			if err := reader.Get(context.TODO(), types.NamespacedName{Name: name}, &nodeImage); err != nil {
				if errors.IsNotFound(err) {
					continue
				}
				return nil, fmt.Errorf("get specific NodeImage %s error: %v", name, err)
			}
			nodeImages = append(nodeImages, &nodeImage)
		}
		return nodeImages, nil
	}

	selector, err := util.ValidatedLabelSelectorAsSelector(&job.Spec.Selector.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("parse selector error: %v", err)
	}
	if err := reader.List(context.TODO(), nodeImageList, client.MatchingLabelsSelector{Selector: selector}, utilclient.DisableDeepCopy); err != nil {
		return nil, err
	}
	return convertNodeImages(nodeImageList), err
}

func convertNodeImages(nodeImageList *appsv1alpha1.NodeImageList) []*appsv1alpha1.NodeImage {
	nodeImages := make([]*appsv1alpha1.NodeImage, 0, len(nodeImageList.Items))
	for i := range nodeImageList.Items {
		nodeImages = append(nodeImages, &nodeImageList.Items[i])
	}
	return nodeImages
}

func GetActiveJobsForNodeImage(reader client.Reader, nodeImage, oldNodeImage *appsv1alpha1.NodeImage) (newJobs, oldJobs []*appsv1alpha1.ImagePullJob, err error) {
	var podsOnNode []*v1.Pod
	jobList := appsv1alpha1.ImagePullJobList{}
	if err = reader.List(context.TODO(), &jobList, client.MatchingFields{fieldindex.IndexNameForIsActive: "true"}, utilclient.DisableDeepCopy); err != nil {
		return nil, nil, err
	}
	for i := range jobList.Items {
		job := &jobList.Items[i]
		var matched bool
		var oldMatched bool

		if job.Spec.PodSelector != nil {
			selector, err := util.ValidatedLabelSelectorAsSelector(&job.Spec.PodSelector.LabelSelector)
			if err != nil {
				return nil, nil, fmt.Errorf("parse podSelector for %s/%s error: %v", job.Namespace, job.Name, err)
			}
			if podsOnNode == nil {
				if podsOnNode, err = getPodsOnNode(reader, nodeImage.Name); err != nil {
					return nil, nil, fmt.Errorf("get pods on node error: %v", err)
				}
			}
			for _, pod := range podsOnNode {
				if selector.Matches(labels.Set(pod.Labels)) {
					matched = true
					oldMatched = true
					break
				}
			}
		} else if job.Spec.Selector == nil {
			matched = true
			oldMatched = true
		} else if job.Spec.Selector.Names != nil {
			if slice.ContainsString(job.Spec.Selector.Names, nodeImage.Name, nil) {
				matched = true
				oldMatched = true
			}
		} else {
			selector, err := util.ValidatedLabelSelectorAsSelector(&job.Spec.Selector.LabelSelector)
			if err != nil {
				return nil, nil, fmt.Errorf("parse selector for %s/%s error: %v", job.Namespace, job.Name, err)
			}
			if !selector.Empty() {
				if selector.Matches(labels.Set(nodeImage.Labels)) {
					matched = true
				}
				if oldNodeImage != nil {
					if selector.Matches(labels.Set(oldNodeImage.Labels)) {
						oldMatched = true
					}
				}
			}
		}
		if matched {
			newJobs = append(newJobs, job)
		}
		if oldNodeImage != nil && oldMatched {
			oldJobs = append(oldJobs, job)
		}
	}
	return
}

func getPodsOnNode(reader client.Reader, nodeName string) (pods []*v1.Pod, err error) {
	podList := v1.PodList{}
	if err = reader.List(context.TODO(), &podList, client.MatchingFields{fieldindex.IndexNameForPodNodeName: nodeName}, utilclient.DisableDeepCopy); err != nil {
		return nil, err
	}
	pods = make([]*v1.Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pods = append(pods, &podList.Items[i])
	}
	return
}

func GetActiveJobsForPod(reader client.Reader, pod, oldPod *v1.Pod) (newJobs, oldJobs []*appsv1alpha1.ImagePullJob, err error) {
	jobList := appsv1alpha1.ImagePullJobList{}
	if err = reader.List(context.TODO(), &jobList, client.InNamespace(pod.Namespace), client.MatchingFields{fieldindex.IndexNameForIsActive: "true"}, utilclient.DisableDeepCopy); err != nil {
		return nil, nil, err
	}
	for i := range jobList.Items {
		job := &jobList.Items[i]

		if job.Spec.PodSelector != nil {
			selector, err := util.ValidatedLabelSelectorAsSelector(&job.Spec.PodSelector.LabelSelector)
			if err != nil {
				return nil, nil, fmt.Errorf("parse podSelector for %s/%s error: %v", job.Namespace, job.Name, err)
			}
			if selector.Matches(labels.Set(pod.Labels)) {
				newJobs = append(newJobs, job)
			}
			if oldPod != nil && selector.Matches(labels.Set(oldPod.Labels)) {
				oldJobs = append(oldJobs, job)
			}
		}
	}
	return
}

func SortSpecImageTags(imageSpec *appsv1alpha1.ImageSpec) {
	if imageSpec == nil || len(imageSpec.Tags) == 0 {
		return
	}
	sort.Slice(imageSpec.Tags, func(i, j int) bool {
		return imageSpec.Tags[i].Tag <= imageSpec.Tags[j].Tag
	})
}

func SortStatusImageTags(imageStatus *appsv1alpha1.ImageStatus) {
	if imageStatus == nil || len(imageStatus.Tags) == 0 {
		return
	}
	sort.Slice(imageStatus.Tags, func(i, j int) bool {
		return imageStatus.Tags[i].Tag <= imageStatus.Tags[j].Tag
	})
}
