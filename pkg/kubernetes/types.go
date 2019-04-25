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
	rc   *rest.RESTClient
}

//HandlerSpec - encapsulates the logic necessary to interface Mixer with OOP adapter
type HandlerSpec struct {
	// Adapter name which this handler should use
	Adapter string `json:"adapter"`
	// Params to pass to adapter configuration
	Params config.Params `json:"params"`
	// Connection allows the operator to specify the endpoint for out-of-process infrastructure backend.
	Connection v1beta1.Connection `json:"connection"`
}
