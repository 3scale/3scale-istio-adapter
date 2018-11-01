// nolint:lll
// Generates the Threescale adapter's resource yaml. It contains the adapter's configuration, name,
// supported template names (metric in this case), and whether it is session or no-session based.

// nolint: lll
//go:generate $GOPATH/src/istio.io/istio/bin/mixer_codegen.sh -a mixer/adapter/3scaleAdapter/config/config.proto -x "-s=false -n Threescale -t authorization"

package threescale

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	backendC "github.com/3scale/3scale-go-client/client"
	sysC "github.com/3scale/3scale-porta-go-client/client"
	"github.com/3scale/istio-integration/3scaleAdapter/config"
	"google.golang.org/grpc"
	"istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/template/authorization"
	"istio.io/istio/pkg/log"
)

type (
	// Server interface
	Server interface {
		Addr() string
		Close() error
		Run(shutdown chan error)
	}

	// Threescale contains the Listener and the server
	Threescale struct {
		listener   net.Listener
		server     *grpc.Server
		client     *http.Client
		proxyCache *ProxyConfigCache
	}
)

// For this PoC I'm using the authorize template, but we should check if the quota template
// is more convenient and we can do some optimizations around.
var _ authorization.HandleAuthorizationServiceServer = &Threescale{}

// HandleAuthorization takes care of the authorization request from mixer
func (s *Threescale) HandleAuthorization(ctx context.Context, r *authorization.HandleAuthorizationRequest) (*v1beta1.CheckResult, error) {
	var result v1beta1.CheckResult

	log.Debugf("Got instance %+v", r.Instance)

	// We set the result to 1 ms valid duration to avoid
	// Mixer caching the request and not reporting everything
	// This is a hack, we should fine a better way to do this.
	// Same happens with ValidUseCount, we need to investigate more.
	result.ValidDuration = 1 * time.Millisecond

	if r.AdapterConfig == nil {
		result.Status.Code = 9
		return &result, fmt.Errorf("adapter config not available")
	}

	cfg := &config.Params{}
	if err := cfg.Unmarshal(r.AdapterConfig.Value); err != nil {
		log.Errorf("error unmarshalling adapter config: %v", err)
		result.Status.Code = 3
		return &result, err
	}

	systemC, err := s.systemClientBuilder(cfg.SystemUrl)
	if err != nil {
		log.Errorf("error building HTTP client for 3scale system - %s", err.Error())
		result.Status.Code = 3
		return &result, err
	}

	status, err := s.isAuthorized(cfg, *r.Instance, systemC)
	if err != nil {
		log.Errorf("error authorizing request - %s", err.Error())
	}

	result.Status.Code = status
	return &result, err
}

// isAuthorized returns code 0 is authorization is successful
// based on grpc return codes https://github.com/grpc/grpc-go/blob/master/codes/codes.go
func (s *Threescale) isAuthorized(cfg *config.Params, request authorization.InstanceMsg, c *sysC.ThreeScaleClient) (int32, error) {
	var pce sysC.ProxyConfigElement
	var proxyConfErr error

	parsedRequest, err := url.ParseRequestURI(request.Action.Path)
	if err != nil {
		return 3, err
	}

	userKey := parsedRequest.Query().Get("user_key")
	if userKey == "" {
		return 7, nil
	}

	if s.proxyCache != nil {
		pce, proxyConfErr = s.proxyCache.get(cfg, c)
	} else {
		pce, proxyConfErr = getFromRemote(cfg, c)
	}

	if proxyConfErr != nil {
		return 9, err
	}

	authType := pce.ProxyConfig.Content.BackendAuthenticationType
	switch authType {
	case "provider_key", "service_token":
		return s.doAuthRep(cfg.ServiceId, userKey, request, pce.ProxyConfig)
	default:
		return 16, fmt.Errorf("unsupported auth type for service %s", cfg.ServiceId)

	}
}

func (s *Threescale) doAuthRep(svcID string, userKey string, request authorization.InstanceMsg, conf sysC.ProxyConfig) (int32, error) {
	var resp backendC.ApiResponse
	var apiErr error

	bc, err := s.backendClientBuilder(conf.Content.Proxy.Backend.Endpoint)
	if err != nil {
		return 3, err
	}

	shouldReport, params := s.generateReports(request, conf)
	if !shouldReport {
		return 7, nil
	}

	if conf.Content.BackendAuthenticationType == "provider_key" {
		resp, apiErr = bc.AuthRepProviderKey(userKey, conf.Content.BackendAuthenticationValue, svcID, params)
	} else {
		resp, apiErr = bc.AuthRepKey(userKey, conf.Content.BackendAuthenticationValue, svcID, params)
	}

	if apiErr != nil {
		return 2, apiErr
	}

	if !resp.Success {
		return 7, nil
	}

	return 0, nil
}

func (s *Threescale) generateReports(request authorization.InstanceMsg, conf sysC.ProxyConfig) (shouldReport bool, params backendC.AuthRepKeyParams) {
	params = backendC.NewAuthRepKeyParams("", "")
	for _, pr := range conf.Content.Proxy.ProxyRules {
		if match, err := regexp.MatchString(pr.Pattern, request.Action.Path); err == nil {
			if match && strings.ToUpper(pr.HTTPMethod) == strings.ToUpper(request.Action.Method) {
				shouldReport = true
				baseDelta := 0
				if val, ok := params.Metrics[pr.MetricSystemName]; ok {
					baseDelta = val
				}
				params.Metrics.Add(pr.MetricSystemName, baseDelta+int(pr.Delta))
			}
		}
	}
	return shouldReport, params
}

func (s *Threescale) systemClientBuilder(systemURL string) (*sysC.ThreeScaleClient, error) {
	sysURL, err := url.ParseRequestURI(systemURL)
	if err != nil {
		return nil, err
	}

	scheme, host, port := parseURL(sysURL)
	ap, err := sysC.NewAdminPortal(scheme, host, port)
	if err != nil {
		return nil, err
	}

	return sysC.NewThreeScale(ap, s.client), nil
}

func (s *Threescale) backendClientBuilder(backendURL string) (*backendC.ThreeScaleClient, error) {
	url, err := url.ParseRequestURI(backendURL)
	if err != nil {
		return nil, err
	}

	scheme, host, port := parseURL(url)
	be, err := backendC.NewBackend(scheme, host, port)
	if err != nil {
		return nil, err
	}

	return backendC.NewThreeScale(be, s.client), nil
}

func parseURL(url *url.URL) (string, string, int) {
	scheme := url.Scheme
	if scheme == "" {
		scheme = "https"
	}

	host, port, _ := net.SplitHostPort(url.Host)
	if port == "" {
		if scheme == "http" {
			port = "80"
		} else if scheme == "https" {
			port = "443"
		}
	}

	if host == "" {
		host = url.Host
	}

	p, _ := strconv.Atoi(port)
	return scheme, host, p
}

// Addr returns the Threescale addrs as a string
func (s *Threescale) Addr() string {
	return s.listener.Addr().String()
}

// Run starts the Threescale grpc Server
func (s *Threescale) Run(shutdown chan error) {
	shutdown <- s.server.Serve(s.listener)
}

// Close stops the Threescale grpc Server
func (s *Threescale) Close() error {
	if s.server != nil {
		s.server.GracefulStop()
	}

	if s.listener != nil {
		_ = s.listener.Close()
	}

	return nil
}

// NewThreescale returns a Server interface
func NewThreescale(addr string, client *http.Client, proxyCache *ProxyConfigCache) (Server, error) {

	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", addr))
	if err != nil {
		return nil, err
	}

	s := &Threescale{
		listener:   listener,
		client:     client,
		proxyCache: proxyCache,
	}

	log.Infof("Threescale Istio Adapter is listening on \"%v\"\n", s.Addr())

	s.server = grpc.NewServer()
	authorization.RegisterHandleAuthorizationServiceServer(s.server, s)
	// TODO: Add report template for metrics.
	return s, nil
}
