/*
Copyright 2021 The Kruise Authors.

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
	"k8s.io/kubernetes/pkg/credentialprovider/plugin"

	"flag"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/daemon"
	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

var (
	bindAddr         = flag.String("addr", ":10221", "The address the metric endpoint and healthz binds to.")
	pprofAddr        = flag.String("pprof-addr", ":10222", "The address the pprof binds to.")
	pluginConfigFile string
	pluginBinDir     string
)

func main() {
	flag.StringVar(&pluginConfigFile, "pluginConfigFile", "/kruise/CredentialProviderPlugin.yaml", "The path of plugin config file.")
	flag.StringVar(&pluginBinDir, "pluginBinDir", "/kruise/plugins", "The path of directory of plugin binaries.")
	utilfeature.DefaultMutableFeatureGate.AddFlag(pflag.CommandLine)
	klog.InitFlags(nil)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	rand.Seed(time.Now().UnixNano())
	features.SetDefaultFeatureGates()
	ctrl.SetLogger(klogr.New())

	cfg := config.GetConfigOrDie()
	cfg.UserAgent = "kruise-daemon"
	if err := client.NewRegistry(cfg); err != nil {
		klog.Fatalf("Failed to init clientset registry: %v", err)
	}
	go func() {
		if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
			klog.Fatal(err, "unable to start pprof")
		}
	}()
	ctx := signals.SetupSignalHandler()
	d, err := daemon.NewDaemon(cfg, *bindAddr)
	if err != nil {
		klog.Fatalf("Failed to new daemon: %v", err)
	}

	err = plugin.RegisterCredentialProviderPlugins(pluginConfigFile, pluginBinDir)
	if err != nil {
		klog.Errorf("Failed to register credential provider plugins: %v", err)
	}

	if err := d.Run(ctx); err != nil {
		klog.Fatalf("Failed to start daemon: %v", err)
	}
}
