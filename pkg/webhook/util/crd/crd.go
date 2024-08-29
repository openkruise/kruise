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
	"bytes"
	"context"
	"fmt"

	"reflect"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionslisters "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"

	"github.com/openkruise/kruise/apis"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
)

var (
	kruiseScheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(apis.AddToScheme(kruiseScheme))
}

func Ensure(client apiextensionsclientset.Interface, lister apiextensionslisters.CustomResourceDefinitionLister, caBundle []byte) error {
	crdList, err := lister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list crds: %v", err)
	}

	if utilfeature.DefaultFeatureGate.Enabled(features.EnableExternalCerts) {
		for _, crd := range crdList {
			if len(crd.Spec.Versions) == 0 || crd.Spec.Conversion == nil || crd.Spec.Conversion.Strategy != apiextensionsv1.WebhookConverter {
				continue
			}
			if !kruiseScheme.Recognizes(schema.GroupVersionKind{Group: crd.Spec.Group, Version: crd.Spec.Versions[0].Name, Kind: crd.Spec.Names.Kind}) {
				continue
			}

			if crd.Spec.Conversion.Webhook == nil || crd.Spec.Conversion.Webhook.ClientConfig == nil {
				return fmt.Errorf("bad conversion configuration of CRD %s", crd.Name)
			}

			if !bytes.Equal(crd.Spec.Conversion.Webhook.ClientConfig.CABundle, caBundle) {
				return fmt.Errorf("caBundle of CRD %s does not match external caBundle", crd.Name)
			}
		}
		return nil
	}
	webhookConfig := &apiextensionsv1.WebhookClientConfig{
		CABundle: caBundle,
	}
	path := "/convert"
	if host := webhookutil.GetHost(); len(host) > 0 {
		url := fmt.Sprintf("https://%s:%d%s", host, webhookutil.GetPort(), path)
		webhookConfig.URL = &url
	} else {
		var port int32 = 443
		webhookConfig.Service = &apiextensionsv1.ServiceReference{
			Namespace: webhookutil.GetNamespace(),
			Name:      webhookutil.GetServiceName(),
			Port:      &port,
			Path:      &path,
		}
	}

	for _, crd := range crdList {
		if len(crd.Spec.Versions) == 0 || crd.Spec.Conversion == nil || crd.Spec.Conversion.Strategy != apiextensionsv1.WebhookConverter {
			continue
		}
		if !kruiseScheme.Recognizes(schema.GroupVersionKind{Group: crd.Spec.Group, Version: crd.Spec.Versions[0].Name, Kind: crd.Spec.Names.Kind}) {
			continue
		}

		if crd.Spec.Conversion.Webhook == nil || !reflect.DeepEqual(crd.Spec.Conversion.Webhook.ClientConfig, webhookConfig) {
			newCRD := crd.DeepCopy()
			newCRD.Spec.Conversion.Webhook = &apiextensionsv1.WebhookConversion{
				ClientConfig:             webhookConfig.DeepCopy(),
				ConversionReviewVersions: []string{"v1", "v1beta1"},
			}
			if _, err := client.ApiextensionsV1().CustomResourceDefinitions().Update(context.TODO(), newCRD, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("failed to update CRD %s: %v", newCRD.Name, err)
			}
			klog.InfoS("Update caBundle success", "CustomResourceDefinitions", klog.KObj(newCRD))
		}
	}

	return nil
}
