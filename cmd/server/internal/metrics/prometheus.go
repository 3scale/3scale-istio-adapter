package metrics

import (
	"net/http"
	"strconv"

	"github.com/3scale/3scale-authorizer/pkg/authorizer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// defaultMetricsPort - Default port that metrics endpoint will be served on
const defaultMetricsPort = 8080

var (
	// Range of buckets, in seconds for which metrics will be placed for 3scale latency
	threescaleBucket = []float64{.01, .02, .03, .05, .08, .1, .15, .2, .3, .5, 1.0, 1.5}

	threescaleLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threescale_latency",
			Help:    "Request latency between adapter and 3scale",
			Buckets: threescaleBucket,
		},
		[]string{"host", "method", "endpoint"},
	)

	threescaleHTTP = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threescale_http_total",
			Help: "HTTP Status response codes for requests to 3scale backend",
		},
		[]string{"host", "method", "endpoint", "status"},
	)

	cacheHitsSystem = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "threescale_system_cache_hits",
			Help: "Total number of requests to 3scale system fetched from cache",
		},
	)

	cacheHitsBackend = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "threescale_backend_cache_hits",
			Help: "Total number of requests to 3scale backend fetched from cache",
		},
	)
)

func ReportCB(tr authorizer.TelemetryReport) {
	latencyObserver := threescaleLatency.WithLabelValues(tr.Host, tr.Method, tr.Endpoint)
	latencyObserver.Observe(tr.TimeTaken.Seconds())

	threescaleHTTP.WithLabelValues(tr.Host, tr.Method, tr.Endpoint, strconv.Itoa(tr.Code)).Inc()
}

// IncrementCacheHits increments proxy configurations that have been read from the cache
func IncrementCacheHits(cache authorizer.Cache) {
	if cache == authorizer.System {
		cacheHitsSystem.Inc()
		return
	}
	cacheHitsBackend.Inc()
}

func Register() {
	prometheus.MustRegister(threescaleLatency, threescaleHTTP, cacheHitsSystem, cacheHitsBackend)
}

func GetHandler() http.Handler {
	return promhttp.Handler()
}
