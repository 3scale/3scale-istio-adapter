package threescale

import (
	"context"
	"github.com/3scale/istio-integration/3scaleAdapter/httpPluginClient"
	"github.com/gogo/protobuf/types"
	"istio.io/api/policy/v1beta1"
	"istio.io/istio/mixer/template/authorization"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func Test_HandleAuthorization(t *testing.T) {

	var handleAuthorizationTests = []struct {
		httpStatus     int
		expectedStatus int32
	}{
		{http.StatusOK, 0},
		{http.StatusAccepted, 0},
		{http.StatusRequestTimeout, 7},
		{http.StatusBadGateway, 7},
		{http.StatusGatewayTimeout, 7},
		{http.StatusForbidden, 7},
	}

	ctx := context.TODO()

	r := &authorization.HandleAuthorizationRequest{
		Instance: &authorization.InstanceMsg{
			Subject: &authorization.SubjectMsg{},
			Action:  &authorization.ActionMsg{},
		},
		AdapterConfig: &types.Any{},
		DedupId:       "",
	}

	c := &Threescale{client: httpPluginClient.NewClient(nil)}

	for _, tt := range handleAuthorizationTests {
		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tt.httpStatus)
		}))
		l, _ := net.Listen("tcp", "127.0.0.1:8090")
		ts.Listener = l
		ts.Start()

		result, err := (*Threescale).HandleAuthorization(c, ctx, r)
		if err != nil {
			t.Errorf("Fail %#v", err)
		}
		if result.Status.Code != tt.expectedStatus {
			t.Errorf("Expected %v got %#v", tt.expectedStatus, result.Status.Code)
		}

		ts.Close()
	}
}

func Test_buildRequestFromInstanceMsg(t *testing.T) {

	expectedURLObject := &http.Request{
		Method: "get",
		URL: &url.URL{
			User: &url.Userinfo{},
			Path: "/path?query=1&query=2&query=3&user_key=12345",
		},
		Header: map[string][]string{"user-key": {"user_key_header"}},
	}

	instanceMsg := &authorization.InstanceMsg{
		Name: "authorizationTest",
		Subject: &authorization.SubjectMsg{
			User:       "",
			Groups:     "",
			Properties: nil,
		},
		Action: &authorization.ActionMsg{
			Namespace: "3scale",
			Service:   "myservice",
			Method:    "get",
			Path:      "/path?query=1&query=2&query=3&user_key=12345",
			Properties: map[string]*v1beta1.Value{"user-key": {
				&v1beta1.Value_StringValue{
					StringValue: "user_key_header",
				},
			}},
		},
	}

	originalRequest := buildRequestFromInstanceMsg(instanceMsg)

	if reflect.DeepEqual(expectedURLObject, originalRequest) {
		t.Logf("the same")
	}
}

func Test_NewThreescale(t *testing.T) {

	addr := "0"
	s, err := NewThreescale(addr)
	if err != nil {
		t.Errorf("Error running threescale server %#v", err)
	}
	shutdown := make(chan error, 1)
	go func() {
		s.Run(shutdown)
	}()
	s.Close()
}
