package templating

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	// Generate templates that support both ApiKey and ApplicationID AuthenticationMethod
	Hybrid AuthenticationMethod = iota
	// Generate templates that support single randomized strings or hashes acting as an identifier and a secret token
	ApiKey
	// Generate templates that support immutable identifier and (optional) mutable secret key strings
	ApplicationID
)

const (
	// Informs the generator to create a template that will parse the query parameters for credentials
	QueryParams CredentialsLocation = iota + 1
	// Informs the generator to create a template that will parse the headers for credentials
	Headers
)

const (
	defaultListenAddress = "threescale-istio-adapter"
	defaultListenPort    = 3333
	defaultUserKeyLabel  = "user_key"
	defaultAppIdLabel    = "app_id"
	defaultAppKeyLabel   = "app_key"

	legalCharSeparator = "-"
)

// NewConfigGenerator - constructor for ConfigGenerator which validates the input and generates a UID
// Constructor should be used in order to guarantee the generation of valid templates.
func NewConfigGenerator(h Handler, i Instance, r Rule) (*ConfigGenerator, error) {
	u, err := h.parseURL()
	if err != nil {
		return nil, err
	}

	uid, err := h.uidGenerator(u)
	if err != nil {
		return nil, fmt.Errorf("UID generation failed: %s", err.Error())
	}

	i.validate()

	return &ConfigGenerator{h, i, r, uid, u.Hostname()}, nil
}

// Return a string of the provided AuthenticationMethod
func (am AuthenticationMethod) String() string {
	return [...]string{"Hybrid", "ApiKey", "ApplicationID"}[am]
}

// Returns the hostname for the ConfigGenerator
func (cg ConfigGenerator) GetHost() string {
	return cg.host
}

// Returns the unique UID for the ConfigGenerator
func (cg ConfigGenerator) GetUID() string {
	return cg.uid
}

// Returns the API/Service ID for the ConfigGenerator
func (cg ConfigGenerator) GetServiceID() string {
	return cg.Handler.ServiceID
}

// OutputHandler - creates a handler for adapter
// See https://istio.io/docs/concepts/policies-and-telemetry/#handlers
func (cg ConfigGenerator) OutputHandler(w io.Writer) error {
	t := template.New("handler - config.istio.io/v1alpha2 ")
	t, err := t.Parse(`# handler for adapter threescale
apiVersion: "config.istio.io/v1alpha2"
kind: handler
metadata:
  name: threescale-{{.GetUID}}
  namespace: istio-system
  labels:
    "service-mesh.3scale.net/host": "{{.GetHost}}"
    "service-mesh.3scale.net/service-id": "{{.GetServiceID}}"
spec:
  adapter: threescale
  params:
    service_id: "{{.GetServiceID}}"
    system_url: "{{.Handler.SystemURL}}"
    access_token: "{{.Handler.AccessToken}}"
  connection:
    address: "{{.Handler.GenerateListenString}}"`)
	if err != nil {
		return err
	}

	err = t.Execute(w, cg)
	if err != nil {
		return err

	}
	return nil
}

// OutputInstance - creates an adapter instance
// See https://istio.io/docs/concepts/policies-and-telemetry/#instances
func (cg ConfigGenerator) OutputInstance(w io.Writer) error {
	t := template.New("instance - config.istio.io/v1alpha2")
	t, err := t.Parse(`# instance for template authorization
apiVersion: "config.istio.io/v1alpha2"
kind: instance
metadata:
  name: threescale-authorization-{{.GetUID}}
  namespace: istio-system
  labels:
    "service-mesh.3scale.net/host": "{{.GetHost}}"
    "service-mesh.3scale.net/service-id": "{{.GetServiceID}}"
spec:
  template: threescale-authorization
  params:
    subject:
      {{.Instance.GenerateAuthenticationAttributes .Instance.AuthnMethod }}
    action:
      path: request.url_path
      method: request.method | "get"`)
	if err != nil {
		return err
	}

	err = t.Execute(w, cg)
	if err != nil {
		return err

	}
	return nil
}

// OutputRule - creates a rule which drives requests through the adapter
// See https://istio.io/docs/concepts/policies-and-telemetry/#rules
func (cg ConfigGenerator) OutputRule(w io.Writer) error {
	t := template.New("rule - config.istio.io/v1alpha2")
	t, err := t.Parse(`# rule to dispatch to handler threescale.handler
apiVersion: "config.istio.io/v1alpha2"
kind: rule
metadata:
  name: threescale-{{.GetUID}}
  namespace: istio-system
  labels:
    "service-mesh.3scale.net/host": "{{.GetHost}}"
    "service-mesh.3scale.net/service-id": "{{.GetServiceID}}"
spec:
  match: {{ .Rule.ConditionsToMatchString }}
  actions:
  - handler: threescale-{{.GetUID}}.handler.istio-system
    instances:
    - threescale-authorization-{{.GetUID}}`)
	if err != nil {
		return err
	}

	err = t.Execute(w, cg)
	if err != nil {
		return err

	}
	return nil
}

// OutputAll - generates all the required templates to manage an API via
// the Istio and the adapter model
func (cg ConfigGenerator) OutputAll(w io.Writer) error {
	var err error
	type outputFn func(w io.Writer) error

	appendObject := func(write outputFn) {
		err = write(w)
		w.Write([]byte("\n---\n"))
	}

	for _, i := range []func(w io.Writer) error{cg.OutputHandler, cg.OutputInstance, cg.OutputRule} {
		appendObject(i)
		if err != nil {
			return err
		}
	}

	return nil
}

// OutputUID - Prints the UID for the ConfigGenerator to the provided writer
func (cg ConfigGenerator) OutputUID(w io.Writer) error {
	msg := fmt.Sprintf("# The UID for this service is %s", cg.GetUID())
	_, err := w.Write([]byte(msg + "\n"))
	return err
}

// PopulateDefaultRules is a helper method exposed to allow to generate the rule based on the constructed ConfigGenerator
func (cg ConfigGenerator) GetDefaultMatchConditions() []string {
	conditions := MatchConditions{
		`destination.labels["service-mesh.3scale.net"] == "true"`,
		fmt.Sprintf(`destination.labels["service-mesh.3scale.net/uid"] == "%s"`, cg.GetUID()),
	}
	return conditions
}

// GenerateListenString - creates a string from the provided Handler replacing unset/invalid values
// with internal defaults.
func (h Handler) GenerateListenString() string {
	if h.ListenAddress == "" {
		h.ListenAddress = defaultListenAddress
	}

	if h.Port == 0 {
		h.Port = defaultListenPort
	}
	return fmt.Sprintf("%s:%d", h.ListenAddress, h.Port)
}

// validates and parses Handler url
func (h Handler) parseURL() (*url.URL, error) {
	u, err := url.ParseRequestURI(h.SystemURL)
	if err != nil {
		return u, fmt.Errorf("error parsing provided url. ensure a valid url has been set on Handler")
	}
	return u, nil
}

// Generates a unique UID from the Handler struct - provided values must conform to k8 validation.
func (h Handler) uidGenerator(url *url.URL) (string, error) {
	var uid string
	if url.Host == "" || h.ServiceID == "" {
		return uid, fmt.Errorf("error generating UID. Required seeds cannot be empty")
	}

	replacer := strings.NewReplacer(":", legalCharSeparator, ".", legalCharSeparator, "/", legalCharSeparator, "--", legalCharSeparator)
	uid = replacer.Replace(url.Host + legalCharSeparator + url.Path + legalCharSeparator + h.ServiceID)
	// safely strip duplicate hyphens
	uid = replacer.Replace(uid)

	//validate that we can create a k8 object with this name
	validationErrs := validation.IsDNS1035Label(uid)
	if len(validation.IsDNS1035Label(uid)) > 0 {
		var errStr string
		for _, err := range validationErrs {
			errStr += err + " "
		}
		return "", fmt.Errorf("error. Generated UID does not conform to requirements. %s. UID %s", errStr, uid)
	}
	return uid, nil
}

// GenerateAuthenticationAttributes - templating accessible function used to
// format the authentication behaviour
func (i Instance) GenerateAuthenticationAttributes(an AuthenticationMethod) string {
	var attributes string

	switch an {
	case ApiKey:
		attributes = i.generateApiKeyAttributes()
	case ApplicationID:
		attributes = i.generateApplicationIdAttributes()
	case Hybrid:
		attributes = i.generateApiKeyAttributes() + "\n      " + i.generateApplicationIdAttributes()
	default:
		panic("unknown field passed to string generator")
	}

	return attributes
}

// formats an authentication attribute into appropriate string based on the credentials location
// if no credentials location set, defaults to checking both query params and headers
func (i Instance) formatCredentialsLocation(key string) string {
	var formatted string

	switch i.CredentialsLocation {
	case QueryParams:
		formatted = fmt.Sprintf(`request.query_params["%s"]`, key)
	case Headers:
		formatted = fmt.Sprintf(`request.headers["%s"]`, formatHeaderLabel(key))
	default:
		//Unspecified
		formatted = fmt.Sprintf(`request.query_params["%s"] | request.headers["%s"]`, key, formatHeaderLabel(key))
	}
	return formatted
}

// yaml generator for api key attribute
func (i Instance) generateApiKeyAttributes() string {
	return fmt.Sprintf(`user: %s | ""`, i.formatCredentialsLocation(i.ApiKeyLabel))
}

// yaml generator for app id/ app key attributes
func (i Instance) generateApplicationIdAttributes() string {
	return fmt.Sprintf("properties:\n        %s\n        %s",
		fmt.Sprintf(`%s: %s | ""`, defaultAppIdLabel, i.formatCredentialsLocation(i.AppIDLabel)),
		fmt.Sprintf(`%s: %s | ""`, defaultAppKeyLabel, i.formatCredentialsLocation(i.AppKeyLabel)))
}

// Validates the input by fetching a string version for the provided authentication method
func (i *Instance) validate() {
	i.AuthnMethod.String()
}

// ConditionsToMatchString returns a valid expression for Istio match condition
func (r Rule) ConditionsToMatchString() string {
	var matchOn string
	conditionLen := len(r.Conditions) - 1
	for i, condition := range r.Conditions {
		matchOn += condition
		if i < conditionLen {
			matchOn += " && "
		}
	}
	return matchOn
}

// Returns an instance with the default values
func GetDefaultInstance() Instance {
	return Instance{
		ApiKeyLabel: defaultUserKeyLabel,
		AppIDLabel:  defaultAppIdLabel,
		AppKeyLabel: defaultAppKeyLabel,
		AuthnMethod: Hybrid,
	}
}

// formatHeaderLabel formats a string to header value in an opinionated way.
// String underscores are replaced with '-' and the canonical string returned replacing the first character of each word in key to uppercase.
// Underscores are allowed in header fields, although uncommon. We choose to replace since some proxies will
// will silently drop them by default if containing underscores.
func formatHeaderLabel(queryLabel string) string {
	return http.CanonicalHeaderKey(strings.Replace(queryLabel, "_", "-", -1))
}
