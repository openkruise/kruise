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
	"fmt"
	"math/rand"
	"reflect"
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type syncAction string

const (
	defaultTTLSecondsForNever            = int32(24 * 3600)
	defaultActiveDeadlineSecondsForNever = int64(1800)

	create   syncAction = "create"
	update   syncAction = "update"
	noAction syncAction = "noAction"
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

func getActiveDeadlineSecondsForNever(job *appsv1alpha1.ImagePullJob) *int64 {
	if job.Spec.PullPolicy != nil && job.Spec.PullPolicy.TimeoutSeconds != nil &&
		int64(*job.Spec.PullPolicy.TimeoutSeconds) > defaultActiveDeadlineSecondsForNever {

		ret := int64(*job.Spec.PullPolicy.TimeoutSeconds)
		return utilpointer.Int64(ret)
	}
	var ret = defaultActiveDeadlineSecondsForNever
	return utilpointer.Int64(ret)
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

func keyFromRef(ref appsv1alpha1.ReferenceObject) types.NamespacedName {
	return types.NamespacedName{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}
}

func keyFromObject(object client.Object) types.NamespacedName {
	return types.NamespacedName{
		Name:      object.GetName(),
		Namespace: object.GetNamespace(),
	}
}

func targetFromSource(source *v1.Secret, keySet referenceSet) *v1.Secret {
	target := source.DeepCopy()
	target.ObjectMeta = metav1.ObjectMeta{
		Namespace:    util.GetKruiseDaemonConfigNamespace(),
		GenerateName: fmt.Sprintf("%s-", source.Name),
		Labels:       source.Labels,
		Annotations:  source.Annotations,
	}
	if target.Labels == nil {
		target.Labels = map[string]string{}
	}
	target.Labels[SourceSecretUIDLabelKey] = string(source.UID)
	if target.Annotations == nil {
		target.Annotations = map[string]string{}
	}
	target.Annotations[SourceSecretKeyAnno] = keyFromObject(source).String()
	target.Annotations[TargetOwnerReferencesAnno] = keySet.String()
	return target
}

func updateTarget(target, source *v1.Secret, keySet referenceSet) *v1.Secret {
	target = target.DeepCopy()
	target.Data = source.Data
	target.StringData = source.StringData
	target.Annotations[TargetOwnerReferencesAnno] = keySet.String()
	return target
}

func referenceSetFromTarget(target *v1.Secret) referenceSet {
	refs := strings.Split(target.Annotations[TargetOwnerReferencesAnno], ",")
	keys := makeReferenceSet()
	for _, ref := range refs {
		namespace, name, err := cache.SplitMetaNamespaceKey(ref)
		if err != nil {
			klog.ErrorS(err, "Failed to parse job key from annotations in target Secret", "secret", klog.KObj(target))
			continue
		}
		keys.Insert(types.NamespacedName{Namespace: namespace, Name: name})
	}
	return keys
}

func computeTargetSyncAction(source, target *v1.Secret, job *appsv1alpha1.ImagePullJob) syncAction {
	if target == nil || len(target.UID) == 0 {
		return create
	}
	keySet := referenceSetFromTarget(target)
	if !keySet.Contains(keyFromObject(job)) ||
		!reflect.DeepEqual(source.Data, target.Data) ||
		!reflect.DeepEqual(source.StringData, target.StringData) {
		return update
	}
	return noAction
}

func makeReferenceSet(items ...types.NamespacedName) referenceSet {
	refSet := map[types.NamespacedName]struct{}{}
	for _, item := range items {
		refSet[item] = struct{}{}
	}
	return refSet
}

type referenceSet map[types.NamespacedName]struct{}

func (set referenceSet) String() string {
	keyList := make([]string, 0, len(set))
	for ref := range set {
		keyList = append(keyList, ref.String())
	}
	return strings.Join(keyList, ",")
}

func (set referenceSet) Contains(key types.NamespacedName) bool {
	_, exists := set[key]
	return exists
}

func (set referenceSet) Insert(key types.NamespacedName) referenceSet {
	set[key] = struct{}{}
	return set
}

func (set referenceSet) Delete(key types.NamespacedName) referenceSet {
	delete(set, key)
	return set
}

func (set referenceSet) IsEmpty() bool {
	return len(set) == 0
}
