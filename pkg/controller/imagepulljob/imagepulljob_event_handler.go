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

package imagepulljob

import (
	"context"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	kruiseutil "github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilimagejob "github.com/openkruise/kruise/pkg/util/imagejob"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type nodeImageEventHandler struct {
	client.Reader
}

var _ handler.EventHandler = &nodeImageEventHandler{}

func (e *nodeImageEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	obj := evt.Object.(*appsv1alpha1.NodeImage)
	e.handle(obj, q)
}

func (e *nodeImageEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	obj := evt.ObjectNew.(*appsv1alpha1.NodeImage)
	oldObj := evt.ObjectOld.(*appsv1alpha1.NodeImage)
	if obj.DeletionTimestamp != nil {
		e.handle(obj, q)
	} else {
		e.handleUpdate(obj, oldObj, q)
	}
}

func (e *nodeImageEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	obj := evt.Object.(*appsv1alpha1.NodeImage)
	resourceVersionExpectations.Delete(obj)
	e.handle(obj, q)
}

func (e *nodeImageEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *nodeImageEventHandler) handle(nodeImage *appsv1alpha1.NodeImage, q workqueue.RateLimitingInterface) {
	// Get jobs related to this NodeImage
	jobs, _, err := utilimagejob.GetActiveJobsForNodeImage(e.Reader, nodeImage, nil)
	if err != nil {
		klog.Errorf("Failed to get jobs for NodeImage %s: %v", nodeImage.Name, err)
	}
	for _, j := range jobs {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: j.Namespace, Name: j.Name}})
	}
}

func (e *nodeImageEventHandler) handleUpdate(nodeImage, oldNodeImage *appsv1alpha1.NodeImage, q workqueue.RateLimitingInterface) {
	changedImages := sets.NewString()
	tmpOldNodeImage := oldNodeImage.DeepCopy()
	for name, imageSpec := range nodeImage.Spec.Images {
		oldImageSpec := tmpOldNodeImage.Spec.Images[name]
		delete(tmpOldNodeImage.Spec.Images, name)
		if !reflect.DeepEqual(imageSpec, oldImageSpec) {
			changedImages.Insert(name)
		}
	}
	for name := range tmpOldNodeImage.Spec.Images {
		changedImages.Insert(name)
	}
	for name, imageStatus := range nodeImage.Status.ImageStatuses {
		oldImageStatus := tmpOldNodeImage.Status.ImageStatuses[name]
		delete(tmpOldNodeImage.Status.ImageStatuses, name)
		if !reflect.DeepEqual(imageStatus, oldImageStatus) {
			changedImages.Insert(name)
		}
	}
	for name := range tmpOldNodeImage.Status.ImageStatuses {
		changedImages.Insert(name)
	}
	klog.V(5).Infof("Find NodeImage %s updated and only affect images: %v", nodeImage.Name, changedImages.List())

	// Get jobs related to this NodeImage
	newJobs, oldJobs, err := utilimagejob.GetActiveJobsForNodeImage(e.Reader, nodeImage, oldNodeImage)
	if err != nil {
		klog.Errorf("Failed to get jobs for NodeImage %s: %v", nodeImage.Name, err)
	}
	diffSet := diffJobs(newJobs, oldJobs)
	for _, j := range newJobs {
		imageName, _, err := daemonutil.NormalizeImageRefToNameTag(j.Spec.Image)
		if err != nil {
			klog.Warningf("Invalid image %s in job %s/%s", j.Spec.Image, j.Namespace, j.Name)
			continue
		}
		if changedImages.Has(imageName) {
			diffSet[types.NamespacedName{Namespace: j.Namespace, Name: j.Name}] = struct{}{}
		}
	}
	for name := range diffSet {
		q.Add(reconcile.Request{NamespacedName: name})
	}
}

type podEventHandler struct {
	client.Reader
}

var _ handler.EventHandler = &podEventHandler{}

func (e *podEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	obj := evt.Object.(*v1.Pod)
	e.handle(obj, q)
}

func (e *podEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	obj := evt.ObjectNew.(*v1.Pod)
	oldObj := evt.ObjectOld.(*v1.Pod)
	if obj.DeletionTimestamp != nil {
		e.handle(obj, q)
	} else {
		e.handleUpdate(obj, oldObj, q)
	}
}

func (e *podEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	obj := evt.Object.(*v1.Pod)
	e.handle(obj, q)
}

func (e *podEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *podEventHandler) handle(pod *v1.Pod, q workqueue.RateLimitingInterface) {
	if pod.Spec.NodeName == "" {
		return
	}
	// Get jobs related to this Pod
	jobs, _, err := utilimagejob.GetActiveJobsForPod(e.Reader, pod, nil)
	if err != nil {
		klog.Errorf("Failed to get jobs for Pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}
	for _, j := range jobs {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: j.Namespace, Name: j.Name}})
	}
}

func (e *podEventHandler) handleUpdate(pod, oldPod *v1.Pod, q workqueue.RateLimitingInterface) {
	if pod.Spec.NodeName == "" {
		return
	}
	if pod.Spec.NodeName == oldPod.Spec.NodeName && reflect.DeepEqual(pod.Labels, oldPod.Labels) {
		return
	}
	// Get jobs related to this NodeImage
	newJobs, oldJobs, err := utilimagejob.GetActiveJobsForPod(e.Reader, pod, oldPod)
	if err != nil {
		klog.Errorf("Failed to get jobs for Pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}
	if oldPod.Spec.NodeName == "" {
		for _, j := range newJobs {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: j.Namespace, Name: j.Name}})
		}
		return
	}
	diffSet := diffJobs(newJobs, oldJobs)
	for name := range diffSet {
		q.Add(reconcile.Request{NamespacedName: name})
	}
}

type secretEventHandler struct {
	client.Reader
}

var _ handler.EventHandler = &secretEventHandler{}

func (e *secretEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	obj := evt.Object.(*v1.Secret)
	e.handle(obj, q)
}

func (e *secretEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	newObj := evt.ObjectNew.(*v1.Secret)
	oldObj := evt.ObjectOld.(*v1.Secret)
	e.handleUpdate(newObj, oldObj, q)
}

func (e *secretEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

func (e *secretEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *secretEventHandler) handle(secret *v1.Secret, q workqueue.RateLimitingInterface) {
	if secret != nil && secret.Namespace == kruiseutil.GetKruiseDaemonConfigNamespace() {
		jobKeySet := referenceSetFromTarget(secret)
		klog.V(4).Infof("Observe secret %s/%s created, uid: %s, refs: %s", secret.Namespace, secret.Name, secret.UID, jobKeySet.String())
		for key := range jobKeySet {
			scaleExpectations.ObserveScale(key.String(), expectations.Create, secret.Labels[SourceSecretUIDLabelKey])
		}
		return
	}

	if secret == nil || secret.DeletionTimestamp != nil {
		return
	}
	// Get jobs related to this Secret
	jobKeys, err := e.getActiveJobKeysForSecret(secret)
	if err != nil {
		klog.Errorf("Failed to get jobs for Secret %s/%s: %v", secret.Namespace, secret.Name, err)
	}
	for _, jKey := range jobKeys {
		q.Add(reconcile.Request{NamespacedName: jKey})
	}
}

func (e *secretEventHandler) handleUpdate(secretNew, secretOld *v1.Secret, q workqueue.RateLimitingInterface) {
	if secretNew != nil && secretNew.Namespace == kruiseutil.GetKruiseDaemonConfigNamespace() {
		jobKeySet := referenceSetFromTarget(secretNew)
		for key := range jobKeySet {
			scaleExpectations.ObserveScale(key.String(), expectations.Create, secretNew.Labels[SourceSecretUIDLabelKey])
		}
		return
	}

	if secretOld == nil || secretNew == nil || secretNew.DeletionTimestamp != nil ||
		(reflect.DeepEqual(secretNew.Data, secretOld.Data) && reflect.DeepEqual(secretNew.StringData, secretOld.StringData)) {
		return
	}
	// Get jobs related to this Secret
	jobKeys, err := e.getActiveJobKeysForSecret(secretNew)
	if err != nil {
		klog.Errorf("Failed to get jobs for Secret %s/%s: %v", secretNew.Namespace, secretNew.Name, err)
	}
	for _, jKey := range jobKeys {
		q.Add(reconcile.Request{NamespacedName: jKey})
	}
}

func (e *secretEventHandler) getActiveJobKeysForSecret(secret *v1.Secret) ([]types.NamespacedName, error) {
	jobLister := &appsv1alpha1.ImagePullJobList{}
	if err := e.List(context.TODO(), jobLister, client.InNamespace(secret.Namespace), utilclient.DisableDeepCopy); err != nil {
		return nil, err
	}
	var jobKeys []types.NamespacedName
	for i := range jobLister.Items {
		job := &jobLister.Items[i]
		if job.DeletionTimestamp != nil {
			continue
		}
		if jobContainsSecret(job, secret.Name) {
			jobKeys = append(jobKeys, keyFromObject(job))
		}
	}
	return jobKeys, nil
}

func jobContainsSecret(job *appsv1alpha1.ImagePullJob, secretName string) bool {
	for _, s := range job.Spec.PullSecrets {
		if secretName == s {
			return true
		}
	}
	return false
}

func diffJobs(newJobs, oldJobs []*appsv1alpha1.ImagePullJob) set {
	setNew := make(set, len(newJobs))
	setOld := make(set, len(oldJobs))
	for _, j := range newJobs {
		setNew[types.NamespacedName{Namespace: j.Namespace, Name: j.Name}] = struct{}{}
	}
	for _, j := range oldJobs {
		setOld[types.NamespacedName{Namespace: j.Namespace, Name: j.Name}] = struct{}{}
	}
	ret := make(set)
	for name, v := range setNew {
		if _, ok := setOld[name]; !ok {
			ret[name] = v
		}
	}
	for name, v := range setOld {
		if _, ok := setNew[name]; !ok {
			ret[name] = v
		}
	}
	return ret
}

type set map[types.NamespacedName]struct{}
