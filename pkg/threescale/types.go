package threescale

import (
	"net"
	"net/http"
	"time"

	"github.com/3scale/3scale-istio-adapter/pkg/threescale/connectors/backend"
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
	backend         backend.Backend
	keepAliveMaxAge time.Duration
}
