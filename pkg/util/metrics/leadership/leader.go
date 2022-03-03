package leadership

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"k8s.io/client-go/tools/leaderelection"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var leaderMetric = promauto.NewGauge(
	prometheus.GaugeOpts{
		Name: "kruise_manager_is_leader",
		Help: "Gauge the leader of kruise-manager",
	},
)

func init() {
	metrics.Registry.MustRegister(leaderMetric)
	leaderelection.SetProvider(newMetricProvider())
}

func newMetricProvider() leaderelection.MetricsProvider {
	return metricsProvider{}
}

type metricsProvider struct{}
type switchMetric struct{}

var (
	_ leaderelection.MetricsProvider = metricsProvider{}
	_ leaderelection.SwitchMetric    = switchMetric{}
)

func (_ metricsProvider) NewLeaderMetric() leaderelection.SwitchMetric {
	return switchMetric{}
}

func (_ switchMetric) On(_ string) {
	leaderMetric.Set(1)
}

func (s switchMetric) Off(string) {
	leaderMetric.Set(0)
}
