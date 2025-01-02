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

package imagepuller

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clientalpha1 "github.com/openkruise/kruise/pkg/client/clientset/versioned/typed/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"golang.org/x/time/rate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func logNewImages(oldObj, newObj *appsv1alpha1.NodeImage) {
	oldImages := make(map[string]struct{})
	if oldObj != nil {
		for image, imageSpec := range oldObj.Spec.Images {
			for _, tagSpec := range imageSpec.Tags {
				fullName := fmt.Sprintf("%v:%v", image, tagSpec.Tag)
				oldImages[fullName] = struct{}{}
			}
		}
	}

	for image, imageSpec := range newObj.Spec.Images {
		for _, tagSpec := range imageSpec.Tags {
			fullName := fmt.Sprintf("%v:%v", image, tagSpec.Tag)
			if _, ok := oldImages[fullName]; !ok {
				klog.V(2).InfoS("Received new image", "fullName", fullName)
			}
		}
	}
}

func isImageInPulling(spec *appsv1alpha1.NodeImageSpec, status *appsv1alpha1.NodeImageStatus) bool {
	if status.Succeeded+status.Failed < status.Desired {
		return true
	}

	tagSpecs := make(map[string]appsv1alpha1.ImageTagSpec)
	for image, imageSpec := range spec.Images {
		for _, tagSpec := range imageSpec.Tags {
			fullName := fmt.Sprintf("%v:%v", image, tagSpec.Tag)
			tagSpecs[fullName] = tagSpec
		}
	}
	for image, imageStatus := range status.ImageStatuses {
		for _, tagStatus := range imageStatus.Tags {
			fullName := fmt.Sprintf("%v:%v", image, tagStatus.Tag)
			if tagSpec, ok := tagSpecs[fullName]; ok && tagSpec.Version != tagStatus.Version {
				return true
			}
		}
	}

	return false
}

type statusUpdater struct {
	imagePullNodeClient clientalpha1.NodeImageInterface

	previousTimestamp time.Time
	previousStatus    *appsv1alpha1.NodeImageStatus
	rateLimiter       *rate.Limiter
}

const (
	statusUpdateQPS   = 0.5
	statusUpdateBurst = 5
)

func newStatusUpdater(imagePullNodeClient clientalpha1.NodeImageInterface) *statusUpdater {
	return &statusUpdater{
		imagePullNodeClient: imagePullNodeClient,
		previousStatus:      &appsv1alpha1.NodeImageStatus{},
		previousTimestamp:   time.Now().Add(-time.Hour * 24),
		rateLimiter:         rate.NewLimiter(statusUpdateQPS, statusUpdateBurst),
	}
}

func (su *statusUpdater) updateStatus(nodeImage *appsv1alpha1.NodeImage, newStatus *appsv1alpha1.NodeImageStatus) (limited bool, err error) {
	if util.IsJSONObjectEqual(newStatus, su.previousStatus) {
		// 12h + 0~60min
		randRefresh := time.Hour*12 + time.Second*time.Duration(rand.Int63n(3600))
		if time.Since(su.previousTimestamp) < randRefresh {
			return false, nil
		}
	}

	// IMPORTANT!!! Make sure rate limiter is working!
	if !su.rateLimiter.Allow() {
		msg := fmt.Sprintf("Updating status is limited qps=%v burst=%v", statusUpdateQPS, statusUpdateBurst)
		klog.V(3).Info(msg)
		return true, nil
	}

	klog.V(5).InfoS("Updating status", "status", util.DumpJSON(newStatus))
	newNodeImage := nodeImage.DeepCopy()
	newNodeImage.Status = *newStatus

	_, err = su.imagePullNodeClient.UpdateStatus(context.TODO(), newNodeImage, metav1.UpdateOptions{})
	if err == nil {
		su.previousStatus = newStatus
	}
	su.previousTimestamp = time.Now()
	return false, err
}
