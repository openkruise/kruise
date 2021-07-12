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

package crd

import (
	"fmt"
	"reflect"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionslisters "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/labels"

	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
)

func Ensure(client apiextensionsclientset.Interface, lister apiextensionslisters.CustomResourceDefinitionLister, caBundle []byte) error {
	crdList, err := lister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list crds: %v", err)
	}

	webhookConfig := apiextensionsv1beta1.WebhookClientConfig{
		CABundle: caBundle,
	}
	path := "/convert"
	if host := webhookutil.GetHost(); len(host) > 0 {
		url := fmt.Sprintf("https://%s:%d%s", host, webhookutil.GetPort(), path)
		webhookConfig.URL = &url
	} else {
		var port int32 = 443
		webhookConfig.Service = &apiextensionsv1beta1.ServiceReference{
			Namespace: webhookutil.GetNamespace(),
			Name:      webhookutil.GetServiceName(),
			Port:      &port,
			Path:      &path,
		}
	}

	for _, crd := range crdList {
		if crd.Spec.Group != "apps.kruise.io" {
			continue
		}
		if crd.Spec.Conversion == nil || crd.Spec.Conversion.Strategy != apiextensionsv1beta1.WebhookConverter {
			continue
		}

		if !reflect.DeepEqual(crd.Spec.Conversion.WebhookClientConfig, webhookConfig) {
			newCRD := crd.DeepCopy()
			newCRD.Spec.Conversion.WebhookClientConfig = webhookConfig.DeepCopy()
			if _, err := client.ApiextensionsV1beta1().CustomResourceDefinitions().Update(newCRD); err != nil {
				return fmt.Errorf("failed to update CRD %s: %v", newCRD.Name, err)
			}
		}
	}
	return nil
}
