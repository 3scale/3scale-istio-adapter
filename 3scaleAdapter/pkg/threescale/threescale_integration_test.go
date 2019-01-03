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

	"github.com/3scale/3scale-go-client/fake"
	sysFake "github.com/3scale/3scale-porta-go-client/fake"
	integration "istio.io/istio/mixer/pkg/adapter/test"
)

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

	sysServer, backendServer := startTestBackends(t)
	defer sysServer.Close()
	defer backendServer.Close()

	inputs := []struct {
		callWith []integration.Call
		expect   string
	}{
		{
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"request.path":       "/thispath?user_key=VALID",
						"request.method":     "get",
						"destination.labels": map[string]string{"service-mesh.3scale.net": "true"},
					},
				},
			},
			expect: `
			{
			    "AdapterState":null,
			    "Returns":[
				{
				    "Check":{
					"Status":{},
					"ValidDuration": 0,
					"ValidUseCount": -1
				    },
				    "Quota":null,
				    "Error":null
				}
			    ]
			}`,
		},
		{
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"request.path":       "/thispath?user_key=INVALID",
						"request.method":     "get",
						"destination.labels": map[string]string{"service-mesh.3scale.net": "true"},
					},
				},
			},
			expect: `
			{
			    "AdapterState":null,
			    "Returns":[
			        {
			            "Check":{
			                "Status":{
			                    "code":7,
			                    "message":"threescale.handler.istio-system:user_key_invalid"
			                },
			                "ValidDuration": 0,
                                        "ValidUseCount": -1
			            },
			            "Quota":null,
			            "Error":null
			        }
			    ]
			}`,
		},
		{
			callWith: []integration.Call{
				{
					CallKind: integration.CHECK,
					Attrs: map[string]interface{}{
						"request.path":       "/thispath",
						"request.method":     "get",
						"destination.labels": map[string]string{"service-mesh.3scale.net": "true"},
					},
				},
			},
			expect: `
			{
			    "AdapterState":null,
			    "Returns":[
			        {
			            "Check":{
			                "Status":{
			                    "code":7,
			                    "message":"threescale.handler.istio-system:user_key required as query parameter"
			                },
			                "ValidDuration": 0,
                                        "ValidUseCount": -1
			            },
			            "Quota":null,
			            "Error":null
			        }
			    ]
			}`,
		},
	}

	for _, input := range inputs {
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

		integration.RunTest(t, nil, s)
	}

}

func startTestBackends(t *testing.T) (*httptest.Server, *httptest.Server) {
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
		io.WriteString(w, strings.Replace(sysFake.GetProxyConfigLatestJson(), "https://su1.3scale.net", "http://127.0.0.1:8091", -1))
	}))
	ts.Listener.Close()
	ts.Listener = sysListener
	ts.Start()

	bs := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("user_key") == "INVALID" {
			io.WriteString(w, fake.GenInvalidUserKey("ANY"))
			return
		}
		io.WriteString(w, fake.GetAuthSuccess())
		return
	}))

	bs.Listener.Close()
	bs.Listener = backendListener
	bs.Start()

	return ts, bs
}
