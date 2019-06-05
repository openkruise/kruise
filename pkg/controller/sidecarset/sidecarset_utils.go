package sidecarset

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/webhook/default_server/pod/mutating"
)

func isIgnoredPod(pod *corev1.Pod) bool {
	for _, namespace := range mutating.SidecarIgnoredNamespaces {
		if pod.Namespace == namespace {
			return true
		}
	}
	return false
}

func calculateStatus(sidecarSet *appsv1alpha1.SidecarSet, pods []*corev1.Pod) (*appsv1alpha1.SidecarSetStatus, error) {
	var matchedPods, updatedPods, readyPods int32
	matchedPods = int32(len(pods))
	for _, pod := range pods {
		updated, err := isPodUpdated(sidecarSet, pod)
		if err != nil {
			return nil, err
		}
		if updated {
			updatedPods++
		}

		if isRunningAndReady(pod) {
			readyPods++
		}
	}

	return &appsv1alpha1.SidecarSetStatus{
		ObservedGeneration: sidecarSet.Generation,
		MatchedPods:        matchedPods,
		UpdatedPods:        updatedPods,
		ReadyPods:          readyPods,
	}, nil
}

func isPodUpdated(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) (bool, error) {
	if pod.Annotations[mutating.SidecarSetGenerationAnnotation] == "" {
		return false, nil
	}

	sidecarSetGeneration := make(map[string]int64)
	if err := json.Unmarshal([]byte(pod.Annotations[mutating.SidecarSetGenerationAnnotation]), &sidecarSetGeneration); err != nil {
		return false, err
	}

	if sidecarSetGeneration[sidecarSet.Name] == sidecarSet.Status.ObservedGeneration {
		return true, nil
	}
	return false, nil
}

func isRunningAndReady(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning && podutil.IsPodReady(pod)
}

func (r *ReconcileSidecarSet) updateSidecarSetStatus(sidecarSet *appsv1alpha1.SidecarSet, status *appsv1alpha1.SidecarSetStatus) error {
	if !inconsistentStatus(sidecarSet, status) {
		return nil
	}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		sidecarSet.Status = *status

		updateErr := r.Status().Update(context.TODO(), sidecarSet)
		if updateErr == nil {
			return nil
		}

		key := types.NamespacedName{
			Name: sidecarSet.Name,
		}

		if err := r.Get(context.TODO(), key, sidecarSet); err != nil {
			klog.Errorf("error getting updated service %s from client", sidecarSet.Name)
		}

		return updateErr
	})

	return err
}

func inconsistentStatus(sidecarSet *appsv1alpha1.SidecarSet, status *appsv1alpha1.SidecarSetStatus) bool {
	return status.ObservedGeneration > sidecarSet.Status.ObservedGeneration ||
		status.MatchedPods != sidecarSet.Status.MatchedPods ||
		status.UpdatedPods != sidecarSet.Status.UpdatedPods ||
		status.ReadyPods != sidecarSet.Status.ReadyPods
}
