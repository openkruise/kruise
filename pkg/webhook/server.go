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

package webhook

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	"github.com/openkruise/kruise/pkg/webhook/types"
	webhookcontroller "github.com/openkruise/kruise/pkg/webhook/util/controller"
	"github.com/openkruise/kruise/pkg/webhook/util/health"
)

type GateFunc func() (enabled bool)
type HandlerPath2GetterMap = map[string]types.HandlerGetter

var (
	// HandlerGetterMap contains all admission webhook handlers.
	HandlerMap   = HandlerPath2GetterMap{}
	handlerGates = map[string]GateFunc{}
)

func addHandlers(m HandlerPath2GetterMap) {
	addHandlersWithGate(m, nil)
}

func addHandlersWithGate(m HandlerPath2GetterMap, fn GateFunc) {
	for path, handler := range m {
		if len(path) == 0 {
			klog.Warning("Skip handler with empty path")
			continue
		}
		if path[0] != '/' {
			path = "/" + path
		}
		_, found := HandlerMap[path]
		if found {
			klog.V(1).InfoS("conflicting webhook builder path in handler map", "path", path)
		}
		HandlerMap[path] = handler
		if fn != nil {
			handlerGates[path] = fn
		}
	}
}

func filterActiveHandlers() {
	disablePaths := sets.NewString()
	for path := range HandlerMap {
		if fn, ok := handlerGates[path]; ok {
			if !fn() {
				disablePaths.Insert(path)
			}
		}
	}
	for _, path := range disablePaths.List() {
		delete(HandlerMap, path)
	}
}

func SetupWithManager(mgr manager.Manager) error {
	server := mgr.GetWebhookServer()

	// register admission handlers
	filterActiveHandlers()
	for path, handlerGetter := range HandlerMap {
		server.Register(path, &webhook.Admission{Handler: handlerGetter(mgr)})
		klog.V(3).InfoS("Registered webhook handler", "path", path)
	}

	// register conversion webhook
	server.Register("/convert", conversion.NewWebhookHandler(mgr.GetScheme()))

	// register health handler
	server.Register("/healthz", &health.Handler{})

	return nil
}

// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;update;patch

func Initialize(ctx context.Context, cfg *rest.Config, webhookInitializeTime time.Duration) error {
	c, err := webhookcontroller.New(cfg, HandlerMap)
	if err != nil {
		return err
	}
	go func() {
		c.Start(ctx)
	}()

	timer := time.NewTimer(webhookInitializeTime)
	defer timer.Stop()
	select {
	case <-webhookcontroller.Inited():
		return nil
	case <-timer.C:
		return fmt.Errorf("failed to start webhook controller for waiting more than %fs", webhookInitializeTime.Seconds())
	}
}

func Checker(req *http.Request) error {
	// Firstly wait webhook controller initialized
	select {
	case <-webhookcontroller.Inited():
	default:
		return fmt.Errorf("webhook controller has not initialized")
	}
	return health.Checker(req)
}

func WaitReady() error {
	startTS := time.Now()
	var err error
	for {
		duration := time.Since(startTS)
		if err = Checker(nil); err == nil {
			return nil
		}

		if duration > time.Second*5 {
			klog.ErrorS(err, "Failed to wait webhook ready", "duration", duration)
		}
		time.Sleep(time.Second * 2)
	}

}
