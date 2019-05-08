// Package kubernetes implements read/write access to various Kubernetes resources

package kubernetes

import (
	"github.com/3scale/3scale-istio-adapter/config"
	"istio.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// IstioResource represents a generic Istio resource of interest (handler,instance,rule)
type IstioResource struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              interface{} `json:"spec"`
}

// K8Client provides access to core Kubernetes resources
type K8Client struct {
	conf *rest.Config
	cs   kubernetes.Interface
}

// IstioClient provides access to a specific set of Istio resources on Kubernetes
// These resources are currently specific to the out-of-process adapters
type IstioClient interface {
	CreateHandler(name string, inNamespace string, spec HandlerSpec) (*IstioResource, error)
}

// IstioClientImpl provides access to a specific set of Istio resources on Kubernetes
// These resources are currently specific to the out-of-process adapters
type IstioClientImpl struct {
	conf *rest.Config
	rc   rest.Interface
}

/*
 3scale specific types
*/

//HandlerSpec - encapsulates the logic necessary to interface Mixer with OOP adapter
type HandlerSpec struct {
	// Adapter name which this handler should use
	Adapter string `json:"adapter"`
	// Params to pass to adapter configuration
	Params config.Params `json:"params"`
	// Connection allows the operator to specify the endpoint for out-of-process infrastructure backend.
	Connection v1beta1.Connection `json:"connection"`
}

// BaseInstance that all 3scale authorization methods build from
type BaseInstance struct {
	// Template name - a template defines parameters for performing policy enforcement within Istio.
	Template string         `json:"template"`
	Params   InstanceParams `json:"params"`
}

// InstanceParams subset of authorization fields required by 3scale
type InstanceParams struct {
	Subject InstanceSubject `json:"subject"`
	Action  InstanceAction  `json:"action"`
}

// InstanceSubject contains information that identifies the caller
type InstanceSubject struct {
	// The user name/ID that the subject represents.
	User string `json:"user,omitempty"`
	// Additional attributes about the subject.
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// InstanceAction defines how a resource is accessed
type InstanceAction struct {
	Path    string `json:"path,omitempty"`
	Method  string `json:"method,omitempty"`
	Service string `json:"service,omitempty"`
}

// MatchConditions - A list of conditions that must be through for a request to match
type MatchConditions []string

// Rule defines when the adapter should be invoked
type Rule v1beta1.Rule

// ThreescaleCredentials required to call 3scale APIs
type ThreescaleCredentials struct {
	systemURL   string
	accessToken string
}

// OutputFormat for configuration
type OutputFormat int

// ConfigGenerator - Used to expose and generate the desired config as Kubernetes resources
type ConfigGenerator struct {
	handler   HandlerSpec
	instance  BaseInstance
	rule      Rule
	name      string
	namespace string
	outputAs  OutputFormat
}
