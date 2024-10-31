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

package features

import (
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/component-base/featuregate"

	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

const (
	// KruiseDaemon enables the features relied on kruise-daemon, such as image pulling and container restarting.
	KruiseDaemon featuregate.Feature = "KruiseDaemon"

	// PodWebhook enables webhook for Pods creations. This is also related to SidecarSet.
	PodWebhook featuregate.Feature = "PodWebhook"

	// CloneSetShortHash enables CloneSet controller only set revision hash name to pod label.
	CloneSetShortHash featuregate.Feature = "CloneSetShortHash"

	// KruisePodReadinessGate enables Kruise webhook to inject 'KruisePodReady' readiness-gate to
	// all Pods during creation.
	// Otherwise, it will only be injected to Pods created by Kruise workloads.
	KruisePodReadinessGate featuregate.Feature = "KruisePodReadinessGate"

	// PreDownloadImageForInPlaceUpdate enables cloneset/statefulset controllers to create ImagePullJobs to
	// pre-download images for in-place update.
	PreDownloadImageForInPlaceUpdate featuregate.Feature = "PreDownloadImageForInPlaceUpdate"

	// CloneSetPartitionRollback enables CloneSet controller to rollback Pods to currentRevision
	// when number of updateRevision pods is bigger than (replicas - partition).
	CloneSetPartitionRollback featuregate.Feature = "CloneSetPartitionRollback"

	// ResourcesDeletionProtection enables protection for resources deletion, currently supports
	// Namespace, Service, Ingress, CustomResourcesDefinition, Deployment, StatefulSet, ReplicaSet, CloneSet, Advanced StatefulSet, UnitedDeployment.
	// It is only supported for Kubernetes version >= 1.16
	// Note that if it is enabled during Kruise installation or upgrade, Kruise will require more authorities:
	// 1. Webhook for deletion operation of namespace, service, ingress, crd, deployment, statefulset, replicaset and workloads in Kruise.
	ResourcesDeletionProtection featuregate.Feature = "ResourcesDeletionProtection"

	// PodUnavailableBudgetDeleteGate enables PUB capability to protect pod from deletion and eviction
	PodUnavailableBudgetDeleteGate featuregate.Feature = "PodUnavailableBudgetDeleteGate"

	// PodUnavailableBudgetUpdateGate enables PUB capability to protect pod from in-place update
	PodUnavailableBudgetUpdateGate featuregate.Feature = "PodUnavailableBudgetUpdateGate"

	// WorkloadSpread enable WorkloadSpread to constrain the spread of the workload.
	WorkloadSpread featuregate.Feature = "WorkloadSpread"

	// DaemonWatchingPod enables kruise-daemon to list watch pods that belong to the same node.
	DaemonWatchingPod featuregate.Feature = "DaemonWatchingPod"

	// TemplateNoDefaults to control whether webhook should inject pod default fields into pod template
	// and pvc default fields into pvc template.
	// If TemplateNoDefaults is false, webhook should inject default fields only when the template changed.
	TemplateNoDefaults featuregate.Feature = "TemplateNoDefaults"

	// InPlaceUpdateEnvFromMetadata enables Kruise to in-place update a container in Pod
	// when its env from labels/annotations changed and pod is in-place updating.
	InPlaceUpdateEnvFromMetadata featuregate.Feature = "InPlaceUpdateEnvFromMetadata"

	// Enables policies controlling deletion of PVCs created by a StatefulSet.
	StatefulSetAutoDeletePVC featuregate.Feature = "StatefulSetAutoDeletePVC"

	// SidecarSetPatchPodMetadataDefaultsAllowed whether sidecarSet patch pod metadata is allowed
	SidecarSetPatchPodMetadataDefaultsAllowed featuregate.Feature = "SidecarSetPatchPodMetadataDefaultsAllowed"

	// SidecarTerminator enables SidecarTerminator to stop sidecar containers when all main containers exited.
	// SidecarTerminator only works for the Pods with 'Never' or 'OnFailure' restartPolicy.
	SidecarTerminator featuregate.Feature = "SidecarTerminator"

	// PodProbeMarkerGate enable Kruise provide the ability to execute custom Probes.
	// Note: custom probe execution requires kruise daemon, so currently only traditional Kubelet is supported, not virtual-kubelet.
	PodProbeMarkerGate featuregate.Feature = "PodProbeMarkerGate"

	// PreDownloadImageForDaemonSetUpdate enables daemonset-controller to create ImagePullJobs to
	// pre-download images for update.
	PreDownloadImageForDaemonSetUpdate featuregate.Feature = "PreDownloadImageForDaemonSetUpdate"

	// CloneSetEventHandlerOptimization enable optimization for cloneset-controller to reduce the
	// queuing frequency cased by pod update.
	CloneSetEventHandlerOptimization featuregate.Feature = "CloneSetEventHandlerOptimization"

	// PreparingUpdateAsUpdate enable CloneSet/Advanced StatefulSet controller to regard preparing-update Pod
	// as updated when calculating update/current revision during scaling.
	PreparingUpdateAsUpdate featuregate.Feature = "PreparingUpdateAsUpdate"

	// ImagePullJobGate enable imagepulljob-controller execute ImagePullJob.
	ImagePullJobGate featuregate.Feature = "ImagePullJobGate"

	// ResourceDistributionGate enable resourcedistribution-controller execute ResourceDistribution.
	ResourceDistributionGate featuregate.Feature = "ResourceDistributionGate"

	// DeletionProtectionForCRDCascadingGate enable deletionProtection for crd Cascading
	DeletionProtectionForCRDCascadingGate featuregate.Feature = "DeletionProtectionForCRDCascadingGate"

	// Enables a enhanced livenessProbe solution
	EnhancedLivenessProbeGate featuregate.Feature = "EnhancedLivenessProbe"

	// RecreatePodWhenChangeVCTInCloneSetGate recreate the pod upon changing volume claim templates in a clone set to ensure PVC consistency.
	RecreatePodWhenChangeVCTInCloneSetGate featuregate.Feature = "RecreatePodWhenChangeVCTInCloneSetGate"

	// Enables a StatefulSet to start from an arbitrary non zero ordinal
	StatefulSetStartOrdinal featuregate.Feature = "StatefulSetStartOrdinal"

	// Set pod completion index as a pod label for Indexed Jobs.
	PodIndexLabel featuregate.Feature = "PodIndexLabel"

	// Use certs generated externally
	EnableExternalCerts featuregate.Feature = "EnableExternalCerts"

	// Enables policies auto resizing PVCs created by a StatefulSet when user expands volumeClaimTemplates.
	StatefulSetAutoResizePVCGate featuregate.Feature = "StatefulSetAutoResizePVCGate"

	// InPlaceWorkloadVerticalScaling enable CloneSet/Advanced StatefulSet controller to support vertical scaling
	// of managed Pods.
	InPlaceWorkloadVerticalScaling featuregate.Feature = "InPlaceWorkloadVerticalScaling"
)

var defaultFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	PodWebhook:        {Default: true, PreRelease: featuregate.Beta},
	KruiseDaemon:      {Default: true, PreRelease: featuregate.Beta},
	DaemonWatchingPod: {Default: true, PreRelease: featuregate.Beta},

	CloneSetShortHash:                         {Default: false, PreRelease: featuregate.Alpha},
	KruisePodReadinessGate:                    {Default: false, PreRelease: featuregate.Alpha},
	PreDownloadImageForInPlaceUpdate:          {Default: false, PreRelease: featuregate.Alpha},
	CloneSetPartitionRollback:                 {Default: false, PreRelease: featuregate.Alpha},
	ResourcesDeletionProtection:               {Default: true, PreRelease: featuregate.Alpha},
	WorkloadSpread:                            {Default: true, PreRelease: featuregate.Alpha},
	PodUnavailableBudgetDeleteGate:            {Default: true, PreRelease: featuregate.Alpha},
	PodUnavailableBudgetUpdateGate:            {Default: false, PreRelease: featuregate.Alpha},
	TemplateNoDefaults:                        {Default: false, PreRelease: featuregate.Alpha},
	InPlaceUpdateEnvFromMetadata:              {Default: true, PreRelease: featuregate.Alpha},
	StatefulSetAutoDeletePVC:                  {Default: true, PreRelease: featuregate.Alpha},
	SidecarSetPatchPodMetadataDefaultsAllowed: {Default: false, PreRelease: featuregate.Alpha},
	SidecarTerminator:                         {Default: false, PreRelease: featuregate.Alpha},
	PodProbeMarkerGate:                        {Default: true, PreRelease: featuregate.Alpha},
	PreDownloadImageForDaemonSetUpdate:        {Default: false, PreRelease: featuregate.Alpha},

	CloneSetEventHandlerOptimization:      {Default: false, PreRelease: featuregate.Alpha},
	PreparingUpdateAsUpdate:               {Default: false, PreRelease: featuregate.Alpha},
	ImagePullJobGate:                      {Default: false, PreRelease: featuregate.Alpha},
	ResourceDistributionGate:              {Default: false, PreRelease: featuregate.Alpha},
	DeletionProtectionForCRDCascadingGate: {Default: false, PreRelease: featuregate.Alpha},

	EnhancedLivenessProbeGate:              {Default: false, PreRelease: featuregate.Alpha},
	RecreatePodWhenChangeVCTInCloneSetGate: {Default: false, PreRelease: featuregate.Alpha},
	StatefulSetStartOrdinal:                {Default: false, PreRelease: featuregate.Alpha},
	PodIndexLabel:                          {Default: true, PreRelease: featuregate.Beta},
	EnableExternalCerts:                    {Default: false, PreRelease: featuregate.Alpha},
	StatefulSetAutoResizePVCGate:           {Default: false, PreRelease: featuregate.Alpha},
	InPlaceWorkloadVerticalScaling:         {Default: false, PreRelease: featuregate.Alpha},
}

func init() {
	compatibleEnv()
	runtime.Must(utilfeature.DefaultMutableFeatureGate.Add(defaultFeatureGates))
}

// Make it compatible with the old CUSTOM_RESOURCE_ENABLE gate in env.
func compatibleEnv() {
	str := strings.TrimSpace(os.Getenv("CUSTOM_RESOURCE_ENABLE"))
	if len(str) == 0 {
		return
	}
	limits := sets.NewString(strings.Split(str, ",")...)
	if !limits.Has("SidecarSet") {
		defaultFeatureGates[PodWebhook] = featuregate.FeatureSpec{Default: false, PreRelease: featuregate.Beta}
	}
}

func SetDefaultFeatureGates() {
	if !utilfeature.DefaultFeatureGate.Enabled(PodWebhook) {
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", KruisePodReadinessGate))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", PodUnavailableBudgetDeleteGate))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", PodUnavailableBudgetUpdateGate))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", WorkloadSpread))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", SidecarSetPatchPodMetadataDefaultsAllowed))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", EnhancedLivenessProbeGate))
	}
	if !utilfeature.DefaultFeatureGate.Enabled(KruiseDaemon) {
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", PreDownloadImageForInPlaceUpdate))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", DaemonWatchingPod))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", InPlaceUpdateEnvFromMetadata))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", PreDownloadImageForDaemonSetUpdate))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", PodProbeMarkerGate))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", SidecarTerminator))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", ImagePullJobGate))
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", EnhancedLivenessProbeGate))
	}
	if utilfeature.DefaultFeatureGate.Enabled(PreDownloadImageForInPlaceUpdate) || utilfeature.DefaultFeatureGate.Enabled(PreDownloadImageForDaemonSetUpdate) {
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", ImagePullJobGate))
	}
	if !utilfeature.DefaultFeatureGate.Enabled(ResourcesDeletionProtection) {
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", DeletionProtectionForCRDCascadingGate))
	}
}
