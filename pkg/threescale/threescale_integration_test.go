// +build integration

package threescale

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/3scale/3scale-istio-adapter/pkg/threescale/authorizer"
	"github.com/3scale/3scale-porta-go-client/client"
	"github.com/gogo/googleapis/google/rpc"
	integration "istio.io/istio/mixer/pkg/adapter/test"
)

const validAuth = "VALID"
const authenticatedSuccess = `
	{
		"AdapterState": null,
		"Returns": [
			{
				"Check": {
					"Status": {},
					"ValidDuration": 0,
					"ValidUseCount": -1
				},
				"Quota": null,
				"Error": null
			}
		]
	}`

func TestAuthorizationCheck(t *testing.T) {
	var conf []string
	var files []string
	var handlerConf []byte

	path, _ := filepath.Abs("../../testdata")
	err := filepath.Walk(path, func(f string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		files = append(files, f)
		return nil
	})

	if err != nil {
		t.Fatalf("error fetching required files")
	}

	for _, f := range files {
		adapterConf, err := ioutil.ReadFile(f)
		if err != nil {
			t.Fatalf("error reading adapter config")
		}
		if strings.Contains(f, "handler.yaml") {
			handlerConf = adapterConf
			continue
		}

		conf = append(conf, string(adapterConf))
	}

	inputs := []struct {
		name       string
		callWith   []integration.Call
		expect     string
		authorizer authorizer.Authorizer
		handler    []byte
	}{
		{
			name: "Test failure when no url_path provided",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "",
						"request.method":        "get",
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig:    client.ProxyConfig{},
				withSystemErr: nil,
			},
			expect: generatedExpectedError(t, rpc.FAILED_PRECONDITION, "request path must be provided."),
		},
		{
			name: "Test failure when no mapping rule matches incoming request",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/",
						"request.method":        "get",
						"request.headers":       map[string]string{"user_key": validAuth},
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodPost,
								},
							},
						},
					},
				},
				withSystemErr: nil,
			},
			expect: generatedExpectedError(t, rpc.NOT_FOUND, "no matching mapping rule for request"),
		},
		{
			name: "Test failure when no access token set in handler",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/",
						"request.method":        "get",
						"request.headers":       map[string]string{"user_key": validAuth},
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/",
								},
							},
						},
					},
				},
				withSystemErr: nil,
			},
			handler: []byte(`apiVersion: config.istio.io/v1alpha2
kind: handler
metadata:
  creationTimestamp: null
  name: threescale
  namespace: istio-system
spec:
  adapter: threescale
  connection:
    address: '[::]:3333'
  params:
    system_url: http://127.0.0.1:8090`),
			expect: generatedExpectedError(t, rpc.FAILED_PRECONDITION, errAccessToken.Error()+"."),
		},
		{
			name: "Test failure when no system url set in handler",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/",
						"request.method":        "get",
						"request.headers":       map[string]string{"user_key": validAuth},
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/",
								},
							},
						},
					},
				},
				withSystemErr: nil,
			},
			handler: []byte(`apiVersion: config.istio.io/v1alpha2
kind: handler
metadata:
  creationTimestamp: null
  name: threescale
  namespace: istio-system
spec:
  adapter: threescale
  connection:
    address: '[::]:3333'
  params:
    access_token: secret-token`),
			expect: generatedExpectedError(t, rpc.FAILED_PRECONDITION, errSystemURL.Error()+"."),
		},
		{
			name: "Test failure when no service ID provided",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/",
						"request.method":        "get",
						"request.headers":       map[string]string{"user_key": validAuth},
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/",
								},
							},
						},
					},
				},
			},
			expect: generatedExpectedError(t, rpc.FAILED_PRECONDITION, errServiceID.Error()+"."),
		},
		{
			name: "Test error when no credentials provided",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/",
						"request.method":        "get",
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/",
								},
							},
						},
					},
				},
			},
			expect: generatedExpectedError(t, rpc.UNAUTHENTICATED, errNoCredentials.Error()),
		},
		{
			name: "Test Authorization API Key via headers success",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/thispath",
						"request.headers":       map[string]string{"user_key": validAuth},
						"request.method":        "get",
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/thispath",
								},
							},
						},
					},
				},
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: false,
					ErrorCode:  "should overwrite",
				},
			},
			expect: authenticatedSuccess,
		},
		{
			name: "Test Authorization API Key via query param success",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/thispath",
						"request.query_params":  map[string]string{"user_key": validAuth},
						"request.method":        "get",
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/thispath",
								},
							},
						},
					},
				},
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: false,
					ErrorCode:  "should overwrite",
				},
			},
			expect: authenticatedSuccess,
		},
		{
			name: "Test Authorization Application ID via headers success",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/thispath",
						"request.headers":       map[string]string{"app_id": validAuth, "app_key": "secret"},
						"request.method":        "get",
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/thispath",
								},
							},
						},
					},
				},
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: false,
					ErrorCode:  "should overwrite",
				},
			},
			expect: authenticatedSuccess,
		},
		{
			name: "Test Authorization Application ID via query param success",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/thispath",
						"request.query_params":  map[string]string{"app_id": validAuth},
						"request.method":        "get",
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/thispath",
								},
							},
						},
					},
				},
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: false,
					ErrorCode:  "should overwrite",
				},
			},
			expect: authenticatedSuccess,
		},
		{
			name: "Test OIDC integration no client_id failure",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/oidc",
						"request.method":        "get",
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						BackendVersion: openIDTypeIdentifier,
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/oidc",
								},
							},
						},
					},
				},
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: false,
					ErrorCode:  "should not overwrite",
				},
			},
			expect: generatedExpectedError(t, rpc.UNAUTHENTICATED, errNoCredentials.Error()),
		},
		{
			name: "Test OIDC integration success",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/oidc",
						"request.method":        "get",
						"request.auth.claims":   map[string]string{"azp": validAuth},
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						BackendVersion: openIDTypeIdentifier,
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/oidc",
								},
							},
						},
					},
				},
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: false,
					ErrorCode:  "should overwrite",
				},
			},
			expect: authenticatedSuccess,
		},
		{
			name: "Test backend response is respected for failed auth",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/oidc",
						"request.method":        "get",
						"request.auth.claims":   map[string]string{"azp": "INVALID"},
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						BackendVersion: openIDTypeIdentifier,
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/oidc",
								},
							},
						},
					},
				},
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: false,
					ErrorCode:  "should not overwrite",
				},
			},
			expect: generatedExpectedError(t, rpc.UNKNOWN, "should not overwrite"),
		},
		{
			name: "Test correct mapping of status codes for 409 from backend using error codes",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/oidc",
						"request.method":        "get",
						"request.auth.claims":   map[string]string{"azp": "INVALID"},
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						BackendVersion: openIDTypeIdentifier,
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/oidc",
								},
							},
						},
					},
				},
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: false,
					ErrorCode:  "application_key_invalid",
				},
			},
			expect: generatedExpectedError(t, rpc.PERMISSION_DENIED, "application_key_invalid"),
		},
		{
			name: "Test rate limited request returns a resource exhausted response",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/oidc",
						"request.method":        "get",
						"request.auth.claims":   map[string]string{"azp": "INVALID"},
						"destination.labels": map[string]string{
							"service-mesh.3scale.net/credentials": "threescale",
							"service-mesh.3scale.net/service-id":  "any",
						},
					},
				},
			},
			authorizer: mockAuthorizer{
				withConfig: client.ProxyConfig{
					Content: client.Content{
						BackendVersion: openIDTypeIdentifier,
						Proxy: client.ContentProxy{
							ProxyRules: []client.ProxyRule{
								{
									HTTPMethod: http.MethodGet,
									Pattern:    "/oidc",
								},
							},
						},
					},
				},
				withAuthResponse: &authorizer.BackendResponse{
					Authorized: false,
					ErrorCode:  "limits_exceeded",
				},
			},
			expect: generatedExpectedError(t, rpc.RESOURCE_EXHAUSTED, "limits_exceeded"),
		},
	}

	for _, input := range inputs {
		s := integration.Scenario{
			Setup: func() (ctx interface{}, err error) {
				config := NewAdapterConfig(input.authorizer, time.Second)

				pServer, err := NewThreescale("3333", config)
				if err != nil {
					return nil, err
				}

				shutdown := make(chan error, 1)
				go func() {
					pServer.Run(shutdown)
				}()

				return pServer, nil

			},
			Teardown: func(ctx interface{}) {
				s := ctx.(Server)
				s.Close()
			},
			GetConfig: func(ctx interface{}) ([]string, error) {
				doConf := func(input []byte) []string {
					var aggregatedConf = make([]string, len(conf))
					copy(aggregatedConf, conf)
					aggregatedConf = append(aggregatedConf, string(input))
					return aggregatedConf
				}

				if input.handler == nil {
					return doConf(handlerConf), nil
				}

				return doConf(input.handler), nil
			},
			GetState: func(ctx interface{}) (interface{}, error) {
				return nil, nil
			},
			ParallelCalls: input.callWith,
			Want:          input.expect,
		}
		integration.RunTest(t, nil, s)

	}

}

func generatedExpectedError(t *testing.T, status rpc.Code, reason string) string {
	t.Helper()
	return fmt.Sprintf(`
	{
		"AdapterState":null,
		"Returns":[
			{
				"Check":{
					"Status":{
						"code":%d,
						"message":"threescale.handler.istio-system:%s"
					},
					"ValidDuration": 0,
					"ValidUseCount": -1
				},
				"Quota":null,
				"Error":null
			}
		]
	}`, status, reason)
}
