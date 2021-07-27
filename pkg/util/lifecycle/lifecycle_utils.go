/*
Copyright 2020 The Kruise Authors.

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

package lifecycle

import (
	"fmt"
	"strings"
	"time"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openkruise/kruise/pkg/util/podadapter"
)

// Interface for managing pods lifecycle.
type Interface interface {
	UpdatePodLifecycle(pod *v1.Pod, state appspub.LifecycleStateType) (bool, error)
	UpdatePodLifecycleWithHandler(pod *v1.Pod, state appspub.LifecycleStateType, inPlaceUpdateHandler *appspub.LifecycleHook) (bool, error)
}

type realControl struct {
	adp podadapter.Adapter
}

func New(c client.Client) Interface {
	return &realControl{adp: &podadapter.AdapterRuntimeClient{Client: c}}
}

func NewForTypedClient(c clientset.Interface) Interface {
	return &realControl{adp: &podadapter.AdapterTypedClient{Client: c}}
}

func NewForInformer(informer coreinformers.PodInformer) Interface {
	return &realControl{adp: &podadapter.AdapterInformer{PodInformer: informer}}
}

func NewForTest(c client.Client) Interface {
	return &realControl{adp: &podadapter.AdapterRuntimeClient{Client: c}}
}

func GetPodLifecycleState(pod *v1.Pod) appspub.LifecycleStateType {
	return appspub.LifecycleStateType(pod.Labels[appspub.LifecycleStateKey])
}

func SetPodLifecycle(state appspub.LifecycleStateType) func(*v1.Pod) {
	return func(pod *v1.Pod) {
		if pod.Labels == nil {
			pod.Labels = make(map[string]string)
		}
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Labels[appspub.LifecycleStateKey] = string(state)
		pod.Annotations[appspub.LifecycleTimestampKey] = time.Now().Format(time.RFC3339)
	}
}

func (c *realControl) UpdatePodLifecycle(pod *v1.Pod, state appspub.LifecycleStateType) (bool, error) {
	if GetPodLifecycleState(pod) == state {
		return false, nil
	}

	var err error
	if adp, ok := c.adp.(podadapter.AdapterWithPatch); ok {
		body := fmt.Sprintf(
			`{"metadata":{"labels":{"%s":"%s"},"annotations":{"%s":"%s"}}}`,
			appspub.LifecycleStateKey,
			string(state),
			appspub.LifecycleTimestampKey,
			time.Now().Format(time.RFC3339),
		)
		err = adp.PatchPod(pod, client.RawPatch(types.StrategicMergePatchType, []byte(body)))
	} else {
		SetPodLifecycle(state)(pod)
		err = c.adp.UpdatePod(pod)
	}

	return true, err
}

func (c *realControl) UpdatePodLifecycleWithHandler(pod *v1.Pod, state appspub.LifecycleStateType, inPlaceUpdateHandler *appspub.LifecycleHook) (bool, error) {
	if inPlaceUpdateHandler == nil || pod == nil {
		return false, nil
	}

	if GetPodLifecycleState(pod) == state {
		return false, nil
	}

	var err error
	if adp, ok := c.adp.(podadapter.AdapterWithPatch); ok {
		var labelsHandler, finalizersHandler string
		for k, v := range inPlaceUpdateHandler.LabelsHandler {
			labelsHandler = fmt.Sprintf(`%s,"%s":"%s"`, labelsHandler, k, v)
		}
		for _, v := range inPlaceUpdateHandler.FinalizersHandler {
			finalizersHandler = fmt.Sprintf(`%s,"%s"`, finalizersHandler, v)
		}
		finalizersHandler = fmt.Sprintf(`[%s]`, strings.TrimLeft(finalizersHandler, ","))

		body := fmt.Sprintf(
			`{"metadata":{"labels":{"%s":"%s"%s},"annotations":{"%s":"%s"},"finalizers":%s}}`,
			appspub.LifecycleStateKey,
			string(state),
			labelsHandler,
			appspub.LifecycleTimestampKey,
			time.Now().Format(time.RFC3339),
			finalizersHandler,
		)
		err = adp.PatchPod(pod, client.RawPatch(types.StrategicMergePatchType, []byte(body)))
	} else {
		if pod.Labels == nil {
			pod.Labels = make(map[string]string)
		}
		for k, v := range inPlaceUpdateHandler.LabelsHandler {
			pod.Labels[k] = v
		}
		pod.Finalizers = append(pod.Finalizers, inPlaceUpdateHandler.FinalizersHandler...)

		SetPodLifecycle(state)(pod)
		err = c.adp.UpdatePod(pod)
	}

	return true, err
}

func IsPodHooked(hook *appspub.LifecycleHook, pod *v1.Pod) bool {
	if hook == nil || pod == nil {
		return false
	}
	for _, f := range hook.FinalizersHandler {
		if controllerutil.ContainsFinalizer(pod, f) {
			return true
		}
	}
	for k, v := range hook.LabelsHandler {
		if pod.Labels[k] == v {
			return true
		}
	}
	return false
}

func IsPodAllHooked(hook *appspub.LifecycleHook, pod *v1.Pod) bool {
	if hook == nil || pod == nil {
		return false
	}
	for _, f := range hook.FinalizersHandler {
		if !controllerutil.ContainsFinalizer(pod, f) {
			return false
		}
	}
	for k, v := range hook.LabelsHandler {
		if pod.Labels[k] != v {
			return false
		}
	}
	return true
}
