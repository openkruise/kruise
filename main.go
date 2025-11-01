// main.go
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
	"crypto/tls"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"
	_ "time/tzdata" // for AdvancedCronJob TZ support

	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register" // JSON log format registration
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
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
)

const (
	defaultLeaseDuration                     = 15 * time.Second
	defaultRenewDeadline                     = 10 * time.Second
	defaultRetryPeriod                       = 2 * time.Second
	defaultControllerCacheSyncTimeout        = 2 * time.Minute
	defaultWebhookInitializeTimeout          = 60 * time.Second
	defaultTtlsecondsForAlwaysNodeimageConst = 300

	defaultMetricsAddr = "127.0.0.1:8080"
	defaultHealthAddr  = "127.0.0.1:8000"
	defaultPprofAddr   = "127.0.0.1:8090"
	defaultWebhookPort = 9876
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
	scheme.AddUnversionedTypes(metav1.SchemeGroupVersion,
		&metav1.UpdateOptions{}, &metav1.DeleteOptions{}, &metav1.CreateOptions{})
}

func main() {
	var metricsAddr, pprofAddr, healthProbeAddr, webhookBindAddress string
	var enableLeaderElection, enablePprof, allowPrivileged bool
	var leaderElectionNamespace, namespace, syncPeriodStr string
	var leaseDuration, renewDeadLine, retryPeriod, controllerCacheSyncTimeout, webhookInitializeTimeout time.Duration
	var leaderElectionResourceLock, leaderElectionId string
	var defaultTtlsecondsForAlwaysNodeimage, webhookPort int

	flag.StringVar(&metricsAddr, "metrics-addr", defaultMetricsAddr, "The address the metrics endpoint binds to (localhost only by default).")
	flag.StringVar(&healthProbeAddr, "health-probe-addr", defaultHealthAddr, "The address the healthz/readyz endpoint binds to (localhost only by default).")
	flag.StringVar(&webhookBindAddress, "webhook-bind-address", "", "The IP address to serve webhooks on (empty = first non-loopback).")
	flag.IntVar(&webhookPort, "webhook-port", defaultWebhookPort, "The port for the webhook server (default 9876).")
	flag.BoolVar(&allowPrivileged, "allow-privileged", true, "If true, allow privileged containers (depends on API server).")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", true, "Enable leader election.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "kruise-system", "Namespace for leader election configmap.")
	flag.StringVar(&namespace, "namespace", "", "Namespace to limit manager cache to (defaults to all).")
	flag.BoolVar(&enablePprof, "enable-pprof", true, "Enable pprof for controller manager.")
	flag.StringVar(&pprofAddr, "pprof-addr", defaultPprofAddr, "The address for pprof (localhost only by default).")
	flag.StringVar(&syncPeriodStr, "sync-period", "", "Minimum frequency for reconciling watched resources.")
	flag.DurationVar(&leaseDuration, "leader-election-lease-duration", defaultLeaseDuration, "Leader election lease duration.")
	flag.DurationVar(&renewDeadLine, "leader-election-renew-deadline", defaultRenewDeadline, "Leader election renew deadline.")
	flag.StringVar(&leaderElectionResourceLock, "leader-election-resource-lock", resourcelock.LeasesResourceLock, "Leader election resource lock type.")
	flag.StringVar(&leaderElectionId, "leader-election-id", "kruise-manager", "Leader election resource id.")
	flag.DurationVar(&retryPeriod, "leader-election-retry-period", defaultRetryPeriod, "Leader election retry period.")
	flag.DurationVar(&controllerCacheSyncTimeout, "controller-cache-sync-timeout", defaultControllerCacheSyncTimeout, "Cache sync timeout.")
	flag.DurationVar(&webhookInitializeTimeout, "webhook-initialize-timeout", defaultWebhookInitializeTimeout, "Webhook initialization timeout.")
	flag.IntVar(&defaultTtlsecondsForAlwaysNodeimage, "default-ttlseconds-for-always-nodeimage", defaultTtlsecondsForAlwaysNodeimageConst, "Default TTL seconds for always-nodeimage.")

	utilfeature.DefaultMutableFeatureGate.AddFlag(pflag.CommandLine)
	logOptions := logs.NewOptions()
	logsapi.AddFlags(logOptions, pflag.CommandLine)
	klog.InitFlags(nil)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(textlogger.NewLogger(textlogger.NewConfig()))

	if err := logsapi.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "logsapi ValidateAndApply failed")
		os.Exit(1)
	}
	features.SetDefaultFeatureGates()
	util.SetControllerCacheSyncTimeout(controllerCacheSyncTimeout)
	if err := util.SetDefaultTtlForAlwaysNodeimage(defaultTtlsecondsForAlwaysNodeimage); err != nil {
		setupLog.Error(err, "default-ttlseconds-for-always-nodeimage validate failed")
		os.Exit(1)
	}

	if enablePprof {
		go func() {
			setupLog.Info("starting pprof server", "address", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				setupLog.Error(err, "unable to start pprof")
			}
		}()
	}

	if allowPrivileged {
		capabilities.Initialize(capabilities.Capabilities{AllowPrivileged: allowPrivileged})
	}

	ctx := ctrl.SetupSignalHandler()
	cfg := ctrl.GetConfigOrDie()
	setRestConfig(cfg)
	cfg.UserAgent = "kruise-manager"

	setupLog.Info("new clientset registry")
	if err := extclient.NewRegistry(cfg); err != nil {
		setupLog.Error(err, "unable to init kruise clientset and informer")
		os.Exit(1)
	}

	if err := util.InitProtectionLogger(); err != nil {
		setupLog.Error(err, "unable to init protection logger")
		os.Exit(1)
	}

	// Resolve webhook bind address
	webhookHost := webhookBindAddress
	if webhookHost == "" {
		ip, err := utilnet.ResolveBindAddress(nil)
		if err != nil {
			setupLog.Error(err, "failed to resolve bind address, defaulting to 127.0.0.1")
			webhookHost = "127.0.0.1"
		} else {
			webhookHost = ip.String()
		}
	}
	setupLog.Info("webhook bind address", "address", webhookHost)

	// Harden TLS: enforce TLS1.2+, AEAD-only suites
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
	}
	setupLog.Info("webhook TLS hardened", "minVersion", "1.2", "cipherSuites", len(tlsConfig.CipherSuites))

	// Parse sync period
	var syncPeriod *time.Duration
	if syncPeriodStr != "" {
		if d, err := time.ParseDuration(syncPeriodStr); err != nil {
			setupLog.Error(err, "invalid sync period flag")
		} else {
			syncPeriod = &d
		}
	}

	// Create manager with secure defaults
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
			Host:    webhookHost,
			Port:    webhookPort,
			CertDir: webhookutil.GetCertDir(),
			TLSOpts: []func(*tls.Config){
				func(c *tls.Config) {
					c.MinVersion = tlsConfig.MinVersion
					c.CipherSuites = tlsConfig.CipherSuites
				},
			},
		}),
		NewCache: utilclient.NewCache,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := controllerfinder.InitControllerFinder(mgr); err != nil {
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
	if err := webhook.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to setup webhook")
		os.Exit(1)
	}

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
		if err := webhook.WaitReady(); err != nil {
			setupLog.Error(err, "unable to wait webhook ready")
			os.Exit(1)
		}
		setupLog.Info("setup controllers")
		if err := controller.SetupWithManager(mgr); err != nil {
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
	return map[string]cache.Config{ns: {}}
}
