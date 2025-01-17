/*
Copyright 2021 The Kruise Authors.

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
	"fmt"
	"net/http"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/sidecarterminator"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

const (
	minDeadlineSeconds = 3
)

// ContainerRecreateRequestHandler handles ContainerRecreateRequest
type ContainerRecreateRequestHandler struct {
	Client  client.Client
	Decoder admission.Decoder
}

// Handle handles admission requests.
func (h *ContainerRecreateRequestHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if !utilfeature.DefaultFeatureGate.Enabled(features.KruiseDaemon) {
		return admission.Errored(http.StatusForbidden, fmt.Errorf("feature-gate %s is not enabled", features.KruiseDaemon))
	}

	obj := &appsv1alpha1.ContainerRecreateRequest{}
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	var copy runtime.Object = obj.DeepCopy()

	if req.AdmissionRequest.Operation == admissionv1.Update {
		oldObj := &appsv1alpha1.ContainerRecreateRequest{}
		if err := h.Decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if !reflect.DeepEqual(obj.Spec, oldObj.Spec) {
			return admission.Errored(http.StatusForbidden, fmt.Errorf("spec of ContainerRecreateRequest is immutable"))
		}
		if obj.Labels[appsv1alpha1.ContainerRecreateRequestPodUIDKey] != oldObj.Labels[appsv1alpha1.ContainerRecreateRequestPodUIDKey] {
			return admission.Errored(http.StatusForbidden, fmt.Errorf("not allowed to update immutable label %s", appsv1alpha1.ContainerRecreateRequestPodUIDKey))
		}
		if obj.Labels[appsv1alpha1.ContainerRecreateRequestNodeNameKey] != oldObj.Labels[appsv1alpha1.ContainerRecreateRequestNodeNameKey] {
			return admission.Errored(http.StatusForbidden, fmt.Errorf("not allowed to update immutable label %s", appsv1alpha1.ContainerRecreateRequestNodeNameKey))
		}
		if oldObj.Annotations[appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey] != "" &&
			obj.Annotations[appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey] != oldObj.Annotations[appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey] {
			return admission.Errored(http.StatusForbidden, fmt.Errorf("not allowed to update immutable annotation %s", appsv1alpha1.ContainerRecreateRequestUnreadyAcquiredKey))
		}
		return admission.Allowed("")
	}

	if obj.Labels == nil {
		obj.Labels = map[string]string{}
	}

	if obj.Spec.PodName == "" {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("podName can not be empty"))
	}
	if obj.Spec.Strategy == nil {
		obj.Spec.Strategy = &appsv1alpha1.ContainerRecreateRequestStrategy{}
	}
	obj.Labels[appsv1alpha1.ContainerRecreateRequestActiveKey] = "true"
	if len(obj.Spec.Containers) == 0 {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("containers list can not be null"))
	}
	if obj.Spec.ActiveDeadlineSeconds != nil && *obj.Spec.ActiveDeadlineSeconds < minDeadlineSeconds {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("activeDeadlineSeconds can not be less than %ds", minDeadlineSeconds))
	}
	if obj.Spec.Strategy.TerminationGracePeriodSeconds != nil && *obj.Spec.Strategy.TerminationGracePeriodSeconds < 0 {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("terminationGracePeriodSeconds must be non-negative integer"))
	}
	if obj.Spec.Strategy.UnreadyGracePeriodSeconds != nil && *obj.Spec.Strategy.UnreadyGracePeriodSeconds < 0 {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unreadyGracePeriodSeconds must be non-negative integer"))
	}

	// defaults
	switch obj.Spec.Strategy.FailurePolicy {
	case "":
		obj.Spec.Strategy.FailurePolicy = appsv1alpha1.ContainerRecreateRequestFailurePolicyFail
	case appsv1alpha1.ContainerRecreateRequestFailurePolicyFail, appsv1alpha1.ContainerRecreateRequestFailurePolicyIgnore:
	default:
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unknown failurePolicy %s", obj.Spec.Strategy.FailurePolicy))
	}

	pod := &v1.Pod{}
	if err := h.Client.Get(ctx, types.NamespacedName{Namespace: obj.Namespace, Name: obj.Spec.PodName}, pod); err != nil {
		if errors.IsNotFound(err) {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("no found Pod named %s", obj.Spec.PodName))
		}
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to find Pod %s: %v", obj.Spec.PodName, err))
	}
	// check isTerminatedBySidecarTerminator,
	// because we need create CRR to kill sidecar container after change pod phase to terminal phase in SidecarTerminator Controller.
	if !kubecontroller.IsPodActive(pod) && !isTerminatedBySidecarTerminator(pod) {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("not allowed to recreate containers in an inactive Pod"))
	} else if pod.Spec.NodeName == "" {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("not allowed to recreate containers in a pending Pod"))
	}

	err = injectPodIntoContainerRecreateRequest(obj, pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if reflect.DeepEqual(obj, copy) {
		return admission.Allowed("")
	}
	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
}

func isTerminatedBySidecarTerminator(pod *v1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == sidecarterminator.SidecarTerminated {
			return true
		}
	}
	return false
}

func injectPodIntoContainerRecreateRequest(obj *appsv1alpha1.ContainerRecreateRequest, pod *v1.Pod) error {
	obj.Labels[appsv1alpha1.ContainerRecreateRequestNodeNameKey] = pod.Spec.NodeName
	obj.Labels[appsv1alpha1.ContainerRecreateRequestPodUIDKey] = string(pod.UID)

	if obj.Spec.Strategy.TerminationGracePeriodSeconds == nil {
		obj.Spec.Strategy.TerminationGracePeriodSeconds = pod.Spec.TerminationGracePeriodSeconds
	}

	restartNames := sets.NewString()
	for i := range obj.Spec.Containers {
		c := &obj.Spec.Containers[i]
		if restartNames.Has(c.Name) {
			return fmt.Errorf("can not recreate %s multi times", c.Name)
		}

		if c.PreStop != nil || c.Ports != nil || c.StatusContext != nil {
			return fmt.Errorf("preStop, ports, statusContext in container are ready-only fields")
		}

		podContainer := util.GetContainer(c.Name, pod)
		if podContainer == nil {
			return fmt.Errorf("container %s not found in Pod", c.Name)
		}
		podContainerStatus := util.GetContainerStatus(c.Name, pod)
		if podContainerStatus == nil {
			return fmt.Errorf("not found %s containerStatus in Pod Status", c.Name)
		} else if podContainerStatus.ContainerID == "" {
			return fmt.Errorf("no containerID in %s containerStatus, maybe the container has not been initialized", c.Name)
		}

		if podContainer.Lifecycle != nil && podContainer.Lifecycle.PreStop != nil {
			c.PreStop = &appsv1alpha1.ProbeHandler{
				Exec:      podContainer.Lifecycle.PreStop.Exec,
				HTTPGet:   podContainer.Lifecycle.PreStop.HTTPGet,
				TCPSocket: podContainer.Lifecycle.PreStop.TCPSocket,
			}
		}

		if c.PreStop != nil && c.PreStop.HTTPGet != nil {
			c.Ports = podContainer.Ports
		}

		c.StatusContext = &appsv1alpha1.ContainerRecreateRequestContainerContext{
			ContainerID:  podContainerStatus.ContainerID,
			RestartCount: podContainerStatus.RestartCount,
		}
	}

	return nil
}
