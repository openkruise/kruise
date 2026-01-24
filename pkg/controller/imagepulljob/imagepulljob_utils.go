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
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	utilpointer "k8s.io/utils/pointer"

	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/openkruise/kruise/pkg/util"
)

const (
	defaultTTLSecondsForNever            = int32(24 * 3600)
	defaultActiveDeadlineSecondsForNever = int64(1800)
)

func getTTLSecondsForAlways(job *appsv1beta1.ImagePullJob) *int32 {
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
	ret += util.GetDefaultTtlsecondsForAlwaysNodeimage() + rand.Int31n(300)
	return &ret
}

func getOwnerRef(job *appsv1beta1.ImagePullJob) *v1.ObjectReference {
	return &v1.ObjectReference{
		APIVersion: controllerKind.GroupVersion().String(),
		Kind:       controllerKind.Kind,
		Name:       job.Name,
		Namespace:  job.Namespace,
		UID:        job.UID,
	}
}

func getImagePullPolicy(job *appsv1beta1.ImagePullJob) *appsv1beta1.ImageTagPullPolicy {
	pullPolicy := &appsv1beta1.ImageTagPullPolicy{}
	if job.Spec.PullPolicy != nil {
		pullPolicy.BackoffLimit = job.Spec.PullPolicy.BackoffLimit
		pullPolicy.TimeoutSeconds = job.Spec.PullPolicy.TimeoutSeconds
	}
	if job.Spec.CompletionPolicy.Type == appsv1beta1.Never {
		pullPolicy.TTLSecondsAfterFinished = getTTLSecondsForNever()
		pullPolicy.ActiveDeadlineSeconds = getActiveDeadlineSecondsForNever(job)
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

func getActiveDeadlineSecondsForNever(job *appsv1beta1.ImagePullJob) *int64 {
	if job.Spec.PullPolicy != nil && job.Spec.PullPolicy.TimeoutSeconds != nil &&
		int64(*job.Spec.PullPolicy.TimeoutSeconds) > defaultActiveDeadlineSecondsForNever {

		ret := int64(*job.Spec.PullPolicy.TimeoutSeconds)
		return utilpointer.Int64(ret)
	}
	var ret = defaultActiveDeadlineSecondsForNever
	return utilpointer.Int64(ret)
}

func containsObject(slice []appsv1beta1.ReferenceObject, obj appsv1beta1.ReferenceObject) bool {
	for _, o := range slice {
		if o.Namespace == obj.Namespace && o.Name == obj.Name {
			return true
		}
	}
	return false
}

func formatStatusMessage(status *appsv1beta1.ImagePullJobStatus) (ret string) {
	if status.CompletionTime != nil {
		return "job has completed"
	}
	if status.Desired == 0 {
		return "job is running, no progress"
	}
	return fmt.Sprintf("job is running, progress %.1f%%", 100.0*float64(status.Succeeded+status.Failed)/float64(status.Desired))
}

func jobAsReferenceObject(job *appsv1beta1.ImagePullJob) appsv1beta1.ReferenceObject {
	return appsv1beta1.ReferenceObject{
		Name:      job.GetName(),
		Namespace: job.GetNamespace(),
	}
}

// getReferencingJobsFromSecret extracts the set of ImagePullJobs that reference the given secret
// by parsing the comma-separated list of job references stored in the secret's annotations.
// It returns a set containing the ReferenceObject entries for each associated ImagePullJob.
func getReferencingJobsFromSecret(secret *v1.Secret) sets.Set[appsv1beta1.ReferenceObject] {
	referJobs := sets.New[appsv1beta1.ReferenceObject]()
	if secret.Annotations[SecretAnnotationReferenceJobs] == "" {
		return referJobs
	}
	refList := strings.Split(secret.Annotations[SecretAnnotationReferenceJobs], ",")
	for _, ref := range refList {
		namespace, name, err := cache.SplitMetaNamespaceKey(ref)
		if err != nil {
			klog.ErrorS(err, "failed to parse imagePullJob key from secret annotations", "secret", klog.KObj(secret))
			continue
		}

		referJobs.Insert(appsv1beta1.ReferenceObject{Namespace: namespace, Name: name})
	}
	return referJobs
}

func getSourceSecret(secret *v1.Secret) appsv1beta1.ReferenceObject {
	return *appsv1beta1.ParseReferenceObject(secret.Annotations[SecretAnnotationSourceSecretKey])
}

// Generate a six-character random string
func defaultGenerateRandomString() string {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)[:6]
}
