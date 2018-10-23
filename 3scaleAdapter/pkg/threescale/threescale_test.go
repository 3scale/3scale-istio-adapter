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
		params       pb.Params
		action       *authorization.ActionMsg
		expectStatus int32
	}{
		{
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "www.invalid.com",
				AccessToken: "789",
			},
			expectStatus: 3,
		},
		{
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "789",
			},
			expectStatus: 3,
			action:       &authorization.ActionMsg{},
		},
		{
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
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "expect7",
			},
			expectStatus: 7,
			action: &authorization.ActionMsg{
				Path: "/test?user_key=invalid-backend-resp",
			},
		},
		{
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "happy-path",
			},
			expectStatus: 0,
			action: &authorization.ActionMsg{
				Path: "/test?user_key=secret",
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

		c := &Threescale{client: httpClient}
		result, _ := c.HandleAuthorization(ctx, r)
		if result.Status.Code != input.expectStatus {
			t.Errorf("Expected %v got %#v", input.expectStatus, result.Status.Code)
		}
	}
}

func Test_NewThreescale(t *testing.T) {

	addr := "0"
	s, err := NewThreescale(addr, http.DefaultClient)
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
