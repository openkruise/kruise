package leadership

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"k8s.io/client-go/tools/leaderelection"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	leaderMetric = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kruise_manager_is_leader",
			Help: "Gauge the leader of kruise-manager",
		},
	)
	leaderSlowpathCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "leader_election_slowpath_total",
		Help: "Total number of slow path exercised in renewing leader leases. 'name' is the string used to identify the lease. Please make sure to group by name.",
	}, []string{"name"})
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
	_ leaderelection.LeaderMetric    = switchMetric{}
)

func (_ metricsProvider) NewLeaderMetric() leaderelection.LeaderMetric {
	return switchMetric{}
}

func (_ switchMetric) On(_ string) {
	leaderMetric.Set(1)
}

func (s switchMetric) Off(string) {
	leaderMetric.Set(0)
}

// todo: why not use leaderelection.NewLeaderMetric?
func (s switchMetric) SlowpathExercised(name string) {
	leaderSlowpathCounter.WithLabelValues(name).Inc()
}
