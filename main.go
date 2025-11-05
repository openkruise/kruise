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
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"
	_ "time/tzdata" // for AdvancedCronJob Time Zone support

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
)

const (
	defaultLeaseDuration                     = 15 * time.Second
	defaultRenewDeadline                     = 10 * time.Second
	defaultRetryPeriod                       = 2 * time.Second
	defaultControllerCacheSyncTimeout        = 2 * time.Minute
	defaultWebhookInitializeTimeout          = 60 * time.Second
	defaultTtlsecondsForAlwaysNodeimageConst = 300
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

// getWebhookBindAddress determines the appropriate bind address for the webhook server
// following the same logic as kube-apiserver's --bind-address handling.
// It uses ResolveBindAddress to choose the first non-loopback address, avoiding binding to all interfaces (0.0.0.0/::).
func getWebhookBindAddress() string {
	// If WEBHOOK_HOST is explicitly set, respect that for backward compatibility
	if host := webhookutil.GetHost(); len(host) > 0 {
		setupLog.Info("Using explicit WEBHOOK_HOST", "address", host)
		return host
	}

	// Determine which address family to prefer based on the host's network configuration
	// Check if we have both IPv4 and IPv6 (dual-stack) or just one
	hasIPv4, hasIPv6 := checkAddressFamilies()

	var bindIP net.IP
	var err error

	if hasIPv4 && !hasIPv6 {
		// IPv4-only environment
		setupLog.Info("Detected IPv4-only environment")
		bindIP, err = utilnet.ResolveBindAddress(net.IPv4zero)
	} else if hasIPv6 && !hasIPv4 {
		// IPv6-only environment
		setupLog.Info("Detected IPv6-only environment")
		bindIP, err = utilnet.ResolveBindAddress(net.IPv6zero)
	} else if hasIPv4 && hasIPv6 {
		// Dual-stack: try IPv4 first, fallback to IPv6
		setupLog.Info("Detected dual-stack environment, preferring IPv4")
		bindIP, err = utilnet.ResolveBindAddress(net.IPv4zero)
		if err != nil {
			setupLog.Info("IPv4 resolution failed, trying IPv6", "error", err.Error())
			bindIP, err = utilnet.ResolveBindAddress(net.IPv6zero)
		}
	} else {
		// No suitable network interfaces found
		err = net.InvalidAddrError("no suitable network interfaces found")
	}

	if err != nil {
		setupLog.Error(err, "Failed to resolve bind address, this may cause connectivity issues")
		// Last resort fallback - but log it clearly for security awareness
		if hasIPv6 {
			setupLog.Info("Falling back to listening on all interfaces (::). Consider setting WEBHOOK_HOST environment variable for tighter security.")
			return "::"
		}
		setupLog.Info("Falling back to listening on all interfaces (0.0.0.0). Consider setting WEBHOOK_HOST environment variable for tighter security.")
		return "0.0.0.0"
	}

	resolvedAddr := bindIP.String()
	setupLog.Info("Resolved webhook bind address to first non-loopback interface", "address", resolvedAddr)
	return resolvedAddr
}

// checkAddressFamilies detects which IP address families (IPv4/IPv6) are available on the host
func checkAddressFamilies() (hasIPv4, hasIPv6 bool) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		setupLog.Error(err, "Failed to get interface addresses, assuming dual-stack")
		// Conservative assumption: assume both are available
		return true, true
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				hasIPv4 = true
			} else if ipnet.IP.To16() != nil {
				hasIPv6 = true
			}
		}
	}

	if !hasIPv4 && !hasIPv6 {
		setupLog.Info("No non-loopback addresses found, assuming localhost-only environment")
		// If we only have loopback, we still need to pick one
		// Check loopback interfaces
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					hasIPv4 = true
				} else if ipnet.IP.To16() != nil {
					hasIPv6 = true
				}
			}
		}
	}

	return hasIPv4, hasIPv6
}

// getSecureTLSConfig returns a TLS configuration with secure settings
// This enforces TLS 1.2+ and secure cipher suites, addressing the security concerns
// about TLS 1.0/1.1 and weak cipher suites (RSA key exchange, CBC mode)
func getSecureTLSConfig() *tls.Config {
	return &tls.Config{
		// Minimum TLS 1.2 - addresses TLS 1.0/1.1 vulnerability
		MinVersion: tls.VersionTLS12,

		// Prefer server cipher suites for better security control
		PreferServerCipherSuites: true,

		// Only allow secure cipher suites:
		// - ECDHE for forward secrecy (not RSA key exchange)
		// - GCM/ChaCha20 for AEAD (not CBC mode which is vulnerable to padding oracle attacks)
		CipherSuites: []uint16{
			// TLS 1.3 cipher suites (Go 1.22+ supports these automatically)
			// TLS 1.2 cipher suites
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},

		// Note: TLS 1.3 is automatically enabled by Go 1.22+ and doesn't use CipherSuites
		// TLS 1.3 cipher suites are not configurable and are always secure
	}
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
	var defaultTtlsecondsForAlwaysNodeimage int

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
	flag.IntVar(&defaultTtlsecondsForAlwaysNodeimage, "default-ttlseconds-for-always-nodeimage", defaultTtlsecondsForAlwaysNodeimageConst, "DefaultTtlsecondsForAlwaysNodeimage refers to the calculation of the time limit the lifetime of a pulling task that has finished execution. Defaults to 300 seconds if not set.")

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
	if err := util.SetDefaultTtlForAlwaysNodeimage(defaultTtlsecondsForAlwaysNodeimage); err != nil {
		setupLog.Error(err, "default-ttlseconds-for-always-nodeimage validate failed")
		os.Exit(1)
	}

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
			Host:    getWebhookBindAddress(),
			Port:    webhookutil.GetPort(),
			CertDir: webhookutil.GetCertDir(),
			TLSOpts: []func(*tls.Config){
				func(config *tls.Config) {
					secureTLS := getSecureTLSConfig()
					config.MinVersion = secureTLS.MinVersion
					config.CipherSuites = secureTLS.CipherSuites
					config.PreferServerCipherSuites = secureTLS.PreferServerCipherSuites
				},
			},
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

	setupLog.Info("Webhook server configured with secure TLS settings",
		"minTLSVersion", "1.2",
		"cipherSuites", "ECDHE-GCM/ChaCha20 only (no RSA key exchange, no CBC mode)")

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
