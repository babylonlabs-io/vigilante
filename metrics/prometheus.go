package metrics

import (
	"net/http"
	_ "net/http/pprof" // #nosec G108 we want this
	"regexp"
	"time"

	"github.com/babylonlabs-io/vigilante/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

func Start(addr string, reg *prometheus.Registry) {
	go start(addr, reg)
}

func start(addr string, reg *prometheus.Registry) {
	// Add Go module build info.
	reg.MustRegister(collectors.NewBuildInfoCollector())
	reg.MustRegister(collectors.NewGoCollector(
		collectors.WithGoCollectorRuntimeMetrics(collectors.GoRuntimeMetricsRule{Matcher: regexp.MustCompile("/.*")})),
	)

	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))
	metricsLogger, err := config.NewRootLogger("auto", "debug")
	if err != nil {
		panic(err)
	}
	log := metricsLogger.With(zap.String("module", "metrics")).Sugar()
	log.Infof("Successfully started Prometheus metrics server at %s", addr)
	srv := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start metrics server: %v", err)
	}
}
