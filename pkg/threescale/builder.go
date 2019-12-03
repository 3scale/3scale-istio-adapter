package threescale

import (
	"net/http"
	"net/url"

	"github.com/3scale/3scale-go-client/threescale"
	apisonator "github.com/3scale/3scale-go-client/threescale/http"
	system "github.com/3scale/3scale-porta-go-client/client"
)

// Builder provides an interface required by the adapter to complete authorization
type Builder interface {
	BuildSystemClient(string) (system.ThreeScaleClient, error)
	BuildBackendClient(string) (threescale.Client, error)
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
func (cb ClientBuilder) BuildSystemClient(systemURL string) (system.ThreeScaleClient, error) {
	var client system.ThreeScaleClient
	sysURL, err := url.ParseRequestURI(systemURL)
	if err != nil {
		return client, err
	}

	scheme, host, port := parseURL(sysURL)
	ap, err := system.NewAdminPortal(scheme, host, port)
	if err != nil {
		return client, err
	}

	return *system.NewThreeScale(ap, cb.httpClient), nil
}

// BuildBackendClient builds a 3scale apisonator http client
func (cb ClientBuilder) BuildBackendClient(backendURL string) (threescale.Client, error) {
	return apisonator.NewClient(backendURL, cb.httpClient)
}
