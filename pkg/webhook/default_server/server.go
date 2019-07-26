/*
Copyright 2019 The Kruise Authors.

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

package defaultserver

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/builder"
)

var (
	log        = logf.Log.WithName("default_server")
	builderMap = map[string]*builder.WebhookBuilder{}
	// HandlerMap contains all admission webhook handlers.
	HandlerMap = map[string][]admission.Handler{}
)

// Add adds itself to the manager
func Add(mgr manager.Manager) error {
	ns := os.Getenv("POD_NAMESPACE")
	if len(ns) == 0 {
		ns = "kruise-system"
	}
	secretName := os.Getenv("SECRET_NAME")
	if len(secretName) == 0 {
		secretName = "kruise-webhook-server-secret"
	}

	bootstrapOptions := &webhook.BootstrapOptions{
		MutatingWebhookConfigName:   "kruise-mutating-webhook-configuration",
		ValidatingWebhookConfigName: "kruise-validating-webhook-configuration",
	}

	if webhookHost := os.Getenv("WEBHOOK_HOST"); len(webhookHost) > 0 {
		bootstrapOptions.Host = &webhookHost
	} else {
		bootstrapOptions.Service = &webhook.Service{
			Namespace: ns,
			Name:      "kruise-webhook-server-service",
			// Selectors should select the pods that runs this webhook server.
			Selectors: map[string]string{
				"control-plane": "controller-manager",
			},
		}
		bootstrapOptions.Secret = &types.NamespacedName{
			Namespace: ns,
			Name:      secretName,
		}
	}

	svr, err := webhook.NewServer("kruise-admission-server", mgr, webhook.ServerOptions{
		Port:             9876,
		CertDir:          "/tmp/cert",
		BootstrapOptions: bootstrapOptions,
	})
	if err != nil {
		return err
	}

	var webhooks []webhook.Webhook
	for k, builder := range builderMap {
		handlers, ok := HandlerMap[k]
		if !ok {
			log.V(1).Info(fmt.Sprintf("can't find handlers for builder: %v", k))
			handlers = []admission.Handler{}
		}
		wh, err := builder.
			Handlers(handlers...).
			WithManager(mgr).
			Build()
		if err != nil {
			if kindMatchErr, ok := err.(*meta.NoKindMatchError); ok {
				log.Info(fmt.Sprintf("CRD %v is not installed,  webhook %v registration is ignored",
					kindMatchErr.GroupKind, k))
				continue
			}
			return err
		}
		webhooks = append(webhooks, wh)
	}
	return svr.Register(webhooks...)
}
