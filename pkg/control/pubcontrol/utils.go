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
	"time"

	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

type Operation string

const (
	UpdateOperation = "UPDATE"
	//DeleteOperation = "DELETE"
)

// parameters:
// 1. allowed(bool) indicates whether to allow this update operation
// 2. err(error)
func PodUnavailableBudgetValidatePod(client client.Client, pod *corev1.Pod, control PubControl, operation Operation, dryRun bool) (allowed bool, reason string, err error) {
	pub := control.GetPodUnavailableBudget()
	// If the pod is not ready, it doesn't count towards healthy and we should not decrement
	if !control.IsPodReady(pod) {
		klog.V(3).Infof("pod(%s.%s) is not ready, then don't need check pub", pod.Namespace, pod.Name)
		return true, "", nil
	}
	// pod is in pub.Status.DisruptedPods or pub.Status.UnavailablePods, then don't need check it
	if isPodRecordedInPub(pod.Name, pub) {
		klog.V(5).Infof("pod(%s.%s) already is recorded in pub(%s.%s)", pod.Namespace, pod.Name, pub.Namespace, pub.Name)
		return true, "", nil
	}

	pubClone := pub.DeepCopy()
	refresh := false
	err = retry.RetryOnConflict(ConflictRetry, func() error {
		if refresh {
			key := types.NamespacedName{
				Name:      pubClone.Name,
				Namespace: pubClone.Namespace,
			}
			if err := client.Get(context.TODO(), key, pubClone); err != nil {
				klog.Errorf("Get PodUnavailableBudget(%s) failed: %s", key.String(), err.Error())
				return err
			}
		}
		// Try to verify-and-decrement
		// If it was false already, or if it becomes false during the course of our retries,
		err := checkAndDecrement(pod.Name, pubClone, operation)
		if err != nil {
			return err
		}

		// If this is a dry-run, we don't need to go any further than that.
		if dryRun {
			klog.V(5).Infof("pod(%s) operation for pub(%s.%s) is a dry run", pod.Name, pubClone.Namespace, pubClone.Name)
			return nil
		}
		klog.V(3).Infof("pub(%s.%s) update status(disruptedPods:%d, unavailablePods:%d, expectedCount:%d, desiredAvailable:%d, currentAvailable:%d, unavailableAllowed:%d)",
			pubClone.Namespace, pubClone.Name, len(pubClone.Status.DisruptedPods), len(pubClone.Status.UnavailablePods),
			pubClone.Status.TotalReplicas, pubClone.Status.DesiredAvailable, pubClone.Status.CurrentAvailable, pubClone.Status.UnavailableAllowed)
		if err = client.Status().Update(context.TODO(), pubClone); err == nil {
			return nil
		}
		// if conflict, then retry
		refresh = true
		return err
	})
	if err != nil && err != wait.ErrWaitTimeout {
		klog.V(3).Infof("pod(%s.%s) operation(%s) for pub(%s.%s) failed: %s", pod.Namespace, pod.Name, operation, pub.Namespace, pub.Name, err.Error())
		return false, err.Error(), nil
	} else if err == wait.ErrWaitTimeout {
		err = errors.NewTimeoutError(fmt.Sprintf("couldn't update PodUnavailableBudget %s due to conflicts", pub.Name), 10)
		klog.Errorf("pod(%s.%s) operation(%s) failed: %s", pod.Namespace, pod.Name, operation, err.Error())
		return false, err.Error(), nil
	}

	klog.V(3).Infof("admit pod(%s.%s) operation(%s) for pub(%s.%s)", pod.Namespace, pod.Name, operation, pub.Namespace, pub.Name)
	return true, "", nil
}

func checkAndDecrement(podName string, pub *policyv1alpha1.PodUnavailableBudget, operation Operation) error {
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

	if operation == UpdateOperation {
		pub.Status.UnavailablePods[podName] = metav1.Time{Time: time.Now()}
		klog.V(3).Infof("pod(%s) is recorded in pub(%s.%s) UnavailablePods", podName, pub.Namespace, pub.Name)
	} else {
		pub.Status.DisruptedPods[podName] = metav1.Time{Time: time.Now()}
		klog.V(3).Infof("pod(%s) is recorded in pub(%s.%s) DisruptedPods", podName, pub.Namespace, pub.Name)
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

func GetPodUnavailableBudgetForPod(kClient client.Client, finders *controllerfinder.ControllerFinder, pod *corev1.Pod) (*policyv1alpha1.PodUnavailableBudget, error) {
	var err error
	if len(pod.Labels) == 0 {
		return nil, nil
	}

	pubList := &policyv1alpha1.PodUnavailableBudgetList{}
	if err = kClient.List(context.TODO(), pubList, &client.ListOptions{Namespace: pod.Namespace}); err != nil {
		return nil, err
	}

	var matchedPubs []policyv1alpha1.PodUnavailableBudget
	for _, pub := range pubList.Items {
		// if targetReference isn't nil, priority to take effect
		if pub.Spec.TargetReference != nil {
			targetRef := pub.Spec.TargetReference
			// check whether APIVersion, Kind, Name is equal
			ref := metav1.GetControllerOf(pod)
			if ref == nil {
				continue
			}
			// recursive fetch pod reference, e.g. ref.Kind=Replicas, return podRef.Kind=Deployment
			podRef, err := finders.GetScaleAndSelectorForRef(ref.APIVersion, ref.Kind, pod.Namespace, ref.Name, ref.UID)
			if err != nil {
				return nil, err
			}
			pubRef, err := finders.GetScaleAndSelectorForRef(targetRef.APIVersion, targetRef.Kind, pub.Namespace, targetRef.Name, "")
			if err != nil {
				return nil, err
			}
			if podRef == nil || pubRef == nil {
				continue
			}
			// belongs the same workload
			if isReferenceEqual(podRef, pubRef) {
				matchedPubs = append(matchedPubs, pub)
			}
		} else {
			// This error is irreversible, so continue
			labelSelector, err := util.GetFastLabelSelector(pub.Spec.Selector)
			if err != nil {
				continue
			}
			// If a PUB with a nil or empty selector creeps in, it should match nothing, not everything.
			if labelSelector.Empty() || !labelSelector.Matches(labels.Set(pod.Labels)) {
				continue
			}
			matchedPubs = append(matchedPubs, pub)
		}
	}

	if len(matchedPubs) == 0 {
		klog.V(6).Infof("could not find PodUnavailableBudget for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
		return nil, nil
	}
	if len(matchedPubs) > 1 {
		klog.Warningf("Pod %q/%q matches multiple PodUnavailableBudgets. Choose %q arbitrarily.", pod.Namespace, pod.Name, matchedPubs[0].Name)
	}

	return &matchedPubs[0], nil
}

// check APIVersion, Kind, Name
func isReferenceEqual(ref1, ref2 *controllerfinder.ScaleAndSelector) bool {
	gv1, err := schema.ParseGroupVersion(ref1.APIVersion)
	if err != nil {
		return false
	}
	gv2, err := schema.ParseGroupVersion(ref2.APIVersion)
	if err != nil {
		return false
	}
	return gv1.Group == gv2.Group && ref1.Kind == ref2.Kind &&
		ref1.Name == ref2.Name && ref1.UID == ref2.UID
}
