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

package main

import (
	"flag"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"
	_ "time/tzdata" // for AdvancedCronJob Time Zone support

	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register" // for JSON log format registration
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	"k8s.io/kubernetes/pkg/capabilities"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	policyv1alpha1 "github.com/openkruise/kruise/apis/policy/v1alpha1"
	extclient "github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/control/pubcontrol"
	"github.com/openkruise/kruise/pkg/controller"
	"github.com/openkruise/kruise/pkg/features"
	"github.com/openkruise/kruise/pkg/util"
	utilclient "github.com/openkruise/kruise/pkg/util/client"
	"github.com/openkruise/kruise/pkg/util/controllerfinder"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
	"github.com/openkruise/kruise/pkg/util/fieldindex"
	_ "github.com/openkruise/kruise/pkg/util/metrics/leadership"
	"github.com/openkruise/kruise/pkg/webhook"
	webhookutil "github.com/openkruise/kruise/pkg/webhook/util"
	// +kubebuilder:scaffold:imports
)

const (
	defaultLeaseDuration              = 15 * time.Second
	defaultRenewDeadline              = 10 * time.Second
	defaultRetryPeriod                = 2 * time.Second
	defaultControllerCacheSyncTimeout = 2 * time.Minute
	defaultWebhookInitializeTimeout   = 60 * time.Second
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	restConfigQPS   = flag.Int("rest-config-qps", 30, "QPS of rest config.")
	restConfigBurst = flag.Int("rest-config-burst", 50, "Burst of rest config.")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(appsv1alpha1.AddToScheme(clientgoscheme.Scheme))
	utilruntime.Must(appsv1beta1.AddToScheme(clientgoscheme.Scheme))

	utilruntime.Must(appsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(appsv1beta1.AddToScheme(scheme))
	utilruntime.Must(policyv1alpha1.AddToScheme(scheme))
	scheme.AddUnversionedTypes(metav1.SchemeGroupVersion, &metav1.UpdateOptions{}, &metav1.DeleteOptions{}, &metav1.CreateOptions{})
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr, pprofAddr string
	var healthProbeAddr string
	var enableLeaderElection, enablePprof, allowPrivileged bool
	var leaderElectionNamespace string
	var namespace string
	var syncPeriodStr string
	var leaseDuration time.Duration
	var renewDeadLine time.Duration
	var leaderElectionResourceLock string
	var leaderElectionId string
	var retryPeriod time.Duration
	var controllerCacheSyncTimeout time.Duration
	var webhookInitializeTimeout time.Duration

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&healthProbeAddr, "health-probe-addr", ":8000", "The address the healthz/readyz endpoint binds to.")
	flag.BoolVar(&allowPrivileged, "allow-privileged", true, "If true, allow privileged containers. It will only work if api-server is also"+
		"started with --allow-privileged=true.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", true, "Whether you need to enable leader election.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "kruise-system",
		"This determines the namespace in which the leader election configmap will be created, it will use in-cluster namespace if empty.")
	flag.StringVar(&namespace, "namespace", "",
		"Namespace if specified restricts the manager's cache to watch objects in the desired namespace. Defaults to all namespaces.")
	flag.BoolVar(&enablePprof, "enable-pprof", true, "Enable pprof for controller manager.")
	flag.StringVar(&pprofAddr, "pprof-addr", ":8090", "The address the pprof binds to.")
	flag.StringVar(&syncPeriodStr, "sync-period", "", "Determines the minimum frequency at which watched resources are reconciled.")
	flag.DurationVar(&leaseDuration, "leader-election-lease-duration", defaultLeaseDuration,
		"leader-election-lease-duration is the duration that non-leader candidates will wait to force acquire leadership. This is measured against time of last observed ack. Default is 15 seconds.")
	flag.DurationVar(&renewDeadLine, "leader-election-renew-deadline", defaultRenewDeadline,
		"leader-election-renew-deadline is the duration that the acting controlplane will retry refreshing leadership before giving up. Default is 10 seconds.")
	flag.StringVar(&leaderElectionResourceLock, "leader-election-resource-lock", resourcelock.LeasesResourceLock,
		"leader-election-resource-lock determines which resource lock to use for leader election, defaults to \"leases\".")
	flag.StringVar(&leaderElectionId, "leader-election-id", "kruise-manager",
		"leader-election-id determines the name of the resource that leader election will use for holding the leader lock, Default is kruise-manager.")
	flag.DurationVar(&retryPeriod, "leader-election-retry-period", defaultRetryPeriod,
		"leader-election-retry-period is the duration the LeaderElector clients should wait between tries of actions. Default is 2 seconds.")
	flag.DurationVar(&controllerCacheSyncTimeout, "controller-cache-sync-timeout", defaultControllerCacheSyncTimeout, "CacheSyncTimeout refers to the time limit set to wait for syncing caches. Defaults to 2 minutes if not set.")
	flag.DurationVar(&webhookInitializeTimeout, "webhook-initialize-timeout", defaultWebhookInitializeTimeout, "WebhookInitializeTimeout refers to the time limit set to wait for webhook initialization. Defaults to 60 seconds if not set.")

	utilfeature.DefaultMutableFeatureGate.AddFlag(pflag.CommandLine)
	logOptions := logs.NewOptions()
	logsapi.AddFlags(logOptions, pflag.CommandLine)
	klog.InitFlags(nil)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	rand.Seed(time.Now().UnixNano())
	ctrl.SetLogger(klogr.New())
	if err := logsapi.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "logsapi ValidateAndApply failed")
		os.Exit(1)
	}
	features.SetDefaultFeatureGates()
	util.SetControllerCacheSyncTimeout(controllerCacheSyncTimeout)

	if enablePprof {
		go func() {
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				setupLog.Error(err, "unable to start pprof")
			}
		}()
	}

	if allowPrivileged {
		capabilities.Initialize(capabilities.Capabilities{
			AllowPrivileged: allowPrivileged,
		})
	}

	ctx := ctrl.SetupSignalHandler()
	cfg := ctrl.GetConfigOrDie()
	setRestConfig(cfg)
	cfg.UserAgent = "kruise-manager"

	setupLog.Info("new clientset registry")
	err := extclient.NewRegistry(cfg)
	if err != nil {
		setupLog.Error(err, "unable to init kruise clientset and informer")
		os.Exit(1)
	}
	err = util.InitProtectionLogger()
	if err != nil {
		setupLog.Error(err, "unable to init protection logger")
		os.Exit(1)
	}

	var syncPeriod *time.Duration
	if syncPeriodStr != "" {
		d, err := time.ParseDuration(syncPeriodStr)
		if err != nil {
			setupLog.Error(err, "invalid sync period flag")
		} else {
			syncPeriod = &d
		}
	}
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress:     healthProbeAddr,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           leaderElectionId,
		LeaderElectionNamespace:    leaderElectionNamespace,
		LeaderElectionResourceLock: leaderElectionResourceLock,
		LeaseDuration:              &leaseDuration,
		RenewDeadline:              &renewDeadLine,
		RetryPeriod:                &retryPeriod,
		Cache: cache.Options{
			SyncPeriod:        syncPeriod,
			DefaultNamespaces: getCacheNamespacesFromFlag(namespace),
		},
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Host:    "0.0.0.0",
			Port:    webhookutil.GetPort(),
			CertDir: webhookutil.GetCertDir(),
		}),
		NewCache: utilclient.NewCache,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	err = controllerfinder.InitControllerFinder(mgr)
	if err != nil {
		setupLog.Error(err, "unable to start ControllerFinder")
		os.Exit(1)
	}
	pubcontrol.InitPubControl(mgr.GetClient(), controllerfinder.Finder, mgr.GetEventRecorderFor("pub-controller"))

	setupLog.Info("register field index")
	if err := fieldindex.RegisterFieldIndexes(mgr.GetCache()); err != nil {
		setupLog.Error(err, "failed to register field index")
		os.Exit(1)
	}

	setupLog.Info("setup webhook")
	if err = webhook.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to setup webhook")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder
	setupLog.Info("initialize webhook")
	if err := webhook.Initialize(ctx, cfg, webhookInitializeTimeout); err != nil {
		setupLog.Error(err, "unable to initialize webhook")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("webhook-ready", webhook.Checker); err != nil {
		setupLog.Error(err, "unable to add readyz check")
		os.Exit(1)
	}

	go func() {
		setupLog.Info("wait webhook ready")
		if err = webhook.WaitReady(); err != nil {
			setupLog.Error(err, "unable to wait webhook ready")
			os.Exit(1)
		}

		setupLog.Info("setup controllers")
		if err = controller.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to setup controllers")
			os.Exit(1)
		}
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setRestConfig(c *rest.Config) {
	if *restConfigQPS > 0 {
		c.QPS = float32(*restConfigQPS)
	}
	if *restConfigBurst > 0 {
		c.Burst = *restConfigBurst
	}
}

func getCacheNamespacesFromFlag(ns string) map[string]cache.Config {
	if ns == "" {
		return nil
	}
	return map[string]cache.Config{
		ns: {},
	}
}
