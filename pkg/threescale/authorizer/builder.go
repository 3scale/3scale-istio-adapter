package authorizer

import (
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/3scale/3scale-go-client/threescale"
	apisonator "github.com/3scale/3scale-go-client/threescale/http"
	system "github.com/3scale/3scale-porta-go-client/client"
)

// Builder provides an interface required by the adapter to complete authorization
type Builder interface {
	BuildSystemClient(systemURL, accessToken string) (SystemClient, error)
	BuildBackendClient(backendURL string) (threescale.Client, error)
}

// SystemClient provides a minimalist interface for the adapters requirements from 3scale system
type SystemClient interface {
	GetLatestProxyConfig(serviceID, environment string) (system.ProxyConfigElement, error)
}

// ClientBuilder builds the 3scale clients, injecting the underlying HTTP client
type ClientBuilder struct {
	httpClient *http.Client
}

// NewClientBuilder returns a pointer to ClientBuilder
func NewClientBuilder(httpClient *http.Client) *ClientBuilder {
	return &ClientBuilder{httpClient: httpClient}
}

// BuildSystemClient builds a 3scale porta client from the provided URL(raw string)
// The provided 'systemURL' must be prepended with a valid scheme
func (cb ClientBuilder) BuildSystemClient(systemURL, accessToken string) (SystemClient, error) {
	var client SystemClient
	sysURL, err := url.ParseRequestURI(systemURL)
	if err != nil {
		return client, err
	}

	scheme, host, port := cb.parseURL(sysURL)
	ap, err := system.NewAdminPortal(scheme, host, port)
	if err != nil {
		return client, err
	}

	return system.NewThreeScale(ap, accessToken, cb.httpClient), nil
}

// BuildBackendClient builds a 3scale apisonator http client
// The provided 'backendURL' must be prepended with a valid scheme
func (cb ClientBuilder) BuildBackendClient(backendURL string) (threescale.Client, error) {
	return apisonator.NewClient(backendURL, cb.httpClient)
}

func (cb ClientBuilder) parseURL(url *url.URL) (string, string, int) {
	var scheme string
	host, port, _ := net.SplitHostPort(url.Host)
	if port == "" {
		scheme = url.Scheme
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
