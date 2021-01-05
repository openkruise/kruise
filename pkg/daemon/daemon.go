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

package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/daemon/imagepuller"
	daemonruntime "github.com/openkruise/kruise/pkg/daemon/runtime"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	varRunMountPath = "/hostvarrun"
)

// Daemon is interface for process to run every node
type Daemon interface {
	Run(stop <-chan struct{}) error
}

type daemon struct {
	runtimeFactory   daemonruntime.Factory
	pullerController *imagepuller.Controller

	listener  net.Listener
	healthz   *daemonutil.Healthz
	errSignal *errSignaler
}

// NewDaemon create a daemon
func NewDaemon(cfg *rest.Config, bindAddress string) (Daemon, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cfg can not be nil")
	}

	nodeName, err := daemonutil.NodeName()
	if err != nil {
		return nil, err
	}
	klog.Infof("Starting daemon on %v ...", nodeName)

	listener, err := net.Listen("tcp", bindAddress)
	if err != nil {
		return nil, fmt.Errorf("new listener error: %v", err)
	}

	healthz := daemonutil.NewHealthz()

	genericClient := client.GetGenericClient()
	if genericClient == nil || genericClient.KubeClient == nil || genericClient.KruiseClient == nil {
		return nil, fmt.Errorf("generic client can not be nil")
	}

	accountManager := daemonutil.NewImagePullAccountManager(genericClient.KubeClient)
	runtimeFactory, err := daemonruntime.NewFactory(varRunMountPath, accountManager)
	if err != nil {
		return nil, fmt.Errorf("failed to new runtime factory: %v", err)
	}

	secretManager := daemonutil.NewCacheBasedSecretManager(genericClient.KubeClient)
	puller, err := imagepuller.NewController(runtimeFactory, secretManager, healthz)
	if err != nil {
		return nil, fmt.Errorf("failed to new image puller controller: %v", err)
	}

	return &daemon{
		runtimeFactory:   runtimeFactory,
		pullerController: puller,

		listener:  listener,
		healthz:   healthz,
		errSignal: &errSignaler{errSignal: make(chan struct{})},
	}, nil
}

func (d *daemon) Run(stop <-chan struct{}) error {
	go d.serve(stop)
	go d.pullerController.Run(stop)

	select {
	case <-stop:
		// We are done
		return nil
	case <-d.errSignal.GotError():
		// Error starting a controller
		return d.errSignal.Error()
	}
}

func (d *daemon) serve(stop <-chan struct{}) {
	handler := promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
	mux := http.NewServeMux()
	mux.Handle("/metrics", handler)
	mux.HandleFunc("/healthz", d.healthz.Handler)
	server := http.Server{
		Handler: mux,
	}
	// Run the server
	go func() {
		if err := server.Serve(d.listener); err != nil && err != http.ErrServerClosed {
			d.errSignal.SignalError(err)
		}
	}()

	// Shutdown the server when stop is closed
	<-stop
	if err := server.Shutdown(context.Background()); err != nil {
		d.errSignal.SignalError(err)
	}
}

type errSignaler struct {
	// errSignal indicates that an error occurred, when closed.  It shouldn't
	// be written to.
	errSignal chan struct{}

	// err is the received error
	err error

	mu sync.Mutex
}

func (r *errSignaler) SignalError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err == nil {
		// non-error, ignore
		klog.Error("SignalError called without an (with a nil) error, which should never happen, ignoring")
		return
	}

	if r.err != nil {
		// we already have an error, don't try again
		return
	}

	// save the error and report it
	r.err = err
	close(r.errSignal)
}

func (r *errSignaler) Error() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.err
}

func (r *errSignaler) GotError() chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.errSignal
}
