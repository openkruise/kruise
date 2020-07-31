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

	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/kruise/pkg/webhook/util/configuration"
	"github.com/openkruise/kruise/pkg/webhook/util/generator"
	"github.com/openkruise/kruise/pkg/webhook/util/health"
	"github.com/openkruise/kruise/pkg/webhook/util/writer"
	"k8s.io/apimachinery/pkg/types"
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

func Initialize(mgr manager.Manager) error {
	c := &client.DelegatingClient{
		Reader:       mgr.GetAPIReader(),
		Writer:       mgr.GetClient(),
		StatusClient: mgr.GetClient(),
	}

	var dnsName string
	var certWriter writer.CertWriter
	var err error

	if dnsName = webhookutil.GetHost(); len(dnsName) > 0 {
		certWriter, err = writer.NewFSCertWriter(writer.FSCertWriterOptions{
			Path: webhookutil.GetCertDir(),
		})
	} else {
		dnsName = generator.ServiceToCommonName(webhookutil.GetNamespace(), webhookutil.GetServiceName())
		certWriter, err = writer.NewSecretCertWriter(writer.SecretCertWriterOptions{
			Client: c,
			Secret: &types.NamespacedName{Namespace: webhookutil.GetNamespace(), Name: webhookutil.GetSecretName()},
		})
	}

	if err != nil {
		return fmt.Errorf("failed to ensure certs: %v", err)
	}

	certs, _, err := certWriter.EnsureCert(dnsName)
	if err != nil {
		return fmt.Errorf("failed to ensure certs: %v", err)
	}
	if err := writer.WriteCertsToDir(webhookutil.GetCertDir(), certs); err != nil {
		return fmt.Errorf("failed to write certs to dir: %v", err)
	}

	if err := configuration.Ensure(c, HandlerMap, certs.CACert); err != nil {
		return fmt.Errorf("failed to ensure configuration: %v", err)
	}

	return nil
}
