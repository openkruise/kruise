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
	"net/http"

	"k8s.io/api/admission/v1beta1"
	"k8s.io/kubernetes/pkg/apis/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"

	appsv1alpha1 "github.com/openkruise/kruise/pkg/apis/apps/v1alpha1"
)

const (
	// SidecarSetHashAnnotation represents the key of a sidecarset hash
	SidecarSetHashAnnotation = "kruise.io/sidecarset-hash"
	// SidecarSetHashWithoutImageAnnotation represents the key of a sidecarset hash without images of sidecar
	SidecarSetHashWithoutImageAnnotation = "kruise.io/sidecarset-hash-without-image"
)

func init() {
	webhookName := "mutating-create-update-sidecarset"
	if HandlerMap[webhookName] == nil {
		HandlerMap[webhookName] = []admission.Handler{}
	}
	HandlerMap[webhookName] = append(HandlerMap[webhookName], &SidecarSetCreateHandler{})
}

// SidecarSetCreateHandler handles SidecarSet
type SidecarSetCreateHandler struct {
	// To use the client, you need to do the following:
	// - uncomment it
	// - import sigs.k8s.io/controller-runtime/pkg/client
	// - uncomment the InjectClient method at the bottom of this file.
	// Client  client.Client

	// Decoder decodes objects
	Decoder types.Decoder
}

func setDefaultSidecarSet(sidecarset *appsv1alpha1.SidecarSet) {
	for i := range sidecarset.Spec.Containers {
		setDefaultContainer(&sidecarset.Spec.Containers[i])
	}
}

func setDefaultContainer(sidecarContainer *appsv1alpha1.SidecarContainer) {
	container := &sidecarContainer.Container
	v1.SetDefaults_Container(container)
	for i := range container.Ports {
		p := &container.Ports[i]
		v1.SetDefaults_ContainerPort(p)
	}
	for i := range container.Env {
		e := &container.Env[i]
		if e.ValueFrom != nil {
			if e.ValueFrom.FieldRef != nil {
				v1.SetDefaults_ObjectFieldSelector(e.ValueFrom.FieldRef)
			}
		}
	}
	v1.SetDefaults_ResourceList(&container.Resources.Limits)
	v1.SetDefaults_ResourceList(&container.Resources.Requests)
	if container.LivenessProbe != nil {
		v1.SetDefaults_Probe(container.LivenessProbe)
		if container.LivenessProbe.Handler.HTTPGet != nil {
			v1.SetDefaults_HTTPGetAction(container.LivenessProbe.Handler.HTTPGet)
		}
	}
	if container.ReadinessProbe != nil {
		v1.SetDefaults_Probe(container.ReadinessProbe)
		if container.ReadinessProbe.Handler.HTTPGet != nil {
			v1.SetDefaults_HTTPGetAction(container.ReadinessProbe.Handler.HTTPGet)
		}
	}
	if container.Lifecycle != nil {
		if container.Lifecycle.PostStart != nil {
			if container.Lifecycle.PostStart.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(container.Lifecycle.PostStart.HTTPGet)
			}
		}
		if container.Lifecycle.PreStop != nil {
			if container.Lifecycle.PreStop.HTTPGet != nil {
				v1.SetDefaults_HTTPGetAction(container.Lifecycle.PreStop.HTTPGet)
			}
		}
	}
}

func setHashSidecarSet(sidecarset *appsv1alpha1.SidecarSet) error {
	if sidecarset.Annotations == nil {
		sidecarset.Annotations = make(map[string]string)
	}

	hash, err := SidecarSetHash(sidecarset)
	if err != nil {
		return err
	}
	sidecarset.Annotations[SidecarSetHashAnnotation] = hash

	hash, err = SidecarSetHashWithoutImage(sidecarset)
	if err != nil {
		return err
	}
	sidecarset.Annotations[SidecarSetHashWithoutImageAnnotation] = hash

	return nil
}

var _ admission.Handler = &SidecarSetCreateHandler{}

// Handle handles admission requests.
func (h *SidecarSetCreateHandler) Handle(ctx context.Context, req types.Request) types.Response {
	obj := &appsv1alpha1.SidecarSet{}

	err := h.Decoder.Decode(req, obj)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	copy := obj.DeepCopy()

	switch req.AdmissionRequest.Operation {
	case v1beta1.Create:
		setDefaultSidecarSet(copy)
		if err := setHashSidecarSet(copy); err != nil {
			return admission.ErrorResponse(http.StatusInternalServerError, err)
		}
	case v1beta1.Update:
		if err := setHashSidecarSet(copy); err != nil {
			return admission.ErrorResponse(http.StatusInternalServerError, err)
		}
	}

	return admission.PatchResponse(obj, copy)
}

//var _ inject.Client = &SidecarSetCreateHandler{}
//
//// InjectClient injects the client into the SidecarSetCreateHandler
//func (h *SidecarSetCreateHandler) InjectClient(c client.Client) error {
//	h.Client = c
//	return nil
//}

var _ inject.Decoder = &SidecarSetCreateHandler{}

// InjectDecoder injects the decoder into the SidecarSetCreateHandler
func (h *SidecarSetCreateHandler) InjectDecoder(d types.Decoder) error {
	h.Decoder = d
	return nil
}
