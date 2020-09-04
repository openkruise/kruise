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
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func GetPodLifecycleState(pod *v1.Pod) appsv1alpha1.LifecycleStateType {
	return appsv1alpha1.LifecycleStateType(pod.Labels[appsv1alpha1.LifecycleStateKey])
}

func SetPodLifecycle(state appsv1alpha1.LifecycleStateType) func(*v1.Pod) {
	return func(pod *v1.Pod) {
		if pod.Labels == nil {
			pod.Labels = make(map[string]string)
		}
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Labels[appsv1alpha1.LifecycleStateKey] = string(state)
		pod.Annotations[appsv1alpha1.LifecycleTimestampKey] = time.Now().Format(time.RFC3339)
	}
}

func PatchPodLifecycle(c client.Client, pod *v1.Pod, state appsv1alpha1.LifecycleStateType) (bool, error) {
	if GetPodLifecycleState(pod) == state {
		return false, nil
	}

	body := fmt.Sprintf(
		`{"metadata":{"labels":{"%s":"%s"},"annotations":{"%s":"%s"}}}`,
		appsv1alpha1.LifecycleStateKey,
		string(state),
		appsv1alpha1.LifecycleTimestampKey,
		time.Now().Format(time.RFC3339),
	)
	return true, c.Patch(nil, pod, client.RawPatch(types.StrategicMergePatchType, []byte(body)))
}

func IsPodHooked(hook *appsv1alpha1.LifecycleHook, pod *v1.Pod) bool {
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
