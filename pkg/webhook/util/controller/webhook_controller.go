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

package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsinformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions/apiextensions/v1"
	apiextensionslisters "k8s.io/apiextensions-apiserver/pkg/client/listers/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	admissionregistrationinformers "k8s.io/client-go/informers/admissionregistration/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	extclient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	webhooktypes "github.com/openkruise/kruise/pkg/webhook/types"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/kruise/pkg/webhook/util/configuration"
	"github.com/openkruise/kruise/pkg/webhook/util/crd"
	"github.com/openkruise/kruise/pkg/webhook/util/generator"
	"github.com/openkruise/kruise/pkg/webhook/util/writer"
)

const (
	mutatingWebhookConfigurationName   = "kruise-mutating-webhook-configuration"
	validatingWebhookConfigurationName = "kruise-validating-webhook-configuration"

	defaultResyncPeriod = time.Minute
)

var (
	namespace  = webhookutil.GetNamespace()
	secretName = webhookutil.GetSecretName()

	uninit   = make(chan struct{})
	onceInit = sync.Once{}
)

func Inited() chan struct{} {
	return uninit
}

type Controller struct {
	kubeClient clientset.Interface
	handlers   map[string]webhooktypes.HandlerGetter

	informerFactory informers.SharedInformerFactory
	crdClient       apiextensionsclientset.Interface
	crdInformer     cache.SharedIndexInformer
	crdLister       apiextensionslisters.CustomResourceDefinitionLister
	synced          []cache.InformerSynced

	queue workqueue.RateLimitingInterface
}

func New(cfg *rest.Config, handlers map[string]webhooktypes.HandlerGetter) (*Controller, error) {
	c := &Controller{
		kubeClient: extclient.GetGenericClientWithName("webhook-controller").KubeClient,
		handlers:   handlers,
		queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "webhook-controller"),
	}

	c.informerFactory = informers.NewSharedInformerFactory(c.kubeClient, 0)

	secretInformer := coreinformers.New(c.informerFactory, namespace, nil).Secrets()
	admissionRegistrationInformer := admissionregistrationinformers.New(c.informerFactory, v1.NamespaceAll, nil)

	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			secret := obj.(*v1.Secret)
			if secret.Name == secretName {
				klog.InfoS("Secret added", "name", secretName)
				c.queue.Add("")
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			secret := cur.(*v1.Secret)
			if secret.Name == secretName {
				klog.InfoS("Secret updated", "name", secretName)
				c.queue.Add("")
			}
		},
	})

	admissionRegistrationInformer.MutatingWebhookConfigurations().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			conf := obj.(*admissionregistrationv1.MutatingWebhookConfiguration)
			if conf.Name == mutatingWebhookConfigurationName {
				klog.InfoS("MutatingWebhookConfiguration added", "name", mutatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			conf := cur.(*admissionregistrationv1.MutatingWebhookConfiguration)
			if conf.Name == mutatingWebhookConfigurationName {
				klog.InfoS("MutatingWebhookConfiguration update", "name", mutatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
	})

	admissionRegistrationInformer.ValidatingWebhookConfigurations().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			conf := obj.(*admissionregistrationv1.ValidatingWebhookConfiguration)
			if conf.Name == validatingWebhookConfigurationName {
				klog.InfoS("ValidatingWebhookConfiguration added", "name", validatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			conf := cur.(*admissionregistrationv1.ValidatingWebhookConfiguration)
			if conf.Name == validatingWebhookConfigurationName {
				klog.InfoS("ValidatingWebhookConfiguration updated", "name", validatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
	})

	c.crdClient = apiextensionsclientset.NewForConfigOrDie(cfg)
	c.crdInformer = apiextensionsinformers.NewCustomResourceDefinitionInformer(c.crdClient, 0, cache.Indexers{})
	c.crdInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			crd := obj.(*apiextensionsv1.CustomResourceDefinition)
			if crd.Spec.Group == "apps.kruise.io" {
				klog.InfoS("CustomResourceDefinition added", "name", crd.Name)
				c.queue.Add("")
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			crd := cur.(*apiextensionsv1.CustomResourceDefinition)
			if crd.Spec.Group == "apps.kruise.io" {
				klog.InfoS("CustomResourceDefinition updated", "name", crd.Name)
				c.queue.Add("")
			}
		},
	})
	c.crdLister = apiextensionslisters.NewCustomResourceDefinitionLister(c.crdInformer.GetIndexer())

	c.synced = []cache.InformerSynced{
		secretInformer.Informer().HasSynced,
		admissionRegistrationInformer.MutatingWebhookConfigurations().Informer().HasSynced,
		admissionRegistrationInformer.ValidatingWebhookConfigurations().Informer().HasSynced,
		c.crdInformer.HasSynced,
	}

	return c, nil
}

func (c *Controller) Start(ctx context.Context) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Info("Starting webhook-controller")
	defer klog.Info("Shutting down webhook-controller")

	c.informerFactory.Start(ctx.Done())
	go func() {
		c.crdInformer.Run(ctx.Done())
	}()
	if !cache.WaitForNamedCacheSync("webhook-controller", ctx.Done(), c.synced...) {
		return
	}

	go wait.Until(func() {
		for c.processNextWorkItem() {
		}
	}, time.Second, ctx.Done())
	klog.Info("Started webhook-controller")

	<-ctx.Done()
}

func (c *Controller) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.sync()
	if err == nil {
		c.queue.AddAfter(key, defaultResyncPeriod)
		c.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("sync %q failed with %v", key, err))
	c.queue.AddRateLimited(key)

	return true
}

func (c *Controller) sync() error {
	klog.Info("Starting to sync webhook certs and configurations")
	defer func() {
		klog.Info("Finished to sync webhook certs and configurations")
	}()

	dnsName := webhookutil.GetHost()
	if len(dnsName) == 0 {
		dnsName = generator.ServiceToCommonName(webhookutil.GetNamespace(), webhookutil.GetServiceName())
	}

	var certWriter writer.CertWriter
	var err error

	certWriterType := webhookutil.GetCertWriter()
	if utilfeature.DefaultFeatureGate.Enabled(features.EnableExternalCerts) {
		certWriter, err = writer.NewExternalCertWriter(writer.ExternalCertWriterOptions{
			Clientset: c.kubeClient,
			Secret:    &types.NamespacedName{Namespace: webhookutil.GetNamespace(), Name: webhookutil.GetSecretName()},
		})
	} else if certWriterType == writer.FsCertWriter || (len(certWriterType) == 0 && len(webhookutil.GetHost()) != 0) {
		certWriter, err = writer.NewFSCertWriter(writer.FSCertWriterOptions{
			Path: webhookutil.GetCertDir(),
		})
	} else {
		certWriter, err = writer.NewSecretCertWriter(writer.SecretCertWriterOptions{
			Clientset: c.kubeClient,
			Secret:    &types.NamespacedName{Namespace: webhookutil.GetNamespace(), Name: webhookutil.GetSecretName()},
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
	if err := configuration.Ensure(c.kubeClient, c.handlers, certs.CACert); err != nil {
		return fmt.Errorf("failed to ensure configuration: %v", err)
	}

	if err := crd.Ensure(c.crdClient, c.crdLister, certs.CACert); err != nil {
		return fmt.Errorf("failed to ensure crd: %v", err)
	}

	onceInit.Do(func() {
		close(uninit)
	})
	return nil
}
