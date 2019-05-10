package kubernetes

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/util/validation"
)

// DefaultNamespace to generate configuration for
const DefaultNamespace = "istio-system"

const (
	// Optional output formatting for configuration
	YAML = iota
)

// NewConfigGenerator constructs and validate a ConfigGenerator. Setting sensible defaults which can be overridden later
func NewConfigGenerator(name string, handler HandlerSpec, instance BaseInstance, rule Rule) (*ConfigGenerator, error) {
	if name == "" {
		return nil, fmt.Errorf("name must be provided")
	}

	errs := validation.IsDNS1123Label(name)
	if len(errs) > 0 {
		var errStr string
		for _, e := range errs {
			errStr += e + "\n"
		}
		return nil, fmt.Errorf("provided name %s fails Kubernetes validation: %s", name, errStr)
	}

	return &ConfigGenerator{
		handler:   handler,
		instance:  instance,
		rule:      rule,
		name:      name,
		namespace: DefaultNamespace,
		outputAs:  YAML,
	}, nil
}

// OutputAll required manifests(instance, handler,rule) to provided writer
func (cg *ConfigGenerator) OutputAll(w io.Writer) error {
	buffer := bytes.Buffer{}

	objs := []*IstioResource{
		getBaseResource(cg.name, cg.namespace, handlerKind).spec(cg.handler),
		getBaseResource(cg.name, cg.namespace, instanceKind).spec(cg.instance),
		getBaseResource(cg.name, cg.namespace, ruleKind).spec(cg.rule),
	}

	for _, obj := range objs {
		b, err := cg.marshalIstioResource(obj)
		if err != nil {
			return err
		}
		buffer.Write(b)
		buffer.Write([]byte("---\n"))
	}
	_, err := w.Write(buffer.Bytes())
	return err
}

// SetNamespace the configuration should be generated for
func (cg *ConfigGenerator) SetNamespace(ns string) *ConfigGenerator {
	cg.namespace = ns
	return cg
}

func (cg *ConfigGenerator) marshalIstioResource(obj *IstioResource) ([]byte, error) {
	if cg.outputAs == YAML {
		return yaml.Marshal(obj)
	}
	return nil, fmt.Errorf("currently unsupported output format provided")
}
