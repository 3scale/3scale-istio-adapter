// +build integration

package threescale

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogo/googleapis/google/rpc"

	"github.com/3scale/3scale-go-client/fake"
	sysFake "github.com/3scale/3scale-porta-go-client/fake"
	integration "istio.io/istio/mixer/pkg/adapter/test"
)

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
			fmt.Println(err)
			t.Fatalf("error reading adapter config")
		}
		conf = append(conf, string(adapterConf))
	}

	inputs := []struct {
		name            string
		callWith        []integration.Call
		expect          string
		injectProxyConf string
	}{
		{
			name: "Test No Authn Credentials Provided Denied",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/",
						"request.method":        "get",
						"destination.labels":    map[string]string{"service-mesh.3scale.net/credentials": "threescale"},
					},
				},
			},
			expect: generatedExpectedError(t, rpc.PERMISSION_DENIED, unauthenticatedErr),
		},
		{
			name: "Test Authorization API Key via headers success",
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"context.reporter.kind": "inbound",
						"request.url_path":      "/thispath",
						"request.headers":       map[string]string{"x-user-key": "VALID"},
						"request.method":        "get",
						"destination.labels":    map[string]string{"service-mesh.3scale.net/credentials": "threescale"},
					},
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
						"request.query_params":  map[string]string{"user_key": "VALID"},
						"request.method":        "get",
						"destination.labels":    map[string]string{"service-mesh.3scale.net/credentials": "threescale"},
					},
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
						"request.method":        "get",
						"request.headers":       map[string]string{"app-id": "test", "app-key": "secret"},
						"destination.labels":    map[string]string{"service-mesh.3scale.net/credentials": "threescale"},
					},
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
						"request.method":        "get",
						"request.query_params":  map[string]string{"app_id": "VALID"},
						"destination.labels":    map[string]string{"service-mesh.3scale.net/credentials": "threescale"},
					},
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
						"destination.labels":    map[string]string{"service-mesh.3scale.net/credentials": "threescale"},
					},
				},
			},
			expect:          generatedExpectedError(t, rpc.PERMISSION_DENIED, unauthenticatedErr),
			injectProxyConf: strings.Replace(sysFake.GetProxyConfigLatestJson(), `"backend_version": "1",`, `"backend_version": "oauth",`, -1),
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
						"request.auth.claims":   map[string]string{"azp": "VALID"},
						"destination.labels":    map[string]string{"service-mesh.3scale.net/credentials": "threescale"},
					},
				},
			},
			expect:          authenticatedSuccess,
			injectProxyConf: strings.Replace(sysFake.GetProxyConfigLatestJson(), `"backend_version": "1",`, `"backend_version": "oauth",`, -1),
		},
	}

	for _, input := range inputs {
		sysServer, backendServer := startTestBackends(t, input.injectProxyConf)

		s := integration.Scenario{
			Setup: func() (ctx interface{}, err error) {
				pServer, err := NewThreescale("3333", http.DefaultClient, &AdapterConfig{})
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
				return conf, nil
			},
			GetState: func(ctx interface{}) (interface{}, error) {
				return nil, nil
			},
			ParallelCalls: input.callWith,
			Want:          input.expect,
		}
		t.Run(input.name, func(t *testing.T) {
			integration.RunTest(t, nil, s)
			sysServer.Close()
			backendServer.Close()
		})

	}

}

func startTestBackends(t *testing.T, proxyConfResp string) (*httptest.Server, *httptest.Server) {
	sysListener, err := net.Listen("tcp", "127.0.0.1:8090")
	if err != nil {
		t.Fatalf("error listening on port for test data")
	}

	backendListener, err := net.Listen("tcp", "127.0.0.1:8091")
	if err != nil {
		t.Fatalf("error listening on port for test data")
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if proxyConfResp == "" {
			proxyConfResp = sysFake.GetProxyConfigLatestJson()
		}
		io.WriteString(w, strings.Replace(proxyConfResp, "https://su1.3scale.net", "http://127.0.0.1:8091", -1))
	}))
	ts.Listener.Close()
	ts.Listener = sysListener
	ts.Start()

	bs := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fake.GetAuthSuccess())
		return
	}))

	bs.Listener.Close()
	bs.Listener = backendListener
	bs.Start()

	return ts, bs
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
