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
	"flag"
	"math/rand"
	"time"

	"github.com/openkruise/kruise/pkg/client"
	"github.com/openkruise/kruise/pkg/daemon"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	bindAddr = flag.String("addr", ":10221", "The address the metric endpoint and healthz binds to.")
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	cfg := config.GetConfigOrDie()
	cfg.UserAgent = "kruise-daemon"
	if err := client.NewRegistry(cfg); err != nil {
		klog.Fatalf("Failed to init clientset registry: %v", err)
	}

	stop := signals.SetupSignalHandler()
	d, err := daemon.NewDaemon(cfg, *bindAddr)
	if err != nil {
		klog.Fatalf("Failed to new daemon: %v", err)
	}
	if err := d.Run(stop); err != nil {
		klog.Fatalf("Failed to start daemon: %v", err)
	}
}
