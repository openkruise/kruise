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

package v1alpha1

import "k8s.io/apimachinery/pkg/runtime"

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

// SetDefaultsBroadcastJob sets any unspecified values to defaults.
func SetDefaultsBroadcastJob(job *BroadcastJob) {
	if job.Spec.CompletionPolicy.Type == "" {
		job.Spec.CompletionPolicy.Type = Always
	}

	if job.Spec.Parallelism == nil {
		parallelism := int32(1<<31 - 1)
		job.Spec.Parallelism = &parallelism
	}

}
