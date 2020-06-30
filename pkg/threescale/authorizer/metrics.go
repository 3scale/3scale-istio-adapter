package authorizer

import (
	"net/http"
	"time"
)

type Cache int

const (
	System Cache = iota
	Backend
)

// TelemetryReport reports HTTP info from the request/response cycle to 3scale
type TelemetryReport struct {
	Host      string
	Method    string
	Endpoint  string
	Code      int
	TimeTaken time.Duration
}

// ResponseHook is a callback function which allows running a function after each HTTP response from 3scale
type ResponseHook func(report TelemetryReport)

// CacheHitHook is called when a hit is successful on system or backend cache
type CacheHitHook func(cache Cache)

// MetricsReporter holds config for reporting metrics
type MetricsReporter struct {
	ReportMetrics bool
	ResponseCB    ResponseHook
	CacheHitCB    CacheHitHook
}

type MetricsTransport struct {
	client *http.Client
	hook   ResponseHook
}

func (mt *MetricsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := mt.client.Transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	timeTaken := time.Now().Sub(start)
	report := TelemetryReport{
		Host:      req.Host,
		Method:    req.Method,
		Endpoint:  req.URL.Path,
		Code:      resp.StatusCode,
		TimeTaken: timeTaken,
	}
	mt.hook(report)
	return resp, err
}
