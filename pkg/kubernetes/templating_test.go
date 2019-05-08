package kubernetes

import (
	"reflect"
	"testing"
)

func TestNewConfigGenerator(t *testing.T) {
	inputs := []struct {
		name        string
		provideName string
		expectErr   bool
		expect      *ConfigGenerator
	}{
		{
			name:      "Test fail with provided name empty",
			expectErr: true,
		},
		{
			name:        "Test fail with invalid name",
			provideName: "bad_value",
			expectErr:   true,
		},
		{
			name:        "Test default values set",
			provideName: "good",
			expectErr:   false,
			expect: &ConfigGenerator{
				handler:   HandlerSpec{},
				instance:  BaseInstance{},
				rule:      Rule{},
				name:      "good",
				namespace: defaultNamespace,
				outputAs:  YAML,
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			cg, err := NewConfigGenerator(input.provideName, HandlerSpec{}, BaseInstance{}, Rule{})
			if err != nil {
				if !input.expectErr {
					t.Errorf("unexpected error")
				}
				return
			}
			if !reflect.DeepEqual(&cg, &input.expect) {
				t.Errorf("unexpected result after running through constructor")
			}

			cg.SetNamespace("any")
			if cg.namespace != "any" {
				t.Errorf("failed to update namespace via setter func")
			}
		})
	}
}
