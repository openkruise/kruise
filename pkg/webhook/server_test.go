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

import (
	"testing"

	"github.com/openkruise/kruise/pkg/features"
	utilfeature "github.com/openkruise/kruise/pkg/util/feature"
)

// TestIngressServiceHandlersSurviveGateFiltering ensures the ingress/service
// deletion-protection webhooks stay registered in HandlerMap regardless of the
// ResourcesDeletionProtection feature gate, just like the namespace/CRD/builtin-workloads
// deletion-protection webhooks. Those handlers already enforce the gate themselves
// (deletionprotection.ValidateIngressDeletion / ValidateServiceDeletion), so gating their
// registration on top of that made the live ValidatingWebhookConfiguration lose the
// vingress.kb.io/vservice.kb.io entries whenever the gate was left at its default (off),
// even though the chart still rendered them.
func TestIngressServiceHandlersSurviveGateFiltering(t *testing.T) {
	if enabled := utilfeature.DefaultFeatureGate.Enabled(features.ResourcesDeletionProtection); enabled {
		t.Fatalf("expected ResourcesDeletionProtection to default to disabled, got enabled")
	}

	for _, path := range []string{"/validate-ingress", "/validate-service"} {
		if _, ok := HandlerMap[path]; !ok {
			t.Fatalf("expected %s to be registered in HandlerMap", path)
		}
		if _, gated := handlerGates[path]; gated {
			t.Fatalf("expected %s to be registered unconditionally (no handlerGates entry), the gate must be enforced inside the handler instead", path)
		}
	}

	filterActiveHandlers()

	for _, path := range []string{"/validate-ingress", "/validate-service"} {
		if _, ok := HandlerMap[path]; !ok {
			t.Errorf("%s was dropped from HandlerMap by filterActiveHandlers with ResourcesDeletionProtection disabled; "+
				"the live ValidatingWebhookConfiguration would then miss this entry even though the chart renders it unconditionally", path)
		}
	}
}
