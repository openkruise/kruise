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
	"strconv"
	"strings"
	"time"

	"github.com/appscode/jsonpatch"
	appspub "github.com/openkruise/kruise/apis/apps/pub"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var inPlaceUpdatePatchRexp = regexp.MustCompile("^/spec/containers/([0-9]+)/image$")

type (
	CustomizeSpecCalculateFunc        func(oldRevision, newRevision *apps.ControllerRevision) *UpdateSpec
	CustomizeSpecPatchFunc            func(pod *v1.Pod, spec *UpdateSpec) (*v1.Pod, error)
	CustomizeCheckUpdateCompletedFunc func(pod *v1.Pod) error
	GetRevisionFunc                   func(rev *apps.ControllerRevision) string
)

type UpdateOptions struct {
	GracePeriodSeconds int32
	AdditionalFuncs    []func(*v1.Pod)

	CustomizeSpecCalculate        CustomizeSpecCalculateFunc
	CustomizeSpecPatch            CustomizeSpecPatchFunc
	CustomizeCheckUpdateCompleted CustomizeCheckUpdateCompletedFunc
	GetRevision                   GetRevisionFunc
}

type RefreshResult struct {
	RefreshErr    error
	DelayDuration time.Duration
}

type UpdateResult struct {
	InPlaceUpdate bool
	UpdateErr     error
	DelayDuration time.Duration
}

// Interface for managing pods in-place update.
type Interface interface {
	Refresh(pod *v1.Pod, opts *UpdateOptions) RefreshResult
	CanUpdateInPlace(oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) bool
	Update(pod *v1.Pod, oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) UpdateResult
}

// UpdateSpec records the images of containers which need to in-place update.
type UpdateSpec struct {
	Revision    string            `json:"revision"`
	Annotations map[string]string `json:"annotations,omitempty"`

	ContainerImages map[string]string `json:"containerImages,omitempty"`
	MetaDataPatch   []byte            `json:"metaDataPatch,omitempty"`
	GraceSeconds    int32             `json:"graceSeconds,omitempty"`

	OldTemplate *v1.PodTemplateSpec `json:"oldTemplate,omitempty"`
	NewTemplate *v1.PodTemplateSpec `json:"newTemplate,omitempty"`
}

type realControl struct {
	adp         adapter
	revisionKey string

	// just for test
	now func() metav1.Time
}

func New(c client.Client, revisionKey string) Interface {
	return &realControl{adp: &adapterRuntimeClient{Client: c}, revisionKey: revisionKey, now: metav1.Now}
}

func NewForTypedClient(c clientset.Interface, revisionKey string) Interface {
	return &realControl{adp: &adapterTypedClient{client: c}, revisionKey: revisionKey, now: metav1.Now}
}

func NewForInformer(informer coreinformers.PodInformer, revisionKey string) Interface {
	return &realControl{adp: &adapterInformer{podInformer: informer}, revisionKey: revisionKey, now: metav1.Now}
}

func NewForTest(c client.Client, revisionKey string, now func() metav1.Time) Interface {
	return &realControl{adp: &adapterRuntimeClient{Client: c}, revisionKey: revisionKey, now: now}
}

func (c *realControl) Refresh(pod *v1.Pod, opts *UpdateOptions) RefreshResult {
	if err := c.refreshCondition(pod, opts); err != nil {
		return RefreshResult{RefreshErr: err}
	}

	var delayDuration time.Duration
	var err error
	if pod.Annotations[appspub.InPlaceUpdateGraceKey] != "" {
		if delayDuration, err = c.finishGracePeriod(pod, opts); err != nil {
			return RefreshResult{RefreshErr: err}
		}
	}

	return RefreshResult{DelayDuration: delayDuration}
}

func (c *realControl) refreshCondition(pod *v1.Pod, opts *UpdateOptions) error {
	// no need to update condition because of no readiness-gate
	if !containsReadinessGate(pod) {
		return nil
	}

	// in-place updating has not completed yet
	checkFunc := CheckInPlaceUpdateCompleted
	if opts != nil && opts.CustomizeCheckUpdateCompleted != nil {
		checkFunc = opts.CustomizeCheckUpdateCompleted
	}
	if checkErr := checkFunc(pod); checkErr != nil {
		klog.V(6).Infof("Check Pod %s/%s in-place update not completed yet: %v", pod.Namespace, pod.Name, checkErr)
		return nil
	}

	// already ready
	if existingCondition := GetCondition(pod); existingCondition != nil && existingCondition.Status == v1.ConditionTrue {
		return nil
	}

	newCondition := v1.PodCondition{
		Type:               appspub.InPlaceUpdateReady,
		Status:             v1.ConditionTrue,
		LastTransitionTime: c.now(),
	}
	return c.updateCondition(pod, newCondition)
}

func (c *realControl) updateCondition(pod *v1.Pod, condition v1.PodCondition) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone, err := c.adp.getPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		setPodCondition(clone, condition)
		updatePodReadyCondition(clone)
		return c.adp.updatePodStatus(clone)
	})
}

func (c *realControl) finishGracePeriod(pod *v1.Pod, opts *UpdateOptions) (time.Duration, error) {
	var delayDuration time.Duration
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone, err := c.adp.getPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		spec := UpdateSpec{}
		updateSpecJSON, ok := clone.Annotations[appspub.InPlaceUpdateGraceKey]
		if !ok {
			return nil
		}
		if err := json.Unmarshal([]byte(updateSpecJSON), &spec); err != nil {
			return err
		}
		graceDuration := time.Second * time.Duration(spec.GraceSeconds)

		updateState := appspub.InPlaceUpdateState{}
		updateStateJSON, ok := clone.Annotations[appspub.InPlaceUpdateStateKey]
		if !ok {
			return fmt.Errorf("pod has %s but %s not found", appspub.InPlaceUpdateGraceKey, appspub.InPlaceUpdateStateKey)
		}
		if err := json.Unmarshal([]byte(updateStateJSON), &updateState); err != nil {
			return nil
		}

		if clone.Labels[c.revisionKey] != spec.Revision {
			// If revision-hash has changed, just drop this GracePeriodSpec and go through the normal update process again.
			delete(clone.Annotations, appspub.InPlaceUpdateGraceKey)
		} else {
			if span := time.Since(updateState.UpdateTimestamp.Time); span < graceDuration {
				delayDuration = roundupSeconds(graceDuration - span)
				return nil
			}

			if clone, err = patchUpdateSpecToPod(clone, &spec, opts); err != nil {
				return err
			}
			delete(clone.Annotations, appspub.InPlaceUpdateGraceKey)
		}

		return c.adp.updatePod(clone)
	})

	return delayDuration, err
}

func (c *realControl) CanUpdateInPlace(oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) bool {
	return calculateInPlaceUpdateSpec(oldRevision, newRevision, opts) != nil
}

func (c *realControl) Update(pod *v1.Pod, oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) UpdateResult {
	// 1. calculate inplace update spec
	spec := calculateInPlaceUpdateSpec(oldRevision, newRevision, opts)
	if spec == nil {
		return UpdateResult{}
	}
	if opts != nil && opts.GracePeriodSeconds > 0 {
		spec.GraceSeconds = opts.GracePeriodSeconds
	}

	// TODO(FillZpp): maybe we should check if the previous in-place update has completed

	// 2. update condition for pod with readiness-gate
	if containsReadinessGate(pod) {
		newCondition := v1.PodCondition{
			Type:               appspub.InPlaceUpdateReady,
			LastTransitionTime: c.now(),
			Status:             v1.ConditionFalse,
			Reason:             "StartInPlaceUpdate",
		}
		if err := c.updateCondition(pod, newCondition); err != nil {
			return UpdateResult{InPlaceUpdate: true, UpdateErr: err}
		}
	}

	// 3. update container images
	if err := c.updatePodInPlace(pod, spec, opts); err != nil {
		return UpdateResult{InPlaceUpdate: true, UpdateErr: err}
	}

	var delayDuration time.Duration
	if opts != nil && opts.GracePeriodSeconds > 0 {
		delayDuration = time.Second * time.Duration(opts.GracePeriodSeconds)
	}
	return UpdateResult{InPlaceUpdate: true, DelayDuration: delayDuration}
}

func (c *realControl) updatePodInPlace(pod *v1.Pod, spec *UpdateSpec, opts *UpdateOptions) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clone, err := c.adp.getPod(pod.Namespace, pod.Name)
		if err != nil {
			return err
		}

		// update new revision
		if c.revisionKey != "" {
			clone.Labels[c.revisionKey] = spec.Revision
		}
		if clone.Annotations == nil {
			clone.Annotations = map[string]string{}
		}
		if opts != nil {
			for _, f := range opts.AdditionalFuncs {
				f(clone)
			}
		}

		// record old containerStatuses
		inPlaceUpdateState := appspub.InPlaceUpdateState{
			Revision:              spec.Revision,
			UpdateTimestamp:       c.now(),
			LastContainerStatuses: make(map[string]appspub.InPlaceUpdateContainerStatus, len(spec.ContainerImages)),
		}
		for _, c := range clone.Status.ContainerStatuses {
			if _, ok := spec.ContainerImages[c.Name]; ok {
				inPlaceUpdateState.LastContainerStatuses[c.Name] = appspub.InPlaceUpdateContainerStatus{
					ImageID: c.ImageID,
				}
			}
		}
		inPlaceUpdateStateJSON, _ := json.Marshal(inPlaceUpdateState)
		clone.Annotations[appspub.InPlaceUpdateStateKey] = string(inPlaceUpdateStateJSON)

		if spec.GraceSeconds <= 0 {
			if clone, err = patchUpdateSpecToPod(clone, spec, opts); err != nil {
				return err
			}
			delete(clone.Annotations, appspub.InPlaceUpdateGraceKey)
		} else {
			inPlaceUpdateSpecJSON, _ := json.Marshal(spec)
			clone.Annotations[appspub.InPlaceUpdateGraceKey] = string(inPlaceUpdateSpecJSON)
		}

		return c.adp.updatePod(clone)
	})
}

// patchUpdateSpecToPod returns new pod that merges spec into old pod
func patchUpdateSpecToPod(pod *v1.Pod, spec *UpdateSpec, opts *UpdateOptions) (*v1.Pod, error) {
	if opts != nil && opts.CustomizeSpecPatch != nil {
		return opts.CustomizeSpecPatch(pod, spec)
	}
	if spec.MetaDataPatch != nil {
		cloneBytes, _ := json.Marshal(pod)
		modified, err := strategicpatch.StrategicMergePatch(cloneBytes, spec.MetaDataPatch, &v1.Pod{})
		if err != nil {
			return nil, err
		}
		pod = &v1.Pod{}
		if err = json.Unmarshal(modified, pod); err != nil {
			return nil, err
		}
	}

	for i := range pod.Spec.Containers {
		if newImage, ok := spec.ContainerImages[pod.Spec.Containers[i].Name]; ok {
			pod.Spec.Containers[i].Image = newImage
		}
	}
	return pod, nil
}

// calculateInPlaceUpdateSpec calculates diff between old and update revisions.
// If the diff just contains replace operation of spec.containers[x].image, it will returns an UpdateSpec.
// Otherwise, it returns nil which means can not use in-place update.
func calculateInPlaceUpdateSpec(oldRevision, newRevision *apps.ControllerRevision, opts *UpdateOptions) *UpdateSpec {
	if opts != nil && opts.CustomizeSpecCalculate != nil {
		return opts.CustomizeSpecCalculate(oldRevision, newRevision)
	}

	if oldRevision == nil || newRevision == nil {
		return nil
	}

	patches, err := jsonpatch.CreatePatch(oldRevision.Data.Raw, newRevision.Data.Raw)
	if err != nil {
		return nil
	}

	oldTemp, err := GetTemplateFromRevision(oldRevision)
	if err != nil {
		return nil
	}
	newTemp, err := GetTemplateFromRevision(newRevision)
	if err != nil {
		return nil
	}

	updateSpec := &UpdateSpec{
		Revision:        newRevision.Name,
		ContainerImages: make(map[string]string),
	}
	if opts != nil && opts.GetRevision != nil {
		updateSpec.Revision = opts.GetRevision(newRevision)
	}

	// all patches for podSpec can just update images
	var metadataChanged bool
	for _, jsonPatchOperation := range patches {
		jsonPatchOperation.Path = strings.Replace(jsonPatchOperation.Path, "/spec/template", "", 1)

		if !strings.HasPrefix(jsonPatchOperation.Path, "/spec/") {
			metadataChanged = true
			continue
		}
		if jsonPatchOperation.Operation != "replace" || !inPlaceUpdatePatchRexp.MatchString(jsonPatchOperation.Path) {
			return nil
		}
		// for example: /spec/containers/0/image
		words := strings.Split(jsonPatchOperation.Path, "/")
		idx, _ := strconv.Atoi(words[3])
		if len(oldTemp.Spec.Containers) <= idx {
			return nil
		}
		updateSpec.ContainerImages[oldTemp.Spec.Containers[idx].Name] = jsonPatchOperation.Value.(string)
	}
	if metadataChanged {
		oldBytes, _ := json.Marshal(v1.Pod{ObjectMeta: oldTemp.ObjectMeta})
		newBytes, _ := json.Marshal(v1.Pod{ObjectMeta: newTemp.ObjectMeta})
		patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldBytes, newBytes, &v1.Pod{})
		if err != nil {
			return nil
		}
		updateSpec.MetaDataPatch = patchBytes
	}
	return updateSpec
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

// CheckInPlaceUpdateCompleted checks whether imageID in pod status has been changed since in-place update.
// If the imageID in containerStatuses has not been changed, we assume that kubelet has not updated
// containers in Pod.
func CheckInPlaceUpdateCompleted(pod *v1.Pod) error {
	inPlaceUpdateState := appspub.InPlaceUpdateState{}
	if stateStr, ok := pod.Annotations[appspub.InPlaceUpdateStateKey]; !ok {
		return nil
	} else if err := json.Unmarshal([]byte(stateStr), &inPlaceUpdateState); err != nil {
		return err
	}

	// this should not happen, unless someone modified pod revision label
	if inPlaceUpdateState.Revision != pod.Labels[apps.StatefulSetRevisionLabel] {
		return fmt.Errorf("currently revision %s not equal to in-place update revision %s",
			pod.Labels[apps.StatefulSetRevisionLabel], inPlaceUpdateState.Revision)
	}

	containerImages := make(map[string]string, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		containerImages[c.Name] = c.Image
		if len(strings.Split(c.Image, ":")) <= 1 {
			containerImages[c.Name] = fmt.Sprintf("%s:latest", c.Image)
		}
	}

	_, isInGraceState := pod.Annotations[appspub.InPlaceUpdateGraceKey]
	for _, cs := range pod.Status.ContainerStatuses {
		if oldStatus, ok := inPlaceUpdateState.LastContainerStatuses[cs.Name]; ok {
			// TODO: we assume that users should not update workload template with new image which actually has the same imageID as the old image
			if oldStatus.ImageID == cs.ImageID {
				if containerImages[cs.Name] != cs.Image || isInGraceState {
					return fmt.Errorf("container %s imageID not changed", cs.Name)
				}
			}
			delete(inPlaceUpdateState.LastContainerStatuses, cs.Name)
		}
	}

	if len(inPlaceUpdateState.LastContainerStatuses) > 0 {
		return fmt.Errorf("not found statuses of containers %v", inPlaceUpdateState.LastContainerStatuses)
	}

	return nil
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
	return getCondition(pod, appspub.InPlaceUpdateReady)
}

func getCondition(pod *v1.Pod, cType v1.PodConditionType) *v1.PodCondition {
	for _, c := range pod.Status.Conditions {
		if c.Type == cType {
			return &c
		}
	}
	return nil
}

func setPodCondition(pod *v1.Pod, condition v1.PodCondition) {
	for i, c := range pod.Status.Conditions {
		if c.Type == condition.Type {
			if c.Status != condition.Status {
				pod.Status.Conditions[i] = condition
			}
			return
		}
	}
	pod.Status.Conditions = append(pod.Status.Conditions, condition)
}

func updatePodReadyCondition(pod *v1.Pod) {
	podReady := getCondition(pod, v1.PodReady)
	if podReady == nil {
		return
	}

	containersReady := getCondition(pod, v1.ContainersReady)
	if containersReady == nil || containersReady.Status != v1.ConditionTrue {
		return
	}

	var unreadyMessages []string
	for _, rg := range pod.Spec.ReadinessGates {
		c := getCondition(pod, rg.ConditionType)
		if c == nil {
			unreadyMessages = append(unreadyMessages, fmt.Sprintf("corresponding condition of pod readiness gate %q does not exist.", string(rg.ConditionType)))
		} else if c.Status != v1.ConditionTrue {
			unreadyMessages = append(unreadyMessages, fmt.Sprintf("the status of pod readiness gate %q is not \"True\", but %v", string(rg.ConditionType), c.Status))
		}
	}

	newPodReady := v1.PodCondition{
		Type:               v1.PodReady,
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
	}
	// Set "Ready" condition to "False" if any readiness gate is not ready.
	if len(unreadyMessages) != 0 {
		unreadyMessage := strings.Join(unreadyMessages, ", ")
		newPodReady = v1.PodCondition{
			Type:    v1.PodReady,
			Status:  v1.ConditionFalse,
			Reason:  "ReadinessGatesNotReady",
			Message: unreadyMessage,
		}
	}

	setPodCondition(pod, newPodReady)
}

func roundupSeconds(d time.Duration) time.Duration {
	if d%time.Second == 0 {
		return d
	}
	return (d/time.Second + 1) * time.Second
}
