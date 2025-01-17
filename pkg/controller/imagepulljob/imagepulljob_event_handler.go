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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	kruiseutil "github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/expectations"
	utilimagejob "github.com/openkruise/kruise/pkg/util/imagejob"
)

type nodeImageEventHandler struct {
	client.Reader
}

var _ handler.TypedEventHandler[*appsv1alpha1.NodeImage] = &nodeImageEventHandler{}

func (e *nodeImageEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*appsv1alpha1.NodeImage], q workqueue.RateLimitingInterface) {
	obj := evt.Object
	e.handle(obj, q)
}

func (e *nodeImageEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*appsv1alpha1.NodeImage], q workqueue.RateLimitingInterface) {
	obj := evt.ObjectNew
	oldObj := evt.ObjectOld
	if obj.DeletionTimestamp != nil {
		e.handle(obj, q)
	} else {
		e.handleUpdate(obj, oldObj, q)
	}
}

func (e *nodeImageEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*appsv1alpha1.NodeImage], q workqueue.RateLimitingInterface) {
	obj := evt.Object
	resourceVersionExpectations.Delete(obj)
	e.handle(obj, q)
}

func (e *nodeImageEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*appsv1alpha1.NodeImage], q workqueue.RateLimitingInterface) {
}

func (e *nodeImageEventHandler) handle(nodeImage *appsv1alpha1.NodeImage, q workqueue.RateLimitingInterface) {
	// Get jobs related to this NodeImage
	jobs, _, err := utilimagejob.GetActiveJobsForNodeImage(e.Reader, nodeImage, nil)
	if err != nil {
		klog.ErrorS(err, "Failed to get jobs for NodeImage", "nodeImageName", nodeImage.Name)
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
	klog.V(5).InfoS("Found NodeImage updated and only affect images", "nodeImageName", nodeImage.Name, "changedImages", changedImages.List())

	// Get jobs related to this NodeImage
	newJobs, oldJobs, err := utilimagejob.GetActiveJobsForNodeImage(e.Reader, nodeImage, oldNodeImage)
	if err != nil {
		klog.ErrorS(err, "Failed to get jobs for NodeImage", "nodeImageName", nodeImage.Name)
	}
	diffSet := diffJobs(newJobs, oldJobs)
	for _, j := range newJobs {
		imageName, _, err := daemonutil.NormalizeImageRefToNameTag(j.Spec.Image)
		if err != nil {
			klog.InfoS("Invalid image in job", "image", j.Spec.Image, "imagePullJob", klog.KObj(j))
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

var _ handler.TypedEventHandler[*v1.Pod] = &podEventHandler{}

func (e *podEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	obj := evt.Object
	e.handle(obj, q)
}

func (e *podEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	obj := evt.ObjectNew
	oldObj := evt.ObjectOld
	if obj.DeletionTimestamp != nil {
		e.handle(obj, q)
	} else {
		e.handleUpdate(obj, oldObj, q)
	}
}

func (e *podEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
	obj := evt.Object
	e.handle(obj, q)
}

func (e *podEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*v1.Pod], q workqueue.RateLimitingInterface) {
}

func (e *podEventHandler) handle(pod *v1.Pod, q workqueue.RateLimitingInterface) {
	if pod.Spec.NodeName == "" {
		return
	}
	// Get jobs related to this Pod
	jobs, _, err := utilimagejob.GetActiveJobsForPod(e.Reader, pod, nil)
	if err != nil {
		klog.ErrorS(err, "Failed to get jobs for Pod", "pod", klog.KObj(pod))
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
		klog.ErrorS(err, "Failed to get jobs for Pod", "pod", klog.KObj(pod))
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

var _ handler.TypedEventHandler[*v1.Secret] = &secretEventHandler{}

func (e *secretEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[*v1.Secret], q workqueue.RateLimitingInterface) {
	obj := evt.Object
	e.handle(obj, q)
}

func (e *secretEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[*v1.Secret], q workqueue.RateLimitingInterface) {
	newObj := evt.ObjectNew
	oldObj := evt.ObjectOld
	e.handleUpdate(newObj, oldObj, q)
}

func (e *secretEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[*v1.Secret], q workqueue.RateLimitingInterface) {
}

func (e *secretEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[*v1.Secret], q workqueue.RateLimitingInterface) {
}

func (e *secretEventHandler) handle(secret *v1.Secret, q workqueue.RateLimitingInterface) {
	if secret != nil && secret.Namespace == kruiseutil.GetKruiseDaemonConfigNamespace() {
		jobKeySet := referenceSetFromTarget(secret)
		klog.V(4).InfoS("Observed Secret created", "secret", klog.KObj(secret), "secretUID", secret.UID, "jobRefs", jobKeySet)
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
		klog.ErrorS(err, "Failed to get jobs for Secret", "secret", klog.KObj(secret))
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
		klog.ErrorS(err, "Failed to get jobs for Secret", "secret", klog.KObj(secretNew))
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
