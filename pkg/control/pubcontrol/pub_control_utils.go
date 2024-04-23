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

package pubcontrol

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	kubeClient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/util"
)

const (
	// MaxUnavailablePodSize is the max size of PUB.DisruptedPods + PUB.UnavailablePods.
	MaxUnavailablePodSize = 2000
)

var ConflictRetry = wait.Backoff{
	Steps:    4,
	Duration: 500 * time.Millisecond,
	Factor:   1.0,
	Jitter:   0.1,
}

const (
	// related-pub annotation in pod
	PodRelatedPubAnnotation = "kruise.io/related-pub"
)

// parameters:
// 1. allowed(bool) indicates whether to allow this update operation
// 2. err(error)
func PodUnavailableBudgetValidatePod(pod *corev1.Pod, operation policyv1alpha1.PubOperation, username string, dryRun bool) (allowed bool, reason string, err error) {
	klog.V(3).InfoS("Validated pod operation for podUnavailableBudget", "pod", klog.KObj(pod), "operation", operation)
	// pods that contain annotations[pod.kruise.io/pub-no-protect]="true" will be ignore
	// and will no longer check the pub quota
	if pod.Annotations[policyv1alpha1.PodPubNoProtectionAnnotation] == "true" {
		klog.V(3).InfoS("Pod contained annotations=true, then didn't need check pub", "pod", klog.KObj(pod), "annotations", policyv1alpha1.PodPubNoProtectionAnnotation)
		return true, "", nil
		// If the pod is not ready or state is inconsistent, it doesn't count towards healthy and we should not decrement
	} else if !PubControl.IsPodReady(pod) || !PubControl.IsPodStateConsistent(pod) {
		klog.V(3).InfoS("Pod was not ready or state was inconsistent, then didn't need check pub", "pod", klog.KObj(pod))
		return true, "", nil
	}

	// pub for pod
	pub, err := PubControl.GetPubForPod(pod)
	if err != nil {
		return false, "", err
		// if there is no matching PodUnavailableBudget, just return true
	} else if pub == nil {
		return true, "", nil
		// if desired available == 0, then allow all request
	} else if pub.Status.DesiredAvailable == 0 {
		return true, "", nil
	} else if !isNeedPubProtection(pub, operation) {
		klog.V(3).InfoS("Pod operation was not in pub protection", "pod", klog.KObj(pod), "operation", operation, "pubName", pub.Name)
		return true, "", nil
		// pod is in pub.Status.DisruptedPods or pub.Status.UnavailablePods, then don't need check it
	} else if isPodRecordedInPub(pod.Name, pub) {
		klog.V(3).InfoS("Pod was already recorded in pub", "pod", klog.KObj(pod), "pub", klog.KObj(pub))
		return true, "", nil
	}
	// check and decrement pub quota
	var conflictTimes int
	var costOfGet, costOfUpdate time.Duration
	refresh := false
	var pubClone *policyv1alpha1.PodUnavailableBudget
	err = retry.RetryOnConflict(ConflictRetry, func() error {
		unlock := util.GlobalKeyedMutex.Lock(string(pub.UID))
		defer unlock()

		start := time.Now()
		if refresh {
			pubClone, err = kubeClient.GetGenericClient().KruiseClient.PolicyV1alpha1().
				PodUnavailableBudgets(pub.Namespace).Get(context.TODO(), pub.Name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				klog.ErrorS(err, "Failed to get podUnavailableBudget form etcd", "pub", klog.KObj(pub))
				return err
			}
		} else {
			// compare local cache and informer cache, then get the newer one
			item, _, err := util.GlobalCache.Get(pub)
			if err != nil {
				klog.ErrorS(err, "Failed to get cache for podUnavailableBudget", "pub", klog.KObj(pub))
			}
			if localCached, ok := item.(*policyv1alpha1.PodUnavailableBudget); ok {
				pubClone = localCached.DeepCopy()
			} else {
				pubClone = pub.DeepCopy()
			}

			informerCached := &policyv1alpha1.PodUnavailableBudget{}
			if err := kclient.Get(context.TODO(), types.NamespacedName{Namespace: pub.Namespace,
				Name: pub.Name}, informerCached); err == nil {
				var localRV, informerRV int64
				_ = runtime.Convert_string_To_int64(&pubClone.ResourceVersion, &localRV, nil)
				_ = runtime.Convert_string_To_int64(&informerCached.ResourceVersion, &informerRV, nil)
				if informerRV > localRV {
					pubClone = informerCached
				}
			}
		}
		costOfGet += time.Since(start)

		// Try to verify-and-decrement
		// If it was false already, or if it becomes false during the course of our retries,
		err = checkAndDecrement(pod.Name, pubClone, operation)
		if err != nil {
			var kind, namespace, name string
			if ref := PubControl.GetPodControllerOf(pod); ref != nil {
				kind = ref.Kind
				name = ref.Name
			} else {
				kind = "unknown"
				name = pod.Name
			}
			namespace = pod.Namespace
			if namespace == "" {
				namespace = "default"
			}
			PodUnavailableBudgetMetrics.WithLabelValues(fmt.Sprintf("%s_%s_%s", kind, namespace, name), username).Add(1)
			recorder.Eventf(pod, corev1.EventTypeWarning, "PubPreventPodDeletion", "openkruise pub prevents pod deletion")
			util.LoggerProtectionInfo(util.ProtectionEventPub, kind, namespace, name, username)
			return err
		}

		// If this is a dry-run, we don't need to go any further than that.
		if dryRun {
			klog.V(3).InfoS("Pod operation for pub was a dry run", "pod", klog.KObj(pod), "pub", klog.KObj(pubClone))
			return nil
		}
		klog.V(3).InfoS("Updated pub status", "pub", klog.KObj(pubClone), "disruptedPods", len(pubClone.Status.DisruptedPods),
			"unavailablePods", len(pubClone.Status.UnavailablePods), "expectedCount", pubClone.Status.TotalReplicas, "desiredAvailable",
			pubClone.Status.DesiredAvailable, "currentAvailable", pubClone.Status.CurrentAvailable, "unavailableAllowed", pubClone.Status.UnavailableAllowed)
		start = time.Now()
		err = kclient.Status().Update(context.TODO(), pubClone)
		costOfUpdate += time.Since(start)
		if err == nil {
			if err = util.GlobalCache.Add(pubClone); err != nil {
				klog.ErrorS(err, "Failed to add cache for podUnavailableBudget", "pub", klog.KObj(pub))
			}
			return nil
		} else {
			if errors.IsNotFound(err) {
				return nil
			}
		}
		// if conflicts, then retry
		conflictTimes++
		refresh = true
		return err
	})
	klog.V(3).InfoS("Webhook cost of pub", "pub", klog.KObj(pub),
		"conflictTimes", conflictTimes, "costOfGet", costOfGet, "costOfUpdate", costOfUpdate)
	if err != nil && err != wait.ErrWaitTimeout {
		klog.V(3).InfoS("Pod operation for pub failed", "pod", klog.KObj(pod), "operation", operation,
			"pub", klog.KObj(pub), "error", err)
		return false, err.Error(), nil
	} else if err == wait.ErrWaitTimeout {
		err = errors.NewTimeoutError(fmt.Sprintf("couldn't update PodUnavailableBudget %s due to conflicts", pub.Name), 10)
		klog.ErrorS(err, "Pod operation failed", "pod", klog.KObj(pod), "operation", operation)
		return false, err.Error(), nil
	}

	klog.V(3).InfoS("Admitted pod operation for pub", "pod", klog.KObj(pod),
		"operation", operation, "pub", klog.KObj(pub))
	return true, "", nil
}

func checkAndDecrement(podName string, pub *policyv1alpha1.PodUnavailableBudget, operation policyv1alpha1.PubOperation) error {
	if pub.Status.UnavailableAllowed <= 0 {
		return errors.NewForbidden(policyv1alpha1.Resource("podunavailablebudget"), pub.Name, fmt.Errorf("pub unavailable allowed is negative"))
	}
	if len(pub.Status.DisruptedPods)+len(pub.Status.UnavailablePods) > MaxUnavailablePodSize {
		return errors.NewForbidden(policyv1alpha1.Resource("podunavailablebudget"), pub.Name, fmt.Errorf("DisruptedPods and UnavailablePods map too big - too many unavailable not confirmed by PUB controller"))
	}

	pub.Status.UnavailableAllowed--

	if pub.Status.DisruptedPods == nil {
		pub.Status.DisruptedPods = make(map[string]metav1.Time)
	}
	if pub.Status.UnavailablePods == nil {
		pub.Status.UnavailablePods = make(map[string]metav1.Time)
	}

	if operation == policyv1alpha1.PubUpdateOperation {
		pub.Status.UnavailablePods[podName] = metav1.Time{Time: time.Now()}
		klog.V(3).InfoS("Pod was recorded in pub unavailablePods", "podName", podName, "pub", klog.KObj(pub))
	} else {
		pub.Status.DisruptedPods[podName] = metav1.Time{Time: time.Now()}
		klog.V(3).InfoS("Pod was recorded in pub disruptedPods", "podName", podName, "pub", klog.KObj(pub))
	}
	return nil
}

func isPodRecordedInPub(podName string, pub *policyv1alpha1.PodUnavailableBudget) bool {
	if _, ok := pub.Status.UnavailablePods[podName]; ok {
		return true
	}
	if _, ok := pub.Status.DisruptedPods[podName]; ok {
		return true
	}
	return false
}

// check APIVersion, Kind, Name
func IsReferenceEqual(ref1, ref2 *policyv1alpha1.TargetReference) bool {
	gv1, err := schema.ParseGroupVersion(ref1.APIVersion)
	if err != nil {
		return false
	}
	gv2, err := schema.ParseGroupVersion(ref2.APIVersion)
	if err != nil {
		return false
	}
	return gv1.Group == gv2.Group && ref1.Kind == ref2.Kind && ref1.Name == ref2.Name
}

func isNeedPubProtection(pub *policyv1alpha1.PodUnavailableBudget, operation policyv1alpha1.PubOperation) bool {
	operationValue, ok := pub.Annotations[policyv1alpha1.PubProtectOperationAnnotation]
	if !ok || operationValue == "" {
		return true
	}
	operations := sets.NewString(strings.Split(operationValue, ",")...)
	return operations.Has(string(operation))
}
