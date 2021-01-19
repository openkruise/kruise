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
	"fmt"
	"time"

	"github.com/openkruise/kruise/pkg/util/gate"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	webhookcontroller "github.com/openkruise/kruise/pkg/webhook/util/controller"
	"github.com/openkruise/kruise/pkg/webhook/util/health"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"
)

var (
	// HandlerMap contains all admission webhook handlers.
	HandlerMap   = map[string]admission.Handler{}
	handlerGates = map[string]runtime.Object{}

	Checker = health.Checker
)

func addHandlers(m map[string]admission.Handler) {
	addHandlersWithGate(m, nil)
}

func addHandlersWithGate(m map[string]admission.Handler, gateObj runtime.Object) {
	for path, handler := range m {
		if len(path) == 0 {
			klog.Warningf("Skip handler with empty path.")
			continue
		}
		if path[0] != '/' {
			path = "/" + path
		}
		_, found := HandlerMap[path]
		if found {
			klog.V(1).Infof("conflicting webhook builder path %v in handler map", path)
		}
		HandlerMap[path] = handler
		if gateObj != nil {
			handlerGates[path] = gateObj
		}
	}
}

func filterActiveHandlers() {
	disablePaths := sets.NewString()
	for path := range HandlerMap {
		if obj, ok := handlerGates[path]; ok {
			if !gate.ResourceEnabled(obj) {
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
	server.Host = "0.0.0.0"
	server.Port = webhookutil.GetPort()
	server.CertDir = webhookutil.GetCertDir()

	// register admission handlers
	filterActiveHandlers()
	for path, handler := range HandlerMap {
		server.Register(path, &webhook.Admission{Handler: handler})
		klog.V(3).Infof("Registered webhook handler %s", path)
	}

	// register conversion webhook
	server.Register("/convert", &conversion.Webhook{})

	// register health handler
	server.Register("/healthz", &health.Handler{})

	return nil
}

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;update;patch

func Initialize(mgr manager.Manager, stopCh <-chan struct{}) error {
	cli := &client.DelegatingClient{
		Reader:       mgr.GetAPIReader(),
		Writer:       mgr.GetClient(),
		StatusClient: mgr.GetClient(),
	}

	filterActiveHandlers()
	c, err := webhookcontroller.New(mgr.GetConfig(), cli, HandlerMap)
	if err != nil {
		return err
	}
	go func() {
		c.Start(stopCh)
	}()

	timer := time.NewTimer(time.Second * 20)
	defer timer.Stop()
	select {
	case <-webhookcontroller.Inited():
		return nil
	case <-timer.C:
		return fmt.Errorf("failed to start webhook controller for waiting more than 20s")
	}
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
			klog.Warningf("Failed to wait webhook ready over %s: %v", duration, err)
		}
		if duration > time.Minute {
			return err
		}
		time.Sleep(time.Second * 2)
	}

}
