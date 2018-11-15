package threescale

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/3scale/3scale-go-client/fake"
	sysFake "github.com/3scale/3scale-porta-go-client/fake"
	pb "github.com/3scale/istio-integration/3scaleAdapter/config"
	"github.com/gogo/protobuf/types"
	"istio.io/api/policy/v1beta1"
	"istio.io/istio/mixer/template/authorization"
	"istio.io/istio/mixer/template/logentry"
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
				ServiceId:         "123",
				SystemUrl:         "www.invalid.com",
				AccessToken:       "789",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},
			expectStatus: 3,
			expectErrMsg: []string{"error building HTTP client for 3scale system", "invalid URI for request"},
		},
		{
			name: "Test fail - missing request path",
			params: pb.Params{
				ServiceId:         "123",
				SystemUrl:         "https://www.fake-system.3scale.net",
				AccessToken:       "789",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},
			expectStatus: 3,
			expectErrMsg: []string{"missing request path"},
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{},
			},
		},
		{
			name: "Test fail - missing user_key",
			params: pb.Params{
				ServiceId:         "123",
				SystemUrl:         "https://www.fake-system.3scale.net",
				AccessToken:       "789",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},
			expectStatus: 7,
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "",
				},
				Action: &authorization.ActionMsg{
					Method: "get",
					Path:   "/",
				},
			},
			expectErrMsg: []string{"user_key must be provided as a query parameter"},
		},
		{
			name: "Test fail - invalid or no response from 3scale backend",
			params: pb.Params{
				ServiceId:         "123",
				SystemUrl:         "https://www.fake-system.3scale.net",
				AccessToken:       "expect9",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},
			expectStatus: 9,
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Method: "get",
					Path:   "/test",
				},
			},
			expectErrMsg: []string{"currently unable to fetch required data from 3scale system"},
		},
		{
			name: "Test fail - Non 2xx status code from 3scale backend",
			params: pb.Params{
				ServiceId:         "123",
				SystemUrl:         "https://www.fake-system.3scale.net",
				AccessToken:       "expect7",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},
			expectStatus: 7,

			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "invalid-backend-resp",
				},
				Action: &authorization.ActionMsg{
					Method: "get",
					Path:   "/test",
				},
			},
			expectErrMsg: []string{"user_key_invalid"},
		},
		{
			name: "Test fail - non-matching mapping rule",
			params: pb.Params{
				ServiceId:         "123",
				SystemUrl:         "https://www.fake-system.3scale.net",
				AccessToken:       "any",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},
			expectStatus: 7,
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Method: "post",
					Path:   "/nop",
				},
			},
			expectErrMsg: []string{"no matching mapping rule for request with method post and path /nop"},
		},
		{
			name: "Test success - 200 response from Authorize call",
			params: pb.Params{
				ServiceId:         "123",
				SystemUrl:         "https://www.fake-system.3scale.net",
				AccessToken:       "happy-path",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},
			expectStatus: 0,
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Method: "get",
					Path:   "/",
				},
			},
		},
	}
	for _, input := range inputs {
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

				if input.template.Subject.User == "invalid-backend-resp" {
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
		c := &Threescale{
			client:        httpClient,
			conf:          &AdapterConfig{},
			reportMetrics: true,
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
	}
}

func TestHandleLogEntry(t *testing.T) {
	ctx := context.TODO()
	inputs := []struct {
		name         string
		params       pb.Params
		instanceMsgs []*logentry.InstanceMsg
		hasToFail    bool
		expectErrMsg []string
	}{
		{
			name: "Test nil config",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "set-nil",
				AccessToken: "789",
			},
			instanceMsgs: nil,
			hasToFail:    true,
			expectErrMsg: []string{"adapter config not available"},
		},
		{
			name: "Test fail - invalid system url in CRD",
			params: pb.Params{
				ServiceId:         "123",
				SystemUrl:         "www.invalid.com",
				AccessToken:       "789",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},
			instanceMsgs: nil,
			hasToFail:    true,
			expectErrMsg: []string{"invalid URI for request"},
		},
		{
			name: "Test fail - missing request path",
			params: pb.Params{
				ServiceId:         "123",
				SystemUrl:         "https://www.fake-system.3scale.net",
				AccessToken:       "789",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},
			instanceMsgs: []*logentry.InstanceMsg{
				{
					Name:                        "",
					Variables:                   nil,
					Timestamp:                   nil,
					Severity:                    "",
					MonitoredResourceType:       "",
					MonitoredResourceDimensions: nil,
				},
			},
			hasToFail:    true,
			expectErrMsg: []string{"missing required parameters"},
		},
		{
			name: "Test Invalid api_key - Forbidden Request",
			params: pb.Params{
				ServiceId:         "123",
				SystemUrl:         "https://www.fake-system.3scale.net",
				AccessToken:       "789",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},

			instanceMsgs: []*logentry.InstanceMsg{
				{
					Name: "",
					Variables: map[string]*v1beta1.Value{
						"url": {
							Value: &v1beta1.Value_StringValue{
								StringValue: "/thispath",
							},
						},
						"user": {
							Value: &v1beta1.Value_StringValue{
								StringValue: "invalid-backend-resp",
							},
						},
						"method": {
							Value: &v1beta1.Value_StringValue{
								StringValue: "get",
							},
						},
					},
					Timestamp:                   nil,
					Severity:                    "",
					MonitoredResourceType:       "",
					MonitoredResourceDimensions: nil,
				},
			},
			hasToFail:    true,
			expectErrMsg: []string{"report has not been successful"},
		},
		{
			name: "Test Ok - Valid Request",
			params: pb.Params{
				ServiceId:         "123",
				SystemUrl:         "https://www.fake-system.3scale.net",
				AccessToken:       "789",
				CacheValidSeconds: "0",
				CacheValidHits:    "0",
			},

			instanceMsgs: []*logentry.InstanceMsg{
				{
					Name: "",
					Variables: map[string]*v1beta1.Value{
						"url": {
							Value: &v1beta1.Value_StringValue{
								StringValue: "/thispath",
							},
						},
						"user": {
							Value: &v1beta1.Value_StringValue{
								StringValue: "valid",
							},
						},
						"method": {
							Value: &v1beta1.Value_StringValue{
								StringValue: "get",
							},
						},
					},
					Timestamp:                   nil,
					Severity:                    "",
					MonitoredResourceType:       "",
					MonitoredResourceDimensions: nil,
				},
			},
			hasToFail:    false,
			expectErrMsg: []string{"missing required parameters"},
		},
	}
	for _, input := range inputs {
		l := &logentry.HandleLogEntryRequest{
			Instances:     input.instanceMsgs,
			AdapterConfig: &types.Any{},
		}

		b, _ := input.params.Marshal()
		l.AdapterConfig.Value = b

		if input.params.SystemUrl == "set-nil" {
			l.AdapterConfig = nil
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

				if input.instanceMsgs[0].Variables["user"].GetStringValue() == "invalid-backend-resp" {
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
		c := &Threescale{
			client: httpClient,
			conf:   &AdapterConfig{},
		}
		_, err := c.HandleLogEntry(ctx, l)

		if err != nil {

			if !input.hasToFail {
				t.Errorf("Test didn't expect any error, but got %s", err)
			}
			for _, errorString := range input.expectErrMsg {
				if !strings.Contains(err.Error(), errorString) {
					t.Errorf("Expected error message: %s  but got: %s", errorString, err)
				}
			}
		} else {
			if input.hasToFail {
				t.Errorf("Expected test to fail, but didn't")

			}
		}

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
