/*
Copyright 2025 The Kruise Authors.

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

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/openkruise/kruise/apis/apps/v1beta1"
)

const (
	// ProgressiveCreatePodAnnotation indicates daemon pods created in manage phase will be controlled by partition.
	ProgressiveCreatePodAnnotation = "daemonset.kruise.io/progressive-create-pod"
)

func (ds *DaemonSet) ConvertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.DaemonSet:
		dsv1beta1 := dst.(*v1beta1.DaemonSet)
		dsv1beta1.ObjectMeta = ds.ObjectMeta

		// spec
		dsv1beta1.Spec = v1beta1.DaemonSetSpec{
			Selector:        ds.Spec.Selector,
			Template:        ds.Spec.Template,
			MinReadySeconds: ds.Spec.MinReadySeconds,
			UpdateStrategy: v1beta1.DaemonSetUpdateStrategy{
				Type: v1beta1.DaemonSetUpdateStrategyType(ds.Spec.UpdateStrategy.Type),
			},
			BurstReplicas:        ds.Spec.BurstReplicas,
			RevisionHistoryLimit: ds.Spec.RevisionHistoryLimit,
			Lifecycle:            ds.Spec.Lifecycle,
		}

		// Convert RollingUpdate strategy, filtering out deprecated Surging type
		if ds.Spec.UpdateStrategy.RollingUpdate != nil {
			rollingUpdateType := ds.Spec.UpdateStrategy.RollingUpdate.Type
			// Map deprecated Surging to Standard
			if rollingUpdateType == DeprecatedSurgingRollingUpdateType {
				rollingUpdateType = StandardRollingUpdateType
			}

			dsv1beta1.Spec.UpdateStrategy.RollingUpdate = &v1beta1.RollingUpdateDaemonSet{
				Type:           v1beta1.RollingUpdateType(rollingUpdateType),
				MaxUnavailable: ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable,
				MaxSurge:       ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge,
				Selector:       ds.Spec.UpdateStrategy.RollingUpdate.Selector,
				Partition:      ds.Spec.UpdateStrategy.RollingUpdate.Partition,
				Paused:         ds.Spec.UpdateStrategy.RollingUpdate.Paused,
			}
		}

		// Convert annotation daemonset.kruise.io/progressive-create-pod to scaleStrategy.partitionedScaling
		if ds.Annotations[ProgressiveCreatePodAnnotation] == "true" {
			dsv1beta1.Spec.ScaleStrategy = &v1beta1.DaemonSetScaleStrategy{
				PartitionedScaling: true,
			}
		}

		// status
		dsv1beta1.Status = v1beta1.DaemonSetStatus{
			CurrentNumberScheduled: ds.Status.CurrentNumberScheduled,
			NumberMisscheduled:     ds.Status.NumberMisscheduled,
			DesiredNumberScheduled: ds.Status.DesiredNumberScheduled,
			NumberReady:            ds.Status.NumberReady,
			ObservedGeneration:     ds.Status.ObservedGeneration,
			UpdatedNumberScheduled: ds.Status.UpdatedNumberScheduled,
			NumberAvailable:        ds.Status.NumberAvailable,
			NumberUnavailable:      ds.Status.NumberUnavailable,
			CollisionCount:         ds.Status.CollisionCount,
			Conditions:             ds.Status.Conditions,
			// Map DaemonSetHash to UpdateRevision for backward compatibility
			UpdateRevision: ds.Status.DaemonSetHash,
		}

		return nil

	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

func (ds *DaemonSet) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.DaemonSet:
		dsv1beta1 := src.(*v1beta1.DaemonSet)
		ds.ObjectMeta = dsv1beta1.ObjectMeta

		// spec
		ds.Spec = DaemonSetSpec{
			Selector:        dsv1beta1.Spec.Selector,
			Template:        dsv1beta1.Spec.Template,
			MinReadySeconds: dsv1beta1.Spec.MinReadySeconds,
			UpdateStrategy: DaemonSetUpdateStrategy{
				Type: DaemonSetUpdateStrategyType(dsv1beta1.Spec.UpdateStrategy.Type),
			},
			BurstReplicas:        dsv1beta1.Spec.BurstReplicas,
			RevisionHistoryLimit: dsv1beta1.Spec.RevisionHistoryLimit,
			Lifecycle:            dsv1beta1.Spec.Lifecycle,
		}

		if dsv1beta1.Spec.UpdateStrategy.RollingUpdate != nil {
			ds.Spec.UpdateStrategy.RollingUpdate = &RollingUpdateDaemonSet{
				Type:           RollingUpdateType(dsv1beta1.Spec.UpdateStrategy.RollingUpdate.Type),
				MaxUnavailable: dsv1beta1.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable,
				MaxSurge:       dsv1beta1.Spec.UpdateStrategy.RollingUpdate.MaxSurge,
				Selector:       dsv1beta1.Spec.UpdateStrategy.RollingUpdate.Selector,
				Partition:      dsv1beta1.Spec.UpdateStrategy.RollingUpdate.Partition,
				Paused:         dsv1beta1.Spec.UpdateStrategy.RollingUpdate.Paused,
			}
		}

		// Convert scaleStrategy.partitionedScaling to annotation daemonset.kruise.io/progressive-create-pod
		if dsv1beta1.Spec.ScaleStrategy != nil && dsv1beta1.Spec.ScaleStrategy.PartitionedScaling {
			if ds.Annotations == nil {
				ds.Annotations = make(map[string]string)
			}
			ds.Annotations[ProgressiveCreatePodAnnotation] = "true"
		}

		// status
		ds.Status = DaemonSetStatus{
			CurrentNumberScheduled: dsv1beta1.Status.CurrentNumberScheduled,
			NumberMisscheduled:     dsv1beta1.Status.NumberMisscheduled,
			DesiredNumberScheduled: dsv1beta1.Status.DesiredNumberScheduled,
			NumberReady:            dsv1beta1.Status.NumberReady,
			ObservedGeneration:     dsv1beta1.Status.ObservedGeneration,
			UpdatedNumberScheduled: dsv1beta1.Status.UpdatedNumberScheduled,
			NumberAvailable:        dsv1beta1.Status.NumberAvailable,
			NumberUnavailable:      dsv1beta1.Status.NumberUnavailable,
			CollisionCount:         dsv1beta1.Status.CollisionCount,
			Conditions:             dsv1beta1.Status.Conditions,
			// Map UpdateRevision to DaemonSetHash for backward compatibility
			DaemonSetHash: dsv1beta1.Status.UpdateRevision,
		}

		return nil
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}
