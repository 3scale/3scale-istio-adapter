package metrics

import (
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

const url = "www.fake.com"
const serviceID = "123"

func TestObserveSystemLatency(t *testing.T) {
	const metricName = "threescale_system_latency"
	const expect = `

                # HELP threescale_system_latency Request latency for requests to 3scale system URL
                # TYPE threescale_system_latency histogram
                threescale_system_latency_bucket{serviceID="123",systemURL="www.fake.com",le="0.05"} 0
                threescale_system_latency_bucket{serviceID="123",systemURL="www.fake.com",le="0.08"} 0
                threescale_system_latency_bucket{serviceID="123",systemURL="www.fake.com",le="0.1"} 0
                threescale_system_latency_bucket{serviceID="123",systemURL="www.fake.com",le="0.15"} 0
                threescale_system_latency_bucket{serviceID="123",systemURL="www.fake.com",le="0.2"} 0
                threescale_system_latency_bucket{serviceID="123",systemURL="www.fake.com",le="0.3"} 0
                threescale_system_latency_bucket{serviceID="123",systemURL="www.fake.com",le="0.5"} 0
                threescale_system_latency_bucket{serviceID="123",systemURL="www.fake.com",le="1"} 0
                threescale_system_latency_bucket{serviceID="123",systemURL="www.fake.com",le="1.5"} 1
                threescale_system_latency_bucket{serviceID="123",systemURL="www.fake.com",le="+Inf"} 1
                threescale_system_latency_sum{serviceID="123",systemURL="www.fake.com"} 1.001
                threescale_system_latency_count{serviceID="123",systemURL="www.fake.com"} 1

        `
	r := NewMetricsReporter(true, 8080)
	l := NewLatencyReport("", time.Second+time.Millisecond, url, System)
	r.ObserveLatency(serviceID, l)
	err := testutil.CollectAndCompare(systemLatency, strings.NewReader(expect), metricName)
	if err != nil {
		t.Fatalf(err.Error())
	}
}

func TestObserveBackendLatency(t *testing.T) {
	const metricName = "threescale_backend_latency"
	const expect = `
		# HELP threescale_backend_latency Request latency for requests to 3scale backend
		# TYPE threescale_backend_latency histogram
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="0.01"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="0.02"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="0.03"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="0.05"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="0.08"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="0.1"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="0.15"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="0.2"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="0.3"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="0.5"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="1"} 1
		threescale_backend_latency_bucket{backendURL="www.fake.com",endpoint="Authorise",serviceID="123",le="+Inf"} 1
		threescale_backend_latency_sum{backendURL="www.fake.com",endpoint="Authorise",serviceID="123"} 1
		threescale_backend_latency_count{backendURL="www.fake.com",endpoint="Authorise",serviceID="123"} 1

	`
	r := NewMetricsReporter(true, 8080)
	l := LatencyReport{
		Endpoint:  "Authorise",
		TimeTaken: time.Second,
		URL:       url,
		Target:    Backend,
	}
	r.ObserveLatency(serviceID, l)
	err := testutil.CollectAndCompare(backendLatency, strings.NewReader(expect), metricName)
	if err != nil {
		t.Fatalf(err.Error())
	}
}

func TestReportStatus(t *testing.T) {
	inputs := []struct {
		name       string
		metricName string
		expect     string
		expectErr  bool
		collector  *prometheus.CounterVec
		code       int
		t          Target
	}{
		{
			name:       "Test invalid status code",
			metricName: "threescale_backend_http_status",
			expect:     ``,
			expectErr:  true,
			collector:  backendStatusCodes,
		},
		{
			name:       "Test Report unsupported target",
			metricName: "threescale_backend_http_status",
			expect:     ``,
			collector:  backendStatusCodes,
			expectErr:  true,
			code:       http.StatusOK,
			t:          "unknown",
		},
		{
			name:       "Test Report Backend HTTP Status",
			metricName: "threescale_backend_http_status",
			expect: `
			       # HELP threescale_backend_http_status HTTP Status response codes for requests to 3scale backend
			       # TYPE threescale_backend_http_status counter
			       threescale_backend_http_status{backendURL="www.fake.com",code="200",serviceID="123"} 1
		       `,
			collector: backendStatusCodes,
			code:      http.StatusOK,
			t:         Backend,
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {

			desc := make(chan *prometheus.Desc, 1)
			input.collector.Describe(desc)
			d := <-desc

			s := NewStatusReport("Authorise", input.code, url, input.t)
			r := NewMetricsReporter(true, 8080)
			err := r.ReportStatus(serviceID, s)
			if err != nil {
				if input.expectErr {
					return
				}
				t.Fatalf("unexpected error")
			}

			err = testutil.CollectAndCompare(input.collector, strings.NewReader(input.expect), input.metricName)
			if err != nil {
				t.Fatalf(err.Error())
			}

			if testutil.ToFloat64(input.collector) != 1 {
				t.Fatalf("unexpected counter value for %s", d.String())
			}
			input.collector.Reset()
		})
	}
}

func TestIncrementCacheHits(t *testing.T) {
	collector := cacheHits
	if testutil.ToFloat64(collector) != 0 {
		t.Fatalf("unexpected counter value for %s", collector.Desc().String())
	}
	r := NewMetricsReporter(true, 8080)

	incrementBy := randCounterInc(t, r.IncrementCacheHits)
	if testutil.ToFloat64(collector) != float64(incrementBy) {
		t.Fatalf("unexpected counter value for %s", collector.Desc().String())
	}
}

func TestServe(t *testing.T) {
	r := NewMetricsReporter(true, 0)
	r.Serve()

}

func randCounterInc(t *testing.T, inc func()) int {
	t.Helper()
	incrementBy := int(rand.Int31n(9) + 1)
	for i := 1; i <= incrementBy; i++ {
		inc()
	}
	return incrementBy
}
