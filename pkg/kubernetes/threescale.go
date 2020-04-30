package kubernetes

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/3scale/3scale-istio-adapter/config"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale"
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

	defaultThreescaleAppIdLabel  = threescale.AppIDAttributeKey
	defaultThreescaleAppKeyLabel = threescale.AppKeyAttributeKey
	defaultThreescaleOIDCLabel   = threescale.OIDCAttributeKey

	//DefaultApiKeyAttribute string for a 3scale adapter instance - Api Key pattern
	DefaultApiKeyAttribute = `request.query_params["user_key"] | request.headers["user_key"] | ""`
	//DefaultAppIDAttribute string for a 3scale adapter instance - App ID pattern
	DefaultAppIDAttribute = `request.query_params["app_id"] | request.headers["app_id"] | ""`
	//DefaultAppKeyAttribute string for a 3scale adapter instance - App ID/OIDC pattern
	DefaultAppKeyAttribute = `request.query_params["app_key"] | request.headers["app_key"] | ""`
	//DefaultOIDCAttribute string for a 3scale adapter instance - OIDC pattern
	DefaultOIDCAttribute = `request.auth.claims["azp"] | ""`
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
			Address: fmt.Sprintf("dns:///%s:%d", defaultThreescaleAdapterListenAddress, defaultThreescaleAdapterListenPort),
		},
	}, nil
}

// NewDefaultHybridInstance - new base instance supporting all authentication methods with default values
func NewDefaultHybridInstance() *BaseInstance {
	return &BaseInstance{
		Template: defaultThreescaleAuthorizationTemplateName,
		Params: InstanceParams{
			Subject: InstanceSubject{
				User: DefaultApiKeyAttribute,
				Properties: map[string]interface{}{
					defaultThreescaleAppIdLabel:  DefaultAppIDAttribute,
					defaultThreescaleAppKeyLabel: DefaultAppKeyAttribute,
					defaultThreescaleOIDCLabel:   DefaultOIDCAttribute,
				},
			},
			Action: getDefaultThreescaleInstanceAction(),
		},
	}
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

// NewAppIDAppKeyInstance - new base instance supporting AppID/App Key authentication
func NewAppIDAppKeyInstance(appIdentifier, appKeyIdentifier string) *BaseInstance {
	return &BaseInstance{
		Template: defaultThreescaleAuthorizationTemplateName,
		Params: InstanceParams{
			Subject: InstanceSubject{
				Properties: map[string]interface{}{
					defaultThreescaleAppIdLabel:  appIdentifier,
					defaultThreescaleAppKeyLabel: appKeyIdentifier,
				},
			},
			Action: getDefaultThreescaleInstanceAction(),
		},
	}
}

// NewOIDCInstance - new base instance supporting config required by OIDC integration
func NewOIDCInstance(appIdentifier, appKeyIdentifier string) *BaseInstance {
	return &BaseInstance{
		Template: defaultThreescaleAuthorizationTemplateName,
		Params: InstanceParams{
			Subject: InstanceSubject{
				Properties: map[string]interface{}{
					defaultThreescaleAppKeyLabel: appKeyIdentifier,
					defaultThreescaleOIDCLabel:   appIdentifier,
				},
			},
			Action: getDefaultThreescaleInstanceAction(),
		},
	}
}

// NewRule constructor for Istio Rule specific to 3scale requirements
// This rule will 'AND' the provided match conditions and does not accept multiple handlers,instances
func NewRule(matchConditions MatchConditions, handler string, instance string) Rule {
	matchOn := matchConditions.conditionsToMatchString()
	r := Rule{
		Match: matchOn,
		Actions: []*v1beta1.Action{
			{
				Handler:   handler,
				Instances: []string{instance}},
		},
	}
	return r
}

func getDefaultThreescaleInstanceAction() InstanceAction {
	return InstanceAction{
		Path:    "request.url_path",
		Method:  `request.method | "get"`,
		Service: `destination.labels["service-mesh.3scale.net/service-id"] | ""`,
	}
}

// GetDefaultMatchConditions for a 3scale adapter rule, formatted for the provided credentials(handler)
func GetDefaultMatchConditions(credentialsName string) MatchConditions {
	return MatchConditions{
		`context.reporter.kind == "inbound"`,
		fmt.Sprintf(`destination.labels["service-mesh.3scale.net/credentials"] == "%s"`, credentialsName),
		`destination.labels["service-mesh.3scale.net/authentication-method"] == ""`,
	}
}

// conditionsToMatchString returns a valid expression for Istio match condition
func (mc MatchConditions) conditionsToMatchString() string {
	return strings.Join(mc, " &&\n")
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
