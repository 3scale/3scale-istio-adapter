package backend

import (
	"time"

	"github.com/3scale/3scale-go-client/threescale"
	"github.com/3scale/3scale-go-client/threescale/api"
	apisonator "github.com/3scale/3scale-go-client/threescale/http"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale/metrics"
)

// Wrapper for requirements for 3scale AuthRep API
type Request struct {
	Auth        api.ClientAuth
	ServiceID   string
	Transaction api.Transaction
}

// Backend for 3scale API management
// Operations supported by this interface require a client as we need to be able to call against multiple remote backends
type Backend interface {
	AuthRep(req Request, c threescale.Client) (Response, error)
}

// Response is the result from calling the remote 3scale API
type Response struct {
	Reason     string
	StatusCode int
	Success    bool
}

// DefaultBackend is a simple implementation which disregards any caching implementation and calls 3scale directly
// Supports reporting metrics.
type DefaultBackend struct {
	ReportFn metrics.ReportMetricsFn
}

// metricsConfig wraps the labels required for reporting metrics allowing them to be set in functions as desired
type metricsConfig struct {
	Endpoint string
	ReportFn metrics.ReportMetricsFn
	Target   metrics.Target
}

// AuthRep provides a combination of authorizing a request and reporting metrics to 3scale
func (db DefaultBackend) AuthRep(req Request, c threescale.Client) (Response, error) {
	mc := metricsConfig{
		ReportFn: db.ReportFn,
		Endpoint: "AuthRep",
		Target:   "Backend",
	}

	return callRemote(req, nil, c, mc)
}

func callRemote(req Request, ext map[string]string, c threescale.Client, mc metricsConfig) (Response, error) {
	var (
		start   time.Time
		elapsed time.Duration
	)

	apiCall := threescale.Request{
		Auth:         req.Auth,
		Extensions:   nil,
		Service:      api.Service(req.ServiceID),
		Transactions: []api.Transaction{req.Transaction},
	}

	start = time.Now()
	resp, apiErr := c.AuthRep(apiCall)
	elapsed = time.Since(start)

	var response Response
	if apiErr == nil {
		response = convertResponse(resp)
		if mc.ReportFn != nil {
			go mc.ReportFn(req.ServiceID, metrics.NewLatencyReport(mc.Endpoint, elapsed, c.GetPeer(), mc.Target),
				metrics.NewStatusReport(mc.Endpoint, response.StatusCode, c.GetPeer(), mc.Target))
		}
	}

	return response, apiErr
}

func convertResponse(original threescale.AuthorizeResult) Response {
	var success bool
	if original != nil {
		success = original.Success()
	}

	detailedResp, isHTTPClient := original.(*apisonator.AuthorizeResponse)
	if isHTTPClient {
		return Response{
			Reason:     detailedResp.Reason,
			StatusCode: detailedResp.StatusCode,
			Success:    success,
		}
	}

	return Response{
		Success: success,
	}
}
