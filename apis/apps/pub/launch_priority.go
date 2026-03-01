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

package pub

const (
	// ContainerLaunchPriorityEnvName is the env name that users have to define in pod container
	// to identity the launch priority of this container.
	ContainerLaunchPriorityEnvName = "KRUISE_CONTAINER_PRIORITY"
	// ContainerLaunchBarrierEnvName is the env name that Kruise webhook will inject into containers
	// if the pod have configured launch priority.
	ContainerLaunchBarrierEnvName = "KRUISE_CONTAINER_BARRIER"

	// ContainerLaunchPriorityKey is the annotation key that users could define in pod annotation
	// to make containers in pod launched by ordinal.
	ContainerLaunchPriorityKey = "apps.kruise.io/container-launch-priority"
	// ContainerLaunchOrdered is the annotation value that indicates containers in pod should be launched by ordinal.
	ContainerLaunchOrdered = "Ordered"

	// ContainerLaunchPriorityCompletedKey is the annotation indicates the pod has all its priorities
	// patched into its barrier configmap.
	ContainerLaunchPriorityCompletedKey = "apps.kruise.io/container-launch-priority-completed"
)
