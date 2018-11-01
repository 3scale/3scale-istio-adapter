package threescale

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"

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
		action       *authorization.ActionMsg
		expectStatus int32
	}{
		{
			name: "Test fail - invalid system url in CRD",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "www.invalid.com",
				AccessToken: "789",
			},
			expectStatus: 3,
		},
		{
			name: "Test fail - missing request path",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "789",
			},
			expectStatus: 3,
			action:       &authorization.ActionMsg{},
		},
		{
			name: "Test fail - missing user_key in query param",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "789",
			},
			expectStatus: 7,
			action: &authorization.ActionMsg{
				Path: "/test",
			},
		},
		{
			name: "Test fail - invalid or no response from 3scale backend",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "expect9",
			},
			expectStatus: 9,
			action: &authorization.ActionMsg{
				Path: "/test?user_key=secret",
			},
		},
		{
			name: "Test fail - Non 2xx status code from 3scale backend",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "expect7",
			},
			expectStatus: 7,
			action: &authorization.ActionMsg{
				Path:   "/test?user_key=invalid-backend-resp",
				Method: "get",
			},
		},
		{
			name: "Test fail - non-matching mapping rule",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "any",
			},
			expectStatus: 7,
			action: &authorization.ActionMsg{
				Path:   "/test?user_key=secret",
				Method: "post",
			},
		},
		{
			name: "Test success - 200 response from AuthRep call",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "happy-path",
			},
			expectStatus: 0,
			action: &authorization.ActionMsg{
				Path:   "/?user_key=secret",
				Method: "get",
			},
		},
	}
	for _, input := range inputs {
		r := &authorization.HandleAuthorizationRequest{
			Instance: &authorization.InstanceMsg{
				Subject: &authorization.SubjectMsg{},
			},
			AdapterConfig: &types.Any{},
			DedupId:       "",
		}

		b, _ := input.params.Marshal()
		r.AdapterConfig.Value = b
		r.Instance.Action = input.action

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

				if params.Get("user_key") == "invalid-backend-resp" {
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
			client:     httpClient,
			proxyCache: nil,
		}
		result, _ := c.HandleAuthorization(ctx, r)
		if result.Status.Code != input.expectStatus {
			t.Errorf("Expected %v got %#v", input.expectStatus, result.Status.Code)
		}
	}
}

func Test_NewThreescale(t *testing.T) {

	addr := "0"
	s, err := NewThreescale(addr, http.DefaultClient, nil)
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
