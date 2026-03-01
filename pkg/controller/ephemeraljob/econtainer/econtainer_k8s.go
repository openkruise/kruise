package econtainer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kubeclient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/util"
)

type ephemeralContainerStatusState int

const (
	SucceededStatus ephemeralContainerStatusState = iota
	FailedStatus
	WaitingStatus
	RunningStatus
	UnknownStatus
)

type k8sControl struct {
	*appsv1alpha1.EphemeralJob
}

var _ EphemeralContainerInterface = &k8sControl{}

func (k *k8sControl) CalculateEphemeralContainerStatus(targetPods []*v1.Pod, status *appsv1alpha1.EphemeralJobStatus) error {

	var success, failed, running, waiting int32
	for _, pod := range targetPods {
		state, err := parseEphemeralPodStatus(k.EphemeralJob, pod.Status.EphemeralContainerStatuses)
		if err != nil {
			return err
		}

		switch state {
		case v1.PodSucceeded:
			success++
		case v1.PodFailed:
			failed++
		case v1.PodRunning:
			running++
		case v1.PodPending:
			waiting++
		}
	}

	status.Succeeded = success
	status.Failed = failed
	status.Running = running
	status.Waiting = waiting

	return nil
}

func (k *k8sControl) GetEphemeralContainersStatus(target *v1.Pod) []v1.ContainerStatus {
	return target.Status.EphemeralContainerStatuses
}

func parseEphemeralContainerStatus(status *v1.ContainerStatus) ephemeralContainerStatusState {
	if status.State.Terminated != nil {
		if !status.State.Terminated.FinishedAt.IsZero() && status.State.Terminated.ExitCode == 0 {
			return SucceededStatus
		}

		if status.State.Terminated.ExitCode != 0 {
			return FailedStatus
		}
	}
	if status.State.Running != nil {
		return RunningStatus
	}
	if status.State.Waiting != nil {
		if status.State.Waiting.Reason == "RunContainerError" {
			return FailedStatus
		}
		return WaitingStatus
	}

	return UnknownStatus
}

func parseEphemeralPodStatus(ejob *appsv1alpha1.EphemeralJob, statuses []v1.ContainerStatus) (v1.PodPhase, error) {
	eContainerMap, empty := getEphemeralContainersMaps(ejob.Spec.Template.EphemeralContainers)
	if empty {
		klog.InfoS("EphemeralJob spec containers is empty")
		return v1.PodUnknown, fmt.Errorf("ephemeral job %s/%s spec containers is empty. ", ejob.Namespace, ejob.Name)
	}

	var waitingCount, runningCount, succeededCount int
	for _, eContainerStatus := range statuses {
		if _, ok := eContainerMap[eContainerStatus.Name]; !ok {
			continue
		}

		status := parseEphemeralContainerStatus(&eContainerStatus)
		klog.V(5).InfoS("Parse ephemeral container status", "ephemeralContainerStatusName", eContainerStatus.Name, "status", status)
		switch status {
		case FailedStatus:
			return v1.PodFailed, nil
		case WaitingStatus:
			waitingCount++
		case RunningStatus:
			runningCount++
		case SucceededStatus:
			succeededCount++
		}
	}

	if succeededCount > 0 && succeededCount == len(eContainerMap) {
		return v1.PodSucceeded, nil
	}
	if runningCount > 0 && runningCount <= len(eContainerMap) {
		return v1.PodRunning, nil
	}
	if waitingCount > 0 && waitingCount <= len(eContainerMap) {
		return v1.PodPending, nil
	}

	return v1.PodUnknown, nil
}

func (k *k8sControl) GetEphemeralContainers(targetPod *v1.Pod) []v1.EphemeralContainer {
	return targetPod.Spec.EphemeralContainers
}

func (k *k8sControl) CreateEphemeralContainer(targetPod *v1.Pod) error {
	var eContainer []v1.EphemeralContainer
	for i := range k.Spec.Template.EphemeralContainers {
		ec := k.Spec.Template.EphemeralContainers[i].DeepCopy()
		ec.Env = append(ec.Env, v1.EnvVar{
			Name:  appsv1alpha1.EphemeralContainerEnvKey,
			Value: string(k.UID),
		})
		eContainer = append(eContainer, *ec)
	}

	err := k.createEphemeralContainer(targetPod, eContainer)
	if err != nil {
		// The apiserver will return a 404 when the EphemeralContainers feature is disabled because the `/ephemeralcontainers` subresource
		// is missing. Unlike the 404 returned by a missing pod, the status details will be empty.
		if serr, ok := err.(*errors.StatusError); ok && serr.Status().Reason == metav1.StatusReasonNotFound && serr.ErrStatus.Details.Name == "" {
			klog.ErrorS(err, "Ephemeral containers were disabled for this cluster (error from server)")
			return nil
		}

		// The Kind used for the /ephemeralcontainers subresource changed in 1.22. When presented with an unexpected
		// Kind the api server will respond with a not-registered error. When this happens we can optimistically try
		// using the old API.
		if runtime.IsNotRegisteredError(err) {
			klog.V(1).ErrorS(err, "Falling back to legacy ephemeral container API because server returned error")
			return k.createEphemeralContainerLegacy(targetPod, eContainer)
		}
	}
	return err
}

// createEphemeralContainer adds ephemeral containers using the 1.22 or newer /ephemeralcontainers API.
func (k *k8sControl) createEphemeralContainer(targetPod *v1.Pod, eContainer []v1.EphemeralContainer) error {
	oldPodJS, _ := json.Marshal(targetPod)
	newPod := targetPod.DeepCopy()
	newPod.Spec.EphemeralContainers = append(newPod.Spec.EphemeralContainers, eContainer...)
	newPodJS, _ := json.Marshal(newPod)

	patch, err := strategicpatch.CreateTwoWayMergePatch(oldPodJS, newPodJS, &v1.Pod{})
	if err != nil {
		return fmt.Errorf("error creating patch to add ephemeral containers: %v", err)
	}

	klog.InfoS("EphemeralJob tried to patch containers to Pod", "ephemeralJob", klog.KObj(k), "pod", klog.KObj(targetPod), "patch", util.DumpJSON(patch))

	kubeClient := kubeclient.GetGenericClient().KubeClient
	_, err = kubeClient.CoreV1().Pods(targetPod.Namespace).
		Patch(context.TODO(), targetPod.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}, "ephemeralcontainers")
	return err
}

// createEphemeralContainerLegacy adds ephemeral containers using the pre-1.22 /ephemeralcontainers API
// This may be removed when we no longer wish to support releases prior to 1.22.
func (k *k8sControl) createEphemeralContainerLegacy(targetPod *v1.Pod, eContainer []v1.EphemeralContainer) error {
	var body []map[string]interface{}
	for _, ec := range eContainer {
		body = append(body, map[string]interface{}{
			"op":    "add",
			"path":  "/ephemeralContainers/-",
			"value": ec,
		})
	}

	// We no longer have the v1.EphemeralContainers Kind since it was removed in 1.22, but
	// we can present a JSON 6902 patch that the api server will apply.
	patch, err := json.Marshal(body)
	if err != nil {
		klog.ErrorS(err, "Failed to creat JSON 6902 patch for old /ephemeralcontainers API")
		return nil
	}

	kubeClient := kubeclient.GetGenericClient().KubeClient
	_, err = kubeClient.CoreV1().Pods(targetPod.Namespace).
		Patch(context.TODO(), targetPod.Name, types.JSONPatchType, patch, metav1.PatchOptions{}, "ephemeralcontainers")
	return err
}

// RemoveEphemeralContainer is not support before kubernetes v1.23
func (k *k8sControl) RemoveEphemeralContainer(target *v1.Pod) (*time.Duration, error) {
	klog.InfoS("RemoveEphemeralContainer is not support before kubernetes v1.23")
	return nil, nil
}

// UpdateEphemeralContainer is not support before kubernetes v1.23
func (k *k8sControl) UpdateEphemeralContainer(target *v1.Pod) error {
	klog.InfoS("UpdateEphemeralContainer is not support before kubernetes v1.23")
	return nil
}

func (k *k8sControl) ContainsEphemeralContainer(target *v1.Pod) (exists, owned bool) {
	ephemeralContainersMaps, _ := getEphemeralContainersMaps(k.GetEphemeralContainers(target))
	for _, e := range k.Spec.Template.EphemeralContainers {
		if targetEC, ok := ephemeralContainersMaps[e.Name]; ok {
			return true, isCreatedByEJob(string(k.UID), targetEC)
		}
	}
	return false, false
}

func isCreatedByEJob(jobUid string, container v1.EphemeralContainer) bool {
	for _, env := range container.Env {
		if env.Name == appsv1alpha1.EphemeralContainerEnvKey && env.Value == jobUid {
			return true
		}
	}
	return false
}
