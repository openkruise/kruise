package mutating

import (
	"encoding/json"
	"fmt"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	AnnotationUsingEnhancedLiveness = "apps.kruise.io/using-enhanced-liveness"
	AnnotationNativeLivenessContext = "apps.kruise.io/livenessprobe-context"
)

type containerLivenessProbe struct {
	Name          string   `json:"name"`
	LivenessProbe v1.Probe `json:"livenessProbe"`
}

func (h *PodCreateHandler) enhancedLivenessProbeWhenPodCreate(req admission.Request, pod *v1.Pod) (skip bool, err error) {

	if len(req.AdmissionRequest.SubResource) > 0 ||
		req.AdmissionRequest.Operation != admissionv1.Create ||
		req.AdmissionRequest.Resource.Resource != "pods" {
		return true, nil
	}

	if !util.IsPodOwnedByKruise(pod) && !utilfeature.DefaultFeatureGate.Enabled(features.EnhancedLivenessProbe) {
		return true, nil
	}

	if !usingEnhancedLivenessProbe(pod) {
		return true, nil
	}

	context, err := removeAndBackUpPodContainerLivenessProbe(pod)
	if err != nil {
		klog.Errorf("remove pod (%v/%v) container livenessProbe config and backup error: %v", pod.Namespace, pod.Name, err)
		return false, err
	}
	klog.V(3).Infof("mutating add pod(%s/%s) annotation[%s]=%s", pod.Namespace, pod.Name, AnnotationNativeLivenessContext, context)
	return false, nil
}

func removeAndBackUpPodContainerLivenessProbe(pod *v1.Pod) (string, error) {
	if len(pod.Spec.Containers) == 0 {
		return "", nil
	}

	containersLivenessProbe := []containerLivenessProbe{}
	for index, _ := range pod.Spec.Containers {
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
		klog.Errorf("failed to json marshal %v for pod: %v/%v, err: %v",
			containersLivenessProbe, pod.Namespace, pod.Name, err)
		return "", fmt.Errorf("failed to json marshal %v for pod: %v/%v, err: %v",
			containersLivenessProbe, pod.Namespace, pod.Name, err)
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[AnnotationNativeLivenessContext] = string(containersLivenessProbeRaw)
	return pod.Annotations[AnnotationNativeLivenessContext], nil
}

func usingEnhancedLivenessProbe(pod *v1.Pod) bool {
	if pod.Annotations[AnnotationUsingEnhancedLiveness] == "true" {
		return true
	}
	return false
}
