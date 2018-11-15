package metrics

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

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
	r.ObserveSystemLatency("www.fake.com", "123", time.Second+time.Millisecond)
	// TODO - Uncomment when https://github.com/prometheus/client_golang/issues/498 is resolved
	//err := testutil.CollectAndCompare(systemLatency, strings.NewReader(expect), metricName)
	//if err != nil {
	//	t.Fatalf(err.Error())
	//}
}

func TestObserveBackendLatency(t *testing.T) {
	const metricName = "threescale_backend_latency"
	const expect = `
		# HELP threescale_backend_latency Request latency for requests to 3scale backend
		# TYPE threescale_backend_latency histogram
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="0.01"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="0.02"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="0.03"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="0.05"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="0.08"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="0.1"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="0.15"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="0.2"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="0.3"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="0.5"} 0
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="1"} 1
		threescale_backend_latency_bucket{backendURL="www.fake.com",serviceID="123",le="+Inf"} 1
		threescale_backend_latency_sum{backendURL="www.fake.com",serviceID="123"} 1
		threescale_backend_latency_count{backendURL="www.fake.com",serviceID="123"} 1

	`
	r := NewMetricsReporter(true, 8080)
	r.ObserveBackendLatency("www.fake.com", "123", time.Second)
	// TODO - Uncomment when https://github.com/prometheus/client_golang/issues/498 is resolved
	//err := testutil.CollectAndCompare(backendLatency, strings.NewReader(expect), metricName)
	//if err != nil {
	//	t.Fatalf(err.Error())
	//}
}

func TestIncrementTotalRequests(t *testing.T) {
	collector := totalRequests
	if testutil.ToFloat64(collector) != 0 {
		t.Fatalf("unexpected counter value for %s", collector.Desc().String())
	}
	r := NewMetricsReporter(true, 8080)

	incrementBy := randCounterInc(t, r.IncrementTotalRequests)
	if testutil.ToFloat64(collector) != float64(incrementBy) {
		t.Fatalf("unexpected counter value for %s", collector.Desc().String())
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
	incrementBy := rand.IntnRange(1, 10)
	for i := 1; i <= incrementBy; i++ {
		inc()
	}
	return incrementBy
}
