package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type ActivationUnbondingMonitorMetrics struct {
	Registry                  *prometheus.Registry
	ActivationDelayHistogram  prometheus.Histogram
	ActivationTimeoutsCounter *prometheus.CounterVec
	TrackedActivationGauge    prometheus.Gauge
}

func NewActivationUnbondingMonitorMetrics() *ActivationUnbondingMonitorMetrics {
	registry := prometheus.NewRegistry()
	registerer := promauto.With(registry)

	return &ActivationUnbondingMonitorMetrics{
		Registry: registry,
		ActivationDelayHistogram: registerer.NewHistogram(prometheus.HistogramOpts{
			Name:    "vigilante_activation_delay_seconds",
			Help:    "Time delay between k-deep confirmation and activation",
			Buckets: []float64{30, 60, 120, 300, 600, 1800, 3600},
		}),
		ActivationTimeoutsCounter: registerer.NewCounterVec(prometheus.CounterOpts{
			Name: "vigilante_activation_timeouts_total",
			Help: "Number of activation timeouts detected",
		}, []string{"delegation_id"}),
		TrackedActivationGauge: registerer.NewGauge(prometheus.GaugeOpts{
			Name: "vigilante_activation_tracked_delegations",
			Help: "Number of delegations being tracked for activation timing",
		}),
	}
}
