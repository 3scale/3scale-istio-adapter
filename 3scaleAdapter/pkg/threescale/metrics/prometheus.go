package metrics

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"istio.io/istio/pkg/log"
)

// Reporter holds configuration for the Prometheus metrics implementation
type Reporter struct {
	shouldReport bool
	serveOnPort  int
}

// defaultMetricsPort - Default port that metrics endpoint will be served on
const defaultMetricsPort = 8080

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

// NewMetricsReporter creates a new Reporter
func NewMetricsReporter(reportMetrics bool, serveOnPort int) *Reporter {
	return &Reporter{reportMetrics, serveOnPort}
}

// ObserveSystemLatency reports a metric to system latency histogram
func (r *Reporter) ObserveSystemLatency(sysURL string, serviceID string, observed time.Duration) {
	if r != nil && r.shouldReport {
		systemLatency.WithLabelValues(sysURL, serviceID).Observe(observed.Seconds())
	}
}

// ObserveBackendLatency reports a metric to backend latency histogram
func (r *Reporter) ObserveBackendLatency(backendURL string, serviceID string, observed time.Duration) {
	if r != nil && r.shouldReport {
		backendLatency.WithLabelValues(backendURL, serviceID).Observe(observed.Seconds())
	}
}

// IncrementTotalRequests increments the request count for authorization handler
func (r *Reporter) IncrementTotalRequests() {
	if r != nil && r.shouldReport {
		totalRequests.Inc()
	}
}

// IncrementCacheHits increments proxy configurations that have been read from the cache
func (r *Reporter) IncrementCacheHits() {
	if r != nil && r.shouldReport {
		cacheHits.Inc()
	}
}

// Serve starts a HTTP server and publishes metrics for scraping at the /metrics endpoint
func (r *Reporter) Serve() {
	if r.serveOnPort == 0 {
		r.serveOnPort = defaultMetricsPort
	}
	prometheus.MustRegister(systemLatency, backendLatency, totalRequests, cacheHits)
	http.Handle("/metrics", promhttp.Handler())
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", r.serveOnPort))
	if err != nil {
		panic(err)
	}
	go http.Serve(listener, nil)
	log.Infof("Serving metrics on port %d", r.serveOnPort)
}
