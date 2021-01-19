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

package nodeimages

import (
	"context"
	"fmt"
	"sort"
	"sync"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
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
	names := make([]string, len(nodeImages))
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

	nodeImageList := &appsv1alpha1.NodeImageList{}
	if job.Spec.Selector == nil {
		if err := reader.List(context.TODO(), nodeImageList); err != nil {
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

	selector, err := metav1.LabelSelectorAsSelector(&job.Spec.Selector.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("parse selector error: %v", err)
	}
	if err := reader.List(context.TODO(), nodeImageList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}
	return convertNodeImages(nodeImageList), err
}

func convertNodeImages(nodeImageList *appsv1alpha1.NodeImageList) []*appsv1alpha1.NodeImage {
	var nodeImages []*appsv1alpha1.NodeImage
	for i := range nodeImageList.Items {
		nodeImages = append(nodeImages, &nodeImageList.Items[i])
	}
	return nodeImages
}

func GetJobsForNodeImage(reader client.Reader, nodeImage *appsv1alpha1.NodeImage) (jobs []*appsv1alpha1.ImagePullJob, err error) {
	jobList := appsv1alpha1.ImagePullJobList{}
	if err = reader.List(context.TODO(), &jobList); err != nil {
		return nil, err
	}
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if job.Spec.Selector == nil {
			jobs = append(jobs, job)
		} else if job.Spec.Selector.Names != nil {
			if slice.ContainsString(job.Spec.Selector.Names, nodeImage.Name, nil) {
				jobs = append(jobs, job)
			}
		} else {
			selector, err := metav1.LabelSelectorAsSelector(&job.Spec.Selector.LabelSelector)
			if err != nil {
				return nil, fmt.Errorf("parse selector error: %v", err)
			}
			if !selector.Empty() && selector.Matches(labels.Set(nodeImage.Labels)) {
				jobs = append(jobs, job)
			}
		}
	}
	return jobs, nil
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
