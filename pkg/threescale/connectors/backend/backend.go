package backend

import (
	"time"

	backend "github.com/3scale/3scale-go-client/client"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale/metrics"
)

// Wrapper for requirements for 3scale Auth/AuthRep API
type AuthRepRequest struct {
	//required
	ServiceID string
	//required
	Request backend.Request
	//optional
	Params backend.AuthRepParams
}

// Backend for 3scale API management
// Operations supported by this interface require a client as we need to be able to call against multiple remote backends
type Backend interface {
	AuthRep(req AuthRepRequest, c *backend.ThreeScaleClient) (Response, error)
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
func (db DefaultBackend) AuthRep(req AuthRepRequest, c *backend.ThreeScaleClient) (Response, error) {
	mc := metricsConfig{
		ReportFn: db.ReportFn,
		Endpoint: "AuthRep",
		Target:   "Backend",
	}

	resp, err := remoteAuthRep(req, nil, c, mc)
	if err != nil {
		return Response{}, err
	}
	return convertResponse(resp), nil
}

func remoteAuth(req AuthRepRequest, ext map[string]string, c *backend.ThreeScaleClient, mc metricsConfig) (backend.ApiResponse, error) {
	var (
		start   time.Time
		elapsed time.Duration
	)

	start = time.Now()
	resp, apiErr := c.Authorize(req.Request, req.ServiceID, req.Params.Metrics, ext)
	elapsed = time.Since(start)

	if mc.ReportFn != nil {
		go mc.ReportFn(req.ServiceID, metrics.NewLatencyReport(mc.Endpoint, elapsed, c.GetPeer(), mc.Target),
			metrics.NewStatusReport(mc.Endpoint, resp.StatusCode, c.GetPeer(), mc.Target))
	}

	return resp, apiErr
}

func remoteAuthRep(req AuthRepRequest, ext map[string]string, c *backend.ThreeScaleClient, mc metricsConfig) (backend.ApiResponse, error) {
	var (
		start   time.Time
		elapsed time.Duration
	)

	start = time.Now()
	resp, apiErr := c.AuthRep(req.Request, req.ServiceID, req.Params, ext)
	elapsed = time.Since(start)

	if mc.ReportFn != nil {
		go mc.ReportFn(req.ServiceID, metrics.NewLatencyReport(mc.Endpoint, elapsed, c.GetPeer(), mc.Target),
			metrics.NewStatusReport(mc.Endpoint, resp.StatusCode, c.GetPeer(), mc.Target))
	}

	return resp, apiErr
}

func convertResponse(original backend.ApiResponse) Response {
	return Response{
		Reason:     original.Reason,
		StatusCode: original.StatusCode,
		Success:    original.Success,
	}
}
