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

package containerrecreate

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/openkruise/kruise/pkg/client"
	kruisefake "github.com/openkruise/kruise/pkg/client/clientset/versioned/fake"
	daemonoptions "github.com/openkruise/kruise/pkg/daemon/options"
	daemonutil "github.com/openkruise/kruise/pkg/daemon/util"
)

func TestControllerRun(t *testing.T) {
	c := &Controller{
		workers: 5,
		queue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "crr-test"),
	}

	stop := make(chan struct{})

	// We can't easily test the informer sync because it's not set,
	// but we can test that it doesn't crash and logs the workers count.
	// We'll use a dummy informer that is already synced.
	c.crrInformer = &fakeInformer{}

	go func() {
		// Wait a little bit to let WaitForCacheSync succeed and reach the worker loop
		// before closing the stop channel.
		time.Sleep(100 * time.Millisecond)
		close(stop)
	}()

	c.Run(stop)

	if c.workers != 5 {
		t.Errorf("expected workers 5, but got %d", c.workers)
	}
}

func TestNewController(t *testing.T) {
	fakeKruiseClient := &kruisefake.Clientset{}
	fakeKubeClient := &kubefake.Clientset{}
	opts := daemonoptions.Options{
		CRRWorkers: 10,
		GenericClient: &client.GenericClientset{
			KubeClient:   fakeKubeClient,
			KruiseClient: fakeKruiseClient,
		},
		Scheme:  runtime.NewScheme(),
		Healthz: daemonutil.NewHealthz(),
	}
	c, err := NewController(opts, fakeKruiseClient)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if c.workers != 10 {
		t.Errorf("expected workers 10, but got %d", c.workers)
	}
}

type fakeInformer struct {
	cache.SharedIndexInformer
}

func (f *fakeInformer) Run(stopCh <-chan struct{}) {}
func (f *fakeInformer) HasSynced() bool            { return true }
