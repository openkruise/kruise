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

package v1beta1

import (
	"context"
	"fmt"
	"sort"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	imageutils "k8s.io/kubernetes/test/utils/image"

	appspub "github.com/openkruise/kruise/apis/apps/pub"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/util"
)

type CloneSetTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
	ns string
}

func NewCloneSetTester(c clientset.Interface, kc kruiseclientset.Interface, ns string) *CloneSetTester {
	return &CloneSetTester{
		c:  c,
		kc: kc,
		ns: ns,
	}
}

func (t *CloneSetTester) NewCloneSet(name string, replicas int32, updateStrategy appsv1beta1.CloneSetUpdateStrategy) *appsv1beta1.CloneSet {
	cloneSet := t.newCloneSet(name, replicas)
	cloneSet.Spec.UpdateStrategy = updateStrategy
	return cloneSet
}

func (t *CloneSetTester) newCloneSet(name string, replicas int32) *appsv1beta1.CloneSet {
	return &appsv1beta1.CloneSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: t.ns,
			Name:      name,
		},
		Spec: appsv1beta1.CloneSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"owner": name}},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"owner": name},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "nginx",
							Image: imageutils.GetE2EImage(imageutils.Nginx),
							Env: []v1.EnvVar{
								{Name: "test", Value: "foo"},
							},
							Resources: v1.ResourceRequirements{
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse("200m"),
									v1.ResourceMemory: resource.MustParse("200Mi"),
								},
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse("1"),
									v1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (t *CloneSetTester) NewCloneSetWithLifecycle(name string, replicas int32, lifecycle *appspub.Lifecycle, finalizers []string) *appsv1beta1.CloneSet {
	cloneSet := t.newCloneSet(name, replicas)
	cloneSet.Spec.Lifecycle = lifecycle

	if len(finalizers) > 0 {
		cloneSet.Spec.Template.Finalizers = finalizers
	}

	return cloneSet
}

func (t *CloneSetTester) NewCloneSetWithSpecificScheduler(name string, replicas int32, scheduler string) *appsv1beta1.CloneSet {
	cloneSet := t.newCloneSet(name, replicas)
	cloneSet.Spec.Template.Spec.SchedulerName = scheduler
	return cloneSet
}

func (t *CloneSetTester) CreateCloneSet(cs *appsv1beta1.CloneSet) (*appsv1beta1.CloneSet, error) {
	return t.kc.AppsV1beta1().CloneSets(t.ns).Create(context.TODO(), cs, metav1.CreateOptions{})
}

func (t *CloneSetTester) GetCloneSet(name string) (*appsv1beta1.CloneSet, error) {
	return t.kc.AppsV1beta1().CloneSets(t.ns).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *CloneSetTester) UpdateCloneSet(name string, fn func(cs *appsv1beta1.CloneSet)) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		cs, err := t.GetCloneSet(name)
		if err != nil {
			return err
		}

		fn(cs)
		_, err = t.kc.AppsV1beta1().CloneSets(t.ns).Update(context.TODO(), cs, metav1.UpdateOptions{})
		return err
	})
}

func (t *CloneSetTester) ListPodsForCloneSet(name string) (pods []*v1.Pod, err error) {
	podList, err := t.c.CoreV1().Pods(t.ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		// ignore deleting pod
		if pod.DeletionTimestamp != nil {
			continue
		}
		if owner := metav1.GetControllerOf(pod); owner != nil && owner.Name == name {
			pods = append(pods, pod)
		}
	}
	sort.SliceStable(pods, func(i, j int) bool {
		return pods[i].Name < pods[j].Name
	})
	return
}

func (t *CloneSetTester) ListPVCForCloneSet() (pvcs []*v1.PersistentVolumeClaim, err error) {
	pvcList, err := t.c.CoreV1().PersistentVolumeClaims(t.ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if pvc.DeletionTimestamp.IsZero() {
			pvcs = append(pvcs, pvc)
		}
	}
	sort.SliceStable(pvcs, func(i, j int) bool {
		return pvcs[i].Name < pvcs[j].Name
	})
	return
}

func (t *CloneSetTester) ListImagePullJobsForCloneSet(name string) (jobs []*appsv1beta1.ImagePullJob, err error) {
	jobList, err := t.kc.AppsV1beta1().ImagePullJobs(t.ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if owner := metav1.GetControllerOf(job); owner != nil && owner.Name == name {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

func (t *CloneSetTester) DeleteCloneSet(name string) error {
	return t.kc.AppsV1beta1().CloneSets(t.ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (t *CloneSetTester) GetSelectorPods(namespace string, selector *metav1.LabelSelector) ([]v1.Pod, error) {
	faster, err := util.ValidatedLabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}
	podList, err := t.c.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: faster.String()})
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}

func (t *CloneSetTester) DeletePod(name string) error {
	return t.c.CoreV1().Pods(t.ns).Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func (t *CloneSetTester) GetCloneSetProgressingCondition(name string) (*appsv1beta1.CloneSetCondition, error) {
	return t.GetCloneSetCondition(name, appsv1beta1.CloneSetConditionTypeProgressing)
}

func (t *CloneSetTester) GetCloneSetProgressingConditionWithoutTime(name string) (*appsv1beta1.CloneSetCondition, error) {
	condition, err := t.GetCloneSetCondition(name, appsv1beta1.CloneSetConditionTypeProgressing)
	if err != nil {
		return nil, err
	}
	if condition == nil {
		return nil, nil
	}
	condition.LastTransitionTime, condition.LastUpdateTime = metav1.Time{}, metav1.Time{}
	return condition, nil
}

func (t *CloneSetTester) GetCloneSetCondition(name string, conditionType appsv1beta1.CloneSetConditionType) (*appsv1beta1.CloneSetCondition, error) {
	cs, err := t.kc.AppsV1beta1().CloneSets(t.ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	klog.Infof("cloneset(%s/%s) status(%s)", cs.Namespace, cs.Name, util.DumpJSON(cs.Status))

	for i := range cs.Status.Conditions {
		c := cs.Status.Conditions[i]
		if c.Type == conditionType {
			return &c, nil
		}
	}
	return nil, nil
}

func (t *CloneSetTester) NewCloneSetAvailableCondition() *appsv1beta1.CloneSetCondition {
	return &appsv1beta1.CloneSetCondition{
		Type:    appsv1beta1.CloneSetConditionTypeProgressing,
		Status:  v1.ConditionTrue,
		Reason:  string(appsv1beta1.CloneSetAvailable),
		Message: "CloneSet is available",
	}
}

func (t *CloneSetTester) NewCloneSetUpdatedCondition() *appsv1beta1.CloneSetCondition {
	return &appsv1beta1.CloneSetCondition{
		Type:    appsv1beta1.CloneSetConditionTypeProgressing,
		Status:  v1.ConditionTrue,
		Reason:  string(appsv1beta1.CloneSetProgressUpdated),
		Message: "CloneSet is progressing",
	}
}

func (t *CloneSetTester) NewCloneSetDeadlineExceededCondition(revision string) *appsv1beta1.CloneSetCondition {
	return &appsv1beta1.CloneSetCondition{
		Type:    appsv1beta1.CloneSetConditionTypeProgressing,
		Status:  v1.ConditionFalse,
		Reason:  string(appsv1beta1.CloneSetProgressDeadlineExceeded),
		Message: fmt.Sprintf("CloneSet revision %s has timed out progressing", revision),
	}
}

func (t *CloneSetTester) NewCloneSetPausedCondition() *appsv1beta1.CloneSetCondition {
	return &appsv1beta1.CloneSetCondition{
		Type:    appsv1beta1.CloneSetConditionTypeProgressing,
		Status:  v1.ConditionTrue,
		Reason:  string(appsv1beta1.CloneSetProgressPaused),
		Message: "CloneSet is paused",
	}
}

func (t *CloneSetTester) NewCloneSetPartitionAvailableCondition() *appsv1beta1.CloneSetCondition {
	return &appsv1beta1.CloneSetCondition{
		Type:    appsv1beta1.CloneSetConditionTypeProgressing,
		Status:  v1.ConditionTrue,
		Reason:  string(appsv1beta1.CloneSetProgressPartitionAvailable),
		Message: "CloneSet has been paused due to partition ready",
	}
}
