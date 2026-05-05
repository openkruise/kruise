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
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	policyv1beta1 "github.com/openkruise/kruise/apis/policy/v1beta1"
	kubeClient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/feature"
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
	// PodRelatedPubAnnotation is the stable pod annotation used to cache the owning PUB name.
	PodRelatedPubAnnotation = policyv1beta1.PodRelatedPubAnnotation
	// DeprecatedPodRelatedPubAnnotation is kept so existing pods keep working during the promotion.
	DeprecatedPodRelatedPubAnnotation = policyv1beta1.DeprecatedPodRelatedPubAnnotation
)

// parameters:
// 1. allowed(bool) indicates whether to allow this update operation
// 2. err(error)
func PodUnavailableBudgetValidatePod(pod *corev1.Pod, operation policyv1beta1.PubOperation, username string, dryRun bool) (allowed bool, reason string, err error) {
	klog.V(3).InfoS("Validated pod operation for podUnavailableBudget", "pod", klog.KObj(pod), "operation", operation)
	// pods that carry pub-no-protect=true are exempt (annotation-only check, no GET)
	if isPodNoProtection(pod) {
		klog.V(3).InfoS("Pod is exempt from PUB enforcement via annotation", "pod", klog.KObj(pod))
		return true, "", nil
		// If the pod is not ready or state is inconsistent, it doesn't count towards healthy and we should not decrement
	} else if !PubControl.IsPodReady(pod) || !PubControl.IsPodStateConsistent(pod) {
		klog.V(3).InfoS("Pod was not ready or state was inconsistent, then didn't need check pub", "pod", klog.KObj(pod))
		return true, "", nil
	}

	// pub for pod — single GET on the critical path
	pub, err := PubControl.GetPubForPod(pod)
	if err != nil {
		return false, "", err
		// if there is no matching PodUnavailableBudget, just return true
	} else if pub == nil {
		return true, "", nil
	}
	// check spec.ignoredPodSelector using the already-fetched PUB (no second GET)
	if matched, err := isPodMatchedIgnoredPubSelector(pub, pod); err != nil {
		return false, "", err
	} else if matched {
		klog.V(3).InfoS("Pod is exempt from PUB enforcement via ignoredPodSelector", "pod", klog.KObj(pod))
		return true, "", nil
	}
	if pub.Status.DesiredAvailable == 0 {
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
	var pubClone *policyv1beta1.PodUnavailableBudget
	err = retry.RetryOnConflict(ConflictRetry, func() error {
		unlock := util.GlobalKeyedMutex.Lock(string(pub.UID))
		defer unlock()

		start := time.Now()
		if refresh {
			pubClone, err = kubeClient.GetGenericClient().KruiseClient.PolicyV1beta1().
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
			if localCached, ok := item.(*policyv1beta1.PodUnavailableBudget); ok {
				pubClone = localCached.DeepCopy()
			} else {
				pubClone = pub.DeepCopy()
			}

			informerCached := &policyv1beta1.PodUnavailableBudget{}
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

func checkAndDecrement(podName string, pub *policyv1beta1.PodUnavailableBudget, operation policyv1beta1.PubOperation) error {
	if pub.Status.UnavailableAllowed <= 0 {
		return errors.NewForbidden(policyv1beta1.Resource("podunavailablebudget"), pub.Name, fmt.Errorf("pub unavailable allowed is negative"))
	}
	if len(pub.Status.DisruptedPods)+len(pub.Status.UnavailablePods) > MaxUnavailablePodSize {
		return errors.NewForbidden(policyv1beta1.Resource("podunavailablebudget"), pub.Name, fmt.Errorf("DisruptedPods and UnavailablePods map too big - too many unavailable not confirmed by PUB controller"))
	}

	pub.Status.UnavailableAllowed--

	if pub.Status.DisruptedPods == nil {
		pub.Status.DisruptedPods = make(map[string]metav1.Time)
	}
	if pub.Status.UnavailablePods == nil {
		pub.Status.UnavailablePods = make(map[string]metav1.Time)
	}

	if operation == policyv1beta1.PubUpdateOperation {
		pub.Status.UnavailablePods[podName] = metav1.Time{Time: time.Now()}
		klog.V(3).InfoS("Pod was recorded in pub unavailablePods", "podName", podName, "pub", klog.KObj(pub))
	} else {
		pub.Status.DisruptedPods[podName] = metav1.Time{Time: time.Now()}
		klog.V(3).InfoS("Pod was recorded in pub disruptedPods", "podName", podName, "pub", klog.KObj(pub))
	}
	return nil
}

func isPodRecordedInPub(podName string, pub *policyv1beta1.PodUnavailableBudget) bool {
	if _, ok := pub.Status.UnavailablePods[podName]; ok {
		return true
	}
	if _, ok := pub.Status.DisruptedPods[podName]; ok {
		return true
	}
	return false
}

// check APIVersion, Kind, Name
func IsReferenceEqual(ref1, ref2 *policyv1beta1.TargetReference) bool {
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

func isNeedPubProtection(pub *policyv1beta1.PodUnavailableBudget, operation policyv1beta1.PubOperation) bool {
	enableInPlacePodVerticalScaling := feature.DefaultFeatureGate.Enabled(features.InPlacePodVerticalScaling)
	operations := getPubProtectOperations(pub)

	// if featureGate InPlacePodVerticalScaling is disabled, resize will be treat as update
	if !enableInPlacePodVerticalScaling && operations.Has(policyv1beta1.PubUpdateOperation) {
		operations.Insert(policyv1beta1.PubResizeOperation)
	}

	return operations.Has(operation)
}

func GetPodRelatedPubName(pod *corev1.Pod) string {
	if pod == nil || len(pod.Annotations) == 0 {
		return ""
	}
	if name := pod.Annotations[PodRelatedPubAnnotation]; name != "" {
		return name
	}
	return pod.Annotations[DeprecatedPodRelatedPubAnnotation]
}

func SetPodRelatedPubAnnotation(annotations map[string]string, pubName string) map[string]string {
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[PodRelatedPubAnnotation] = pubName
	annotations[DeprecatedPodRelatedPubAnnotation] = pubName
	return annotations
}

// isPodNoProtection returns true if the pod carries the pub-no-protect=true annotation.
// The spec.ignoredPodSelector check is done separately after the PUB is fetched,
// to avoid an extra GET call on the webhook critical path.
func isPodNoProtection(pod *corev1.Pod) bool {
	if pod != nil && len(pod.Annotations) > 0 {
		value := pod.Annotations[policyv1beta1.PodPubNoProtectionAnnotation]
		if allow, err := strconv.ParseBool(value); err == nil && allow {
			return true
		}
	}
	return false
}

// getPubProtectOperations returns the effective beta protection list.
// It prefers the typed beta spec field and falls back to the deprecated annotation for compatibility.
func getPubProtectOperations(pub *policyv1beta1.PodUnavailableBudget) sets.Set[policyv1beta1.PubOperation] {
	operations := sets.New[policyv1beta1.PubOperation]()

	if len(pub.Spec.ProtectOperations) > 0 {
		operations.Insert(pub.Spec.ProtectOperations...)
		return operations
	}

	if operationValue := pub.Annotations[policyv1beta1.PubProtectOperationAnnotation]; operationValue != "" {
		for _, action := range strings.Split(operationValue, ",") {
			if action == "" {
				continue
			}
			operations.Insert(policyv1beta1.PubOperation(action))
		}
		if operations.Len() > 0 {
			return operations
		}
	}

	// Default protection is preserved so old objects and beta objects without an explicit setting
	// still protect the historical DELETE, UPDATE, and EVICT operations.
	operations.Insert(
		policyv1beta1.PubDeleteOperation,
		policyv1beta1.PubUpdateOperation,
		policyv1beta1.PubEvictOperation,
	)
	return operations
}

// getPubProtectTotalReplicas returns spec.protectTotalReplicas for v1beta1 PodUnavailableBudget.
func getPubProtectTotalReplicas(pub *policyv1beta1.PodUnavailableBudget) *int32 {
	return pub.Spec.ProtectTotalReplicas
}

// isPodMatchedIgnoredPubSelector checks whether pod matches pub.Spec.IgnoredPodSelector.
// The caller is responsible for fetching the PUB; no additional GET is performed here.
func isPodMatchedIgnoredPubSelector(pub *policyv1beta1.PodUnavailableBudget, pod *corev1.Pod) (bool, error) {
	if pub.Spec.IgnoredPodSelector == nil {
		return false, nil
	}
	selector, err := util.ValidatedLabelSelectorAsSelector(pub.Spec.IgnoredPodSelector)
	if err != nil || selector.Empty() {
		return false, err
	}
	return selector.Matches(labels.Set(pod.Labels)), nil
}
