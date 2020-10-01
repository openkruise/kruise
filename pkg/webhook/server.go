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

	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	webhookcontroller "github.com/openkruise/kruise/pkg/webhook/util/controller"
	"github.com/openkruise/kruise/pkg/webhook/util/health"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	// HandlerMap contains all admission webhook handlers.
	HandlerMap = map[string]admission.Handler{}

	Checker = health.Checker
)

func addHandlers(m map[string]admission.Handler) {
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
	}
}

func SetupWithManager(mgr manager.Manager) error {
	server := mgr.GetWebhookServer()
	server.Host = "0.0.0.0"
	server.Port = webhookutil.GetPort()
	server.CertDir = webhookutil.GetCertDir()

	// register admission handlers
	for path, handler := range HandlerMap {
		server.Register(path, &webhook.Admission{Handler: handler})
		klog.V(3).Infof("Registered webhook handler %s", path)
	}

	// register health handler
	server.Register("/healthz", &health.Handler{})

	return nil
}

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete

func Initialize(mgr manager.Manager, stopCh <-chan struct{}) error {
	cli := &client.DelegatingClient{
		Reader:       mgr.GetAPIReader(),
		Writer:       mgr.GetClient(),
		StatusClient: mgr.GetClient(),
	}

	c, err := webhookcontroller.New(cli, HandlerMap)
	if err != nil {
		return err
	}
	go func() {
		c.Start(stopCh)
	}()

	timer := time.NewTimer(time.Second * 5)
	defer timer.Stop()
	select {
	case <-webhookcontroller.Inited():
		return nil
	case <-timer.C:
		return fmt.Errorf("failed to start webhook controller for waiting more than 5s")
	}
}
