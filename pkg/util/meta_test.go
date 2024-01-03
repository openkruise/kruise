/*
Copyright 2022 The Kruise Authors.

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

package util

import (
	"os"
	"testing"
)

func TestMetaGetNamespace(t *testing.T) {
	if GetKruiseNamespace() != "kruise-system" {
		t.Fatalf("expect(kruise-system), but get(%s)", GetKruiseNamespace())
	}
	_ = os.Setenv("POD_NAMESPACE", "test")
	if GetKruiseNamespace() != "test" {
		t.Fatalf("expect(test), but get(%s)", GetKruiseNamespace())
	}
	if GetKruiseDaemonConfigNamespace() != "kruise-daemon-config" {
		t.Fatalf("expect(kruise-daemon-config), but get(%s)", GetKruiseDaemonConfigNamespace())
	}
	_ = os.Setenv("KRUISE_DAEMON_CONFIG_NS", "test")
	if GetKruiseDaemonConfigNamespace() != "test" {
		t.Fatalf("expect(test), but get(%s)", GetKruiseDaemonConfigNamespace())
	}
}
