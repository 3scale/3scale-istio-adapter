package kubernetes

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation"
)

const defaultNamespace = "istio-system"

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
		namespace: defaultNamespace,
		outputAs:  YAML,
	}, nil
}

// SetNamespace the configuration should be generated for
func (cg *ConfigGenerator) SetNamespace(ns string) *ConfigGenerator {
	cg.namespace = ns
	return cg
}
