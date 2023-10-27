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

package imagepulljob

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/util/globallimiter"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultTTLSecondsForNever = int32(24 * 3600)

	defaultActiveDeadlineSecondsForNever = int64(1800)
)

func getTTLSecondsForAlways(job *appsv1alpha1.ImagePullJob) *int32 {
	var ret int32
	if job.Spec.CompletionPolicy.TTLSecondsAfterFinished != nil {
		ret = *job.Spec.CompletionPolicy.TTLSecondsAfterFinished
	} else if job.Spec.CompletionPolicy.ActiveDeadlineSeconds != nil {
		ret = int32(*job.Spec.CompletionPolicy.ActiveDeadlineSeconds)
	} else {
		timeoutSeconds := int32(600)
		backoffLimit := int32(3)
		if job.Spec.PullPolicy != nil && job.Spec.PullPolicy.TimeoutSeconds != nil {
			timeoutSeconds = *job.Spec.PullPolicy.TimeoutSeconds
		}
		if job.Spec.PullPolicy != nil && job.Spec.PullPolicy.BackoffLimit != nil {
			backoffLimit = *job.Spec.PullPolicy.BackoffLimit
		}
		ret = timeoutSeconds * backoffLimit
	}
	ret += 300 + rand.Int31n(300)
	return &ret
}

func getOwnerRef(job *appsv1alpha1.ImagePullJob) *v1.ObjectReference {
	return &v1.ObjectReference{
		APIVersion: controllerKind.GroupVersion().String(),
		Kind:       controllerKind.Kind,
		Name:       job.Name,
		Namespace:  job.Namespace,
		UID:        job.UID,
	}
}

func getSecrets(job *appsv1alpha1.ImagePullJob) []appsv1alpha1.ReferenceObject {
	var secrets []appsv1alpha1.ReferenceObject
	for _, secret := range job.Spec.PullSecrets {
		secrets = append(secrets,
			appsv1alpha1.ReferenceObject{
				Namespace: job.Namespace,
				Name:      secret,
			})
	}
	return secrets
}

func getImagePullPolicy(job *appsv1alpha1.ImagePullJob) *appsv1alpha1.ImageTagPullPolicy {
	pullPolicy := &appsv1alpha1.ImageTagPullPolicy{}
	if job.Spec.PullPolicy != nil {
		pullPolicy.BackoffLimit = job.Spec.PullPolicy.BackoffLimit
		pullPolicy.TimeoutSeconds = job.Spec.PullPolicy.TimeoutSeconds
	}
	if job.Spec.CompletionPolicy.Type == appsv1alpha1.Never {
		pullPolicy.TTLSecondsAfterFinished = getTTLSecondsForNever()
		pullPolicy.ActiveDeadlineSeconds = getActiveDeadlineSecondsForNever()
	} else {
		pullPolicy.TTLSecondsAfterFinished = getTTLSecondsForAlways(job)
		pullPolicy.ActiveDeadlineSeconds = job.Spec.CompletionPolicy.ActiveDeadlineSeconds
	}
	return pullPolicy
}

func getTTLSecondsForNever() *int32 {
	// 24h +- 10min
	var ret = defaultTTLSecondsForNever + rand.Int31n(1200) - 600
	return &ret
}

func getActiveDeadlineSecondsForNever() *int64 {
	var ret = defaultActiveDeadlineSecondsForNever
	return &ret
}

func containsObject(slice []appsv1alpha1.ReferenceObject, obj appsv1alpha1.ReferenceObject) bool {
	for _, o := range slice {
		if o.Namespace == obj.Namespace && o.Name == obj.Name {
			return true
		}
	}
	return false
}

func formatStatusMessage(status *appsv1alpha1.ImagePullJobStatus) (ret string) {
	if status.CompletionTime != nil {
		return "job has completed"
	}
	if status.Desired == 0 {
		return "job is running, no progress"
	}
	return fmt.Sprintf("job is running, progress %.1f%%", 100.0*float64(status.Succeeded+status.Failed)/float64(status.Desired))
}

func initializedGlobalLimiter(keyTimeout time.Duration) (globallimiter.ParallelismLimiter, error) {
	kruiseClient := kruiseclient.GetGenericClient().KruiseClient
	jobLister, err := kruiseClient.AppsV1alpha1().ImagePullJobs("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to list ImagePullJobs when initialize global limiter: %v", err)
		return nil, err
	}
	currentTime := time.Now()
	pullingJobs := map[string]time.Time{}
	for _, job := range jobLister.Items {
		if !job.DeletionTimestamp.IsZero() {
			continue
		}
		if isJobFinished(&job.Status) {
			continue
		}
		if isJobPulling(&job.Status) {
			pullingJobs[keyOf(&job)] = currentTime
		}
	}
	limiter := globallimiter.NewParallelismLimiter(globalParallelism, pullingJobs, keyTimeout)
	return limiter, nil
}

func isJobFinished(status *appsv1alpha1.ImagePullJobStatus) bool {
	if status.CompletionTime != nil {
		return true
	}
	return (status.Desired - status.Succeeded - status.Failed) == 0
}

func isJobPulling(status *appsv1alpha1.ImagePullJobStatus) bool {
	if status.Desired == 0 || isJobFinished(status) {
		return false
	}
	return status.Active > 0 || (status.Desired-status.Succeeded-status.Failed) > 0
}

func keyOf(object client.Object) string {
	return client.ObjectKeyFromObject(object).String()
}
