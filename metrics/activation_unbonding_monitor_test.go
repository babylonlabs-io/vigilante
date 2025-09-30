package metrics

import (
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestActivationUnbonding(t *testing.T) {
	metrics := NewActivationUnbondingMonitorMetrics()

	// 0 from point of initialisation
	require.Equal(t, 0.0, promtestutil.ToFloat64(metrics.ActivationTimeoutsCounter))
	require.Equal(t, 0.0, promtestutil.ToFloat64(metrics.TrackedActivationGauge))
	require.Equal(t, 0.0, promtestutil.ToFloat64(metrics.ActivationTimeoutsCounter))

	metric := &dto.Metric{}
	err := metrics.ActivationDelayHistogram.Write(metric)
	require.NoError(t, err)

	hist := metric.GetHistogram()
	require.Equal(t, uint64(0), hist.GetSampleCount())

	metrics.TrackedActivationGauge.Inc()
	metrics.ActivationTimeoutsCounter.Inc()
	metrics.ActivationDelayHistogram.Observe(45)

	metric2 := &dto.Metric{}
	err = metrics.ActivationDelayHistogram.Write(metric2)
	require.NoError(t, err)
	hist2 := metric2.GetHistogram()

	require.InDelta(t, 45.0, hist2.GetSampleSum(), 0.01)
	require.Equal(t, 1.0, promtestutil.ToFloat64(metrics.TrackedActivationGauge))
	require.Equal(t, 1.0, promtestutil.ToFloat64(metrics.ActivationTimeoutsCounter))

	metrics.TrackedActivationGauge.Dec()

	require.Equal(t, 0.0, promtestutil.ToFloat64(metrics.
		TrackedActivationGauge))
}
