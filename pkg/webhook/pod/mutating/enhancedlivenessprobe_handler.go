package mutating

import (
	"context"
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

type containerLivenessProbe struct {
	Name          string   `json:"name"`
	LivenessProbe v1.Probe `json:"livenessProbe"`
}

func (h *PodCreateHandler) enhancedLivenessProbeWhenPodCreate(ctx context.Context, req admission.Request, pod *v1.Pod) (skip bool, err error) {

	if len(req.AdmissionRequest.SubResource) > 0 ||
		req.AdmissionRequest.Operation != admissionv1.Create ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return true, nil
	}

	if !util.IsPodOwnedByKruise(pod) {
		return true, nil
	}

	if !usingEnhancedLivenessProbe(pod) {
		return true, nil
	}

	context, err := removeAndBackUpPodContainerLivenessProbe(pod)
	if err != nil {
		klog.ErrorS(err, "Remove pod container livenessProbe config and backup error", "namespace", pod.Namespace, "name", pod.Name)
		return false, err
	}
	if context == "" {
		return true, nil
	}
	klog.V(3).InfoS("Mutating add pod annotation", "namespace", pod.Namespace, "name", pod.Name, "key", alpha1.AnnotationNativeContainerProbeContext, "value", context)
	return false, nil
}

// return two parameters:
// 1. the json string of the pod containers native livenessProbe configurations.
// 2. the error reason of the function.
func removeAndBackUpPodContainerLivenessProbe(pod *v1.Pod) (string, error) {
	containersLivenessProbe := []containerLivenessProbe{}
	for index := range pod.Spec.Containers {
		getContainer := &pod.Spec.Containers[index]
		if getContainer.LivenessProbe == nil {
			continue
		}
		containersLivenessProbe = append(containersLivenessProbe, containerLivenessProbe{
			Name:          getContainer.Name,
			LivenessProbe: *getContainer.LivenessProbe,
		})
		getContainer.LivenessProbe = nil
	}

	if len(containersLivenessProbe) == 0 {
		return "", nil
	}
	containersLivenessProbeRaw, err := json.Marshal(containersLivenessProbe)
	if err != nil {
		klog.ErrorS(err, "Failed to json marshal liveness probe for pod",
			"probe", containersLivenessProbe, "namespace", pod.Namespace, "name", pod.Name)
		return "", fmt.Errorf("Failed to json marshal %v for pod: %v/%v, err: %v",
			containersLivenessProbe, pod.Namespace, pod.Name, err)
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[alpha1.AnnotationNativeContainerProbeContext] = string(containersLivenessProbeRaw)
	return pod.Annotations[alpha1.AnnotationNativeContainerProbeContext], nil
}

// return one parameter:
// 1. the native container livenessprobe is enabled when the alpha1.AnnotationUsingEnhancedLiveness is true.
func usingEnhancedLivenessProbe(pod *v1.Pod) bool {
	return pod.Annotations[alpha1.AnnotationUsingEnhancedLiveness] == "true"
}
