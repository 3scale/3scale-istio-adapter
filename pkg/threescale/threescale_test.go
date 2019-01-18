package threescale

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/3scale/istio-integration/3scaleAdapter/pkg/threescale/metrics"

	"github.com/3scale/3scale-go-client/fake"
	sysFake "github.com/3scale/3scale-porta-go-client/fake"
	pb "github.com/3scale/istio-integration/3scaleAdapter/config"
	"github.com/gogo/protobuf/types"
	"istio.io/istio/mixer/template/authorization"
)

func TestHandleAuthorization(t *testing.T) {
	ctx := context.TODO()
	inputs := []struct {
		name         string
		params       pb.Params
		template     authorization.InstanceMsg
		expectStatus int32
		expectErrMsg []string
	}{
		{
			name: "Test nil config",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "set-nil",
				AccessToken: "789",
			},
			expectStatus: 13,
			expectErrMsg: []string{"internal error - adapter config is not available"},
		},
		{
			name: "Test fail - invalid system url in CRD",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "www.invalid.com",
				AccessToken: "789",
			},
			expectStatus: 3,
			expectErrMsg: []string{"error building HTTP client for 3scale system", "invalid URI for request"},
		},
		{
			name: "Test fail - missing request path",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "789",
			},
			expectStatus: 3,
			expectErrMsg: []string{"missing request path"},
			template: authorization.InstanceMsg{
				Name:   "",
				Action: &authorization.ActionMsg{},
			},
		},
		{
			name: "Test fail - missing user_key",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "789",
			},
			expectStatus: 7,
			template: authorization.InstanceMsg{
				Name: "",
				Action: &authorization.ActionMsg{
					Method: "get",
					Path:   "/",
				},
			},
			expectErrMsg: []string{"user_key required"},
		},
		{
			name: "Test fail - invalid or no response from 3scale backend",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "expect9",
			},
			expectStatus: 9,
			template: authorization.InstanceMsg{
				Name: "",
				Action: &authorization.ActionMsg{
					Method: "get",
					Path:   "/test?user_key=secret&test=curveball",
				},
			},
			expectErrMsg: []string{"currently unable to fetch required data from 3scale system"},
		},
		{
			name: "Test fail - Non 2xx status code from 3scale backend",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "expect7",
			},
			expectStatus: 7,

			template: authorization.InstanceMsg{
				Name: "",
				Action: &authorization.ActionMsg{
					Method: "get",
					Path:   "/test?user_key=invalid-backend-resp",
				},
			},
			expectErrMsg: []string{"user_key_invalid"},
		},
		{
			name: "Test fail - non-matching mapping rule",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "any",
			},
			expectStatus: 7,
			template: authorization.InstanceMsg{
				Name: "",
				Action: &authorization.ActionMsg{
					Method: "post",
					Path:   "/nop?user_key=secret",
				},
			},
			expectErrMsg: []string{"no matching mapping rule for request with method post and path /nop"},
		},
		{
			name: "Test success - 200 response from Authorize call",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "happy-path",
			},
			expectStatus: 0,
			template: authorization.InstanceMsg{
				Name: "",
				Action: &authorization.ActionMsg{
					Method: "get",
					Path:   "/?user_key=secret",
				},
			},
		},
	}
	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			r := &authorization.HandleAuthorizationRequest{
				Instance:      &input.template,
				AdapterConfig: &types.Any{},
				DedupId:       "",
			}

			b, _ := input.params.Marshal()
			r.AdapterConfig.Value = b

			if input.params.SystemUrl == "set-nil" {
				r.AdapterConfig = nil
			}

			httpClient := NewTestClient(func(req *http.Request) *http.Response {
				params := req.URL.Query()

				if req.URL.Host == "www.fake-system.3scale.net:443" {
					if params.Get("access_token") == "expect9" {
						return &http.Response{
							Body:   ioutil.NopCloser(bytes.NewBufferString("invalid resp")),
							Header: make(http.Header),
						}
					}
					return sysFake.GetProxyConfigLatestSuccess()
				} else {

					if strings.Contains(req.URL.RawQuery, "invalid-backend-resp") {
						return &http.Response{
							StatusCode: 403,
							Body:       ioutil.NopCloser(bytes.NewBufferString(fake.GenInvalidUserKey("secret"))),
							Header:     make(http.Header),
						}
					}

					return &http.Response{
						StatusCode: 200,
						Body:       ioutil.NopCloser(bytes.NewBufferString(fake.GetAuthSuccess())),
						Header:     make(http.Header),
					}
				}
			})
			reporter := metrics.NewMetricsReporter(true, 8080)
			c := &Threescale{
				client: httpClient,
				conf: &AdapterConfig{
					metricsReporter: reporter,
				},
			}
			result, _ := c.HandleAuthorization(ctx, r)
			if result.Status.Code != input.expectStatus {
				t.Errorf("Expected %v got %#v", input.expectStatus, result.Status.Code)
			}

			if result.Status.Code != 0 {
				if len(input.expectErrMsg) == 0 {
					t.Errorf("Error tests should produce a message - failed test: %s", input.name)
				}

				for _, msg := range input.expectErrMsg {
					if !strings.Contains(result.Status.Message, msg) {
						t.Errorf("expected message not delivered to end user\n %s vs %s\n", result.Status.Message, msg)
					}
				}
			}
		})
	}
}

func Test_NewThreescale(t *testing.T) {
	addr := "0"
	threescaleConf := NewAdapterConfig(nil, nil)
	s, err := NewThreescale(addr, http.DefaultClient, threescaleConf)
	if err != nil {
		t.Errorf("Error running threescale server %#v", err)
	}
	shutdown := make(chan error, 1)
	go func() {
		s.Run(shutdown)
	}()
	s.Close()
}

// Mocking objects for HTTP tests
type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

// Get a test client with transport overridden for mocking
func NewTestClient(fn RoundTripFunc) *http.Client {
	return &http.Client{
		Transport: RoundTripFunc(fn),
	}
}
