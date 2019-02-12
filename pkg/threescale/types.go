package threescale

import (
	"net"
	"net/http"

	"github.com/3scale/3scale-go-client/client"

	prometheus "github.com/3scale/3scale-istio-adapter/pkg/threescale/metrics"
	"google.golang.org/grpc"
)

// Server interface - specifies the interface for gRPC server/adapter
type Server interface {
	Addr() string
	Close() error
	Run(shutdown chan error)
}

// Threescale contains the Listener and the server
type Threescale struct {
	listener net.Listener
	server   *grpc.Server
	client   *http.Client
	conf     *AdapterConfig
}

// AdapterConfig wraps optional configuration for the 3scale adapter
type AdapterConfig struct {
	systemCache     *ProxyConfigCache
	metricsReporter *prometheus.Reporter
}

// reportMetrics - function that defines requirements for reporting metrics around interactions between 3scale and the adapter
type reportMetrics func(serviceID string, l prometheus.LatencyReport, s prometheus.StatusReport)

type authRepFn func(auth client.TokenAuth, key string, svcID string, params client.AuthRepParams) (client.ApiResponse, error)

type authRepRequest struct {
	svcID   string
	authKey string
	params  client.AuthRepParams
	auth    client.TokenAuth
}
