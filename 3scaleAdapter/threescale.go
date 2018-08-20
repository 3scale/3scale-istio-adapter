// nolint:lll
// Generates the Threescale adapter's resource yaml. It contains the adapter's configuration, name,
// supported template names (metric in this case), and whether it is session or no-session based.

// nolint: lll
//go:generate $GOPATH/src/istio.io/istio/bin/mixer_codegen.sh -a mixer/adapter/3scaleAdapter/config/config.proto -x "-s=false -n threescale -t authorization"

package threescale

import (
	"context"
	"fmt"
	"github.com/3scale/istio-integration/3scaleAdapter/config"
	"github.com/3scale/istio-integration/3scaleAdapter/httpPluginClient"
	"google.golang.org/grpc"
	"istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/template/authorization"
	"istio.io/istio/pkg/log"
	"net"
	"net/http"
	"net/url"
	"time"
)

type (
	Server interface {
		Addr() string
		Close() error
		Run(shutdown chan error)
	}

	Threescale struct {
		listener net.Listener
		server   *grpc.Server
	}
)

// For this PoC I'm using the authorize template, but we should check if the quota template
// is more convenient and we can do some optimizations around.
var _ authorization.HandleAuthorizationServiceServer = &Threescale{}

func (s *Threescale) HandleAuthorization(ctx context.Context, r *authorization.HandleAuthorizationRequest) (*v1beta1.CheckResult, error) {
	var result v1beta1.CheckResult

	log.Debugf("Got instance %v\n", r.Instance)

	// We set the result to 1 ms valid duration to avoid
	// Mixer caching the request and not reporting everything
	// This is a hack, we should fine a better way to do this.
	// Same happens with ValidUseCount, we need to investigate more.
	result.ValidDuration = 1 * time.Millisecond

	cfg := &config.Params{}
	if r.AdapterConfig != nil {
		if err := cfg.Unmarshal(r.AdapterConfig.Value); err != nil {
			log.Errorf("error unmarshalling adapter config: %v", err)
			return nil, err
		}
	}
	log.Debugf("adapter config: %v\n", cfg.String())

	// Creates URL object from the config system URL.
	systemUrl, err := url.Parse(cfg.SystemUrl)

	if err != nil {
		log.Errorf("Couldn't parse the SystemURL url: %s", err)
		result.Status.Code = 13
		return &result, nil
	}

	originalRequest := buildRequestFromInstanceMsg(r.Instance)

	c := httpPluginClient.NewClient(nil)
	ok, err := c.Authorize(cfg.AccessToken, cfg.ServiceId, systemUrl, originalRequest)

	if err != nil {
		log.Infof("Problem with the threescale Client: %v\n", err)
		result.Status.Code = 7
		return &result, nil
	}

	if ok {
		// 0 -> Ok
		// check https://github.com/grpc/grpc-go/blob/master/codes/codes.go
		result.Status.Code = 0
	} else {
		// 7 -> PERMISSION DENIED
		result.Status.Code = 7
	}

	return &result, nil
}

func buildRequestFromInstanceMsg(instanceMsg *authorization.InstanceMsg) *http.Request {

	// Using the Properties from the authorization template, so the user can define
	// the required headers for different authentication methods.
	headers := make(map[string][]string)

	for k, v := range instanceMsg.Action.Properties {
		if k != "" && v.GetStringValue() != "" {
			var value []string
			headers[k] = append(value, v.GetStringValue())
		}
	}
	// Let's create the request object based on the original request from the user.
	originalRequest := &http.Request{
		Method: instanceMsg.Action.Method,
		URL: &url.URL{
			User:       &url.Userinfo{},
			Path:       instanceMsg.Action.Path,
			ForceQuery: false,
		},
		Header: headers,
	}

	return originalRequest
}

func (s *Threescale) Addr() string {
	return s.listener.Addr().String()
}

func (s *Threescale) Run(shutdown chan error) {
	shutdown <- s.server.Serve(s.listener)
}

func (s *Threescale) Close() error {
	if s.server != nil {
		s.server.GracefulStop()
	}

	if s.listener != nil {
		_ = s.listener.Close()
	}

	return nil
}

func NewThreescale(addr string) (Server, error) {
	if addr == "" {
		// Random port
		addr = "0"
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", addr))
	if err != nil {
		return nil, fmt.Errorf("unable to listen on socket: %v", err)
	}
	s := &Threescale{
		listener: listener,
	}
	fmt.Printf("listening on \"%v\"\n", s.Addr())

	s.server = grpc.NewServer()

	authorization.RegisterHandleAuthorizationServiceServer(s.server, s)
	// TODO: Add report template for metrics.
	return s, nil
}
