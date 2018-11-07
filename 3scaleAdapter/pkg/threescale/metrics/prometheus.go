package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const DefaultMetricsPort = 8080

var (
	// Range of buckets, in seconds for which metrics will be placed for system latency
	defaultSystemBucket = []float64{.05, .08, .1, .15, .2, .3, .5, 1.0, 1.5}

	// Range of buckets, in seconds for which metrics will be placed for backend latency
	defaultBackendBucket = []float64{.01, .02, .03, .05, .08, .1, .15, .2, .3, .5, 1.0}

	systemLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threescale_system_latency",
			Help:    "Request latency for requests to 3scale system URL",
			Buckets: defaultSystemBucket,
		},
		[]string{"systemURL", "serviceID"},
	)

	backendLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threescale_backend_latency",
			Help:    "Request latency for requests to 3scale backend",
			Buckets: defaultBackendBucket,
		},
		[]string{"backendURL", "serviceID"},
	)

	totalRequests = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "handle_authorization_requests",
			Help: "Total number of requests to adapter",
		},
	)

	cacheHits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "system_cache_hits",
			Help: "Total number of requests to 3scale system fetched from cache",
		},
	)
)

func init() {
	prometheus.MustRegister(systemLatency, backendLatency)
}

func ObserveSystemLatency(sysURL string, serviceId string, observed time.Duration) {
	systemLatency.WithLabelValues(sysURL, serviceId).Observe(observed.Seconds())
}

func ObserveBackendLatency(backendURL string, serviceId string, observed time.Duration) {
	backendLatency.WithLabelValues(backendURL, serviceId).Observe(observed.Seconds())
}

func IncrementTotalRequests() {
	totalRequests.Inc()
}

// IncrementCacheHits increments proxy configurations that have been read from the cache
func IncrementCacheHits() {
	cacheHits.Inc()
}
