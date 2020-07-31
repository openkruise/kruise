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

package configuration

import (
	"context"
	"fmt"
	"net/url"

	"k8s.io/klog"

	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	mutatingWebhookConfigurationName   = "kruise-mutating-webhook-configuration"
	validatingWebhookConfigurationName = "kruise-validating-webhook-configuration"
)

func Ensure(c client.Client, handlers map[string]admission.Handler, caBundle []byte) error {
	mutatingConfig := &v1beta1.MutatingWebhookConfiguration{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: mutatingWebhookConfigurationName}, mutatingConfig); err != nil {
		return fmt.Errorf("not found MutatingWebhookConfiguration %s", mutatingWebhookConfigurationName)
	}
	validatingConfig := &v1beta1.ValidatingWebhookConfiguration{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: validatingWebhookConfigurationName}, validatingConfig); err != nil {
		return fmt.Errorf("not found ValidatingWebhookConfiguration %s", validatingWebhookConfigurationName)
	}

	for i := range mutatingConfig.Webhooks {
		wh := &mutatingConfig.Webhooks[i]
		wh.ClientConfig.CABundle = caBundle
		path, err := getPath(&wh.ClientConfig)
		if err != nil {
			return err
		}
		if _, ok := handlers[path]; !ok {
			klog.Warningf("Find path %s not in handlers %v", path, handlers)
			wh.Rules = nil
		}
		if host := webhookutil.GetHost(); len(host) > 0 && wh.ClientConfig.Service != nil {
			convertClientConfig(&wh.ClientConfig, host, webhookutil.GetPort())
		}
	}
	for i := range validatingConfig.Webhooks {
		wh := &validatingConfig.Webhooks[i]
		wh.ClientConfig.CABundle = caBundle
		path, err := getPath(&wh.ClientConfig)
		if err != nil {
			return err
		}
		if _, ok := handlers[path]; !ok {
			klog.Warningf("Find path %s not in handlers %v", path, handlers)
			wh.Rules = nil
		}
		if host := webhookutil.GetHost(); len(host) > 0 && wh.ClientConfig.Service != nil {
			convertClientConfig(&wh.ClientConfig, host, webhookutil.GetPort())
		}
	}

	if err := c.Update(context.TODO(), validatingConfig); err != nil {
		return fmt.Errorf("failed to update %s: %v", validatingWebhookConfigurationName, err)
	}
	if err := c.Update(context.TODO(), mutatingConfig); err != nil {
		return fmt.Errorf("failed to update %s: %v", mutatingWebhookConfigurationName, err)
	}

	return nil
}

func getPath(clientConfig *v1beta1.WebhookClientConfig) (string, error) {
	if clientConfig.Service != nil {
		return *clientConfig.Service.Path, nil
	} else if clientConfig.URL != nil {
		u, err := url.Parse(*clientConfig.URL)
		if err != nil {
			return "", err
		}
		return u.Path, nil
	}
	return "", fmt.Errorf("invalid clientConfig: %+v", clientConfig)
}

func convertClientConfig(clientConfig *v1beta1.WebhookClientConfig, host string, port int) {
	url := fmt.Sprintf("https://%s:%d%s", host, port, *clientConfig.Service.Path)
	clientConfig.URL = &url
	clientConfig.Service = nil
}
