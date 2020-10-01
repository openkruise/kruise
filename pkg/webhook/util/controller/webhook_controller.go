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
	"fmt"
	"sync"
	"time"

	extclient "github.com/openkruise/kruise/pkg/client"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	"github.com/openkruise/kruise/pkg/webhook/util/configuration"
	"github.com/openkruise/kruise/pkg/webhook/util/generator"
	"github.com/openkruise/kruise/pkg/webhook/util/writer"
	"k8s.io/api/admissionregistration/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	admissionregistrationinformers "k8s.io/client-go/informers/admissionregistration/v1beta1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	admissionregistrationlisters "k8s.io/client-go/listers/admissionregistration/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	mutatingWebhookConfigurationName   = "kruise-mutating-webhook-configuration"
	validatingWebhookConfigurationName = "kruise-validating-webhook-configuration"

	namespace  = webhookutil.GetNamespace()
	secretName = webhookutil.GetSecretName()

	uninit   = make(chan struct{})
	onceInit = sync.Once{}
)

func Inited() chan struct{} {
	return uninit
}

type Controller struct {
	kubeClient    clientset.Interface
	runtimeClient client.Client
	handlers      map[string]admission.Handler

	informerFactory    informers.SharedInformerFactory
	secretLister       corelisters.SecretNamespaceLister
	mutatingWCLister   admissionregistrationlisters.MutatingWebhookConfigurationLister
	validatingWCLister admissionregistrationlisters.ValidatingWebhookConfigurationLister
	synced             []cache.InformerSynced

	queue workqueue.RateLimitingInterface
}

func New(cli client.Client, handlers map[string]admission.Handler) (*Controller, error) {
	c := &Controller{
		kubeClient:    extclient.GetGenericClient().KubeClient,
		runtimeClient: cli,
		handlers:      handlers,

		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "webhook-controller"),
	}

	c.informerFactory = informers.NewSharedInformerFactory(c.kubeClient, 0)

	secretInformer := coreinformers.New(c.informerFactory, namespace, nil).Secrets()
	c.secretLister = secretInformer.Lister().Secrets(namespace)

	admissionRegistrationInformer := admissionregistrationinformers.New(c.informerFactory, v1.NamespaceAll, nil)
	c.mutatingWCLister = admissionRegistrationInformer.MutatingWebhookConfigurations().Lister()
	c.validatingWCLister = admissionRegistrationInformer.ValidatingWebhookConfigurations().Lister()

	c.synced = []cache.InformerSynced{
		secretInformer.Informer().HasSynced,
		admissionRegistrationInformer.MutatingWebhookConfigurations().Informer().HasSynced,
		admissionRegistrationInformer.ValidatingWebhookConfigurations().Informer().HasSynced,
	}

	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			secret := obj.(*v1.Secret)
			if secret.Name == secretName {
				klog.Infof("Secret %s added", secretName)
				c.queue.Add("")
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			secret := cur.(*v1.Secret)
			if secret.Name == secretName {
				klog.Infof("Secret %s updated", secretName)
				c.queue.Add("")
			}
		},
	})

	admissionRegistrationInformer.MutatingWebhookConfigurations().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			conf := obj.(*v1beta1.MutatingWebhookConfiguration)
			if conf.Name == mutatingWebhookConfigurationName {
				klog.Infof("MutatingWebhookConfiguration %s added", mutatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			conf := cur.(*v1beta1.MutatingWebhookConfiguration)
			if conf.Name == mutatingWebhookConfigurationName {
				klog.Infof("MutatingWebhookConfiguration %s update", mutatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
	})

	admissionRegistrationInformer.ValidatingWebhookConfigurations().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			conf := obj.(*v1beta1.ValidatingWebhookConfiguration)
			if conf.Name == validatingWebhookConfigurationName {
				klog.Infof("ValidatingWebhookConfiguration %s added", validatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			conf := cur.(*v1beta1.ValidatingWebhookConfiguration)
			if conf.Name == validatingWebhookConfigurationName {
				klog.Infof("ValidatingWebhookConfiguration %s updated", validatingWebhookConfigurationName)
				c.queue.Add("")
			}
		},
	})

	return c, nil
}

func (c *Controller) Start(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("Starting webhook-controller")
	defer klog.Infof("Shutting down webhook-controller")

	c.informerFactory.Start(stopCh)
	if !cache.WaitForNamedCacheSync("webhook-controller", stopCh, c.synced...) {
		return
	}

	go wait.Until(func() {
		for c.processNextWorkItem() {
		}
	}, time.Second, stopCh)
	klog.Infof("Started webhook-controller")

	<-stopCh
}

func (c *Controller) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.sync()
	if err == nil {
		c.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("sync %q failed with %v", key, err))
	c.queue.AddRateLimited(key)

	return true
}

func (c *Controller) sync() error {
	klog.Infof("Starting to sync webhook certs and configurations")
	defer func() {
		klog.Infof("Finished to sync webhook certs and configurations")
	}()

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
			Client: c.runtimeClient,
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

	if err := configuration.Ensure(c.runtimeClient, c.handlers, certs.CACert); err != nil {
		return fmt.Errorf("failed to ensure configuration: %v", err)
	}

	onceInit.Do(func() {
		close(uninit)
	})
	return nil
}
