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

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	kruiseapis "github.com/openkruise/kruise/apis"
	"github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/daemon/containermeta"
	"github.com/openkruise/kruise/pkg/daemon/containerrecreate"
	daemonruntime "github.com/openkruise/kruise/pkg/daemon/criruntime"
	"github.com/openkruise/kruise/pkg/daemon/imagepuller"
	daemonoptions "github.com/openkruise/kruise/pkg/daemon/options"
	"github.com/openkruise/kruise/pkg/daemon/podprobe"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	varRunMountPath = "/hostvarrun"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kruiseapis.AddToScheme(scheme))
}

// Runnable allows a component to be started.
// It's very important that Start blocks until
// it's done running.
type Runnable interface {
	// Run starts running the component. The component will stop running
	// when the channel is closed. Run blocks until the channel is closed or
	// an error occurs.
	Run(<-chan struct{})
}

// Daemon is interface for process to run every node
type Daemon interface {
	Run(ctx context.Context) error
}

type daemon struct {
	runtimeFactory daemonruntime.Factory
	podInformer    cache.SharedIndexInformer
	runnables      []Runnable

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
	klog.InfoS("Starting daemon", "nodeName", nodeName)

	listener, err := net.Listen("tcp", bindAddress)
	if err != nil {
		return nil, fmt.Errorf("new listener error: %v", err)
	}

	healthz := daemonutil.NewHealthz()

	runtimeClient, err := runtimeclient.New(cfg, runtimeclient.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to new controller-runtime client: %v", err)
	}

	genericClient := client.GetGenericClient()
	if genericClient == nil || genericClient.KubeClient == nil || genericClient.KruiseClient == nil {
		return nil, fmt.Errorf("generic client can not be nil")
	}

	var podInformer cache.SharedIndexInformer
	if utilfeature.DefaultFeatureGate.Enabled(features.DaemonWatchingPod) {
		podInformer = newPodInformer(genericClient.KubeClient, nodeName)
	}

	accountManager := daemonutil.NewImagePullAccountManager(genericClient.KubeClient)
	runtimeFactory, err := daemonruntime.NewFactory(varRunMountPath, accountManager)
	if err != nil {
		return nil, fmt.Errorf("failed to new runtime factory: %v", err)
	}

	secretManager := daemonutil.NewCacheBasedSecretManager(genericClient.KubeClient)

	opts := daemonoptions.Options{
		NodeName:       nodeName,
		Scheme:         scheme,
		RuntimeClient:  runtimeClient,
		PodInformer:    podInformer,
		RuntimeFactory: runtimeFactory,
		Healthz:        healthz,
	}

	puller, err := imagepuller.NewController(opts, secretManager, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to new image puller controller: %v", err)
	}

	crrController, err := containerrecreate.NewController(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to new crr daemon controller: %v", err)
	}

	var runnables = []Runnable{
		puller,
		crrController,
	}

	// node pod probe
	if utilfeature.DefaultFeatureGate.Enabled(features.PodProbeMarkerGate) {
		nppController, err := podprobe.NewController(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to new nodePodProbe daemon controller: %v", err)
		}
		runnables = append(runnables, nppController)
	}

	if utilfeature.DefaultFeatureGate.Enabled(features.DaemonWatchingPod) {
		containerMetaController, err := containermeta.NewController(opts)
		if err != nil {
			return nil, fmt.Errorf("failed to new containermeta controller: %v", err)
		}
		runnables = append(runnables, containerMetaController)
	}

	return &daemon{
		runtimeFactory: runtimeFactory,
		podInformer:    podInformer,
		runnables:      runnables,
		listener:       listener,
		healthz:        healthz,
		errSignal:      &errSignaler{errSignal: make(chan struct{})},
	}, nil
}

func newPodInformer(client clientset.Interface, nodeName string) cache.SharedIndexInformer {
	tweakListOptionsFunc := func(opt *metav1.ListOptions) {
		opt.FieldSelector = "spec.nodeName=" + nodeName
	}
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				tweakListOptionsFunc(&options)
				return client.CoreV1().Pods(v1.NamespaceAll).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				tweakListOptionsFunc(&options)
				return client.CoreV1().Pods(v1.NamespaceAll).Watch(context.TODO(), options)
			},
		},
		&v1.Pod{},
		0, // do not resync
		cache.Indexers{},
	)
}

func (d *daemon) Run(ctx context.Context) error {
	if d.podInformer != nil {
		go d.podInformer.Run(ctx.Done())
		if !cache.WaitForCacheSync(ctx.Done(), d.podInformer.HasSynced) {
			return fmt.Errorf("error waiting for pod informer synced")
		}
	}

	go d.serve(ctx)
	for _, r := range d.runnables {
		go r.Run(ctx.Done())
	}

	select {
	case <-ctx.Done():
		// We are done
		return nil
	case <-d.errSignal.GotError():
		// Error starting a controller
		return d.errSignal.Error()
	}
}

func (d *daemon) serve(ctx context.Context) {
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
	<-ctx.Done()
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
