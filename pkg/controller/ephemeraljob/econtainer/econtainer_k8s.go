package econtainer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openkruise/kruise/pkg/util"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/util/strategicpatch"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kubeclient "github.com/openkruise/kruise/pkg/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
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
		klog.Error("ephemeral job spec containers is empty")
		return v1.PodUnknown, fmt.Errorf("ephemeral job %s/%s spec containers is empty. ", ejob.Namespace, ejob.Name)
	}

	var waitingCount, runningCount, succeededCount int
	for _, eContainerStatus := range statuses {
		if _, ok := eContainerMap[eContainerStatus.Name]; !ok {
			continue
		}

		status := parseEphemeralContainerStatus(&eContainerStatus)
		klog.V(5).Infof("parse ephemeral container %s status %s", eContainerStatus.Name, status)
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
	oldPodJS, _ := json.Marshal(targetPod)
	newPod := targetPod.DeepCopy()
	for i := range k.Spec.Template.EphemeralContainers {
		ec := k.Spec.Template.EphemeralContainers[i].DeepCopy()
		ec.Env = append(ec.Env, v1.EnvVar{
			Name:  appsv1alpha1.EphemeralContainerEnvKey,
			Value: string(k.UID),
		})
		newPod.Spec.EphemeralContainers = append(newPod.Spec.EphemeralContainers, *ec)
	}
	newPodJS, _ := json.Marshal(newPod)

	patch, err := strategicpatch.CreateTwoWayMergePatch(oldPodJS, newPodJS, &v1.Pod{})
	if err != nil {
		return fmt.Errorf("error creating patch to add ephemeral containers: %v", err)
	}

	klog.Infof("EphemeralJob %s/%s tries to patch containers to pod %s: %v", k.Namespace, k.Name, targetPod.Name, util.DumpJSON(patch))

	kubeClient := kubeclient.GetGenericClient().KubeClient
	_, err = kubeClient.CoreV1().Pods(targetPod.Namespace).
		Patch(context.TODO(), targetPod.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}, "ephemeralcontainers")
	return err
}

// RemoveEphemeralContainer is not support before kubernetes v1.23
func (k *k8sControl) RemoveEphemeralContainer(target *v1.Pod) error {
	klog.Warning("RemoveEphemeralContainer is not support before kubernetes v1.23")
	return nil
}

// UpdateEphemeralContainer is not support before kubernetes v1.23
func (k *k8sControl) UpdateEphemeralContainer(target *v1.Pod) error {
	klog.Warning("UpdateEphemeralContainer is not support before kubernetes v1.23")
	return nil
}
