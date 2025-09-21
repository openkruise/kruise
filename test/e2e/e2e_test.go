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
	"os"
	"testing"

	"github.com/openkruise/kruise/test/e2e/framework/common"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	// Never, ever remove the line with "/ginkgo". Without it,
	// the ginkgo test runner will not detect that this
	// directory contains a Ginkgo test suite.
	// See https://github.com/kubernetes/kubernetes/issues/74827
	// "github.com/onsi/ginkgo/v2"

	kruiseapis "github.com/openkruise/kruise/apis"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	// test sources
	_ "github.com/openkruise/kruise/test/e2e/apps/v1alpha1"
	_ "github.com/openkruise/kruise/test/e2e/policy/v1alpha1"

	_ "github.com/openkruise/kruise/test/e2e/apps/v1beta1"
)

// handleFlags sets up all flags and parses the command line.
func handleFlags() {
	common.RegisterCommonFlags(flag.CommandLine)
	common.RegisterClusterFlags(flag.CommandLine)
	flag.Parse()
}

func TestMain(m *testing.M) {
	// Register test flags, then parse flags.
	handleFlags()

	common.AfterReadingAllFlags(&common.TestContext)

	os.Exit(m.Run())
}

func TestE2E(t *testing.T) {

	utilruntime.Must(kruiseapis.AddToScheme(scheme.Scheme))

	klog.Infof("Args: %v", os.Args)
	RunE2ETests(t)
}
