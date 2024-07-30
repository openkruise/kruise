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
	preparingDeleteHookKey = "preDeleteHook"
	preparingUpdateHookKey = "preUpdateHook"
)

// Interface for managing pods lifecycle.
type Interface interface {
	UpdatePodLifecycle(pod *v1.Pod, state appspub.LifecycleStateType, markPodNotReady bool) (bool, *v1.Pod, error)
	UpdatePodLifecycleWithHandler(pod *v1.Pod, state appspub.LifecycleStateType, inPlaceUpdateHandler *appspub.LifecycleHook) (bool, *v1.Pod, error)
}

type realControl struct {
	adp                 podadapter.Adapter
	podReadinessControl podreadiness.Interface
}

func New(c client.Client) Interface {
	adp := &podadapter.AdapterRuntimeClient{Client: c}
	return &realControl{
		adp:                 adp,
		podReadinessControl: podreadiness.NewForAdapter(adp),
	}
}

func NewForTypedClient(c clientset.Interface) Interface {
	adp := &podadapter.AdapterTypedClient{Client: c}
	return &realControl{
		adp:                 adp,
		podReadinessControl: podreadiness.NewForAdapter(adp),
	}
}

func NewForInformer(informer coreinformers.PodInformer) Interface {
	adp := &podadapter.AdapterInformer{PodInformer: informer}
	return &realControl{
		adp:                 adp,
		podReadinessControl: podreadiness.NewForAdapter(adp),
	}
}

func GetPodLifecycleState(pod *v1.Pod) appspub.LifecycleStateType {
	return appspub.LifecycleStateType(pod.Labels[appspub.LifecycleStateKey])
}

func IsHookMarkPodNotReady(lifecycleHook *appspub.LifecycleHook) bool {
	if lifecycleHook == nil {
		return false
	}
	return lifecycleHook.MarkPodNotReady
}

func IsLifecycleMarkPodNotReady(lifecycle *appspub.Lifecycle) bool {
	if lifecycle == nil {
		return false
	}
	return IsHookMarkPodNotReady(lifecycle.PreDelete) || IsHookMarkPodNotReady(lifecycle.InPlaceUpdate)
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
	switch state {
	case appspub.LifecycleStatePreparingDelete:
		err = c.podReadinessControl.AddNotReadyKey(pod, getReadinessMessage(preparingDeleteHookKey))
	case appspub.LifecycleStatePreparingUpdate:
		err = c.podReadinessControl.AddNotReadyKey(pod, getReadinessMessage(preparingUpdateHookKey))
	case appspub.LifecycleStateUpdated:
		err = c.podReadinessControl.RemoveNotReadyKey(pod, getReadinessMessage(preparingUpdateHookKey))
	}

	if err != nil {
		klog.ErrorS(err, "Failed to set pod Ready/NotReady at lifecycle state",
			"pod", client.ObjectKeyFromObject(pod), "state", state)
	}
	return
}

func (c *realControl) UpdatePodLifecycle(pod *v1.Pod, state appspub.LifecycleStateType, markPodNotReady bool) (updated bool, gotPod *v1.Pod, err error) {
	if markPodNotReady {
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
	return podreadiness.Message{UserAgent: "Lifecycle", Key: key}
}
