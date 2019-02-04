package metrics

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
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

// LatencyReport defines the time taken for a report to 3scale
// Endpoint should be set as "Authorize" or "Report" for backend requests
// Target must be one of "Backend" or "System" for a report to be processed
type LatencyReport struct {
	Endpoint  string
	TimeTaken time.Duration
	URL       string
	Target    Target
}

// StatusReport defines a HTTP status code report from 3scale
// Endpoint should be set as "Authorize" or "Report" for backend requests
// Currently only "Backend" Target supported
type StatusReport struct {
	Endpoint string
	Code     int
	URL      string
	Target   Target
}

// Target is a legitimate target to report 3scale metrics from
type Target string

// Backend target should be used when reporting latency or status codes from 3scale backend
const Backend Target = "Backend"

// System target should be used when reporting latency or status codes from 3scale system
const System Target = "System"

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
		[]string{"backendURL", "serviceID", "endpoint"},
	)

	backendStatusCodes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threescale_backend_http_status",
			Help: "HTTP Status response codes for requests to 3scale backend",
		},
		[]string{"backendURL", "serviceID", "code"},
	)

	cacheHits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "threescale_system_cache_hits",
			Help: "Total number of requests to 3scale system fetched from cache",
		},
	)
)

// NewMetricsReporter creates a new Reporter
func NewMetricsReporter(reportMetrics bool, serveOnPort int) *Reporter {
	return &Reporter{reportMetrics, serveOnPort}
}

// NewLatencyReport creates a LatencyReport
func NewLatencyReport(endpoint string, duration time.Duration, url string, target Target) LatencyReport {
	return LatencyReport{
		Endpoint:  endpoint,
		TimeTaken: duration,
		URL:       url,
		Target:    target,
	}
}

// NewStatusReport creates a StatusReport
func NewStatusReport(endpoint string, code int, url string, target Target) StatusReport {
	return StatusReport{
		Endpoint: endpoint,
		Code:     code,
		URL:      url,
		Target:   target,
	}
}

// ReportMetrics reports a LatencyReport and StatusReport to Prometheus.
// It ignores errors from creating metrics so if the error needs to be handled outside
// of being logged, the metrics should be reported directly.
func (r *Reporter) ReportMetrics(serviceID string, l LatencyReport, s StatusReport) {
	if r != nil && r.shouldReport {
		r.ObserveLatency(serviceID, l)
		r.ReportStatus(serviceID, s)
	}
}

// ObserveLatency reports a metric to a latency histogram.
// Logs and returns an error in cases where the metric has not been reported.
func (r *Reporter) ObserveLatency(serviceID string, l LatencyReport) error {
	if r != nil && r.shouldReport {
		o, err := l.getObserver(serviceID)
		if err != nil {
			log.Errorf(err.Error())
			return err
		}

		o.Observe(l.TimeTaken.Seconds())
	}
	return nil
}

// ReportStatus reports a hit to 3scale backend and reports status code of the result
// Logs and returns an error in cases where the metric has not been reported.
func (r *Reporter) ReportStatus(serviceID string, s StatusReport) error {
	if r != nil && r.shouldReport {
		codeStr := strconv.Itoa(s.Code)
		if len(codeStr) != 3 {
			return errors.New("invalid status code reported")
		}

		switch s.Target {
		case Backend:
			if s.Endpoint != "" {
				backendStatusCodes.WithLabelValues(s.URL, serviceID, codeStr).Inc()
			}

		default:
			return fmt.Errorf("unknown target %s", s.Target)
		}
	}
	return nil
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
	prometheus.MustRegister(systemLatency, backendLatency, cacheHits)
	http.Handle("/metrics", promhttp.Handler())
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", r.serveOnPort))
	if err != nil {
		panic(err)
	}
	go http.Serve(listener, nil)
	log.Infof("Serving metrics on port %d", r.serveOnPort)
}

// getObserver generates a Prometheus Observer from the fields of a LatencyReport
// Returns an error in cases where the LatencyReport contains missing or incomplete
// data for the chosen Target
func (l LatencyReport) getObserver(serviceID string) (prometheus.Observer, error) {
	var o prometheus.Observer
	var err error

	if l.URL == "" {
		return o, fmt.Errorf("url label must be set when reporting request latency")
	}

	switch l.Target {
	case Backend:
		if l.Endpoint != "" {
			o = backendLatency.WithLabelValues(l.URL, serviceID, l.Endpoint)
		} else {
			err = fmt.Errorf("reporting latency to 3scale backend requires an endpoint label to be set")
		}
	case System:
		o = systemLatency.WithLabelValues(l.URL, serviceID)
	default:
		err = fmt.Errorf("unsupported target %s", l.Target)
	}

	return o, err
}
