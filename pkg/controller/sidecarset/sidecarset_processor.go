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

package sidecarset

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/expectations"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Processor struct {
	Client             client.Client
	recorder           record.EventRecorder
	updateExpectations expectations.UpdateExpectations
}

func NewSidecarSetProcessor(cli client.Client, expectations expectations.UpdateExpectations, rec record.EventRecorder) *Processor {
	return &Processor{
		Client:             cli,
		updateExpectations: expectations,
		recorder:           rec,
	}
}

func (p *Processor) UpdateSidecarSet(sidecarSet *appsv1alpha1.SidecarSet) (reconcile.Result, error) {
	control := sidecarcontrol.New(sidecarSet)
	// 1. get matching pods with the sidecarSet
	pods, err := p.getMatchingPods(sidecarSet)
	if err != nil {
		klog.Errorf("sidecarSet get matching pods error, err: %v, name: %s", err, sidecarSet.Name)
		return reconcile.Result{}, err
	}

	// 2. calculate SidecarSet status based on pods
	status := calculateStatus(control, pods)
	//update sidecarSet status in store
	if err := p.updateSidecarSetStatus(sidecarSet, status); err != nil {
		return reconcile.Result{}, err
	}

	// in case of informer cache latency
	for _, pod := range pods {
		p.updateExpectations.ObserveUpdated(sidecarSet.Name, sidecarcontrol.GetSidecarSetRevision(sidecarSet), pod)
	}
	allUpdated, _, inflightPods := p.updateExpectations.SatisfiedExpectations(sidecarSet.Name, sidecarcontrol.GetSidecarSetRevision(sidecarSet))
	if !allUpdated {
		klog.V(3).Infof("sidecarset %s matched pods has some update in flight: %v, will sync later", sidecarSet.Name, inflightPods)
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}

	// 4. SidecarSet upgrade strategy type is NotUpdate
	if !isSidecarSetNotUpdate(sidecarSet) {
		return reconcile.Result{}, nil
	}

	// 5. sidecarset already updates all matched pods, then return
	if isSidecarSetUpdateFinish(status) {
		klog.V(3).Infof("sidecarSet update pod finished, name: %s", sidecarSet.Name)
		return reconcile.Result{}, nil
	}

	// 6. Paused indicates that the SidecarSet is paused to update matched pods
	if sidecarSet.Spec.UpdateStrategy.Paused {
		klog.V(3).Infof("sidecarSet is paused, name: %s", sidecarSet.Name)
		return reconcile.Result{}, nil
	}

	// 7. upgrade pod sidecar
	if err := p.updatePods(control, pods); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (p *Processor) updatePods(control sidecarcontrol.SidecarControl, pods []*corev1.Pod) error {
	sidecarset := control.GetSidecarset()
	// compute next updated pods based on the sidecarset upgrade strategy
	upgradePods := NewStrategy().GetNextUpgradePods(control, pods)
	if len(upgradePods) == 0 {
		klog.V(3).Infof("sidecarSet next update is nil, skip this round, name: %s", sidecarset.Name)
		return nil
	}
	// upgrade pod sidecar
	for _, pod := range upgradePods {
		if err := p.updatePodSidecarAndHash(control, pod); err != nil {
			err := fmt.Errorf("updatePodSidecarAndHash error, s:%s, pod:%s, err:%v", sidecarset.Name, pod.Name, err)
			return err
		}
		p.updateExpectations.ExpectUpdated(sidecarset.Name, sidecarcontrol.GetSidecarSetRevision(sidecarset), pod)
	}

	// mark upgrade pods list
	podNames := make([]string, 0, len(upgradePods))
	for _, pod := range upgradePods {
		podNames = append(podNames, pod.Name)
	}
	klog.V(3).Infof("sidecarSet inject pod step, name: %s, pods: %v", sidecarset.Name, podNames)
	return nil
}

func (p *Processor) updatePodSidecarAndHash(control sidecarcontrol.SidecarControl, pod *corev1.Pod) error {
	podClone := pod.DeepCopy()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// update pod sidecar container
		updatePodSidecarContainer(control, podClone)

		// older pod don't have SidecarSetListAnnotation
		// which is to improve the performance of the sidecarSet controller
		sidecarSetNames, ok := podClone.Annotations[sidecarcontrol.SidecarSetListAnnotation]
		if !ok || len(sidecarSetNames) == 0 {
			podClone.Annotations[sidecarcontrol.SidecarSetListAnnotation] = p.listMatchedSidecarSets(podClone)
		}

		//update pod in store
		updateErr := p.Client.Update(context.TODO(), podClone)
		if updateErr == nil {
			return nil
		}

		key := types.NamespacedName{
			Namespace: podClone.Namespace,
			Name:      podClone.Name,
		}
		if err := p.Client.Get(context.TODO(), key, podClone); err != nil {
			klog.Errorf("error getting updated pod %s from client", control.GetSidecarset().Name)
		}
		return updateErr
	})

	return err
}

func (p *Processor) listMatchedSidecarSets(pod *corev1.Pod) string {
	sidecarSetList := &appsv1alpha1.SidecarSetList{}
	if err := p.Client.List(context.TODO(), sidecarSetList); err != nil {
		klog.Errorf("List SidecarSets failed: %s", err.Error())
		return ""
	}

	//matched SidecarSet.Name list
	sidecarSetNames := make([]string, 0)
	for _, sidecarSet := range sidecarSetList.Items {
		if matched, _ := sidecarcontrol.PodMatchedSidecarSet(pod, sidecarSet); matched {
			sidecarSetNames = append(sidecarSetNames, sidecarSet.Name)
		}
	}

	return strings.Join(sidecarSetNames, ",")
}

func (p *Processor) updateSidecarSetStatus(sidecarSet *appsv1alpha1.SidecarSet, status *appsv1alpha1.SidecarSetStatus) error {
	if !inconsistentStatus(sidecarSet, status) {
		return nil
	}

	sidecarSetClone := sidecarSet.DeepCopy()
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		sidecarSetClone.Status = *status
		sidecarSetClone.Status.ObservedGeneration = sidecarSetClone.Generation

		updateErr := p.Client.Status().Update(context.TODO(), sidecarSetClone)
		if updateErr == nil {
			return nil
		}

		key := types.NamespacedName{
			Name: sidecarSetClone.Name,
		}
		if err := p.Client.Get(context.TODO(), key, sidecarSetClone); err != nil {
			klog.Errorf("error getting updated sidecarset %s from client", sidecarSetClone.Name)
		}
		return updateErr
	}); err != nil {
		return err
	}

	klog.V(3).Infof("sidecarSet update status success, name: %s", sidecarSet.Name)
	return nil
}

// If you need update the pod object, you must DeepCopy it
func (p *Processor) getMatchingPods(s *appsv1alpha1.SidecarSet) ([]*corev1.Pod, error) {
	// get more faster selector
	selector, err := util.GetFastLabelSelector(s.Spec.Selector)
	if err != nil {
		return nil, err
	}

	// If sidecarSet.Spec.Namespace is empty, then select in cluster
	scopedNamespaces := []string{s.Spec.Namespace}
	selectedPods, err := p.getSelectedPods(scopedNamespaces, selector)
	if err != nil {
		return nil, err
	}

	// filter out pods that don't require updated, include the following:
	// 1. Deletion pod
	// 2. ignore namespace: "kube-system", "kube-public"
	// 3. never be injected sidecar container
	var filteredPods []*corev1.Pod
	for _, pod := range selectedPods {
		if sidecarcontrol.IsActivePod(pod) && isPodInjectedSidecar(s, pod) {
			filteredPods = append(filteredPods, pod)
		}
	}
	return filteredPods, nil
}

// get selected pods(DisableDeepCopy:true, indicates must be deep copy before update pod objection)
func (p *Processor) getSelectedPods(namespaces []string, selector labels.Selector) (relatedPods []*corev1.Pod, err error) {
	// DisableDeepCopy:true, indicates must be deep copy before update pod objection
	listOpts := &client.ListOptions{LabelSelector: selector}
	for _, ns := range namespaces {
		allPods := &corev1.PodList{}
		listOpts.Namespace = ns
		if listErr := p.Client.List(context.TODO(), allPods, listOpts); listErr != nil {
			err = fmt.Errorf("sidecarSet list pods by ns error, ns[%s], err:%v", ns, listErr)
			return
		}
		for i := range allPods.Items {
			relatedPods = append(relatedPods, &allPods.Items[i])
		}
	}
	return
}

// calculate SidecarSet status
// MatchedPods: all matched pods number
// UpdatedPods: updated pods number
// ReadyPods: ready pods number
// UpdatedReadyPods: updated and ready pods number
// UnavailablePods: MatchedPods - UpdatedReadyPods
func calculateStatus(control sidecarcontrol.SidecarControl, pods []*corev1.Pod) *appsv1alpha1.SidecarSetStatus {
	sidecarset := control.GetSidecarset()
	var matchedPods, updatedPods, readyPods, updatedAndReady int32
	matchedPods = int32(len(pods))
	for _, pod := range pods {
		updated := sidecarcontrol.IsPodSidecarUpdated(sidecarset, pod)
		if updated {
			updatedPods++
		}
		if control.IsPodConsistentAndReady(pod) {
			readyPods++
			if updated {
				updatedAndReady++
			}
		}
	}
	return &appsv1alpha1.SidecarSetStatus{
		ObservedGeneration: sidecarset.Generation,
		MatchedPods:        matchedPods,
		UpdatedPods:        updatedPods,
		ReadyPods:          readyPods,
		UpdatedReadyPods:   updatedAndReady,
	}
}

// whether this pod has been injected sidecar container based on the sidecarSet
func isPodInjectedSidecar(sidecarSet *appsv1alpha1.SidecarSet, pod *corev1.Pod) bool {
	// if pod annotations contain sidecarset hash, then indicates the pod has been injected in sidecar container
	return sidecarcontrol.GetPodSidecarSetRevision(sidecarSet.Name, pod) != ""
}

func isSidecarSetNotUpdate(s *appsv1alpha1.SidecarSet) bool {
	if s.Spec.UpdateStrategy.Type == appsv1alpha1.NotUpdateSidecarSetStrategyType {
		klog.V(3).Infof("sidecarSet spreading RollingUpdate config type, name: %s, type: %s", s.Name, s.Spec.UpdateStrategy.Type)
		return false
	}
	return true
}

func updateColdUpgradeContainerInPod(sidecarContainer *appsv1alpha1.SidecarContainer, control sidecarcontrol.SidecarControl, pod *corev1.Pod) (changedContainer string) {
	var containerToUpgrade corev1.Container
	for _, containerInPod := range pod.Spec.Containers {
		if containerInPod.Name == sidecarContainer.Name {
			containerToUpgrade = containerInPod
			break
		}
	}
	beforeContainerSpec := util.DumpJSON(containerToUpgrade)
	newContainer := control.UpdateSidecarContainerToLatest(sidecarContainer.Container, containerToUpgrade)
	afterContainerSpec := util.DumpJSON(newContainer)

	// pod.container definition changed, then update container spec in pod
	if beforeContainerSpec != afterContainerSpec {
		klog.V(3).Infof("try to update container %v/%v/%v, before: %v, after: %v",
			pod.Namespace, pod.Name, newContainer.Name, beforeContainerSpec, afterContainerSpec)
		updateContainerInPod(newContainer, pod)
		changedContainer = newContainer.Name
	}
	return
}

func updateContainerInPod(container corev1.Container, pod *corev1.Pod) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == container.Name {
			pod.Spec.Containers[i] = container
			return
		}
	}
}

func updatePodSidecarContainer(control sidecarcontrol.SidecarControl, pod *corev1.Pod) {
	sidecarset := control.GetSidecarset()

	var changedContainers []string
	for _, sidecarContainer := range sidecarset.Spec.Containers {
		//sidecarContainer := &sidecarset.Spec.Containers[i]
		// volumeMounts that injected into sidecar container
		// when volumeMounts SubPathExpr contains expansions, then need copy container EnvVars(injectEnvs)
		injectedMounts, injectedEnvs := sidecarcontrol.GetInjectedVolumeMountsAndEnvs(control, &sidecarContainer, pod)
		// merge VolumeMounts from sidecar.VolumeMounts and shared VolumeMounts
		sidecarContainer.VolumeMounts = util.MergeVolumeMounts(sidecarContainer.VolumeMounts, injectedMounts)

		// get injected env & mounts explicitly so that can be compared with old ones in pod
		transferEnvs := sidecarcontrol.GetSidecarTransferEnvs(&sidecarContainer, pod)
		// append volumeMounts SubPathExpr environments
		transferEnvs = util.MergeEnvVar(transferEnvs, injectedEnvs)
		// merged Env from sidecar.Env and transfer envs
		sidecarContainer.Env = util.MergeEnvVar(sidecarContainer.Env, transferEnvs)

		var changedContainer string
		changedContainer = updateColdUpgradeContainerInPod(&sidecarContainer, control, pod)
		if changedContainer != "" {
			changedContainers = append(changedContainers, changedContainer)
		}
	}
	// update pod information in upgrade
	control.UpdatePodAnnotationsInUpgrade(changedContainers, pod)
	return
}

func inconsistentStatus(sidecarSet *appsv1alpha1.SidecarSet, status *appsv1alpha1.SidecarSetStatus) bool {
	return status.ObservedGeneration > sidecarSet.Status.ObservedGeneration ||
		status.MatchedPods != sidecarSet.Status.MatchedPods ||
		status.UpdatedPods != sidecarSet.Status.UpdatedPods ||
		status.ReadyPods != sidecarSet.Status.ReadyPods ||
		status.UpdatedReadyPods != sidecarSet.Status.UpdatedReadyPods
}
