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

package pubcontrol

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	PodUnavailableBudgetMetrics = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pod_unavailable_budget",
			Help: "Pod Unavailable Budget Metrics",
			// kind = CloneSet, Deployment, StatefulSet, etc.
			// name = workload.name, if pod don't have workload, then name is pod.name
			// username = client useragent
		}, []string{"kind_namespace_name", "username"},
	)
)

func init() {
	metrics.Registry.MustRegister(PodUnavailableBudgetMetrics)
}
