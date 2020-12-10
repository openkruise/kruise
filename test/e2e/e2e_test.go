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
	"testing"

	kruiseapis "github.com/openkruise/kruise/apis"
	"github.com/openkruise/kruise/test/e2e/framework"
	"k8s.io/client-go/kubernetes/scheme"

	// test sources
	_ "github.com/openkruise/kruise/test/e2e/apps"
)

func init() {
	// Register framework flags, then handle flags and Viper config.
	//framework.HandleFlags()

	//framework.AfterReadingAllFlags(&framework.TestContext)
}

func TestE2E(t *testing.T) {
	// Register framework flags, then handle flags and Viper config.
	framework.HandleFlags()

	framework.AfterReadingAllFlags(&framework.TestContext)

	err := kruiseapis.AddToScheme(scheme.Scheme)
	if err != nil {
		t.Fatal(err)
	}

	RunE2ETests(t)
}
