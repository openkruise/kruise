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

package imagepuller

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	runtimeimage "github.com/openkruise/kruise/pkg/daemon/criruntime/imageruntime"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
)

const (
	// maxParallelism                          = 1000
	// maxParallelismPerImage                  = maxParallelism
	//defaultImagePullingProgressReadInterval = 5 * time.Millisecond
	defaultImagePullingProgressLogInterval = 5 * time.Second
	defaultImagePullingTimeout             = 10 * time.Minute
	defaultImagePullingBackoffLimit        = 3

	// Events
	PullImageSucceed = "PullImageSucceed"
	PullImageFailed  = "PullImageFailed"
)

type puller interface {
	Sync(obj *appsv1alpha1.NodeImage, ref *v1.ObjectReference) error
	GetStatus(imageName string) *appsv1alpha1.ImageStatus
}

type realPuller struct {
	sync.Mutex
	runtime       runtimeimage.ImageService
	secretManager daemonutil.SecretManager
	eventRecorder record.EventRecorder

	workerPools map[string]workerPool
}

var _ puller = &realPuller{}

func newRealPuller(runtime runtimeimage.ImageService, secretManager daemonutil.SecretManager, eventRecorder record.EventRecorder) (*realPuller, error) {
	p := &realPuller{
		runtime:       runtime,
		secretManager: secretManager,
		eventRecorder: eventRecorder,
		workerPools:   make(map[string]workerPool),
	}
	return p, nil
}

// Sync all images to pull
func (p *realPuller) Sync(obj *appsv1alpha1.NodeImage, ref *v1.ObjectReference) error {
	klog.V(5).InfoS("sync puller", "spec", util.DumpJSON(obj))

	p.Lock()
	defer p.Unlock()
	// stop all workers not in the spec
	for imageName := range p.workerPools {
		if _, ok := obj.Spec.Images[imageName]; !ok {
			klog.V(3).InfoS("stop workerpool", "imageName", imageName)
			pool := p.workerPools[imageName]
			delete(p.workerPools, imageName)
			pool.Stop()
		}
	}
	var ret error
	for imageName, imageSpec := range obj.Spec.Images {
		pool, ok := p.workerPools[imageName]
		if !ok {
			klog.V(3).InfoS("starting new workerpool", "imageName", imageName)
			pool = newRealWorkerPool(imageName, p.runtime, p.secretManager, p.eventRecorder)
			p.workerPools[imageName] = pool
		}
		var imageStatus *appsv1alpha1.ImageStatus
		if s, ok := obj.Status.ImageStatuses[imageName]; ok {
			imageStatus = &s
		}
		if err := pool.Sync(&imageSpec, imageStatus, ref); err != nil {
			ret = err
		}
	}
	return ret
}

func (p *realPuller) GetStatus(imageName string) *appsv1alpha1.ImageStatus {
	p.Lock()
	defer p.Unlock()
	pool, ok := p.workerPools[imageName]
	if !ok {
		return nil
	}
	return pool.GetStatus()
}

type imageStatusUpdater interface {
	UpdateStatus(*appsv1alpha1.ImageTagStatus)
}

type workerPool interface {
	Sync(spec *appsv1alpha1.ImageSpec, status *appsv1alpha1.ImageStatus, ref *v1.ObjectReference) error
	GetStatus() *appsv1alpha1.ImageStatus
	Stop()
}

type realWorkerPool struct {
	sync.Mutex

	name          string
	runtime       runtimeimage.ImageService
	secretManager daemonutil.SecretManager
	eventRecorder record.EventRecorder
	pullWorkers   map[string]*pullWorker
	tagStatuses   map[string]*appsv1alpha1.ImageTagStatus
	active        bool

	lastSyncSpec *appsv1alpha1.ImageSpec
}

func newRealWorkerPool(name string, runtime runtimeimage.ImageService, secretManager daemonutil.SecretManager, eventRecorder record.EventRecorder) *realWorkerPool {
	w := &realWorkerPool{
		name:          name,
		runtime:       runtime,
		secretManager: secretManager,
		eventRecorder: eventRecorder,
		pullWorkers:   make(map[string]*pullWorker),
		tagStatuses:   make(map[string]*appsv1alpha1.ImageTagStatus),
		active:        true,
	}
	return w
}

func (w *realWorkerPool) Sync(spec *appsv1alpha1.ImageSpec, status *appsv1alpha1.ImageStatus, ref *v1.ObjectReference) error {
	if !w.active {
		klog.InfoS("workerPool has exited", "name", w.name)
		return nil
	}

	klog.V(5).InfoS("sync worker pool", "name", w.name)

	secrets, err := w.secretManager.GetSecrets(spec.PullSecrets)
	if err != nil {
		klog.ErrorS(err, "failed to get secrets", "pullSecrets", spec.PullSecrets)
		return err
	}

	w.Lock()
	defer w.Unlock()
	w.lastSyncSpec = spec.DeepCopy()

	allTags := sets.NewString()
	activeTags := make(map[string]appsv1alpha1.ImageTagSpec)
	for _, tagSpec := range spec.Tags {
		var tagStatus *appsv1alpha1.ImageTagStatus
		if status != nil {
			for i := range status.Tags {
				if status.Tags[i].Tag == tagSpec.Tag && status.Tags[i].Version == tagSpec.Version {
					tagStatus = &status.Tags[i]
					break
				}
			}
		}
		if tagStatus == nil || tagStatus.CompletionTime == nil {
			activeTags[tagSpec.Tag] = tagSpec
		} else {
			w.tagStatuses[tagStatus.Tag] = tagStatus
		}
		allTags.Insert(tagSpec.Tag)
	}
	// Stop workers not active or version changed
	for tag, worker := range w.pullWorkers {
		tagSpec, ok := activeTags[tag]
		if !ok {
			klog.V(4).InfoS("stopping worker which is not in spec", "imageRef", worker.ImageRef())
			delete(w.pullWorkers, tag)
			worker.Stop()
		} else if tagSpec.Version != worker.tagSpec.Version {
			klog.V(4).InfoS("stopping worker with old version", "imageRef", worker.ImageRef(), "old", worker.tagSpec.Version, "new", tagSpec.Version)
			delete(w.pullWorkers, tag)
			worker.Stop()
		}
	}
	// Remove statuses not in spec
	for tag := range w.tagStatuses {
		if !allTags.Has(tag) {
			delete(w.tagStatuses, tag)
		}
	}
	// Start new workers
	for _, tagSpec := range activeTags {
		_, ok := w.pullWorkers[tagSpec.Tag]

		if !ok {
			worker := newPullWorker(w.name, tagSpec, spec.SandboxConfig, secrets, w.runtime, w, ref, w.eventRecorder)
			w.pullWorkers[tagSpec.Tag] = worker
		}
	}

	return nil
}

func (w *realWorkerPool) GetStatus() *appsv1alpha1.ImageStatus {
	w.Lock()
	defer w.Unlock()
	if w.lastSyncSpec == nil {
		return nil
	}

	// Keep the order of spec unchanged
	var tagsStatus []appsv1alpha1.ImageTagStatus
	for _, tagSpec := range w.lastSyncSpec.Tags {
		if status, ok := w.tagStatuses[tagSpec.Tag]; ok {
			tagsStatus = append(tagsStatus, *status)
		}
	}
	if tagsStatus == nil {
		return nil
	}
	return &appsv1alpha1.ImageStatus{Tags: tagsStatus}
}

func (w *realWorkerPool) Stop() {
	w.Lock()
	defer w.Unlock()
	if w.active {
		for _, worker := range w.pullWorkers {
			worker.Stop()
		}
		w.active = false
	}
}

func (w *realWorkerPool) UpdateStatus(status *appsv1alpha1.ImageTagStatus) {
	w.Lock()
	defer w.Unlock()
	if !w.active {
		return
	}
	w.tagStatuses[status.Tag] = status
}

func newPullWorker(name string, tagSpec appsv1alpha1.ImageTagSpec, sandboxConfig *appsv1alpha1.SandboxConfig, secrets []v1.Secret, runtime runtimeimage.ImageService, statusUpdater imageStatusUpdater, ref *v1.ObjectReference, eventRecorder record.EventRecorder) *pullWorker {
	o := &pullWorker{
		name:          name,
		tagSpec:       tagSpec,
		sandboxConfig: sandboxConfig,
		secrets:       secrets,
		runtime:       runtime,
		statusUpdater: statusUpdater,
		ref:           ref,
		eventRecorder: eventRecorder,
		active:        true,
		stopCh:        make(chan struct{}),
	}
	go o.Run()
	return o
}

type pullWorker struct {
	sync.Mutex

	name          string
	tagSpec       appsv1alpha1.ImageTagSpec
	sandboxConfig *appsv1alpha1.SandboxConfig
	secrets       []v1.Secret
	runtime       runtimeimage.ImageService
	statusUpdater imageStatusUpdater
	ref           *v1.ObjectReference
	eventRecorder record.EventRecorder

	active bool
	stopCh chan struct{}
}

func (w *pullWorker) ImageRef() string {
	return fmt.Sprintf("%v:%v", w.name, w.tagSpec.Tag)
}

func (w *pullWorker) Stop() {
	w.Lock()
	defer w.Unlock()
	if w.active {
		klog.InfoS("Worker to pull image is stopped", "name", w.name, "tag", w.tagSpec.Tag)
		w.active = false
		close(w.stopCh)
	}
}

func (w *pullWorker) IsActive() bool {
	return w.active
}

func (w *pullWorker) Run() {
	klog.V(3).InfoS("starting worker", "image", w.ImageRef(), "version", w.tagSpec.Version)

	tag := w.tagSpec.Tag
	startTime := metav1.Now()
	newStatus := &appsv1alpha1.ImageTagStatus{
		Tag:       tag,
		Phase:     appsv1alpha1.ImagePhasePulling,
		StartTime: &startTime,
		Version:   w.tagSpec.Version,
	}

	// We should update the image status when we start pulling images,
	// which can meet the scenario that some large size images cannot return the result from CRI.PullImage within 60s. For one reason:
	// For nodeimage controller will mark image:tag task failed (not responded for a long time) if daemon does not report status in 60s.
	// Ref: https://github.com/openkruise/kruise/issues/1273
	w.statusUpdater.UpdateStatus(newStatus)

	defer func() {
		cost := time.Since(startTime.Time)
		if newStatus.Phase == appsv1alpha1.ImagePhaseFailed {
			klog.ErrorS(errors.New(newStatus.Message), "Worker failed to pull image", "name", w.name, "tag", tag, "cost", cost)
		} else {
			klog.InfoS("Successfully pull image", "name", w.name, "tag", tag, "cost", cost)
		}
		if w.IsActive() {
			w.statusUpdater.UpdateStatus(newStatus)
		}
	}()

	timeout := defaultImagePullingTimeout
	if w.tagSpec.PullPolicy != nil && w.tagSpec.PullPolicy.TimeoutSeconds != nil {
		timeout = time.Duration(*w.tagSpec.PullPolicy.TimeoutSeconds) * time.Second
	}
	backoffLimit := defaultImagePullingBackoffLimit
	if w.tagSpec.PullPolicy != nil && w.tagSpec.PullPolicy.BackoffLimit != nil {
		backoffLimit = int(*w.tagSpec.PullPolicy.BackoffLimit)
	}
	if backoffLimit < 0 {
		backoffLimit = defaultImagePullingBackoffLimit
	}
	var deadline *time.Time
	if w.tagSpec.PullPolicy != nil && w.tagSpec.PullPolicy.ActiveDeadlineSeconds != nil {
		d := startTime.Time.Add(time.Duration(*w.tagSpec.PullPolicy.ActiveDeadlineSeconds) * time.Second)
		deadline = &d
	}

	var (
		step       = time.Second
		maxBackoff = 30 * time.Second
	)

	var lastError error
	for i := 0; i <= backoffLimit; i++ {
		onceTimeout := timeout
		if deadline != nil {
			if deadlineLeft := time.Since(*deadline); deadlineLeft >= 0 {
				lastError = fmt.Errorf("pulling exceeds the activeDeadlineSeconds")
				break
			} else if (-deadlineLeft) < onceTimeout {
				onceTimeout = -deadlineLeft
			}
		}

		pullContext, cancel := context.WithTimeout(context.Background(), onceTimeout)
		lastError = w.doPullImage(pullContext, newStatus, w.tagSpec.ImagePullPolicy)
		if lastError != nil {
			cancel()
			if !w.IsActive() {
				break
			}

			klog.ErrorS(lastError, "Pulling image backoff", "name", w.name, "tag", tag, "backoff", i+1)
			time.Sleep(step)
			step = minDuration(2*step, maxBackoff)
			continue
		}

		if imageInfo, err := w.getImageInfo(pullContext); err == nil {
			newStatus.ImageID = fmt.Sprintf("%v@%v", w.name, imageInfo.ID)
		}
		w.finishPulling(newStatus, appsv1alpha1.ImagePhaseSucceeded, "")
		if w.ref != nil && w.eventRecorder != nil {
			w.eventRecorder.Eventf(w.ref, v1.EventTypeNormal, PullImageSucceed, "Image %v:%v, ecalpsedTime %v", w.name, w.tagSpec.Tag, time.Since(startTime.Time))
		}
		cancel()
		return
	}
	w.finishPulling(newStatus, appsv1alpha1.ImagePhaseFailed, lastError.Error())

	if w.eventRecorder != nil {
		for _, owner := range w.tagSpec.OwnerReferences {
			w.eventRecorder.Eventf(&owner, v1.EventTypeWarning, PullImageFailed, "Image %v:%v %v", w.name, w.tagSpec.Tag, lastError.Error())
		}
		if w.ref != nil {
			w.eventRecorder.Eventf(w.ref, v1.EventTypeWarning, PullImageFailed, "Image %v:%v %v", w.name, w.tagSpec.Tag, lastError.Error())
		}
	}
}

func (w *pullWorker) getImageInfo(ctx context.Context) (*runtimeimage.ImageInfo, error) {
	imageInfos, err := w.runtime.ListImages(ctx)
	if err != nil {
		klog.V(5).ErrorS(err, "List images failed")
		return nil, err
	}
	for _, info := range imageInfos {
		if info.ContainsImage(w.name, w.tagSpec.Tag) {
			return &info, nil
		}
	}
	return nil, fmt.Errorf("image %v:%v not found", w.name, w.tagSpec.Tag)
}

// Pulling image and update process in status
func (w *pullWorker) doPullImage(ctx context.Context, newStatus *appsv1alpha1.ImageTagStatus, imagePullPolicy appsv1alpha1.ImagePullPolicy) (err error) {
	tag := w.tagSpec.Tag
	startTime := metav1.Now()

	klog.InfoS("Worker is starting to pull image", "name", w.name, "tag", tag, "version", w.tagSpec.Version)

	if info, _ := w.getImageInfo(ctx); info != nil && imagePullPolicy == appsv1alpha1.PullIfNotPresent {
		klog.InfoS("Image is already exists", "name", w.name, "tag", tag)
		newStatus.Progress = 100
		return nil
	}

	// make it asynchronous for CRI runtime will block in pulling image
	var statusReader runtimeimage.ImagePullStatusReader
	pullChan := make(chan struct{})
	go func() {
		statusReader, err = w.runtime.PullImage(ctx, w.name, tag, w.secrets, w.sandboxConfig)
		close(pullChan)
	}()

	closeStatusReader := func() {
		select {
		case <-pullChan:
		}
		if statusReader != nil {
			statusReader.Close()
		}
	}

	select {
	case <-w.stopCh:
		go closeStatusReader()
		klog.V(2).InfoS("Pulling image stopped", "name", w.name, "tag", tag)
		return fmt.Errorf("pulling image %s:%s is stopped", w.name, tag)
	case <-ctx.Done():
		go closeStatusReader()
		klog.V(2).InfoS("Pulling image canceled", "name", w.name, "tag", tag)
		return fmt.Errorf("pulling image %s:%s is canceled", w.name, tag)
	case <-pullChan:
		if err != nil {
			return err
		}
	}
	defer statusReader.Close()

	progress := 0
	var progressInfo string
	logTicker := time.NewTicker(defaultImagePullingProgressLogInterval)
	defer logTicker.Stop()

	for {
		select {
		case <-w.stopCh:
			klog.V(2).InfoS("Pulling image stopped", "name", w.name, "tag", tag)
			return fmt.Errorf("pulling image %s:%s is stopped", w.name, tag)
		case <-ctx.Done():
			klog.V(2).InfoS("Pulling image canceled", w.name, tag)
			return fmt.Errorf("pulling image %s:%s is canceled", w.name, tag)
		case <-logTicker.C:
			klog.V(2).InfoS("Pulling image", "name", w.name, "tag", tag, "cost", time.Since(startTime.Time), "progress", progress, "detail", progressInfo)
		case progressStatus, ok := <-statusReader.C():
			if !ok {
				return fmt.Errorf("pulling image %s:%s internal error", w.name, tag)
			}
			progress = progressStatus.Process
			progressInfo = progressStatus.DetailInfo
			newStatus.Progress = int32(progressStatus.Process)
			klog.V(5).InfoS("Pulling image", "name", w.name, "tag", tag, "cost", time.Since(startTime.Time), "progress", progress, "detail", progressInfo)
			if progressStatus.Finish {
				if progressStatus.Err == nil {
					return nil
				}
				return fmt.Errorf("pulling image %s:%s error %v", w.name, tag, progressStatus.Err)
			}
			w.statusUpdater.UpdateStatus(newStatus)
		}
	}
}

func (w *pullWorker) finishPulling(newStatus *appsv1alpha1.ImageTagStatus, phase appsv1alpha1.ImagePullPhase, message string) {
	newStatus.Phase = phase
	now := metav1.Now()
	newStatus.CompletionTime = &now
	newStatus.Message = message
	//klog.V(5).Infof("pulling image %v finished, status=%#v", w.ImageRef(), newStatus)
	//w.statusUpdater.UpdateStatus(newStatus)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
