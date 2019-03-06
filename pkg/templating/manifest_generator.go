package templating

import (
	"fmt"
	"io"
	"text/template"
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
    service_id: "{{.GetUID}}"
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
  template: authorization
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
func (cg ConfigGenerator) PopulateDefaultRules() {
	conditions := MatchConditions{
		`destination.labels["service-mesh.3scale.net"] == "true"`,
		fmt.Sprintf(`destination.labels["service-mesh.3scale.net/uid"] == "%s"`, cg.GetUID()),
	}
	cg.Rule.Conditions = append(cg.Rule.Conditions, conditions...)
}
