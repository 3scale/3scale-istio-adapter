package templating

/*
The templating package exposes functionality that is used to generate CustomResource manifests for
an out-of-process adapter. To route your application traffic for a service within Istio mesh through Mixer and
in turn an OOP adapter three different custom resources are required. The templating package can be used to generate these:
handler, instance and service.

This package is bound to Istio 1.1 specific attributes.
*/

// AuthenticationMethod specifies the authentication pattern that should be used to generate the templates
type AuthenticationMethod int

// ConfigGenerator - Used to expose and generate the desired config
type ConfigGenerator struct {
	Handler  Handler
	Instance Instance
	Rule     Rule
	uid      string
	host     string
}

// Handler - encapsulates the logic necessary to interface Mixer with 3scale system
type Handler struct {
	// Access token for the 3scale service behind the generated config
	AccessToken string
	// URL of 3scale admin portal
	SystemURL string
	// The DNS name that Mixer will attempt to connect to adapter on
	ListenAddress string
	// The port that Mixer will attempt to connect to the adapter over
	Port int
	// Upstream Service ID that the handler should be called for
	ServiceID string
}

// CredentialsLocation - where to look for authn settings - should be one of "header", "query"
// "header" values maps to istio attribute request.headers
// "query" values maps to istio attribute request.query_params
type CredentialsLocation int

// Instance - specifies the request mapping from attributes
// An Instance is used to convert authentication details into the appropriate Istio related resource
type Instance struct {
	CredentialsLocation CredentialsLocation
	ApiKeyLabel         string
	AppIDLabel          string
	AppKeyLabel         string
	AuthnMethod         AuthenticationMethod
}

// MatchConditions - A list of conditions that must be through for a request to match
type MatchConditions []string

// Rule - specifies when a particular handler is invoked.
type Rule struct {
	Conditions MatchConditions
}
