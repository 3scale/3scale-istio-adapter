package backend

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"sync"
	"testing"

	"github.com/3scale/3scale-go-client/fake"
	"github.com/3scale/3scale-go-client/threescale/api"
	apisonator "github.com/3scale/3scale-go-client/threescale/http"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale/metrics"
)

func TestDefaultBackend(t *testing.T) {
	var reported = false
	var wg = sync.WaitGroup{}
	mockRequest := Request{
		Auth: api.ClientAuth{
			Type:  api.ProviderKey,
			Value: "any",
		},
	}

	inputs := []struct {
		name                string
		responseCode        int
		xmlResponse         string
		expectAuthorization bool
		expectError         bool
		reportWith          metrics.ReportMetricsFn
	}{
		{
			name:        "Test error case",
			xmlResponse: "invalid",
			expectError: true,
		},
		{
			name:         "Test unauthorized",
			responseCode: http.StatusTooManyRequests,
			xmlResponse:  fake.GetLimitExceededResp(),
		},
		{
			name:                "Test authorized",
			responseCode:        http.StatusOK,
			xmlResponse:         fake.GetAuthSuccess(),
			expectAuthorization: true,
		},
		{
			name:                "Test reporting",
			responseCode:        http.StatusOK,
			xmlResponse:         fake.GetAuthSuccess(),
			expectAuthorization: true,
			reportWith: func(serviceID string, l metrics.LatencyReport, s metrics.StatusReport) {
				reported = true
				wg.Done()
			},
		},
	}
	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			httpClient := NewTestClient(t, func(req *http.Request) *http.Response {
				return &http.Response{
					StatusCode: input.responseCode,
					Body:       ioutil.NopCloser(bytes.NewBufferString(input.xmlResponse)),
					Header:     make(http.Header),
				}
			})

			if input.reportWith != nil {
				wg.Add(1)
			}

			threescaleClient, _ := apisonator.NewClient("http://any.com", httpClient)
			b := DefaultBackend{ReportFn: input.reportWith}
			resp, err := b.AuthRep(mockRequest, threescaleClient)

			if err != nil && !input.expectError {
				t.Error("unexpected error response")
			}
			if resp.Success != input.expectAuthorization {
				t.Errorf("incorrect authorization result")
			}

			if input.reportWith != nil {
				wg.Wait()
				if !reported {
					t.Errorf("expected reporting to have happened")
				}
			}
			reported = false
		})
	}
}

// Mocking objects for HTTP tests
type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

// Helper - Get a test client with transport overridden for mocking
func NewTestClient(t *testing.T, fn RoundTripFunc) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: RoundTripFunc(fn),
	}
}
