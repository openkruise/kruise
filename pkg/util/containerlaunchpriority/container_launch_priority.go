/*
Copyright 2022 The Kruise Authors.

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

package containerlaunchpriority

import (
	"fmt"
	"strconv"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	v1 "k8s.io/api/core/v1"
)

const (
	// priority string parse starting index
	priorityStartIndex = 2
)

func GetContainerPriority(c *v1.Container) *int {
	for _, e := range c.Env {
		if e.Name == appspub.ContainerLaunchBarrierEnvName {
			p, _ := strconv.Atoi(e.ValueFrom.ConfigMapKeyRef.Key[priorityStartIndex:])
			return &p
		}
	}
	return nil
}

func GeneratePriorityEnv(priority int, podName string) v1.EnvVar {
	return v1.EnvVar{
		Name: appspub.ContainerLaunchBarrierEnvName,
		ValueFrom: &v1.EnvVarSource{
			ConfigMapKeyRef: &v1.ConfigMapKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: podName + "-barrier"},
				Key:                  GetKey(priority),
			},
		},
	}
}

func GetKey(priority int) string {
	return fmt.Sprintf("p_%d", priority)
}
