package datasource

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	metricPublishes = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mcsync_pubsub_publish",
			Help: "Number of times published",
		},
	)
	metricReceives = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mcsync_pubsub_received",
			Help: "Number of times a message was received",
		},
	)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		metricPublishes,
		metricReceives,
	)
}
