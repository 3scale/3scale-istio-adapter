package kubernetes

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
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
				namespace: DefaultNamespace,
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

func TestOutputAll(t *testing.T) {
	const credentialsName = "threescale"
	const accessToken = "secret-token"
	const systemURL = "http://127.0.0.1:8090"
	const configSource = "threescale-adapter-config.yaml"

	var w bytes.Buffer

	path, _ := filepath.Abs("../../testdata")
	testdata, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", path, configSource))
	if err != nil {
		t.Fatalf("error finding testdata file")
	}

	conditions := GetDefaultMatchConditions(credentialsName)

	h, _ := NewThreescaleHandlerSpec(accessToken, systemURL, "")
	h.Connection.Address = "[::]:3333"

	instance := NewDefaultHybridInstance()

	rule := NewRule(conditions,
		fmt.Sprintf("%s.handler.istio-system", credentialsName),
		fmt.Sprintf("%s.instance.istio-system", credentialsName))

	cg, err := NewConfigGenerator(credentialsName, *h, *instance, rule)
	if err != nil {
		t.Errorf("unexpected error when crearting config generator")
	}

	cg.OutputAll(&w)
	if !bytes.Equal(testdata, w.Bytes()) {
		t.Fatal("Output should match integration testing test fixtures")
	}

}
