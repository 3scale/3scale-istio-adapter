package metrics

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/3scale/3scale-authorizer/pkg/authorizer"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

const url = "www.fake.com"
const endpoint = "/test"

func TestRegister(t *testing.T) {
	Register()
	// test that registration does not panic
}

func TestReportCB(t *testing.T) {
	const metricName = "threescale_latency"
	const expect = `

                # HELP threescale_latency Request latency between adapter and 3scale
                # TYPE threescale_latency histogram
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="0.01"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="0.02"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="0.03"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="0.05"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="0.08"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="0.1"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="0.15"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="0.2"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="0.3"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="0.5"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="1"} 0
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="1.5"} 1
                threescale_latency_bucket{endpoint="/test",host="www.fake.com",method="GET",le="+Inf"} 1
                threescale_latency_sum{endpoint="/test",host="www.fake.com",method="GET"} 1.001
                threescale_latency_count{endpoint="/test",host="www.fake.com",method="GET"} 1

        `

	tr := authorizer.TelemetryReport{
		Host:      url,
		Method:    http.MethodGet,
		Endpoint:  endpoint,
		Code:      http.StatusOK,
		TimeTaken: time.Second + time.Millisecond,
	}
	ReportCB(tr)
	err := testutil.CollectAndCompare(threescaleLatency, strings.NewReader(expect), metricName)
	if err != nil {
		t.Errorf(err.Error())
	}
}

func TestIncrementCacheHits(t *testing.T) {
	sysCollector := cacheHitsSystem
	if testutil.ToFloat64(sysCollector) != 0 {
		t.Errorf("unexpected counter value for %s", sysCollector.Desc().String())
	}

	backendCollector := cacheHitsBackend
	if testutil.ToFloat64(backendCollector) != 0 {
		t.Errorf("unexpected counter value for %s", backendCollector.Desc().String())
	}

	IncrementCacheHits(authorizer.System)
	if testutil.ToFloat64(sysCollector) != 1 {
		t.Errorf("unexpected counter value for %s", sysCollector.Desc().String())
	}

	IncrementCacheHits(authorizer.Backend)
	if testutil.ToFloat64(backendCollector) != 1 {
		t.Errorf("unexpected counter value for %s", backendCollector.Desc().String())
	}
}
