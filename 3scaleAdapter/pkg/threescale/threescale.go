// nolint:lll
// Generates the Threescale adapter's resource yaml. It contains the adapter's configuration, name,
// supported template names (metric in this case), and whether it is session or no-session based.

// nolint: lll
//go:generate $GOPATH/src/istio.io/istio/bin/mixer_codegen.sh -a mixer/adapter/3scaleAdapter/config/config.proto -x "-s=false -n Threescale -t authorization -t logentry"
package threescale

import (
	"context"
	"errors"
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
	prometheus "github.com/3scale/istio-integration/3scaleAdapter/pkg/threescale/metrics"
	"github.com/gogo/googleapis/google/rpc"
	"google.golang.org/grpc"
	"istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/pkg/status"
	"istio.io/istio/mixer/template/authorization"
	"istio.io/istio/mixer/template/logentry"
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
		listener      net.Listener
		server        *grpc.Server
		client        *http.Client
		conf          *AdapterConfig
		reportMetrics bool
	}

	// AdapterConfig wraps optional configuration for the 3scale adapter
	AdapterConfig struct {
		systemCache     *ProxyConfigCache
		metricsReporter *prometheus.Reporter
	}

	reportLatency func(systemURL string, serviceID string, duration time.Duration)
)

// For this PoC I'm using the authorize template, but we should check if the quota template
// is more convenient and we can do some optimizations around.
var _ authorization.HandleAuthorizationServiceServer = &Threescale{}
var _ logentry.HandleLogEntryServiceServer = &Threescale{}

func (s *Threescale) HandleLogEntry(ctx context.Context, l *logentry.HandleLogEntryRequest) (*v1beta1.ReportResult, error) {
	var report v1beta1.ReportResult
	var err error

	if l.AdapterConfig == nil {
		return &report, fmt.Errorf("adapter config not available")
	}

	cfg := &config.Params{}
	if err := cfg.Unmarshal(l.AdapterConfig.Value); err != nil {
		log.Errorf("error unmarshalling adapter config: %v", err)
		return &report, err
	}

	systemC, err := s.systemClientBuilder(cfg.SystemUrl)

	if err != nil {
		log.Errorf("error building HTTP client for 3scale system - %s", err.Error())
		return &report, err
	}

	err = s.reportUsage(cfg, l.Instances, systemC)

	if err != nil {
		log.Errorf("error reporting usage - %s", err.Error())
	}

	return &report, err
}
func (s *Threescale) reportUsage(cfg *config.Params, instances []*logentry.InstanceMsg, c *sysC.ThreeScaleClient) error {
	for _, instance := range instances {
		var pce sysC.ProxyConfigElement
		var proxyConfErr error

		urlWithPath := instance.Variables["url"].GetStringValue()
		userKey := instance.Variables["user"].GetStringValue()
		method := instance.Variables["method"].GetStringValue()

		//TODO: Report proper timestamp
		//timestamp := request.Timestamp.GetValue()

		u, err := url.Parse(urlWithPath)
		if err != nil {
			return err
		}

		if urlWithPath == "" || userKey == "" || method == "" {
			return errors.New("missing required parameters")
		}

		if s.conf.systemCache != nil {
			pce, proxyConfErr = s.conf.systemCache.get(cfg, c)
		} else {
			pce, proxyConfErr = getFromRemote(cfg, c, s.conf.metricsReporter.ObserveBackendLatency)
		}

		if proxyConfErr != nil {
			return errors.New("internal error - unable to unmarshal adapter config")
		}

		authType := pce.ProxyConfig.Content.BackendAuthenticationType

		switch authType {
		case "provider_key", "service_token":
			return s.doReport(cfg.ServiceId, userKey, u.Path, method, pce.ProxyConfig)
		default:
			return fmt.Errorf("unsupported auth type for service %s", cfg.ServiceId)

		}
	}
	return nil
}

func (s *Threescale) doReport(svcID string, userKey string, path string, method string, conf sysC.ProxyConfig) error {
	var resp backendC.ApiResponse
	var apiErr error

	bc, err := s.backendClientBuilder(conf.Content.Proxy.Backend.Endpoint)
	if err != nil {
		return err
	}

	metrics := generateMetrics(path, method, conf)
	if len(metrics) == 0 {
		return errors.New("report failed, no matching mapping rule")
	}

	params := backendC.NewAuthRepParamsUserKey("", "", metrics, nil)

	auth := backendC.TokenAuth{
		Type:  conf.Content.BackendAuthenticationType,
		Value: conf.Content.BackendAuthenticationValue,
	}

	resp, apiErr = bc.AuthRepUserKey(auth, userKey, svcID, params)

	if apiErr != nil {
		return apiErr
	}

	if !resp.Success {
		return errors.New("report has not been successful")
	}

	return nil
}

// HandleAuthorization takes care of the authorization request from mixer
func (s *Threescale) HandleAuthorization(ctx context.Context, r *authorization.HandleAuthorizationRequest) (*v1beta1.CheckResult, error) {
	var result v1beta1.CheckResult

	go s.conf.metricsReporter.IncrementTotalRequests()

	log.Debugf("Got instance %+v", r.Instance)

	// We set the result to 1 ms valid duration to avoid
	// Mixer caching the request and not reporting everything
	// This is a hack, we should fine a better way to do this.
	// Same happens with ValidUseCount, we need to investigate more.
	result.ValidDuration = 0 * time.Second

	if r.AdapterConfig == nil {
		err := errors.New("internal error - adapter config is not available")
		log.Error(err.Error())
		result.Status = status.WithError(err)
		return &result, err
	}

	cfg := &config.Params{}
	if err := cfg.Unmarshal(r.AdapterConfig.Value); err != nil {
		unmarshalErr := errors.New("internal error - unable to unmarshal adapter config")
		log.Errorf("%s: %v", unmarshalErr, err)
		result.Status = status.WithError(unmarshalErr)
		return &result, err
	}

	systemC, err := s.systemClientBuilder(cfg.SystemUrl)
	if err != nil {
		err = fmt.Errorf("error building HTTP client for 3scale system - %s", err.Error())
		log.Error(err.Error())
		result.Status = status.WithInvalidArgument(err.Error())
		return &result, err
	}

	returnedStatus, err := s.isAuthorized(cfg, *r.Instance, systemC)
	if err != nil {
		log.Errorf("error authorizing request - %s", err.Error())
	}

	result.Status = returnedStatus

	// Add caching to valid calls.
	if returnedStatus.Code == 0 {
		seconds, _ := strconv.Atoi(cfg.CacheValidSeconds)
		hits, _ := strconv.Atoi(cfg.CacheValidHits)
		result.ValidDuration = time.Duration(seconds) * time.Second
		result.ValidUseCount = int32(hits)
	} else {
		// No Cache for failed auth
		result.ValidDuration = 0 * time.Second
		result.ValidUseCount = 0
	}

	return &result, err
}

// isAuthorized returns code 0 if authorization is successful
// based on grpc return codes https://github.com/grpc/grpc-go/blob/master/codes/codes.go
func (s *Threescale) isAuthorized(cfg *config.Params, request authorization.InstanceMsg, c *sysC.ThreeScaleClient) (rpc.Status, error) {
	var pce sysC.ProxyConfigElement
	var proxyConfErr error

	userKey := request.Subject.User

	if userKey == "" {
		return status.WithPermissionDenied("user_key must be provided as a query parameter"), nil
	}

	if s.conf.systemCache != nil {
		pce, proxyConfErr = s.conf.systemCache.get(cfg, c)
	} else {
		pce, proxyConfErr = getFromRemote(cfg, c, s.conf.metricsReporter.ObserveSystemLatency)
	}

	if proxyConfErr != nil {
		return status.WithMessage(
			9, fmt.Sprintf("currently unable to fetch required data from 3scale system - %s", proxyConfErr.Error())), proxyConfErr
	}

	return s.doAuth(cfg.ServiceId, userKey, request, pce.ProxyConfig)
}
func (s *Threescale) doAuth(svcID string, userKey string, request authorization.InstanceMsg, conf sysC.ProxyConfig) (rpc.Status, error) {
	var resp backendC.ApiResponse
	var apiErr error

	if request.Action.Path == "" {
		return status.WithInvalidArgument("missing request path"), nil
	}

	bc, err := s.backendClientBuilder(conf.Content.Proxy.Backend.Endpoint)
	if err != nil {
		return status.WithInvalidArgument(
			fmt.Sprintf("error creating 3scale backend client - %s", err.Error())), err
	}

	metrics := generateMetrics(request.Action.Path, request.Action.Method, conf)
	if len(metrics) == 0 {
		return status.WithPermissionDenied(fmt.Sprintf("no matching mapping rule for request with method %s and path %s",
			request.Action.Method, request.Action.Path)), nil
	}

	start := time.Now()
	switch conf.Content.BackendAuthenticationType {

	case "provider_key", "service_token":
		params := backendC.NewAuthorizeKeyParams("", "")
		params.Metrics = metrics
		resp, apiErr = bc.AuthorizeKey(userKey, conf.Content.BackendAuthenticationValue, svcID, params)

	default:
		return status.WithInvalidArgument("invalid authentication type"), nil
	}
	elapsed := time.Since(start)

	if apiErr != nil {
		return status.WithMessage(2, fmt.Sprintf("error calling 3scale Auth -  %s", apiErr.Error())), apiErr
	}

	if !resp.Success {
		return status.WithPermissionDenied(resp.Reason), nil
	}

	if s.conf.metricsReporter != nil {
		go s.conf.metricsReporter.ObserveBackendLatency(conf.Content.Proxy.Backend.Endpoint, svcID, elapsed)
	}

	return status.OK, nil
}

func generateMetrics(path string, method string, conf sysC.ProxyConfig) backendC.Metrics {
	metrics := make(backendC.Metrics)

	for _, pr := range conf.Content.Proxy.ProxyRules {
		if match, err := regexp.MatchString(pr.Pattern, path); err == nil {
			if match && strings.ToUpper(pr.HTTPMethod) == strings.ToUpper(method) {
				baseDelta := 0
				if val, ok := metrics[pr.MetricSystemName]; ok {
					baseDelta = val
				}
				metrics.Add(pr.MetricSystemName, baseDelta+int(pr.Delta))
			}
		}
	}
	return metrics
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
	parsedUrl, err := url.ParseRequestURI(backendURL)
	if err != nil {
		return nil, err
	}

	scheme, host, port := parseURL(parsedUrl)
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
func NewThreescale(addr string, client *http.Client, conf *AdapterConfig) (Server, error) {

	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", addr))
	if err != nil {
		return nil, err
	}

	s := &Threescale{
		listener: listener,
		client:   client,
		conf:     conf,
	}

	if conf != nil {
		if conf.metricsReporter != nil {
			conf.metricsReporter.Serve()
		}
	}

	log.Infof("Threescale Istio Adapter is listening on \"%v\"\n", s.Addr())

	s.server = grpc.NewServer()
	authorization.RegisterHandleAuthorizationServiceServer(s.server, s)
	logentry.RegisterHandleLogEntryServiceServer(s.server, s)
	return s, nil
}

// NewAdapterConfig - Creates configuration for Threescale adapter
func NewAdapterConfig(cache *ProxyConfigCache, metrics *prometheus.Reporter) *AdapterConfig {
	if cache != nil {
		cache.metricsReporter = metrics
	}
	return &AdapterConfig{cache, metrics}
}
