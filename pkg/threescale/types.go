package threescale

import (
	"net"
	"time"

	"github.com/3scale/3scale-istio-adapter/pkg/threescale/authorizer"
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
	conf     *AdapterConfig
}

// AdapterConfig wraps optional configuration for the 3scale adapter
type AdapterConfig struct {
	authorizer authorizer.Authorizer
	//gRPC connection keepalive duration
	keepAliveMaxAge time.Duration
}
