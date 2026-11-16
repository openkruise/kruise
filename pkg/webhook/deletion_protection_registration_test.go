/*
Copyright 2026 The Kruise Authors.

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

package webhook

import "testing"

// The deletion-protection webhooks must be registered whatever the
// ResourcesDeletionProtection gate says, so the live
// ValidatingWebhookConfiguration keeps every entry the chart installs.
// The handlers check the gate themselves.
func TestDeletionProtectionWebhooksAreNotGated(t *testing.T) {
	for _, path := range []string{"/validate-ingress", "/validate-service", "/validate-namespace"} {
		if _, ok := HandlerMap[path]; !ok {
			t.Errorf("%s is not registered", path)
		}
		if _, gated := handlerGates[path]; gated {
			t.Errorf("%s is registered behind a gate, so kruise-manager drops it when the gate is off", path)
		}
	}
}
