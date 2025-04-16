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

package e2e

import (
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"github.com/openkruise/kruise/test/e2e/framework"

	runtimeutils "k8s.io/apimachinery/pkg/util/runtime"
)

// Similar to SynchronizedBeforeSuite, we want to run some operations only once (such as collecting cluster logs).
// Here, the order of functions is reversed; first, the function which runs everywhere,
// and then the function that only runs on the first Ginkgo node.
var _ = ginkgo.SynchronizedAfterSuite(func() {
	// Run on all Ginkgo nodes
	framework.Logf("Running AfterSuite actions on all nodes")
	framework.RunCleanupActions()
}, func() {
	// Run only Ginkgo on node 1
	framework.Logf("Running AfterSuite actions on node 1")
})

// RunE2ETests checks configuration parameters (specified through flags) and then runs
// E2E tests using the Ginkgo runner.
// If a "report directory" is specified, one or more JUnit test reports will be
// generated in this directory, and cluster logs will also be saved.
// This function is called on each Ginkgo node in parallel mode.
func RunE2ETests(t *testing.T) {
	runtimeutils.ReallyCrash = true

	gomega.RegisterFailHandler(ginkgo.Fail)
	// Disable skipped tests unless they are explicitly requested.
	if len(config.GinkgoConfig.FocusStrings) == 0 && len(config.GinkgoConfig.SkipStrings) == 0 {
		config.GinkgoConfig.SkipStrings = []string{`\[Flaky\]|\[Feature:.+\]`}
	}

	// Run tests through the Ginkgo runner with output to console + JUnit for Jenkins
	var r []ginkgo.Reporter
	klog.Infof("Starting e2e run %q on Ginkgo node %d", framework.RunID, config.GinkgoConfig.ParallelNode)

	ginkgo.RunSpecsWithDefaultAndCustomReporters(t, "Kruise e2e suite", r)
}
