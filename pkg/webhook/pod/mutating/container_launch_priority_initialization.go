package mutating

import (
	"context"
	"strconv"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	storagenames "k8s.io/apiserver/pkg/storage/names"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	utilcontainerlaunchpriority "github.com/openkruise/kruise/pkg/util/containerlaunchpriority"
)

// start containers based on priority order
func (h *PodCreateHandler) containerLaunchPriorityInitialization(_ context.Context, req admission.Request, pod *corev1.Pod) (skip bool, err error) {
	if len(req.AdmissionRequest.SubResource) > 0 ||
		req.AdmissionRequest.Operation != admissionv1.Create ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return true, nil
	}

	if len(pod.Spec.Containers) == 1 {
		return true, nil
	}

	// if ordered flag has been set, then just process ordered logic and skip check for priority
	if pod.Annotations[appspub.ContainerLaunchPriorityKey] == appspub.ContainerLaunchOrdered {
		priority := make([]int, len(pod.Spec.Containers))
		for i := range priority {
			priority[i] = 0 - i
		}
		h.setPodEnv(priority, pod)
		klog.V(3).InfoS("Injected ordered container launch priority for Pod", "namespace", pod.Namespace, "name", pod.Name)
		return false, nil
	}

	// check whether containers have KRUISE_CONTAINER_PRIORITY key value pairs
	priority, priorityFlag, err := h.getPriority(pod)
	if err != nil {
		return false, err
	}
	if !priorityFlag {
		return true, nil
	}

	h.setPodEnv(priority, pod)
	klog.V(3).InfoS("Injected customized container launch priority for Pod", "namespace", pod.Namespace, "name", pod.Name)
	return false, nil
}

// the return []int is priority for each container in the pod, ordered as container
// order list in pod spec.
// the priorityFlag indicates whether this pod needs to launch containers with priority.
// return error is there is any (e.g. priority value less than minimum possible int value)
func (h *PodCreateHandler) getPriority(pod *corev1.Pod) ([]int, bool, error) {
	var priorityFlag bool
	var priority = make([]int, len(pod.Spec.Containers))
	for i, c := range pod.Spec.Containers {
		for _, e := range c.Env {
			if e.Name == appspub.ContainerLaunchPriorityEnvName {
				p, err := strconv.Atoi(e.Value)
				if err != nil {
					return nil, false, err
				}
				priority[i] = p
				if p != 0 {
					priorityFlag = true
				}
			}
		}
	}

	// if all priorities are same, than no priority is needed
	if priorityFlag {
		var equityFlag = true
		for _, v := range priority {
			if v != priority[0] {
				equityFlag = false
			}
		}
		priorityFlag = !equityFlag
	}
	return priority, priorityFlag, nil
}

func (h *PodCreateHandler) setPodEnv(priority []int, pod *corev1.Pod) {
	// Generate name for pods that only have generateName field
	if len(pod.Name) == 0 && len(pod.GenerateName) > 0 {
		pod.Name = storagenames.SimpleNameGenerator.GenerateName(pod.GenerateName)
	}
	for i := range priority {
		pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, utilcontainerlaunchpriority.GeneratePriorityEnv(priority[i], pod.Name))
	}
}
