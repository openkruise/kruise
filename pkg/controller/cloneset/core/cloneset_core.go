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

package core

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/appscode/jsonpatch"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// "k8s.io/apimachinery/pkg/util/sets" // Not strictly needed for PR #2060's change in this func
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	clonesetutils "github.com/openkruise/kruise/pkg/controller/cloneset/utils"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/inplaceupdate"
)

var (
	inPlaceUpdateTemplateSpecPatchRexp = regexp.MustCompile("^/containers/([0-9]+)/image$")
)

type commonControl struct {
	*appsv1alpha1.CloneSet
}

var _ Control = &commonControl{}

func (c *commonControl) IsInitializing() bool {
	return false
}

func (c *commonControl) SetRevisionTemplate(revisionSpec map[string]interface{}, template map[string]interface{}) {
	revisionSpec["template"] = template
	template["$patch"] = "replace"
}

func (c *commonControl) ApplyRevisionPatch(patched []byte) (*appsv1alpha1.CloneSet, error) {
	restoredSet := &appsv1alpha1.CloneSet{}
	if err := json.Unmarshal(patched, restoredSet); err != nil {
		return nil, err
	}
	return restoredSet, nil
}

func (c *commonControl) IsReadyToScale() bool {
	return true
}

func (c *commonControl) NewVersionedPods(currentCS, updateCS *appsv1alpha1.CloneSet,
	currentRevision, updateRevision string,
	expectedCreations, expectedCurrentCreations int,
	availableIDs []string,
) ([]*v1.Pod, error) {
	var newPods []*v1.Pod
	if expectedCreations <= expectedCurrentCreations {
		newPods = c.newVersionedPods(currentCS, currentRevision, expectedCreations, &availableIDs)
	} else {
		newPods = c.newVersionedPods(currentCS, currentRevision, expectedCurrentCreations, &availableIDs)
		newPods = append(newPods, c.newVersionedPods(updateCS, updateRevision, expectedCreations-expectedCurrentCreations, &availableIDs)...)
	}
	return newPods, nil
}

func (c *commonControl) newVersionedPods(cs *appsv1alpha1.CloneSet, revision string, replicas int, availableIDs *[]string) []*v1.Pod {
	var newPods []*v1.Pod
	for i := 0; i < replicas; i++ {
		if len(*availableIDs) == 0 {
			return newPods
		}
		id := (*availableIDs)[0]
		*availableIDs = (*availableIDs)[1:]

		pod, _ := kubecontroller.GetPodFromTemplate(&cs.Spec.Template, cs, metav1.NewControllerRef(cs, clonesetutils.ControllerKind))
		if pod.Labels == nil {
			pod.Labels = make(map[string]string)
		}
		clonesetutils.WriteRevisionHash(pod, revision)

		pod.Name = fmt.Sprintf("%s-%s", cs.Name, id)
		pod.Namespace = cs.Namespace
		pod.Labels[appsv1alpha1.CloneSetInstanceID] = id

		inplaceupdate.InjectReadinessGate(pod)
		clonesetutils.UpdateStorage(cs, pod)

		newPods = append(newPods, pod)
	}
	return newPods
}

func (c *commonControl) GetPodSpreadConstraint() []clonesetutils.PodSpreadConstraint {
	var constraints []clonesetutils.PodSpreadConstraint
	for _, c := range c.Spec.Template.Spec.TopologySpreadConstraints {
		constraints = append(constraints, clonesetutils.PodSpreadConstraint{TopologyKey: c.TopologyKey})
	}
	return constraints
}

func (c *commonControl) IsPodUpdatePaused(pod *v1.Pod) bool {
	return false
}

func (c *commonControl) IsPodUpdateReady(pod *v1.Pod, minReadySeconds int32) bool {
	if !clonesetutils.IsRunningAndAvailable(pod, minReadySeconds) {
		return false
	}
	condition := inplaceupdate.GetCondition(pod)
	if condition != nil && condition.Status != v1.ConditionTrue {
		return false
	}
	return true
}

func (c *commonControl) GetPodsSortFunc(pods []*v1.Pod, waitUpdateIndexes []int) func(i, j int) bool {
	return func(i, j int) bool {
		return kubecontroller.ActivePods(pods).Less(waitUpdateIndexes[i], waitUpdateIndexes[j])
	}
}

func (c *commonControl) GetUpdateOptions() *inplaceupdate.UpdateOptions {
	opts := &inplaceupdate.UpdateOptions{}
	if c.Spec.UpdateStrategy.InPlaceUpdateStrategy != nil {
		opts.GracePeriodSeconds = c.Spec.UpdateStrategy.InPlaceUpdateStrategy.GracePeriodSeconds
	}
	if c.Spec.UpdateStrategy.Type == appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
		opts.IgnoreVolumeClaimTemplatesHashDiff = true
	}
	return opts
}

func (c *commonControl) ValidateCloneSetUpdate(oldCS, newCS *appsv1alpha1.CloneSet) error {
	if newCS.Spec.UpdateStrategy.Type != appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType {
		return nil
	}

	oldTempJSON, _ := json.Marshal(oldCS.Spec.Template.Spec)
	newTempJSON, _ := json.Marshal(newCS.Spec.Template.Spec)
	patches, err := jsonpatch.CreatePatch(oldTempJSON, newTempJSON)
	if err != nil {
		return fmt.Errorf("failed calculate patches between old/new template spec")
	}

	for _, p := range patches {
		if p.Operation != "replace" || !inPlaceUpdateTemplateSpecPatchRexp.MatchString(p.Path) {
			return fmt.Errorf("only allowed to update images in spec for %s, but found %s %s",
				appsv1alpha1.InPlaceOnlyCloneSetUpdateStrategyType, p.Operation, p.Path)
		}
	}
	return nil
}

func (c *commonControl) ExtraStatusCalculation(status *appsv1alpha1.CloneSetStatus, pods []*v1.Pod) error {
	return nil
}

func (c *commonControl) IgnorePodUpdateEvent(oldPod, curPod *v1.Pod) bool {
	if oldPod.Generation != curPod.Generation {
		klog.V(5).Infof("CloneSet %s/%s: Pod %s/%s generation changed, not ignoring update.",
			c.CloneSet.Namespace, c.CloneSet.Name, curPod.Namespace, curPod.Name)
		return false
	}

	if lifecycleFinalizerChanged(c.CloneSet, oldPod, curPod) { // This uses PR #2060's version
		klog.V(4).Infof("CloneSet %s/%s: Pod %s/%s lifecycle finalizer changed, not ignoring update.",
			c.CloneSet.Namespace, c.CloneSet.Name, curPod.Namespace, curPod.Name)
		return false
	}

	if c.IsPodUpdatePaused(oldPod) != c.IsPodUpdatePaused(curPod) {
		klog.V(5).Infof("CloneSet %s/%s: Pod %s/%s pause status changed, not ignoring update.",
			c.CloneSet.Namespace, c.CloneSet.Name, curPod.Namespace, curPod.Name)
		return false
	}

	if podutil.IsPodReady(oldPod) != podutil.IsPodReady(curPod) {
		klog.V(5).Infof("CloneSet %s/%s: Pod %s/%s readiness changed, not ignoring update.",
			c.CloneSet.Namespace, c.CloneSet.Name, curPod.Namespace, curPod.Name)
		return false
	}

	containsReadinessGate := func(pod *v1.Pod) bool {
		for _, r := range pod.Spec.ReadinessGates {
			if r.ConditionType == appspub.InPlaceUpdateReady {
				return true
			}
		}
		return false
	}
	isPodConsideredActivelyUpdating := func(pod *v1.Pod) bool {
		if len(pod.Labels) > 0 {
			state := appspub.LifecycleStateType(pod.Labels[appspub.LifecycleStateKey])
			// Only consider actual update-related states, not all non-Normal states
			return state == appspub.LifecycleStatePreparingUpdate || state == appspub.LifecycleStateUpdating
		}
		return false
	}

	if containsReadinessGate(curPod) || isPodConsideredActivelyUpdating(curPod) { // Use modified function
		opts := c.GetUpdateOptions()
		opts = inplaceupdate.SetOptionsDefaults(opts)
		if err := containersUpdateCompleted(curPod, opts.CheckContainersUpdateCompleted); err == nil {
			if cond := inplaceupdate.GetCondition(curPod); cond == nil || cond.Status != v1.ConditionTrue {
				klog.V(5).Infof("CloneSet %s/%s: Pod %s/%s in-place update condition not true, not ignoring update.",
					c.CloneSet.Namespace, c.CloneSet.Name, curPod.Namespace, curPod.Name)
				return false
			}
			if utilfeature.DefaultFeatureGate.Enabled(features.InPlaceWorkloadVerticalScaling) {
				klog.V(5).Infof("CloneSet %s/%s: Pod %s/%s in-place update complete and InPlaceWorkloadVerticalScaling enabled, not ignoring update.",
					c.CloneSet.Namespace, c.CloneSet.Name, curPod.Namespace, curPod.Name)
				return false
			}
		} else {
			// If containersUpdateCompleted fails (e.g. no annotation), it's not necessarily an active update needing reconciliation *if* the lifecycle state isn't an update state.
			// However, if it IS an update state, then this failure is significant.
			// The original logic: if err != nil, it means the update isn't complete or state is missing, so don't ignore.
			// This seems reasonable IF isPodConsideredActivelyUpdating is true.
			klog.V(5).Infof("CloneSet %s/%s: Pod %s/%s containersUpdateCompleted check failed (%v) or indicates ongoing update, not ignoring update.",
				c.CloneSet.Namespace, c.CloneSet.Name, curPod.Namespace, curPod.Name, err)
			return false
		}
	}
	klog.V(5).Infof("CloneSet %s/%s: Pod %s/%s update event meets criteria for optimized ignoring.",
		c.CloneSet.Namespace, c.CloneSet.Name, curPod.Namespace, curPod.Name)
	return true
}

func containersUpdateCompleted(pod *v1.Pod, checkFunc func(pod *v1.Pod, state *appspub.InPlaceUpdateState) error) error {
	if stateStr, ok := appspub.GetInPlaceUpdateState(pod); ok {
		state := appspub.InPlaceUpdateState{}
		if err := json.Unmarshal([]byte(stateStr), &state); err != nil {
			return err
		}
		return checkFunc(pod, &state)
	}
	return fmt.Errorf("pod %v has no in-place update state annotation", klog.KObj(pod))
}

func lifecycleFinalizerChanged(cs *appsv1alpha1.CloneSet, oldPod, curPod *v1.Pod) bool {
	if cs.Spec.Lifecycle == nil {
		return false
	}

	if cs.Spec.Lifecycle.PreNormal != nil {
		for _, f := range cs.Spec.Lifecycle.PreNormal.FinalizersHandler {
			if controllerutil.ContainsFinalizer(oldPod, f) != controllerutil.ContainsFinalizer(curPod, f) {
				return true
			}
		}
	}

	if cs.Spec.Lifecycle.PreDelete != nil {
		for _, f := range cs.Spec.Lifecycle.PreDelete.FinalizersHandler {
			if controllerutil.ContainsFinalizer(oldPod, f) != controllerutil.ContainsFinalizer(curPod, f) {
				return true
			}
		}
	}

	if cs.Spec.Lifecycle.InPlaceUpdate != nil {
		for _, f := range cs.Spec.Lifecycle.InPlaceUpdate.FinalizersHandler {
			if controllerutil.ContainsFinalizer(oldPod, f) != controllerutil.ContainsFinalizer(curPod, f) {
				return true
			}
		}
	}

	return false
}
