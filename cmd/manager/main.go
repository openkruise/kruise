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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/openkruise/kruise/pkg/apis"
	extclient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/controller"
	"github.com/openkruise/kruise/pkg/webhook"
	"github.com/openkruise/kruise/pkg/webhook/default_server/pod/mutating"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var leaderElectionNamespace string
	var namespace string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false, "Whether you need to enable leader election.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "kruise-system",
		"This determines the namespace in which the leader election configmap will be created, it will use in-cluster namespace if empty.")
	flag.StringVar(&namespace, "namespace", "",
		"Namespace if specified restricts the manager's cache to watch objects in the desired namespace. Defaults to all namespaces.")

	klog.InitFlags(nil)
	flag.Parse()
	logf.SetLogger(klogr.New())
	log := logf.Log.WithName("entrypoint")

	// Get a config to talk to the apiserver
	log.Info("setting up client for manager")
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	log.Info("setting up manager")
	managerOptions := manager.Options{
		MetricsBindAddress: metricsAddr,
		Namespace:          namespace,
	}
	if enableLeaderElection {
		managerOptions.LeaderElection = true
		managerOptions.LeaderElectionID = "kruise-manager"
		managerOptions.LeaderElectionNamespace = leaderElectionNamespace
	}
	mgr, err := manager.New(cfg, managerOptions)
	if err != nil {
		log.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	log.Info("setting up scheme")
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable add APIs to scheme")
		os.Exit(1)
	}

	// Create clientset by client-go
	err = extclient.NewRegistry(mgr)
	if err != nil {
		log.Error(err, "unable to init kruise clientset and informer")
		os.Exit(1)
	}

	// Setup all Controllers
	log.Info("Setting up controller")
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "unable to register controllers to the manager")
		os.Exit(1)
	}

	log.Info("setting up webhooks")
	if err := webhook.AddToManager(mgr); err != nil {
		log.Error(err, "unable to register webhooks to the manager")
		os.Exit(1)
	}

	// Start an async self-check to see if webhook works properly
	go func(c client.Client) {
		time.Sleep(10 * time.Second)
		log.Info("Start self webhook check")
		tp := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: mutating.WebhookTester.Namespace,
				Name:      mutating.WebhookTester.Name,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					corev1.Container{
						Name:  "dummy",
						Image: "kubernetes/pause",
					},
				},
			},
		}
		test := func() bool {
			defer c.Delete(context.TODO(), tp)
			if err = c.Create(context.TODO(), tp); err != nil {
				log.Error(err, "fail to create test pod")
				return false
			}
			err = wait.Poll(1*time.Second, 5*time.Second, func() (bool, error) {
				err := c.Get(context.TODO(), mutating.WebhookTester, tp)
				if err == nil {
					return true, nil
				} else {
					return false, err
				}
			})
			if err != nil {
				log.Error(err, "fail to get test pod")
				return false
			}
			if tp.Annotations == nil || tp.Annotations[mutating.WebhookTestAnnotationKey] != mutating.WebhookTestAnnotationVal {
				err = fmt.Errorf("Pod mutating webhook does not work. Please make sure APIServer enables MutatingAdmissionWebhook and ValidatingAdmissionWebhook admission plugins")
				log.Error(err, "fail to validate webhook test pod")
				return false
			}
			return true
		}
		if test() {
			log.Info("Webhook is working!")
		} else {
			log.Info("Kruise controller relies on webhooks in order to work properly. Stop the controller since webhook test fails!")
			os.Exit(1)
		}
	}(mgr.GetClient())

	// Start the Cmd
	log.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
