package servicesync

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	metricServicesExposed = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mcsync_services_exposed",
			Help: "How many services does this cluster expose",
		},
	)
	metricServicesConfigured = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mcsync_services_configured",
			Help: "How many services (unique by cluster + service name) does this cluster receive & configure",
		},
	)
	metricUpdateMaxAge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			// it is convention to end the metric name with the unit (is there is a unit)
			Name: "mcsync_max_update_age_seconds",
			Help: "How long ago the last update was received from any cluster",
		},
	)
	metricUpdateMinAge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			// it is convention to end the metric name with the unit (is there is a unit)
			Name: "mcsync_min_update_age_seconds",
			Help: "How long ago the most recent update was received from any cluster",
		},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		metricServicesExposed,
		metricServicesConfigured,
		metricUpdateMaxAge,
	)
}
