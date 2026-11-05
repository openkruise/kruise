/*
Copyright 2023 The Kruise Authors.

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
	"github.com/openkruise/kruise/pkg/webhook/ingress/validating"
)

func init() {
	// The ResourcesDeletionProtection gate is checked inside IngressHandler.Handle via
	// deletionprotection.ValidateIngressDeletion, not at registration time, so that the live
	// ValidatingWebhookConfiguration always matches what the chart renders, consistent with
	// the namespace/CRD/builtin-workloads webhooks.
	addHandlers(validating.HandlerGetterMap)
}
