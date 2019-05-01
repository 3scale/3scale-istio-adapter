package kubernetes

import (
	"fmt"
	"net/url"

	"github.com/3scale/3scale-istio-adapter/config"
	"istio.io/api/policy/v1beta1"
	v1 "k8s.io/api/core/v1"
)

// The functions declared here are intended to be specific to 3scale requirements and the 3scale adapter/ utils

const (
	accessTokenKey = "access_token"
	systemURLKey   = "system_url"

	defaultThreescaleAdapterName          = "threescale"
	defaultThreescaleAdapterListenAddress = "threescale-istio-adapter"
	defaultThreescaleAdapterListenPort    = 3333

	defaultThreescaleAuthorizationTemplateName = "threescale-authorization"
)

// NewThreescaleHandlerSpec returns a handler spec as per 3scale config
func NewThreescaleHandlerSpec(accessToken, systemURL, svcID string) (*HandlerSpec, error) {
	if accessToken == "" || systemURL == "" {
		return nil, fmt.Errorf("access token and url are required for valid handler and cannot be empty")
	}

	if _, err := parseURL(systemURL); err != nil {
		return nil, err
	}

	return &HandlerSpec{
		Adapter: defaultThreescaleAdapterName,
		Params: config.Params{
			ServiceId:   svcID,
			SystemUrl:   systemURL,
			AccessToken: accessToken,
		},
		Connection: v1beta1.Connection{
			Address: fmt.Sprintf("%s:%d", defaultThreescaleAdapterListenAddress, defaultThreescaleAdapterListenPort),
		},
	}, nil
}

// NewApiKeyInstance - new base instance supporting Api Key authentication
func NewApiKeyInstance(userIdentifier string) *BaseInstance {
	return &BaseInstance{
		Template: defaultThreescaleAuthorizationTemplateName,
		Params: InstanceParams{
			Subject: InstanceSubject{
				User: userIdentifier,
			},
			Action: getDefaultThreescaleInstanceAction(),
		},
	}
}

func getDefaultApiKeyAttributeString() string {
	return `request.query_params["user_key"] | request.headers["x-user-key"] | ""`
}

func getDefaultThreescaleInstanceAction() InstanceAction {
	return InstanceAction{
		Path:    "request.url_path",
		Method:  `request.method | "get"`,
		Service: `destination.labels["service-mesh.3scale.net/service-id"] | ""`,
	}
}

// convertSecret contents to 3scale credentials
// Returns credentials if successfully validated and boolean to verify the validation
func convertSecret(secret *v1.Secret) (*ThreescaleCredentials, bool) {
	accessToken := secret.Data[accessTokenKey]
	systemURL := secret.Data[systemURLKey]
	if len(accessToken) == 0 || len(systemURL) == 0 {
		return nil, false
	}
	return &ThreescaleCredentials{string(systemURL), string(accessToken)}, true
}

// parseURL parses a URL from a raw string. Returns error if parsing fails.
func parseURL(uri string) (*url.URL, error) {
	u, err := url.ParseRequestURI(uri)
	if err != nil {
		return u, fmt.Errorf("error parsing provided url")
	}

	return u, nil
}
