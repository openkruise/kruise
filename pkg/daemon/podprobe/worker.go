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

package podprobe

import (
	"fmt"
	"math/rand"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/util/runtime"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
)

// worker handles the periodic probing of its assigned container. Each worker has a go-routine
// associated with it which runs the probe loop until the container permanently terminates, or the
// stop channel is closed. The worker uses the probe Manager's statusManager to get up-to-date
// container IDs.
type worker struct {
	// Channel for stopping the probe.
	stopCh chan struct{}

	// pod uid, container name, probe name, ip
	key probeKey

	// Describes the probe configuration
	spec *appsv1alpha1.ContainerProbeSpec

	// The probe value during the initial delay.
	initialValue appsv1alpha1.ProbeState

	probeController *Controller

	// The last known container ID for this worker.
	containerID string
	// The last probe result for this worker.
	lastResult appsv1alpha1.ProbeState
	// How many times in a row the probe has returned the same result.
	resultRun int
}

// Creates and starts a new probe worker.
func newWorker(c *Controller, key probeKey, probe *appsv1alpha1.ContainerProbeSpec) *worker {

	w := &worker{
		stopCh:          make(chan struct{}, 1), // Buffer so stop() can be non-blocking.
		key:             key,
		spec:            probe,
		probeController: c,
		initialValue:    appsv1alpha1.ProbeUnknown,
	}

	return w
}

// run periodically probes the container.
func (w *worker) run() {
	periodSecond := w.spec.PeriodSeconds
	if periodSecond < 1 {
		periodSecond = 1
	}
	probeTickerPeriod := time.Duration(periodSecond) * time.Second
	// If kruise daemon restarted the probes could be started in rapid succession.
	// Let the worker wait for a random portion of tickerPeriod before probing.
	// Do it only if the kruise daemon has started recently.
	if probeTickerPeriod > time.Since(w.probeController.start) {
		time.Sleep(time.Duration(rand.Float64() * float64(probeTickerPeriod)))
	}
	probeTicker := time.NewTicker(probeTickerPeriod)
	defer func() {
		// Clean up.
		probeTicker.Stop()
		if w.containerID != "" {
			w.probeController.result.remove(w.containerID)
		}
		w.probeController.removeWorker(w.key)
	}()

probeLoop:
	for w.doProbe() {
		// Wait for next probe tick.
		select {
		case <-w.stopCh:
			break probeLoop
		case <-probeTicker.C:
		}
	}
}

// stop stops the probe worker. The worker handles cleanup and removes itself from its manager.
// It is safe to call stop multiple times.
func (w *worker) stop() {
	select {
	case w.stopCh <- struct{}{}:
	default: // Non-blocking.
	}
}

// doProbe probes the container once and records the result.
// Returns whether the worker should continue.
func (w *worker) doProbe() (keepGoing bool) {
	defer func() { recover() }() // Actually eat panics (HandleCrash takes care of logging)
	defer runtime.HandleCrash(func(_ interface{}) { keepGoing = true })

	container, _ := w.probeController.fetchLatestPodContainer(w.key.podUID, w.key.containerName)
	if container == nil {
		klog.V(5).InfoS("Pod container Not Found", "namespace", w.key.podNs, "podName", w.key.podName, "containerName", w.key.containerName)
		return true
	}

	if w.containerID != container.Id {
		if w.containerID != "" {
			w.probeController.result.remove(w.containerID)
		}
		klog.V(5).InfoS("Pod container Id changed", "namespace", w.key.podNs, "podName", w.key.podName, "from", w.containerID, "to", container.Id)
		w.containerID = container.Id
		w.probeController.result.set(w.containerID, w.key, w.initialValue, "")
	}
	if container.State != runtimeapi.ContainerState_CONTAINER_RUNNING {
		klog.V(5).InfoS("Pod Non-running container probed", "namespace", w.key.podNs, "podName", w.key.podName, "containerName", w.key.containerName)
		w.probeController.result.set(w.containerID, w.key, appsv1alpha1.ProbeFailed, fmt.Sprintf("Container(%s) is Non-running", w.key.containerName))
	}

	// Probe disabled for InitialDelaySeconds.
	initialDelay := w.spec.InitialDelaySeconds
	if initialDelay < 1 {
		initialDelay = 1
	}
	curDelay := int32(time.Since(time.Unix(0, container.StartedAt)).Seconds())
	if curDelay < initialDelay {
		klog.V(5).InfoS("Pod container probe initialDelay is smaller than curDelay",
			"namespace", w.key.podNs, "podName", w.key.podName, "containerName", w.key.containerName, "probeName", w.key.probeName, "initialDelay", initialDelay, "curDelay", curDelay)
		return true
	}

	// the full container environment here, OR we must make a call to the CRI in order to get those environment
	// values from the running container.
	result, msg, err := w.probeController.prober.probe(w.spec, w.key, container, w.containerID)
	if err != nil {
		klog.ErrorS(err, "Pod do container probe spec failed",
			"namespace", w.key.podNs, "podName", w.key.podName, "containerName", w.key.containerName, "probeName", w.key.probeName, "spec", util.DumpJSON(w.spec))
		return true
	}
	if w.lastResult == result {
		w.resultRun++
	} else {
		w.lastResult = result
		w.resultRun = 1
	}

	failureThreshold := w.spec.FailureThreshold
	if failureThreshold <= 0 {
		failureThreshold = 1
	}
	successThreshold := w.spec.SuccessThreshold
	if successThreshold <= 0 {
		successThreshold = 1
	}
	if (result == appsv1alpha1.ProbeFailed && w.resultRun < int(failureThreshold)) ||
		(result == appsv1alpha1.ProbeSucceeded && w.resultRun < int(successThreshold)) {
		// Success or failure is below threshold - leave the probe state unchanged.
		return true
	}
	w.probeController.result.set(w.containerID, w.key, result, msg)
	return true
}

func (w *worker) getProbeSpec() *appsv1alpha1.ContainerProbeSpec {
	return w.spec
}

func (w *worker) updateProbeSpec(spec *appsv1alpha1.ContainerProbeSpec) {
	if !reflect.DeepEqual(w.spec.ProbeHandler, spec.ProbeHandler) {
		if w.containerID != "" {
			klog.InfoS("Pod container probe spec changed", "podUID", w.key.podUID, "containerName", w.key.containerName)
			w.probeController.result.set(w.containerID, w.key, w.initialValue, "")
		}
	}
	w.spec = spec
}
