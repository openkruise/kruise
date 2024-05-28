package ephemeraljob

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/ephemeraljob/econtainer"
	"github.com/openkruise/kruise/pkg/util"
)

// pastActiveDeadline checks if job has ActiveDeadlineSeconds field set and if it is exceeded.
func pastActiveDeadline(job *appsv1alpha1.EphemeralJob) bool {
	if job.Spec.ActiveDeadlineSeconds == nil || job.Status.StartTime == nil {
		return false
	}
	start := job.Status.StartTime.Time
	duration := time.Since(start)
	allowedDuration := time.Duration(*job.Spec.ActiveDeadlineSeconds) * time.Second
	return duration >= allowedDuration
}

func podMatchedEphemeralJob(pod *v1.Pod, ejob *appsv1alpha1.EphemeralJob) (bool, error) {
	// if selector not matched, then continue
	if pod.Namespace != ejob.Namespace {
		return false, nil
	}
	selector, err := util.ValidatedLabelSelectorAsSelector(ejob.Spec.Selector)
	if err != nil {
		return false, err
	}
	if !selector.Empty() && selector.Matches(labels.Set(pod.Labels)) {
		return true, nil
	}

	return false, nil
}

func addConditions(conditions []appsv1alpha1.EphemeralJobCondition, conditionType appsv1alpha1.EphemeralJobConditionType, reason, message string) []appsv1alpha1.EphemeralJobCondition {
	condition := newCondition(conditionType, reason, message)
	if len(conditions) == 0 {
		return []appsv1alpha1.EphemeralJobCondition{
			condition,
		}
	}

	for i := range conditions {
		if conditions[i].Type == condition.Type {
			conditions[i] = condition
			return conditions
		}
	}

	conditions = append(conditions, condition)
	return conditions
}

func newCondition(conditionType appsv1alpha1.EphemeralJobConditionType, reason, message string) appsv1alpha1.EphemeralJobCondition {
	return appsv1alpha1.EphemeralJobCondition{
		Type:               conditionType,
		Status:             v1.ConditionTrue,
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
}

func timeNow() *metav1.Time {
	now := metav1.Now()
	return &now
}

func getEphemeralContainersMaps(containers []v1.EphemeralContainer) (map[string]v1.EphemeralContainer, bool) {
	res := make(map[string]v1.EphemeralContainer)
	for _, c := range containers {
		res[c.Name] = c
	}
	return res, len(res) == 0
}

func getPodEphemeralContainers(pod *v1.Pod, ejob *appsv1alpha1.EphemeralJob) []string {
	eContainers := ejob.Spec.Template.EphemeralContainers
	podEphemeralNames := make([]string, len(eContainers))
	for i := range ejob.Spec.Template.EphemeralContainers {
		podEphemeralNames[i] = pod.Name + "-" + eContainers[i].Name
	}
	return podEphemeralNames
}

func existDuplicatedEphemeralContainer(job *appsv1alpha1.EphemeralJob, targetPod *v1.Pod) bool {
	ephemeralContainersMaps, _ := getEphemeralContainersMaps(econtainer.New(job).GetEphemeralContainers(targetPod))
	for _, e := range job.Spec.Template.EphemeralContainers {
		if targetEC, ok := ephemeralContainersMaps[e.Name]; ok && !isCreatedByEJob(string(job.UID), targetEC) {
			return true
		}
	}

	return false
}

func isCreatedByEJob(jobUid string, container v1.EphemeralContainer) bool {
	for _, env := range container.Env {
		if env.Name == appsv1alpha1.EphemeralContainerEnvKey && env.Value == jobUid {
			return true
		}
	}
	return false
}

func getSyncPods(job *appsv1alpha1.EphemeralJob, pods []*v1.Pod) (toCreate, toUpdate, toDelete []*v1.Pod) {
	eContainersMap, empty := getEphemeralContainersMaps(job.Spec.Template.EphemeralContainers)
	if empty {
		return
	}
	for _, pod := range pods {
		if len(pod.Spec.EphemeralContainers) == 0 {
			toCreate = append(toCreate, pod)
			continue
		}

		isAddToPod := true
		for _, c := range pod.Spec.EphemeralContainers {
			if _, ok := eContainersMap[c.Name]; ok && isCreatedByEJob(string(job.UID), c) {
				isAddToPod = false
				break
			}
		}
		if isAddToPod {
			toCreate = append(toCreate, pod)
		}
	}
	return
}

func calculateEphemeralContainerStatus(job *appsv1alpha1.EphemeralJob, pods []*v1.Pod) error {
	var success, failed, running, waiting int32
	for _, pod := range pods {
		state, err := parseEphemeralPodStatus(job, econtainer.New(job).GetEphemeralContainersStatus(pod))
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

	job.Status.Succeeded = success
	job.Status.Failed = failed
	job.Status.Running = running
	job.Status.Waiting = waiting

	return nil
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

type ephemeralContainerStatusState int

const (
	SucceededStatus ephemeralContainerStatusState = iota
	FailedStatus
	WaitingStatus
	RunningStatus
	UnknownStatus
)

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

func hasEphemeralContainerFinalizer(finalizers []string) bool {
	if len(finalizers) == 0 {
		return false
	}

	for _, f := range finalizers {
		if f == EphemeralContainerFinalizer {
			return true
		}
	}

	return false
}

func deleteEphemeralContainerFinalizer(finalizers []string, finalizer string) []string {
	if len(finalizers) == 0 {
		return finalizers
	}

	for i, f := range finalizers {
		if f == finalizer {
			finalizers = append(finalizers[:i], finalizers[i+1:]...)
		}
	}

	return finalizers
}
