/*
Copyright 2019 The Kruise Authors.
Copyright 2015 The Kubernetes Authors.

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

package e2e

import (
	"flag"
	"math/rand"
	"os"
	"testing"
	"time"

	// Never, ever remove the line with "/ginkgo". Without it,
	// the ginkgo test runner will not detect that this
	// directory contains a Ginkgo test suite.
	// See https://github.com/kubernetes/kubernetes/issues/74827
	// "github.com/onsi/ginkgo"

	kruiseapis "github.com/openkruise/kruise/apis"
	"github.com/openkruise/kruise/test/e2e/framework"
	"github.com/openkruise/kruise/test/e2e/generated"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"

	// test sources
	_ "github.com/openkruise/kruise/test/e2e/apps"
)

// handleFlags sets up all flags and parses the command line.
func handleFlags() {
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)
	flag.Parse()
}

func TestMain(m *testing.M) {
	// Register test flags, then parse flags.
	handleFlags()

	framework.AfterReadingAllFlags(&framework.TestContext)

	// Enable bindata file lookup as fallback.
	testfiles.AddFileSource(testfiles.BindataFileSource{
		Asset:      generated.Asset,
		AssetNames: generated.AssetNames,
	})

	// TODO: Deprecating repo-root over time... instead just use gobindata_util.go , see #23987.
	// Right now it is still needed, for example by
	// test/e2e/framework/ingress/ingress_utils.go
	// for providing the optional secret.yaml file and by
	// test/e2e/framework/util.go for cluster/log-dump.
	if framework.TestContext.RepoRoot != "" {
		testfiles.AddFileSource(testfiles.RootFileSource{Root: framework.TestContext.RepoRoot})
	}

	rand.Seed(time.Now().UnixNano())
	os.Exit(m.Run())
}

func TestE2E(t *testing.T) {

	err := kruiseapis.AddToScheme(scheme.Scheme)
	if err != nil {
		t.Fatal(err)
	}

	klog.Infof("Args: %v", os.Args)
	RunE2ETests(t)
}
