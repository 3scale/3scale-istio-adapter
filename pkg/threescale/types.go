package threescale

import (
	"net"
	"time"

	"github.com/3scale/3scale-porta-go-client/client"

	"github.com/3scale/3scale-authorizer/pkg/authorizer"
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

type Authorizer interface {
	GetSystemConfiguration(systemURL string, request authorizer.SystemRequest) (client.ProxyConfig, error)
	AuthRep(backendURL string, request authorizer.BackendRequest) (*authorizer.BackendResponse, error)
	Shutdown()
}

// AdapterConfig wraps optional configuration for the 3scale adapter
type AdapterConfig struct {
	Authorizer Authorizer
	//gRPC connection keepalive duration
	KeepAliveMaxAge time.Duration
}
