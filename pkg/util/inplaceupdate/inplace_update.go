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

package inplaceupdate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/podadapter"
	"github.com/openkruise/kruise/pkg/util/revisionadapter"
)

var (
	containerImagePatchRexp     = regexp.MustCompile("^/spec/containers/([0-9]+)/image$")
	containerResourcesPatchRexp = regexp.MustCompile("^/spec/containers/([0-9]+)/resources/.*$")
	rfc6901Decoder              = strings.NewReplacer("~1", "/", "~0", "~")

	Clock clock.Clock = clock.RealClock{}
)

type RefreshResult struct {
	RefreshErr    error
	DelayDuration time.Duration
}

type UpdateResult struct {
	InPlaceUpdate      bool
	UpdateErr          error
	DelayDuration      time.Duration
	NewResourceVersion string
}

type UpdateOptions struct {
	IgnoreVolumeClaimTemplatesHashDiff bool

	GracePeriodSeconds int32
	AdditionalFuncs    []func(*v1.Pod)

	CalculateSpec                  func(oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) *UpdateSpec
	PatchSpecToPod                 func(pod *v1.Pod, spec *UpdateSpec, state *appspub.InPlaceUpdateState) (*v1.Pod, error)
	CheckPodUpdateCompleted        func(pod *v1.Pod) error
	CheckContainersUpdateCompleted func(pod *v1.Pod, state *appspub.InPlaceUpdateState) error
	GetRevision                    func(rev *apps.ControllerRevision) string
}

// Interface for managing pods in-place update.
type Interface interface {
	CanUpdateInPlace(oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) bool
	Update(pod *v1.Pod, oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) UpdateResult
	Refresh(pod *v1.Pod, opts *UpdateOptions) RefreshResult
}

// UpdateSpec records the images of containers which need to in-place update.
type UpdateSpec struct {
	Revision string `json:"revision"`

	ContainerImages       map[string]string                  `json:"containerImages,omitempty"`
	ContainerRefMetadata  map[string]metav1.ObjectMeta       `json:"containerRefMetadata,omitempty"`
	ContainerResources    map[string]v1.ResourceRequirements `json:"containerResources,omitempty"`
	MetaDataPatch         []byte                             `json:"metaDataPatch,omitempty"`
	UpdateEnvFromMetadata bool                               `json:"updateEnvFromMetadata,omitempty"`
	GraceSeconds          int32                              `json:"graceSeconds,omitempty"`

	OldTemplate *v1.PodTemplateSpec `json:"oldTemplate,omitempty"`
	NewTemplate *v1.PodTemplateSpec `json:"newTemplate,omitempty"`
}

type realControl struct {
	podAdapter      podadapter.Adapter
	revisionAdapter revisionadapter.Interface
}

func New(c client.Client, revisionAdapter revisionadapter.Interface) Interface {
	return &realControl{podAdapter: &podadapter.AdapterRuntimeClient{Client: c}, revisionAdapter: revisionAdapter}
}

func NewForTypedClient(c clientset.Interface, revisionAdapter revisionadapter.Interface) Interface {
	return &realControl{podAdapter: &podadapter.AdapterTypedClient{Client: c}, revisionAdapter: revisionAdapter}
}

func NewForInformer(informer coreinformers.PodInformer, revisionAdapter revisionadapter.Interface) Interface {
	return &realControl{podAdapter: &podadapter.AdapterInformer{PodInformer: informer}, revisionAdapter: revisionAdapter}
}

func (c *realControl) Refresh(pod *v1.Pod, opts *UpdateOptions) RefreshResult {
	opts = SetOptionsDefaults(opts)

	// check if it is in grace period
	if gracePeriod, _ := appspub.GetInPlaceUpdateGrace(pod); gracePeriod != "" {
		delayDuration, err := c.finishGracePeriod(pod, opts)
		if err != nil {
			return RefreshResult{RefreshErr: err}
		}
		return RefreshResult{DelayDuration: delayDuration}
	}

	if stateStr, ok := appspub.GetInPlaceUpdateState(pod); ok {
		state := appspub.InPlaceUpdateState{}
		if err := json.Unmarshal([]byte(stateStr), &state); err != nil {
			return RefreshResult{RefreshErr: err}
		}

		// check in-place updating has not completed yet
		if checkErr := opts.CheckContainersUpdateCompleted(pod, &state); checkErr != nil {
			klog.V(6).ErrorS(checkErr, "Check Pod in-place update not completed yet", "namespace", pod.Namespace, "name", pod.Name)
			return RefreshResult{}
		}

		// check if there are containers with lower-priority that have to in-place update in next batch
		if len(state.NextContainerImages) > 0 || len(state.NextContainerRefMetadata) > 0 || len(state.NextContainerResources) > 0 {

			// pre-check the previous updated containers
			if checkErr := doPreCheckBeforeNext(pod, state.PreCheckBeforeNext); checkErr != nil {
				klog.V(5).ErrorS(checkErr, "Pod in-place update pre-check not passed", "namespace", pod.Namespace, "name", pod.Name)
				return RefreshResult{}
			}

			// do update the next containers
			if updated, err := c.updateNextBatch(pod, opts); err != nil {
				return RefreshResult{RefreshErr: err}
			} else if updated {
				return RefreshResult{}
			}
		}
	}

	if !containsReadinessGate(pod) {
		return RefreshResult{}
	}

	newCondition := v1.PodCondition{
		Type:               appspub.InPlaceUpdateReady,
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(Clock.Now()),
	}
	err := c.updateCondition(pod, newCondition)
	return RefreshResult{RefreshErr: err}
}

func (c *realControl) updateCondition(pod *v1.Pod, condition v1.PodCondition) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone, err := c.podAdapter.GetPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		if hasEqualCondition(clone, &condition) {
			return nil
		}

		util.SetPodCondition(clone, condition)
		// We only update the ready condition to False, and let Kubelet update it to True
		if condition.Status == v1.ConditionFalse {
			util.SetPodReadyCondition(clone)
		}
		return c.podAdapter.UpdatePodStatus(clone)
	})
}

func (c *realControl) finishGracePeriod(pod *v1.Pod, opts *UpdateOptions) (time.Duration, error) {
	var delayDuration time.Duration
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone, err := c.podAdapter.GetPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		spec := UpdateSpec{}
		updateSpecJSON, ok := appspub.GetInPlaceUpdateGrace(clone)
		if !ok {
			return nil
		}
		if err := json.Unmarshal([]byte(updateSpecJSON), &spec); err != nil {
			return err
		}
		graceDuration := time.Second * time.Duration(spec.GraceSeconds)

		updateState := appspub.InPlaceUpdateState{}
		updateStateJSON, ok := appspub.GetInPlaceUpdateState(clone)
		if !ok {
			return fmt.Errorf("pod has %s but %s not found", appspub.InPlaceUpdateGraceKey, appspub.InPlaceUpdateStateKey)
		}
		if err := json.Unmarshal([]byte(updateStateJSON), &updateState); err != nil {
			return nil
		}

		if !c.revisionAdapter.EqualToRevisionHash("", clone, spec.Revision) {
			// If revision-hash has changed, just drop this GracePeriodSpec and go through the normal update process again.
			appspub.RemoveInPlaceUpdateGrace(clone)
		} else {
			if span := time.Since(updateState.UpdateTimestamp.Time); span < graceDuration {
				delayDuration = roundupSeconds(graceDuration - span)
				return nil
			}

			if clone, err = opts.PatchSpecToPod(clone, &spec, &updateState); err != nil {
				return err
			}
			appspub.RemoveInPlaceUpdateGrace(clone)
		}

		_, err = c.podAdapter.UpdatePod(clone)
		return err
	})

	return delayDuration, err
}

func (c *realControl) updateNextBatch(pod *v1.Pod, opts *UpdateOptions) (bool, error) {
	var updated bool
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		updated = false
		clone, err := c.podAdapter.GetPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		state := appspub.InPlaceUpdateState{}
		if stateStr, ok := appspub.GetInPlaceUpdateState(pod); !ok {
			return nil
		} else if err := json.Unmarshal([]byte(stateStr), &state); err != nil {
			return err
		}

		if len(state.NextContainerImages) == 0 && len(state.NextContainerRefMetadata) == 0 {
			return nil
		}

		spec := UpdateSpec{
			ContainerImages:       state.NextContainerImages,
			ContainerRefMetadata:  state.NextContainerRefMetadata,
			UpdateEnvFromMetadata: state.UpdateEnvFromMetadata,
			ContainerResources:    state.NextContainerResources,
		}
		if clone, err = opts.PatchSpecToPod(clone, &spec, &state); err != nil {
			return err
		}

		updated = true
		_, err = c.podAdapter.UpdatePod(clone)
		return err
	})
	return updated, err
}

func (c *realControl) CanUpdateInPlace(oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) bool {
	opts = SetOptionsDefaults(opts)
	return opts.CalculateSpec(oldRevision, newRevision, opts) != nil
}

func (c *realControl) Update(pod *v1.Pod, oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) UpdateResult {
	opts = SetOptionsDefaults(opts)

	// 1. calculate inplace update spec
	spec := opts.CalculateSpec(oldRevision, newRevision, opts)
	if spec == nil {
		return UpdateResult{}
	}

	// TODO(FillZpp): maybe we should check if the previous in-place update has completed

	// 2. update condition for pod with readiness-gate
	if containsReadinessGate(pod) {
		newCondition := v1.PodCondition{
			Type:               appspub.InPlaceUpdateReady,
			LastTransitionTime: metav1.NewTime(Clock.Now()),
			Status:             v1.ConditionFalse,
			Reason:             "StartInPlaceUpdate",
		}
		if err := c.updateCondition(pod, newCondition); err != nil {
			return UpdateResult{InPlaceUpdate: true, UpdateErr: err}
		}
	}

	// 3. update container images
	newResourceVersion, err := c.updatePodInPlace(pod, spec, opts)
	if err != nil {
		return UpdateResult{InPlaceUpdate: true, UpdateErr: err}
	}

	var delayDuration time.Duration
	if opts.GracePeriodSeconds > 0 {
		delayDuration = time.Second * time.Duration(opts.GracePeriodSeconds)
	}
	return UpdateResult{InPlaceUpdate: true, DelayDuration: delayDuration, NewResourceVersion: newResourceVersion}
}

func (c *realControl) updatePodInPlace(pod *v1.Pod, spec *UpdateSpec, opts *UpdateOptions) (string, error) {
	var newResourceVersion string
	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone, err := c.podAdapter.GetPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		// update new revision
		c.revisionAdapter.WriteRevisionHash(clone, spec.Revision)
		if clone.Annotations == nil {
			clone.Annotations = map[string]string{}
		}
		for _, f := range opts.AdditionalFuncs {
			f(clone)
		}

		inPlaceUpdateState := appspub.InPlaceUpdateState{
			Revision:              spec.Revision,
			UpdateTimestamp:       metav1.NewTime(Clock.Now()),
			UpdateEnvFromMetadata: spec.UpdateEnvFromMetadata,
		}
		inPlaceUpdateStateJSON, _ := json.Marshal(inPlaceUpdateState)
		clone.Annotations[appspub.InPlaceUpdateStateKey] = string(inPlaceUpdateStateJSON)
		delete(clone.Annotations, appspub.InPlaceUpdateStateKeyOld)

		if spec.GraceSeconds <= 0 {
			if clone, err = opts.PatchSpecToPod(clone, spec, &inPlaceUpdateState); err != nil {
				return err
			}
			appspub.RemoveInPlaceUpdateGrace(clone)
		} else {
			inPlaceUpdateSpecJSON, _ := json.Marshal(spec)
			clone.Annotations[appspub.InPlaceUpdateGraceKey] = string(inPlaceUpdateSpecJSON)
		}

		newPod, updateErr := c.podAdapter.UpdatePod(clone)
		if updateErr == nil {
			newResourceVersion = newPod.ResourceVersion
		}
		return updateErr
	})
	return newResourceVersion, retryErr
}

// GetTemplateFromRevision returns the pod template parsed from ControllerRevision.
func GetTemplateFromRevision(revision *apps.ControllerRevision) (*v1.PodTemplateSpec, error) {
	var patchObj *struct {
		Spec struct {
			Template v1.PodTemplateSpec `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(revision.Data.Raw, &patchObj); err != nil {
		return nil, err
	}
	return &patchObj.Spec.Template, nil
}

// InjectReadinessGate injects InPlaceUpdateReady into pod.spec.readinessGates
func InjectReadinessGate(pod *v1.Pod) {
	for _, r := range pod.Spec.ReadinessGates {
		if r.ConditionType == appspub.InPlaceUpdateReady {
			return
		}
	}
	pod.Spec.ReadinessGates = append(pod.Spec.ReadinessGates, v1.PodReadinessGate{ConditionType: appspub.InPlaceUpdateReady})
}

func containsReadinessGate(pod *v1.Pod) bool {
	for _, r := range pod.Spec.ReadinessGates {
		if r.ConditionType == appspub.InPlaceUpdateReady {
			return true
		}
	}
	return false
}

// GetCondition returns the InPlaceUpdateReady condition in Pod.
func GetCondition(pod *v1.Pod) *v1.PodCondition {
	return util.GetCondition(pod, appspub.InPlaceUpdateReady)
}

func roundupSeconds(d time.Duration) time.Duration {
	if d%time.Second == 0 {
		return d
	}
	return (d/time.Second + 1) * time.Second
}

func doPreCheckBeforeNext(pod *v1.Pod, preCheck *appspub.InPlaceUpdatePreCheckBeforeNext) error {
	if preCheck == nil {
		return nil
	}
	for _, cName := range preCheck.ContainersRequiredReady {
		cStatus := util.GetContainerStatus(cName, pod)
		if cStatus == nil {
			return fmt.Errorf("not found container %s in pod status", cName)
		}
		if !cStatus.Ready {
			return fmt.Errorf("waiting container %s to be ready", cName)
		}
	}
	return nil
}

func hasEqualCondition(pod *v1.Pod, newCondition *v1.PodCondition) bool {
	oldCondition := util.GetCondition(pod, newCondition.Type)
	isEqual := oldCondition != nil && oldCondition.Status == newCondition.Status &&
		oldCondition.Reason == newCondition.Reason && oldCondition.Message == newCondition.Message
	return isEqual
}
