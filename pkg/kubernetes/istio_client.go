package kubernetes

import (
	"fmt"

	"k8s.io/client-go/rest"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	istioObjGroupName    = "config.istio.io"
	istioObjGroupVersion = "v1alpha2"

	handlerKind   = "handler"
	handlerPlural = "handlers"
	instanceKind  = "instance"
	ruleKind      = "rule"
)

// NewIstioClient creates a new client from the provided configuration path
// capable of manipulating known custom resources handler, instance and rule.
// It does not take care of creating the CRD for these extensions
func NewIstioClient(confPath string, conf *rest.Config) (*IstioClientImpl, error) {
	k8, err := NewK8Client(confPath, conf)
	if err != nil {
		return nil, err
	}
	return k8.NewIstioClient()
}

// CreateHandler for Istio adapter
func (c *IstioClientImpl) CreateHandler(name string, inNamespace string, spec HandlerSpec) (*IstioResource, error) {
	result := IstioResource{}
	obj := getBaseResource(name, handlerKind).spec(spec)
	err := c.rc.Post().Namespace(inNamespace).Resource(handlerPlural).Body(obj).Do().Into(&result)
	return &result, err
}

func getBaseResource(name, kind string) *IstioResource {
	return &IstioResource{
		TypeMeta: getTypeMeta(kind),
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
	}
}

func getTypeMeta(kind string) v1.TypeMeta {
	return v1.TypeMeta{
		Kind:       kind,
		APIVersion: fmt.Sprintf("%s/%s", istioObjGroupName, istioObjGroupVersion),
	}
}

/*
 Receiver functions for IstioResource required to implement the Kubernetes runtime.Object interface
*/

// DeepCopyInto copies all properties of this object into another object of the same type that is provided as a pointer. in must be non-nil.
func (in *IstioResource) DeepCopyInto(out *IstioResource) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
}

// DeepCopy copies the receiver, creating a new IstioResource.
func (in *IstioResource) DeepCopy() *IstioResource {
	if in == nil {
		return nil
	}
	out := new(IstioResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject copies the receiver, creating a new runtime.Object.
func (in *IstioResource) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}

	return nil
}

func (in *IstioResource) spec(spec interface{}) *IstioResource {
	in.Spec = spec
	return in
}
