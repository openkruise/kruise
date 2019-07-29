/*
Copyright 2019 The Kruise Authors.

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

package mutating

import (
	"context"
	"encoding/json"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/webhook/default_server/sidecarset/mutating"
)

func init() {
	webhookName := "mutating-create-pod"
	if HandlerMap[webhookName] == nil {
		HandlerMap[webhookName] = []admission.Handler{}
	}
	HandlerMap[webhookName] = append(HandlerMap[webhookName], &PodCreateHandler{})
}

var (
	// SidecarIgnoredNamespaces specifies the namespaces where Pods won't get injected
	SidecarIgnoredNamespaces = []string{"kube-system", "kube-public"}
	// SidecarEnvKey specifies the environment variable which marks a container as injected
	SidecarEnvKey = "IS_INJECTED"
)

// PodCreateHandler handles Pod
type PodCreateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	Client client.Client

	// Decoder decodes objects
	Decoder types.Decoder
}

func (h *PodCreateHandler) mutatingPodFn(ctx context.Context, obj *corev1.Pod) error {
	return h.sidecarsetMutatingPod(ctx, obj)
}

func (h *PodCreateHandler) sidecarsetMutatingPod(ctx context.Context, pod *corev1.Pod) error {
	for _, namespace := range SidecarIgnoredNamespaces {
		if pod.Namespace == namespace {
			return nil
		}
	}

	klog.V(3).Infof("[sidecar inject] begin to process %s/%s", pod.Namespace, pod.Name)

	sidecarSets := &appsv1alpha1.SidecarSetList{}
	if err := h.Client.List(ctx, &client.ListOptions{}, sidecarSets); err != nil {
		return err
	}

	var sidecarContainers []corev1.Container
	sidecarSetHash := make(map[string]string)
	sidecarSetHashWithoutImage := make(map[string]string)
	matchNothing := true
	for _, sidecarSet := range sidecarSets.Items {
		needInject, err := PodMatchSidecarSet(pod, sidecarSet)
		if err != nil {
			return err
		}
		if !needInject {
			continue
		}
		matchNothing = false

		sidecarSetHash[sidecarSet.Name] = sidecarSet.Annotations[mutating.SidecarSetHashAnnotation]
		sidecarSetHashWithoutImage[sidecarSet.Name] = sidecarSet.Annotations[mutating.SidecarSetHashWithoutImageAnnotation]

		for i := range sidecarSet.Spec.Containers {
			sidecarContainer := &sidecarSet.Spec.Containers[i]

			// add env to container
			sidecarContainer.Env = append(sidecarContainer.Env, corev1.EnvVar{Name: SidecarEnvKey, Value: "true"})

			sidecarContainers = append(sidecarContainers, sidecarContainer.Container)
		}
	}
	if matchNothing {
		return nil
	}

	klog.V(4).Infof("[sidecar inject] before mutating: %v", util.DumpJSON(pod))
	// apply sidecar info into pod
	// 1. apply containers
	pod.Spec.Containers = append(pod.Spec.Containers, sidecarContainers...)
	// 2. apply annotations
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	if len(sidecarSetHash) != 0 {
		encodedStr, err := json.Marshal(sidecarSetHash)
		if err != nil {
			return err
		}
		pod.Annotations[mutating.SidecarSetHashAnnotation] = string(encodedStr)

		encodedStr, err = json.Marshal(sidecarSetHashWithoutImage)
		if err != nil {
			return err
		}
		pod.Annotations[mutating.SidecarSetHashWithoutImageAnnotation] = string(encodedStr)
	}
	klog.V(4).Infof("[sidecar inject] after mutating: %v", util.DumpJSON(pod))

	return nil
}

// PodMatchSidecarSet determines if pod match Selector of sidecar.
func PodMatchSidecarSet(pod *corev1.Pod, sidecarSet appsv1alpha1.SidecarSet) (bool, error) {
	selector, err := metav1.LabelSelectorAsSelector(sidecarSet.Spec.Selector)
	if err != nil {
		return false, err
	}

	if !selector.Empty() && selector.Matches(labels.Set(pod.Labels)) {
		return true, nil
	}
	return false, nil
}

var _ admission.Handler = &PodCreateHandler{}

// Handle handles admission requests.
func (h *PodCreateHandler) Handle(ctx context.Context, req types.Request) types.Response {
	obj := &corev1.Pod{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	copy := obj.DeepCopy()

	err = h.mutatingPodFn(ctx, copy)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}
	return admission.PatchResponse(obj, copy)
}

var _ inject.Client = &PodCreateHandler{}

// InjectClient injects the client into the PodCreateHandler
func (h *PodCreateHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

var _ inject.Decoder = &PodCreateHandler{}

// InjectDecoder injects the decoder into the PodCreateHandler
func (h *PodCreateHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}
