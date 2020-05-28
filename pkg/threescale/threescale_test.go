package threescale

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/3scale/3scale-istio-adapter/config"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale/authorizer"
	"github.com/3scale/3scale-porta-go-client/client"
	"github.com/gogo/googleapis/google/rpc"
	"github.com/gogo/protobuf/types"

	"istio.io/istio/mixer/template/authorization"
)

const internalBackend = "use-internal"

func TestHandleAuthorization(t *testing.T) {
	ctx := context.TODO()

	const internalBackendUrl = "some.internal.address.sv.cluster.local:3000"

	inputs := []struct {
		name                 string
		request              *authorization.HandleAuthorizationRequest
		params               config.Params
		expectStatus         int32
		expectErrMsgContains string
		authorizer           authorizer.Authorizer
	}{
		{
			name: "Test nil config should error",
			request: &authorization.HandleAuthorizationRequest{
				Instance:      nil,
				AdapterConfig: nil,
				DedupId:       "",
			},
			expectStatus:         int32(rpc.INTERNAL),
			expectErrMsgContains: "adapter config cannot be nil",
		},
		{
			name: "Test fail - error response from 3scale system",
			params: config.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "expect14",
			},
			request: &authorization.HandleAuthorizationRequest{
				Instance: &authorization.InstanceMsg{
					Action: &authorization.ActionMsg{
						Method: "get",
						Path:   "/test",
					},
					Subject: &authorization.SubjectMsg{
						User: "secret",
					},
				},
				AdapterConfig: &types.Any{},
			},
			authorizer: mockAuthorizer{
				withSystemErr: client.ApiErr{},
				withConfig:    client.ProxyConfig{},
			},
			expectStatus:         int32(rpc.UNKNOWN),
			expectErrMsgContains: "error calling 3scale system",
		},
		{
			name: "Test fail - error response from 3scale backend",
			params: config.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "expect14",
			},
			request: &authorization.HandleAuthorizationRequest{
				Instance: &authorization.InstanceMsg{
					Action: &authorization.ActionMsg{
						Method: "get",
						Path:   "/test",
					},
					Subject: &authorization.SubjectMsg{
						User: "secret",
					},
				},
				AdapterConfig: &types.Any{},
			},
			authorizer: mockAuthorizer{
				withSystemErr: nil,
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/test",
								},
							},
						},
					},
				},
				withBackendErr: errors.New("backend error"),
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: false,
				},
			},
			expectStatus: int32(rpc.UNKNOWN),
		},
		{
			name: "Test override with internal DNS",
			params: config.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "expect14",
				BackendUrl:  internalBackend,
			},
			request: &authorization.HandleAuthorizationRequest{
				Instance: &authorization.InstanceMsg{
					Action: &authorization.ActionMsg{
						Method: "get",
						Path:   "/test",
					},
					Subject: &authorization.SubjectMsg{
						User: "secret",
					},
				},
				AdapterConfig: &types.Any{},
			},
			authorizer: mockAuthorizer{
				withSystemErr: nil,
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							Backend: client.Backend{
								Endpoint: "some-other-endpoint",
								Host:     "some-other-host",
							},
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/test",
								},
							},
						},
					},
				},
				withBackendErr: nil,
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: true,
				},
			},
			expectStatus: int32(rpc.OK),
		},
	}
	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			r := input.request

			if r.AdapterConfig != nil {
				b, _ := input.params.Marshal()
				r.AdapterConfig.Value = b
			}

			c := &Threescale{
				conf: NewAdapterConfig(input.authorizer, time.Second),
			}
			result, _ := c.HandleAuthorization(ctx, r)
			if result.Status.Code != input.expectStatus {
				t.Errorf("Expected %v got %#v", input.expectStatus, result.Status.Code)
			}

			if result.Status.Code != int32(rpc.OK) {
				if !strings.Contains(result.Status.Message, input.expectErrMsgContains) {
					t.Errorf("expected message not delivered to end user\n %s", result.Status.Message)
				}
			}
		})
	}
}

func Test_NewThreescale(t *testing.T) {
	addr := "0"
	threescaleConf := NewAdapterConfig(nil, time.Minute)
	s, err := NewThreescale(addr, threescaleConf)
	if err != nil {
		t.Errorf("Error running threescale server %#v", err)
	}
	shutdown := make(chan error, 1)
	go func() {
		s.Run(shutdown)
	}()
	s.Close()
}

type mockAuthorizer struct {
	withSystemErr    error
	withBackendErr   error
	withConfig       client.ProxyConfig
	withAuthResponse *authorizer.BackendResponse
	t                *testing.T
}

func (m mockAuthorizer) GetSystemConfiguration(systemURL string, request authorizer.SystemRequest) (client.ProxyConfig, error) {
	return m.withConfig, m.withSystemErr
}

func (m mockAuthorizer) AuthRep(backendURL string, request authorizer.BackendRequest) (*authorizer.BackendResponse, error) {
	if backendURL != "" {
		// we can expect this to be empty for majority of requests,
		// we expect it to be over ridden in cases where it was provided by handler config so fail
		// if it does not match the global const here
		if backendURL != internalBackend {
			m.t.Errorf("expecetd %s but got %s", internalBackend, backendURL)
		}

	}
	params := request.Transactions[0].Params
	if params.UserKey == "VALID" || params.AppID == "VALID" {
		m.withAuthResponse.Authorized = true
	}
	return m.withAuthResponse, m.withBackendErr
}
