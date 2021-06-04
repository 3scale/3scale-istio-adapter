// nolint:lll
// Generates the Threescale adapter's resource yaml. It contains the adapter's configuration, name,
// supported template names (metric in this case), and whether it is session or no-session based.

// nolint: lll
//go:generate $GOPATH/src/istio.io/istio/bin/mixer_codegen.sh -a mixer/adapter/3scale-istio-adapter/config/config.proto -x "-s=false -n threescale -t threescale-authorization"
package threescale

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/3scale/3scale-authorizer/pkg/authorizer"
	"github.com/3scale/3scale-go-client/threescale/api"
	convert "github.com/3scale/3scale-go-client/threescale/http"
	"github.com/3scale/3scale-istio-adapter/config"
	system "github.com/3scale/3scale-porta-go-client/client"
	"github.com/gogo/googleapis/google/rpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/pkg/status"
	"istio.io/istio/mixer/template/authorization"
	"istio.io/istio/pkg/log"
)

// Implement required interface
var _ authorization.HandleAuthorizationServiceServer = &Threescale{}

const (
	// consts reflect key values in instance config - exported as required for yaml generation by cli
	AppIDAttributeKey  = "app_id"
	AppKeyAttributeKey = "app_key"
	OIDCAttributeKey   = "client_id"

	// oauthTypeIdentifier refers to the name by which 3scale config described oauth OpenID connect authentication pattern
	openIDTypeIdentifier = "oauth"

	environment = "production"
)

// HandleAuthorization takes care of the authorization request from mixer
func (s *Threescale) HandleAuthorization(ctx context.Context, r *authorization.HandleAuthorizationRequest) (*v1beta1.CheckResult, error) {

	log.Debugf("Got instance %+v", r.Instance)
	result := &v1beta1.CheckResult{
		// Caching at Mixer/Envoy layer needs to be disabled currently since we would miss reporting
		// cached requests. We can determine caching values going forward by splitting the check
		// and report functionality and using cache values obtained from 3scale extension api

		// Setting a negative value will invalidate the cache - it seems from integration test
		// and manual testing that zero values for a successful check set a large default value
		ValidDuration: 0 * time.Second,
		ValidUseCount: -1,
	}

	cfg, err := s.parseConfigParams(r)
	if err != nil {
		// this theoretically should not happen
		log.Errorf("error parsing params - %v", err)
		result.Status = status.WithInternal(err.Error())
		return result, err
	}

	err = s.validateRequestAndConfigParams(r, cfg)
	if err != nil {
		// intentionally return nil as error here as failed rpc.Status is sufficient
		result.Status = status.WithFailedPrecondition(err.Error())
		return result, nil
	}

	proxyConf, err := s.conf.Authorizer.GetSystemConfiguration(cfg.SystemUrl, s.systemRequestFromHandlerConfig(cfg))
	if err != nil {
		result.Status, err = rpcStatusErrorHandler("error fetching config from 3scale", systemErrorToRpcStatus(err), err)
		return result, err
	}

	backendReq := s.requestFromConfig(proxyConf, *r.Instance, *cfg)
	rpcFN, err := s.validateBackendRequest(backendReq)
	if err != nil {
		result.Status = rpcFN(err.Error())
		// intentionally return nil as error here as failed rpc.Status is sufficient
		return result, nil
	}

	if cfg.BackendUrl == "" {
		//if not set in the handler, take it from 3scale config
		cfg.BackendUrl = proxyConf.Content.Proxy.Backend.Endpoint
	}

	var authResult *authorizer.BackendResponse

	if proxyConf.Content.BackendVersion == openIDTypeIdentifier {
		authResult, err = s.conf.Authorizer.OauthAuthRep(cfg.BackendUrl, backendReq)
	} else {
		authResult, err = s.conf.Authorizer.AuthRep(cfg.BackendUrl, backendReq)
	}
	return s.convertAuthResponse(authResult, result, err)
}

// parseConfigParams - parses the configuration passed to the adapter from mixer
// Where an error occurs during parsing, error is formatted and logged and nil value returned for config
func (s *Threescale) parseConfigParams(r *authorization.HandleAuthorizationRequest) (*config.Params, error) {
	if r.AdapterConfig == nil {
		err := errors.New("adapter config cannot be nil")
		return nil, err
	}

	cfg := &config.Params{}
	if err := cfg.Unmarshal(r.AdapterConfig.Value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal adapter config")
	}

	// Support receiving service_id as both hardcoded value in handler and at request time
	if cfg.ServiceId == "" {
		cfg.ServiceId = r.Instance.Action.Service
	}

	return cfg, nil
}

func (s *Threescale) validateRequestAndConfigParams(r *authorization.HandleAuthorizationRequest, config *config.Params) error {
	var errMsgs []string
	if config.AccessToken == "" {
		errMsgs = append(errMsgs, errAccessToken.Error())
	}

	if config.SystemUrl == "" {
		errMsgs = append(errMsgs, errSystemURL.Error())
	}

	if config.ServiceId == "" {
		errMsgs = append(errMsgs, errServiceID.Error())
	}

	if r.Instance.Action.Path == "" {
		errMsgs = append(errMsgs, errRequestPath.Error())
	}

	if len(errMsgs) > 0 {
		var errMsg string
		for _, msg := range errMsgs {
			errMsg += fmt.Sprintf("%s. ", msg)
		}
		return errors.New(strings.TrimSpace(errMsg))
	}
	return nil
}

func (s *Threescale) systemRequestFromHandlerConfig(cfg *config.Params) authorizer.SystemRequest {
	return authorizer.SystemRequest{
		AccessToken: cfg.AccessToken,
		ServiceID:   cfg.ServiceId,
		Environment: environment,
	}
}

func (s *Threescale) requestFromConfig(systemConf system.ProxyConfig, istioConf authorization.InstanceMsg, cfg config.Params) authorizer.BackendRequest {
	var (
		// Application ID/OpenID Connect authentication pattern - App Key is optional when using this authn
		appID, appKey string
		// Application Key auth pattern
		userKey string
	)

	if istioConf.Subject != nil {
		var appIdentifierKey string

		if systemConf.Content.BackendVersion == openIDTypeIdentifier {
			// OIDC integration configured so force app identifier to come from jwt claims
			appIdentifierKey = OIDCAttributeKey
		} else {
			appIdentifierKey = AppIDAttributeKey
		}

		appID = istioConf.Subject.Properties[appIdentifierKey].GetStringValue()
		appKey = istioConf.Subject.Properties[AppKeyAttributeKey].GetStringValue()
		userKey = istioConf.Subject.User
	}
	metrics := generateMetrics(istioConf.Action.Path, istioConf.Action.Method, systemConf)

	request := authorizer.BackendRequest{
		Auth: authorizer.BackendAuth{
			Type:  systemConf.Content.BackendAuthenticationType,
			Value: systemConf.Content.BackendAuthenticationValue,
		},
		Service: cfg.ServiceId,
		Transactions: []authorizer.BackendTransaction{
			{
				Metrics: metrics,
				Params: authorizer.BackendParams{
					AppID:   appID,
					AppKey:  appKey,
					UserKey: userKey,
				},
			},
		},
	}

	return request
}

// validateBackendRequest will help us reduce network calls by verifying that required auth credentials have been set
func (s *Threescale) validateBackendRequest(request authorizer.BackendRequest) (func(string) rpc.Status, error) {
	for _, transaction := range request.Transactions {
		if transaction.Params.AppID == "" && transaction.Params.UserKey == "" {
			return status.WithUnauthenticated, errNoCredentials
		}

		if len(transaction.Metrics) == 0 {
			return status.WithNotFound, errNoMappingRule
		}
	}
	return nil, nil
}

func (s *Threescale) convertAuthResponse(resp *authorizer.BackendResponse, result *v1beta1.CheckResult, err error) (*v1beta1.CheckResult, error) {
	if err != nil {
		// Try to obtain a correct mapping for the cause of failure. This will occur in events of 500+ status codes from
		// upstream where we have not managed to get an actual response from Apisonator.
		result.Status, _ = rpcStatusErrorHandler("request authorization failed", backendResponseToRpcStatus(resp), err)
		return result, nil

	}
	if !resp.Authorized {
		result.Status = errorCodeToRpcStatus(resp.ErrorCode)(resp.ErrorCode)
	} else {
		result.Status = status.OK
	}

	return result, nil
}

func generateMetrics(path string, method string, conf system.ProxyConfig) api.Metrics {
	metrics := make(api.Metrics)

	// sort proxy rules based on Position field to establish priority
	sort.Slice(conf.Content.Proxy.ProxyRules, func(i, j int) bool {
		return conf.Content.Proxy.ProxyRules[i].Position < conf.Content.Proxy.ProxyRules[j].Position
	})

	for _, pr := range conf.Content.Proxy.ProxyRules {
		if match, err := regexp.MatchString(pr.Pattern, path); err == nil {
			if match && strings.ToUpper(pr.HTTPMethod) == strings.ToUpper(method) {
				metrics.Add(pr.MetricSystemName, int(pr.Delta))
				// stop matching if this rule has been marked as Last
				if pr.Last {
					break
				}
			}
		}
	}
	return metrics
}

// rpcStatusErrorHandler provides a uniform way to log and format error messages and status which should be
// returned to the user in cases where the authorization request is rejected.
func rpcStatusErrorHandler(userFacingErrMsg string, fn func(string) rpc.Status, err error) (rpc.Status, error) {
	if userFacingErrMsg != "" {
		var errMsg string
		if err != nil {
			errMsg = fmt.Sprintf("- %s", err.Error())
		}
		err = fmt.Errorf("%s %s", userFacingErrMsg, errMsg)
	}

	log.Error(err.Error())
	return fn(err.Error()), err
}

func systemErrorToRpcStatus(err error) func(string) rpc.Status {
	switch e := err.(type) {
	case system.ApiErr:
		code, ok := httpStatusToRpcStatus[e.Code()]
		if !ok {
			return status.WithUnknown
		}
		return code
	default:
		return status.WithUnknown
	}
}

func backendResponseToRpcStatus(result *authorizer.BackendResponse) func(string) rpc.Status {
	respondWith := status.WithUnknown
	if result != nil && result.RawResponse != nil {
		if val, ok := result.RawResponse.(*http.Response); ok {
			if code, ok := httpStatusToRpcStatus[val.StatusCode]; ok {
				respondWith = code
			}
		}
	}
	return respondWith
}

func errorCodeToRpcStatus(threescaleErrorCode string) func(string) rpc.Status {
	if threescaleErrorCode == "limits_exceeded" {
		// return equiv of 429
		return status.WithResourceExhausted
	}

	switch convert.CodeToStatusCode(threescaleErrorCode) {
	//this should never occur unless we are passed an empty reason/code by backend
	// or backend provides us with an unmapped code
	case 0:
		return status.WithUnknown
	case http.StatusConflict:
		return status.WithPermissionDenied
	default:
		// for all other cases that have reached backend return equiv of 403
		return status.WithPermissionDenied
	}
}

var httpStatusToRpcStatus = map[int]func(string) rpc.Status{
	http.StatusInternalServerError: status.WithUnknown,
	http.StatusBadRequest:          status.WithInvalidArgument,
	http.StatusGatewayTimeout:      status.WithDeadlineExceeded,
	http.StatusNotFound:            status.WithNotFound,
	http.StatusForbidden:           status.WithPermissionDenied,
	http.StatusUnauthorized:        status.WithUnauthenticated,
	http.StatusTooManyRequests:     status.WithResourceExhausted,
	http.StatusServiceUnavailable:  status.WithUnavailable,
}

var (
	errAccessToken   = errors.New("access token must be set in configuration")
	errSystemURL     = errors.New("3scale system URL must be provided in configuration")
	errServiceID     = errors.New("service ID must be provided in configuration")
	errRequestPath   = errors.New("request path must be provided")
	errNoMappingRule = errors.New("no matching mapping rule for request")
	errNoCredentials = errors.New("no auth credentials provided or provided in invalid location")
)

// NewThreescale returns a Server interface
func NewThreescale(addr string, conf *AdapterConfig) (Server, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", addr))
	if err != nil {
		return nil, err
	}

	s := &Threescale{
		listener: listener,
		conf:     conf,
	}

	log.Infof("Threescale Istio Adapter is listening on \"%v\"\n", s.Addr())

	s.server = grpc.NewServer(grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionAge: conf.KeepAliveMaxAge,
	}))
	authorization.RegisterHandleAuthorizationServiceServer(s.server, s)
	return s, nil
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
