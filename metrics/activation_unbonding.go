package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type ActivationUnbondingMonitorMetrics struct {
	Registry *prometheus.Registry

	ActivationDelayHistogram  prometheus.Histogram
	ActivationTimeoutsCounter *prometheus.CounterVec
	TrackedActivationGauge    prometheus.Gauge

	UnbondingDelayHistogram  prometheus.Histogram
	UnbondingTimeoutsCounter *prometheus.CounterVec
	TrackedUnbondingGauge    prometheus.Gauge

	CheckRunsTotal   prometheus.Counter
	CheckErrorsTotal prometheus.Counter
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
		UnbondingDelayHistogram: registerer.NewHistogram(prometheus.HistogramOpts{
			Name:    "vigilante_unbonding_delay_seconds",
			Help:    "Time delay between spending detection and Babylon recognition",
			Buckets: []float64{60, 300, 600, 1800, 3600, 7200},
		}),
		UnbondingTimeoutsCounter: registerer.NewCounterVec(prometheus.CounterOpts{
			Name: "vigilante_unbonding_timeouts_total",
			Help: "Number of unbonding timeouts detected",
		}, []string{"delegation_id"}),
		TrackedUnbondingGauge: registerer.NewGauge(prometheus.GaugeOpts{
			Name: "vigilante_unbonding_tracked_delegations",
			Help: "Number of delegations being tracked for unbonding timing",
		}),
		CheckRunsTotal: registerer.NewCounter(prometheus.CounterOpts{
			Name: "vigilante_timing_check_runs_total",
			Help: "Total number of timing check executions",
		}),
		CheckErrorsTotal: registerer.NewCounter(prometheus.CounterOpts{
			Name: "vigilante_timing_check_errors_total",
			Help: "Total number of errors during timing checks",
		}),
	}
}
