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

package podprobemarker

import (
	"context"
	"fmt"
	"reflect"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	nodeutil "k8s.io/kubernetes/pkg/controller/util/node"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	nodeImageCreationDelayAfterNodeReady = time.Second * 30
)

func (r *ReconcilePodProbeMarker) syncNodePodProbe(name string) error {
	// Fetch the NodePodProbe instance
	npp := &appsv1alpha1.NodePodProbe{}
	err := r.Get(context.TODO(), client.ObjectKey{Name: name}, npp)
	if err != nil {
		if errors.IsNotFound(err) {
			npp.Name = name
			return r.Create(context.TODO(), npp)
		}
		// Error reading the object - requeue the request.
		return err
	}
	// Fetch Node
	node := &corev1.Node{}
	err = r.Get(context.TODO(), client.ObjectKey{Name: name}, node)
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) || !node.DeletionTimestamp.IsZero() {
		return r.Delete(context.TODO(), npp)
	}
	// If Pod is deleted, then remove podProbe from NodePodProbe.Spec
	matchedPods, ppms, err := r.syncPodFromNodePodProbe(npp)
	if err != nil {
		return err
	}
	for _, status := range npp.Status.PodProbeStatuses {
		pod, ok := matchedPods[fmt.Sprintf("%s/%s/%s", status.Namespace, status.Name, status.Uid)]
		if !ok || !pod.DeletionTimestamp.IsZero() {
			continue
		}
		// Write podProbe result to Pod metadata and condition
		if err = r.updatePodProbeStatus(pod, ppms, status); err != nil {
			return err
		}
	}
	return nil
}

func (r *ReconcilePodProbeMarker) syncPodFromNodePodProbe(npp *appsv1alpha1.NodePodProbe) (map[string]*corev1.Pod, map[string]string, error) {
	// map[ns/name/uid]=Pod
	matchedPods := map[string]*corev1.Pod{}
	// map[pod.uid/probe.name]=PodProbeMarker.Name
	ppms := map[string]string{}
	for _, obj := range npp.Spec.PodProbes {
		pod := &corev1.Pod{}
		err := r.Get(context.TODO(), client.ObjectKey{Namespace: obj.Namespace, Name: obj.Name}, pod)
		if err != nil && !errors.IsNotFound(err) {
			klog.Errorf("PodProbeMarker get pod(%s/%s) failed: %s", obj.Namespace, obj.Name, err.Error())
			return nil, nil, err
		}
		if errors.IsNotFound(err) || !pod.DeletionTimestamp.IsZero() || string(pod.UID) != obj.Uid {
			continue
		}
		matchedPods[fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, pod.UID)] = pod
	}

	newSpec := appsv1alpha1.NodePodProbeSpec{}
	for i := range npp.Spec.PodProbes {
		obj := npp.Spec.PodProbes[i]
		if _, ok := matchedPods[fmt.Sprintf("%s/%s/%s", obj.Namespace, obj.Name, obj.Uid)]; ok {
			newSpec.PodProbes = append(newSpec.PodProbes, obj)
			for _, probe := range obj.Probes {
				ppms[fmt.Sprintf("%s/%s", obj.Uid, probe.Name)] = probe.PodProbeMarkerName
			}
		}
	}
	if reflect.DeepEqual(newSpec, npp.Spec) {
		return matchedPods, ppms, nil
	}

	nppClone := &appsv1alpha1.NodePodProbe{}
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: npp.Name}, nppClone); err != nil {
			klog.Errorf("error getting updated npp %s from client", npp.Name)
		}
		if reflect.DeepEqual(newSpec, nppClone.Spec) {
			return nil
		}
		nppClone.Spec = newSpec
		return r.Client.Update(context.TODO(), nppClone)
	})
	if err != nil {
		klog.Errorf("PodProbeMarker update NodePodProbe(%s) failed:%s", npp.Name, err.Error())
		return nil, nil, err
	}
	klog.V(3).Infof("PodProbeMarker update NodePodProbe(%s) from(%s) -> to(%s) success", npp.Name, util.DumpJSON(npp.Spec), util.DumpJSON(newSpec))
	return matchedPods, ppms, nil
}

func (r *ReconcilePodProbeMarker) updatePodProbeStatus(pod *corev1.Pod, ppms map[string]string, status appsv1alpha1.PodProbeStatus) error {
	// map[probe.name]->probeState
	currentConditions := make(map[string]appsv1alpha1.ProbeState)
	for _, condition := range pod.Status.Conditions {
		currentConditions[string(condition.Type)] = appsv1alpha1.ProbeState(condition.Status)
	}
	type metadata struct {
		Labels      map[string]interface{} `json:"labels,omitempty"`
		Annotations map[string]interface{} `json:"annotations,omitempty"`
	}
	// patch labels or annotations in pod
	probeMetadata := metadata{
		Labels:      map[string]interface{}{},
		Annotations: map[string]interface{}{},
	}
	// pod status condition record probe result
	var probeConditions []corev1.PodCondition
	var err error
	for i := range status.ProbeStates {
		probeState := status.ProbeStates[i]
		if probeState.State == "" || probeState.State == appsv1alpha1.ProbeUnknown {
			continue
		}
		if current, ok := currentConditions[probeState.Name]; !ok || current != probeState.State {
			probeConditions = append(probeConditions, corev1.PodCondition{
				Type:               corev1.PodConditionType(probeState.Name),
				Status:             corev1.ConditionStatus(probeState.State),
				LastProbeTime:      probeState.LastProbeTime,
				LastTransitionTime: probeState.LastProbeTime,
				Message:            probeState.Message,
			})
			// marker pod labels & annotations according to probe state
			// fetch PodProbeMarker
			ppmName := ppms[fmt.Sprintf("%s/%s", pod.UID, probeState.Name)]
			if ppmName == "" {
				continue
			}
			ppm := &appsv1alpha1.PodProbeMarker{}
			err = r.Get(context.TODO(), client.ObjectKey{Namespace: pod.Namespace, Name: ppmName}, ppm)
			if err != nil {
				// when PodProbeMarker is deleted, should delete probes from NodePodProbe.spec
				if errors.IsNotFound(err) {
					continue
				}
				klog.Errorf("PodProbeMarker(%s) get pod(%s/%s) failed: %s", ppmName, pod.Namespace, pod.Name, err.Error())
				return err
			} else if !ppm.DeletionTimestamp.IsZero() {
				continue
			}
			var policy []appsv1alpha1.ProbeMarkerPolicy
			for _, probe := range ppm.Spec.Probes {
				if probe.Name == probeState.Name {
					policy = probe.MarkerPolicy
					break
				}
			}
			if len(policy) == 0 {
				continue
			}
			labels := sets.NewString()
			annotations := sets.NewString()
			for _, obj := range policy {
				for k := range obj.Labels {
					labels.Insert(k)
				}
				for k := range obj.Annotations {
					annotations.Insert(k)
				}
			}
			for _, obj := range policy {
				if obj.State == probeState.State {
					for k, v := range obj.Labels {
						probeMetadata.Labels[k] = v
						labels.Delete(k)
					}
					for k, v := range obj.Annotations {
						probeMetadata.Annotations[k] = v
						annotations.Delete(k)
					}
				}
			}
			// If Condition only configures one behavior(True or False),
			// then the other behavior defaults to Delete the patched metadata
			for k := range labels {
				probeMetadata.Labels[k] = nil
			}
			for k := range annotations {
				probeMetadata.Annotations[k] = nil
			}
		}
	}
	if len(probeConditions) == 0 {
		return nil
	}

	// patch metadata
	podClone := pod.DeepCopy()
	if len(probeMetadata.Labels) > 0 || len(probeMetadata.Annotations) > 0 {
		body := fmt.Sprintf(`{"metadata":%s}`, util.DumpJSON(probeMetadata))
		if err = r.Patch(context.TODO(), podClone, client.RawPatch(types.MergePatchType, []byte(body))); err != nil {
			klog.Errorf("PodProbeMarker patch pod(%s/%s) metadata failed: %s", podClone.Namespace, podClone.Name, err.Error())
			return err
		}
		klog.V(3).Infof("PodProbeMarker patch pod(%s/%s) metadata(%s) success", podClone.Namespace, podClone.Name, body)
	}

	//update pod status condition
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err = r.Client.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, podClone); err != nil {
			klog.Errorf("error getting updated pod(%s/%s) from client", pod.Namespace, pod.Name)
			return err
		}
		oldStatus := podClone.DeepCopy()
		for i := range probeConditions {
			condition := probeConditions[i]
			util.SetPodCondition(podClone, condition)
		}
		if reflect.DeepEqual(oldStatus, podClone.Status) {
			return nil
		}
		return r.Client.Status().Update(context.TODO(), podClone)
	}); err != nil {
		klog.Errorf("PodProbeMarker patch pod(%s/%s) status failed: %s", podClone.Namespace, podClone.Name, err.Error())
		return err
	}
	klog.V(3).Infof("PodProbeMarker update pod(%s/%s) status conditions(%s) success", podClone.Namespace, podClone.Name, util.DumpJSON(probeConditions))
	return nil
}

func (r *ReconcilePodProbeMarker) removePodProbeFromNodePodProbe(ppmName, nppName string) error {
	npp := &appsv1alpha1.NodePodProbe{}
	err := r.Get(context.TODO(), client.ObjectKey{Name: nppName}, npp)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	newSpec := appsv1alpha1.NodePodProbeSpec{}
	for _, podProbe := range npp.Spec.PodProbes {
		newPodProbe := appsv1alpha1.PodProbe{Name: podProbe.Name, Namespace: podProbe.Namespace, Uid: podProbe.Uid}
		for i := range podProbe.Probes {
			probe := podProbe.Probes[i]
			if probe.PodProbeMarkerName != ppmName {
				newPodProbe.Probes = append(newPodProbe.Probes, probe)
			}
		}
		if len(newPodProbe.Probes) > 0 {
			newSpec.PodProbes = append(newSpec.PodProbes, newPodProbe)
		}
	}
	if reflect.DeepEqual(npp.Spec, newSpec) {
		return nil
	}
	npp.Spec = newSpec
	err = r.Update(context.TODO(), npp)
	if err != nil {
		klog.Errorf("NodePodProbe(%s) remove PodProbe(%s) failed: %s", nppName, ppmName, err.Error())
		return err
	}
	klog.V(3).Infof("NodePodProbe(%s) remove PodProbe(%s) success", nppName, ppmName)
	return nil
}

func getNodeReadyAndDelayTime(node *corev1.Node) (bool, time.Duration) {
	_, condition := nodeutil.GetNodeCondition(&node.Status, corev1.NodeReady)
	if condition == nil || condition.Status != corev1.ConditionTrue {
		return false, 0
	}
	delay := nodeImageCreationDelayAfterNodeReady - time.Since(condition.LastTransitionTime.Time)
	if delay > 0 {
		return true, delay
	}
	return true, 0
}
