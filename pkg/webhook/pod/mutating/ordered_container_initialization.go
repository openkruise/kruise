package mutating

import (
	"context"
	"strconv"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	priorityName    = "KRUISE_CONTAINER_PRIORITY"
	priorityBarrier = "KRUISE_CONTAINER_BARRIER"
)

// start containers based on priority order
func (h *PodCreateHandler) containerLaunchPriorityInitialization(ctx context.Context, req admission.Request, pod *corev1.Pod) error {
	if len(req.AdmissionRequest.SubResource) > 0 ||
		req.AdmissionRequest.Operation != admissionv1beta1.Create ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return nil
	}

	// check whether pod has KRUISE_CONTAINER_PRIORITY key value pairs
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
