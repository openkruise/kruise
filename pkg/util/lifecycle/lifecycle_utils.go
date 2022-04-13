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
	"github.com/openkruise/kruise/pkg/util/podadapter"
	"github.com/openkruise/kruise/pkg/util/podreadiness"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// these keys for MarkPodNotReady Policy of pod lifecycle
	preparingDeleteKey = "preparingDelete"
	preparingUpdateKey = "preparingUpdate"
)

// Interface for managing pods lifecycle.
type Interface interface {
	UpdatePodLifecycle(pod *v1.Pod, state appspub.LifecycleStateType, requiredPodNotReady bool) (bool, *v1.Pod, error)
	UpdatePodLifecycleWithHandler(pod *v1.Pod, state appspub.LifecycleStateType, inPlaceUpdateHandler *appspub.LifecycleHook) (bool, *v1.Pod, error)
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

func GetPodLifecycleState(pod *v1.Pod) appspub.LifecycleStateType {
	return appspub.LifecycleStateType(pod.Labels[appspub.LifecycleStateKey])
}

func IsRequiredPodNotReady(lifecycle *appspub.Lifecycle) bool {
	if lifecycle == nil {
		return false
	}
	if lifecycle.PreDelete != nil && lifecycle.PreDelete.MarkPodNotReady {
		return true
	}
	if lifecycle.InPlaceUpdate != nil && lifecycle.InPlaceUpdate.MarkPodNotReady {
		return true
	}
	return false
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

func (c *realControl) executePodNotReadyPolicy(pod *v1.Pod, state appspub.LifecycleStateType) (err error) {
	defer func() {
		if err != nil {
			klog.Errorf("Failed to set pod(%v) Ready/NotReady at %s lifecycle state, error: %v",
				client.ObjectKeyFromObject(pod), state, err)
		}
	}()
	switch state {
	case appspub.LifecycleStatePreparingDelete:
		err = podreadiness.NewCommonForAdapter(c.adp).AddNotReadyKey(pod, getReadinessMessage(preparingDeleteKey))
	case appspub.LifecycleStatePreparingUpdate:
		err = podreadiness.NewCommonForAdapter(c.adp).AddNotReadyKey(pod, getReadinessMessage(preparingUpdateKey))
	case appspub.LifecycleStateUpdated:
		err = podreadiness.NewCommonForAdapter(c.adp).RemoveNotReadyKey(pod, getReadinessMessage(preparingUpdateKey))
	}

	return
}

func (c *realControl) UpdatePodLifecycle(pod *v1.Pod, state appspub.LifecycleStateType, requiredPodNotReady bool) (updated bool, gotPod *v1.Pod, err error) {
	if requiredPodNotReady {
		if err = c.executePodNotReadyPolicy(pod, state); err != nil {
			return false, nil, err
		}
	}

	if GetPodLifecycleState(pod) == state {
		return false, pod, nil
	}

	pod = pod.DeepCopy()
	if adp, ok := c.adp.(podadapter.AdapterWithPatch); ok {
		body := fmt.Sprintf(
			`{"metadata":{"labels":{"%s":"%s"},"annotations":{"%s":"%s"}}}`,
			appspub.LifecycleStateKey,
			string(state),
			appspub.LifecycleTimestampKey,
			time.Now().Format(time.RFC3339),
		)
		gotPod, err = adp.PatchPod(pod, client.RawPatch(types.StrategicMergePatchType, []byte(body)))
	} else {
		SetPodLifecycle(state)(pod)
		gotPod, err = c.adp.UpdatePod(pod)
	}

	return true, gotPod, err
}

func (c *realControl) UpdatePodLifecycleWithHandler(pod *v1.Pod, state appspub.LifecycleStateType, inPlaceUpdateHandler *appspub.LifecycleHook) (updated bool, gotPod *v1.Pod, err error) {
	if inPlaceUpdateHandler == nil || pod == nil {
		return false, pod, nil
	}

	if inPlaceUpdateHandler.MarkPodNotReady {
		if err = c.executePodNotReadyPolicy(pod, state); err != nil {
			return false, nil, err
		}
	}

	if GetPodLifecycleState(pod) == state {
		return false, pod, nil
	}

	pod = pod.DeepCopy()
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
		gotPod, err = adp.PatchPod(pod, client.RawPatch(types.StrategicMergePatchType, []byte(body)))
	} else {
		if pod.Labels == nil {
			pod.Labels = make(map[string]string)
		}
		for k, v := range inPlaceUpdateHandler.LabelsHandler {
			pod.Labels[k] = v
		}
		pod.Finalizers = append(pod.Finalizers, inPlaceUpdateHandler.FinalizersHandler...)

		SetPodLifecycle(state)(pod)
		gotPod, err = c.adp.UpdatePod(pod)
	}

	return true, gotPod, err
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

func getReadinessMessage(key string) podreadiness.Message {
	return podreadiness.Message{UserAgent: "ContainerRecreateRequest", Key: key}
}
