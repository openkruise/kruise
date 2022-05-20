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

package pub

const (
	LifecycleStateKey     = "lifecycle.apps.kruise.io/state"
	LifecycleTimestampKey = "lifecycle.apps.kruise.io/timestamp"

	LifecycleStateNormal          LifecycleStateType = "Normal"
	LifecycleStatePreparingUpdate LifecycleStateType = "PreparingUpdate"
	LifecycleStateUpdating        LifecycleStateType = "Updating"
	LifecycleStateUpdated         LifecycleStateType = "Updated"
	LifecycleStatePreparingDelete LifecycleStateType = "PreparingDelete"
)

type LifecycleStateType string

// Lifecycle contains the hooks for Pod lifecycle.
type Lifecycle struct {
	// PreDelete is the hook before Pod to be deleted.
	PreDelete *LifecycleHook `json:"preDelete,omitempty"`
	// InPlaceUpdate is the hook before Pod to update and after Pod has been updated.
	InPlaceUpdate *LifecycleHook `json:"inPlaceUpdate,omitempty"`
}

type LifecycleHook struct {
	LabelsHandler     map[string]string `json:"labelsHandler,omitempty"`
	FinalizersHandler []string          `json:"finalizersHandler,omitempty"`
	// MarkPodNotReady = true means:
	// - Pod will be set to 'NotReady' at preparingDelete/preparingUpdate state.
	// - Pod will be restored to 'Ready' at Updated state if it was set to 'NotReady' at preparingUpdate state.
	// Default to false.
	MarkPodNotReady bool `json:"markPodNotReady,omitempty"`
}
