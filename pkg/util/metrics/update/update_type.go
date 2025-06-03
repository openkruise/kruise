package update

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	RecreateDueToVolumesChange = "volumes_change"
	InplacePrefix              = "inplace_"

	ImageUpdateMark    = 1
	MetadataUpdateMark = 1 << 1
	ResourceUpdateMark = 1 << 2
)

var (
	updateTypeMetric = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kruise_pod_update_type",
			Help: "Number of pod updates by type.",
		},
		[]string{"type"})
	updateDurationMetric = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kruise_pod_update_duration",
		Help:    "Pod update duration in seconds",
		Buckets: []float64{15, 30, 60, 120, 240},
	}, []string{"type"})
)

func init() {
	metrics.Registry.MustRegister(updateTypeMetric)
	metrics.Registry.MustRegister(updateDurationMetric)
}

func RecordUpdateType(updateType string) {
	updateTypeMetric.WithLabelValues(updateType).Inc()
}

func RecordUpdateDuration(updateType string, seconds int) {
	updateDurationMetric.WithLabelValues(updateType).Observe(float64(seconds))
}

func GetInplaceUpdateType(imageUpdate bool, metadataUpdate bool, resourceUpdate bool) string {
	flag := 0
	if imageUpdate {
		flag |= ImageUpdateMark
	}
	if metadataUpdate {
		flag |= MetadataUpdateMark
	}
	if resourceUpdate {
		flag |= ResourceUpdateMark
	}
	return fmt.Sprintf("%s%d", InplacePrefix, flag)
}

func RecordInplaceReason(imageUpdate bool, metadataUpdate bool, resourceUpdate bool) {
	RecordUpdateType(GetInplaceUpdateType(imageUpdate, metadataUpdate, resourceUpdate))
}
