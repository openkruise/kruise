package mutating

import (
	"context"
	"strconv"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	priorityName       = "KRUISE_CONTAINER_PRIORITY"
	priorityBarrier    = "KRUISE_CONTAINER_BARRIER"
	priorityAnnotation = "apps.kruise.io/container-launch-priority"
	priorityOrdered    = "Ordered"
)

// start containers based on priority order
func (h *PodCreateHandler) containerLaunchPriorityInitialization(ctx context.Context, req admission.Request, pod *corev1.Pod) error {
	if len(req.AdmissionRequest.SubResource) > 0 ||
		req.AdmissionRequest.Operation != admissionv1.Create ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return nil
	}

	if len(pod.Spec.Containers) == 1 {
		return nil
	}

	// if ordered flag has been set, then just process ordered logic and skip check for priority
	if pod.Annotations[priorityAnnotation] == priorityOrdered {
		priority := make([]int, len(pod.Spec.Containers))
		for i := range priority {
			priority[i] = 0 - i
		}
		h.setPodEnv(priority, pod)
		return nil
	}

	// check whether containers have KRUISE_CONTAINER_PRIORITY key value pairs
	priority, priorityFlag, err := h.getPriority(pod)
	if err != nil {
		return err
	}
	if !priorityFlag {
		return nil
	}

	h.setPodEnv(priority, pod)
	return nil
}

// the return []int is prioirty for each container in the pod, ordered as container
// order list in pod spec.
// the priorityFlag indicates whether this pod needs to launch containers with priority.
// return error is there is any (e.g. priority value less than minimum possible int value)
func (h *PodCreateHandler) getPriority(pod *corev1.Pod) ([]int, bool, error) {
	var priorityFlag bool
	var priority = make([]int, len(pod.Spec.Containers))
	for i, c := range pod.Spec.Containers {
		for _, e := range c.Env {
			if e.Name == priorityName {
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
	for i := range priority {
		env := corev1.EnvVar{
			Name: priorityBarrier,
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: pod.Name + "-barrier"},
					Key:                  "p_" + strconv.Itoa(priority[i]),
				},
			},
		}
		pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, env)
	}
}
