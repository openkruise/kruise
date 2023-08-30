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

package v1alpha1

import (
	"fmt"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func (src *DaemonSet) ConvertTo(dstRaw conversion.Hub) error {
	switch t := dstRaw.(type) {
	case *v1beta1.DaemonSet:
		dst := dstRaw.(*v1beta1.DaemonSet)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Template = src.Spec.Template
		dst.Spec.UpdateStrategy.Type = v1beta1.DaemonSetUpdateStrategyType(src.Spec.UpdateStrategy.Type)
		if src.Spec.UpdateStrategy.RollingUpdate != nil {
			dst.Spec.UpdateStrategy.RollingUpdate = &v1beta1.RollingUpdateDaemonSet{}
			dst.Spec.UpdateStrategy.RollingUpdate.Type = v1beta1.RollingUpdateType(src.Spec.UpdateStrategy.RollingUpdate.Type)
			dst.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = src.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable
			dst.Spec.UpdateStrategy.RollingUpdate.MaxSurge = src.Spec.UpdateStrategy.RollingUpdate.MaxSurge
			dst.Spec.UpdateStrategy.RollingUpdate.Selector = src.Spec.UpdateStrategy.RollingUpdate.Selector
			dst.Spec.UpdateStrategy.RollingUpdate.Partition = src.Spec.UpdateStrategy.RollingUpdate.Partition
			dst.Spec.UpdateStrategy.RollingUpdate.Paused = src.Spec.UpdateStrategy.RollingUpdate.Paused
		}
		dst.Spec.MinReadySeconds = src.Spec.MinReadySeconds
		dst.Spec.BurstReplicas = src.Spec.BurstReplicas
		dst.Spec.RevisionHistoryLimit = src.Spec.RevisionHistoryLimit
		dst.Spec.Lifecycle = src.Spec.Lifecycle
		dst.Status.CurrentNumberScheduled = src.Status.CurrentNumberScheduled
		dst.Status.NumberMisscheduled = src.Status.NumberMisscheduled
		dst.Status.DesiredNumberScheduled = src.Status.DesiredNumberScheduled
		dst.Status.NumberReady = src.Status.NumberReady
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		dst.Status.UpdatedNumberScheduled = src.Status.UpdatedNumberScheduled
		dst.Status.NumberAvailable = src.Status.NumberAvailable
		dst.Status.NumberUnavailable = src.Status.NumberUnavailable
		dst.Status.CollisionCount = src.Status.CollisionCount
		dst.Status.Conditions = src.Status.Conditions
		dst.Status.DaemonSetHash = src.Status.DaemonSetHash
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}

func (dst *DaemonSet) ConvertFrom(srcRaw conversion.Hub) error {
	switch t := srcRaw.(type) {
	case *v1beta1.DaemonSet:
		src := srcRaw.(*v1beta1.DaemonSet)
		dst.ObjectMeta = src.ObjectMeta
		dst.Spec.Selector = src.Spec.Selector
		dst.Spec.Template = src.Spec.Template
		dst.Spec.UpdateStrategy.Type = DaemonSetUpdateStrategyType(src.Spec.UpdateStrategy.Type)
		if src.Spec.UpdateStrategy.RollingUpdate != nil {
			dst.Spec.UpdateStrategy.RollingUpdate = &RollingUpdateDaemonSet{}
			dst.Spec.UpdateStrategy.RollingUpdate.Type = RollingUpdateType(src.Spec.UpdateStrategy.RollingUpdate.Type)
			dst.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable = src.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable
			dst.Spec.UpdateStrategy.RollingUpdate.MaxSurge = src.Spec.UpdateStrategy.RollingUpdate.MaxSurge
			dst.Spec.UpdateStrategy.RollingUpdate.Selector = src.Spec.UpdateStrategy.RollingUpdate.Selector
			dst.Spec.UpdateStrategy.RollingUpdate.Partition = src.Spec.UpdateStrategy.RollingUpdate.Partition
			dst.Spec.UpdateStrategy.RollingUpdate.Paused = src.Spec.UpdateStrategy.RollingUpdate.Paused
		}
		dst.Spec.MinReadySeconds = src.Spec.MinReadySeconds
		dst.Spec.BurstReplicas = src.Spec.BurstReplicas
		dst.Spec.RevisionHistoryLimit = src.Spec.RevisionHistoryLimit
		dst.Spec.Lifecycle = src.Spec.Lifecycle
		dst.Status.CurrentNumberScheduled = src.Status.CurrentNumberScheduled
		dst.Status.NumberMisscheduled = src.Status.NumberMisscheduled
		dst.Status.DesiredNumberScheduled = src.Status.DesiredNumberScheduled
		dst.Status.NumberReady = src.Status.NumberReady
		dst.Status.ObservedGeneration = src.Status.ObservedGeneration
		dst.Status.UpdatedNumberScheduled = src.Status.UpdatedNumberScheduled
		dst.Status.NumberAvailable = src.Status.NumberAvailable
		dst.Status.NumberUnavailable = src.Status.NumberUnavailable
		dst.Status.CollisionCount = src.Status.CollisionCount
		dst.Status.Conditions = src.Status.Conditions
		dst.Status.DaemonSetHash = src.Status.DaemonSetHash
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
	return nil
}
