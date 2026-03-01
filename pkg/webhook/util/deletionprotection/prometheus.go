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

package deletionprotection

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	NamespaceDeletionProtectionMetrics = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "namespace_deletion_protection",
			Help: "Namespace Deletion Protection",
		}, []string{"name", "username"},
	)

	CRDDeletionProtectionMetrics = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "crd_deletion_protection",
			Help: "CustomResourceDefinition Deletion Protection",
		}, []string{"name", "username"},
	)

	WorkloadDeletionProtectionMetrics = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workload_deletion_protection",
			Help: "Workload Deletion Protection",
		}, []string{"kind_namespace_name", "username"},
	)
)

func init() {
	metrics.Registry.MustRegister(NamespaceDeletionProtectionMetrics)
	metrics.Registry.MustRegister(CRDDeletionProtectionMetrics)
	metrics.Registry.MustRegister(WorkloadDeletionProtectionMetrics)
}
