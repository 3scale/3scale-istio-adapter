package templating

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
)

const configSource = "threescale-adapter-config.yaml"
const testUid = "123456"
const testListenAddr = "[::]"
const host = "127.0.0.1"

var defaultTestFixture = ConfigGenerator{
	uid:  testUid,
	host: host,
	Handler: Handler{
		AccessToken:   "secret-token",
		SystemURL:     "http://" + host + ":8090",
		ListenAddress: testListenAddr,
		ServiceID:     testUid,
	},
	Rule: Rule{
		Conditions: MatchConditions{
			`destination.labels["service-mesh.3scale.net"] == "true"`,
			fmt.Sprintf(`destination.labels["service-mesh.3scale.net/uid"] == "%s"`, testUid),
		},
	},
	Instance: GetDefaultInstance(),
}

func TestNewConfigGenerator(t *testing.T) {
	// expect failure no uid provided and fixup set to false
	_, errs := NewConfigGenerator(Handler{}, Instance{}, "", false)
	if len(errs) < 1 {
		t.Fatalf("expected validation to fail - empty uid")
	}

	validHandler := Handler{
		ServiceID: "any",
		SystemURL: "http://valid.com",
	}
	// expect success, no uid provided but asked for fixup
	_, errs = NewConfigGenerator(validHandler, Instance{}, "", true)
	if len(errs) > 0 {
		var errString string
		for _, e := range errs {
			errString = errString + " " + e.Error()
		}
		t.Fatalf("unexpected errors - %s", errString)
	}

	// expect a panic when the user provides an invalid input
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected validation to fail - invalid auth method")
		}
	}()
	NewConfigGenerator(validHandler, Instance{AuthnMethod: AuthenticationMethod(5)}, "", true)
}

func TestNewHandler(t *testing.T) {
	inputs := []struct {
		name       string
		token      string
		svcID      string
		url        string
		fixup      bool
		expectErrs bool
	}{
		{
			name:       "Test expect un-fixable errors - no svc id",
			token:      "12345",
			url:        "https://test.com",
			fixup:      true,
			expectErrs: true,
		},
		{
			name:       "Test expect un-fixable errors -  no url",
			token:      "1234",
			svcID:      "12345",
			fixup:      true,
			expectErrs: true,
		},
		{
			name:       "Test expect un-fixable errors -  no token",
			svcID:      "12345",
			url:        "https://test.com",
			fixup:      true,
			expectErrs: true,
		},
		{
			name:       "Test expect fixable errors -  invalid service id. no fix",
			token:      "12345",
			svcID:      "--",
			url:        "https://test.com",
			fixup:      false,
			expectErrs: true,
		},
		{
			name:       "Test expect fixable errors -  invalid service id. fix",
			token:      "12345",
			svcID:      "--",
			url:        "https://test.com",
			fixup:      true,
			expectErrs: false,
		},
	}
	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			_, errs := NewHandler(input.token, input.url, input.svcID, input.fixup)
			if input.expectErrs {
				if len(errs) < 1 {
					t.Errorf("expected errors but got none")
				}
				return
			} else {
				if len(errs) > 0 {
					t.Errorf("uneexpected errors. expected 0 but got %d", len(errs))
				}
			}
		})
	}
}

func TestOutputHandler(t *testing.T) {
	expect := fmt.Sprintf(`# handler for adapter threescale
apiVersion: "config.istio.io/v1alpha2"
kind: handler
metadata:
  name: threescale-%s
  namespace: istio-system
  labels:
    "service-mesh.3scale.net/host": "%s"
    "service-mesh.3scale.net/service-id": "%s"
spec:
  adapter: threescale
  params:
    service_id: "123456"
    system_url: "http://127.0.0.1:8090"
    access_token: "secret-token"
  connection:
    address: "[::]:3333"`, testUid, host, testUid)

	b := newTestBuffer(t)
	err := defaultTestFixture.OutputHandler(b)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if b.String() != expect {
		t.Fatalf("unexpected template output for handler ")
	}
}

func TestOutputInstance(t *testing.T) {
	userKeyFixtureQueryCopy := defaultTestFixture
	userKeyFixtureQueryCopy.Instance = Instance{
		CredentialsLocation: QueryParams,
		ApiKeyLabel:         "test",
		AuthnMethod:         1,
	}

	userKeyFixtureHeaderCopy := defaultTestFixture
	userKeyFixtureHeaderCopy.Instance = Instance{
		CredentialsLocation: Headers,
		ApiKeyLabel:         "test",
		AuthnMethod:         1,
	}

	inputs := []struct {
		name    string
		fixture ConfigGenerator
		expect  string
	}{
		{
			name:    "Test Hybrid Instance Output",
			fixture: defaultTestFixture,
			expect: fmt.Sprintf(`# instance for template authorization
apiVersion: "config.istio.io/v1alpha2"
kind: instance
metadata:
  name: threescale-authorization-%s
  namespace: istio-system
  labels:
    "service-mesh.3scale.net/host": "%s"
    "service-mesh.3scale.net/service-id": "%s"
spec:
  template: threescale-authorization
  params:
    subject:
      user: request.query_params["user_key"] | request.headers["User-Key"] | ""
      properties:
        app_id: request.query_params["app_id"] | request.headers["App-Id"] | ""
        app_key: request.query_params["app_key"] | request.headers["App-Key"] | ""
    action:
      path: request.url_path
      method: request.method | "get"`, testUid, host, testUid),
		},
		{
			name: "Test User Key Query Pattern Output",
			fixture: instanceModifier(t, Instance{
				CredentialsLocation: QueryParams,
				ApiKeyLabel:         "test",
				AuthnMethod:         1,
			}),
			expect: fmt.Sprintf(`# instance for template authorization
apiVersion: "config.istio.io/v1alpha2"
kind: instance
metadata:
  name: threescale-authorization-%s
  namespace: istio-system
  labels:
    "service-mesh.3scale.net/host": "%s"
    "service-mesh.3scale.net/service-id": "%s"
spec:
  template: threescale-authorization
  params:
    subject:
      user: request.query_params["test"] | ""
    action:
      path: request.url_path
      method: request.method | "get"`, testUid, host, testUid),
		},
		{
			name: "Test User Key Header Pattern Output",
			fixture: instanceModifier(t, Instance{
				CredentialsLocation: Headers,
				ApiKeyLabel:         "x-test",
				AuthnMethod:         1,
			}),
			expect: fmt.Sprintf(`# instance for template authorization
apiVersion: "config.istio.io/v1alpha2"
kind: instance
metadata:
  name: threescale-authorization-%s
  namespace: istio-system
  labels:
    "service-mesh.3scale.net/host": "%s"
    "service-mesh.3scale.net/service-id": "%s"
spec:
  template: threescale-authorization
  params:
    subject:
      user: request.headers["X-Test"] | ""
    action:
      path: request.url_path
      method: request.method | "get"`, testUid, host, testUid),
		},
		{
			name: "Test App ID Query Pattern Output",
			fixture: instanceModifier(t, Instance{
				CredentialsLocation: QueryParams,
				AppIDLabel:          "test_id",
				AppKeyLabel:         "test_key",
				AuthnMethod:         2,
			}),
			expect: fmt.Sprintf(`# instance for template authorization
apiVersion: "config.istio.io/v1alpha2"
kind: instance
metadata:
  name: threescale-authorization-%s
  namespace: istio-system
  labels:
    "service-mesh.3scale.net/host": "%s"
    "service-mesh.3scale.net/service-id": "%s"
spec:
  template: threescale-authorization
  params:
    subject:
      properties:
        app_id: request.query_params["test_id"] | ""
        app_key: request.query_params["test_key"] | ""
    action:
      path: request.url_path
      method: request.method | "get"`, testUid, host, testUid),
		},
		{
			name: "Test App ID Header Pattern Output",
			fixture: instanceModifier(t, Instance{
				CredentialsLocation: Headers,
				AppIDLabel:          "x-test-id",
				AppKeyLabel:         "x-test-key",
				AuthnMethod:         2,
			}),
			expect: fmt.Sprintf(`# instance for template authorization
apiVersion: "config.istio.io/v1alpha2"
kind: instance
metadata:
  name: threescale-authorization-%s
  namespace: istio-system
  labels:
    "service-mesh.3scale.net/host": "%s"
    "service-mesh.3scale.net/service-id": "%s"
spec:
  template: threescale-authorization
  params:
    subject:
      properties:
        app_id: request.headers["X-Test-Id"] | ""
        app_key: request.headers["X-Test-Key"] | ""
    action:
      path: request.url_path
      method: request.method | "get"`, testUid, host, testUid),
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			b := newTestBuffer(t)
			err := input.fixture.OutputInstance(b)
			if err != nil {
				t.Errorf(err.Error())
			}

			if b.String() != input.expect {
				fmt.Println(b.String())
				fmt.Println()
				fmt.Println(input.expect)
				t.Fatalf("unexpected template output for instance")
			}
		})
	}
}

func TestOutputRule(t *testing.T) {
	expect := fmt.Sprintf(`# rule to dispatch to handler threescale.handler
apiVersion: "config.istio.io/v1alpha2"
kind: rule
metadata:
  name: threescale-%s
  namespace: istio-system
  labels:
    "service-mesh.3scale.net/host": "%s"
    "service-mesh.3scale.net/service-id": "%s"
spec:
  match: destination.labels["service-mesh.3scale.net"] == "true" && destination.labels["service-mesh.3scale.net/uid"] == "%s"
  actions:
  - handler: threescale-%s.handler.istio-system
    instances:
    - threescale-authorization-%s`, testUid, host, testUid, testUid, testUid, testUid)

	b := newTestBuffer(t)
	err := defaultTestFixture.OutputRule(b)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if b.String() != expect {
		t.Fatalf("unexpected template output for handler ")
	}
}

func TestOutputAll(t *testing.T) {
	b := newTestBuffer(t)
	defaultTestFixture.OutputAll(b)
	path, _ := filepath.Abs("../../testdata")
	testdata, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", path, configSource))
	if err != nil {
		t.Fatalf("error finding testdata file")
	}

	if !bytes.Equal(testdata, b.Bytes()) {
		t.Fatal("Output should match integration testing test fixtures")
	}
}

func TestOutputUID(t *testing.T) {
	b := newTestBuffer(t)
	err := defaultTestFixture.OutputUID(b)
	if err != nil {
		t.Errorf("unexpected error printing UID")
	}
	if b.String() != fmt.Sprintf("# The UID for this service is %s\n", defaultTestFixture.GetUID()) {
		t.Errorf("unexpected UID output")
	}

}

func TestPopulateDefaultRules(t *testing.T) {
	copy := defaultTestFixture
	rules := copy.GetDefaultMatchConditions()
	if len(rules) != 2 {
		t.Errorf("expected two rules")
	}

	expect := `destination.labels["service-mesh.3scale.net"] == "true" && destination.labels["service-mesh.3scale.net/uid"] == "123456"`
	if copy.Rule.ConditionsToMatchString() != expect {
		t.Errorf("unexpected rule condition, expected %s, got %s", expect, copy.Rule.ConditionsToMatchString())
	}

}

func TestUidGenerator(t *testing.T) {
	const serviceID = "12345"
	inputs := []struct {
		name      string
		url       string
		expect    string
		expectErr bool
	}{
		{
			name:   "Test simple input",
			url:    "https://test.com",
			expect: "test.com-12345",
		},
		{
			name:   "Test port colon replaced",
			url:    "https://test.com:443",
			expect: "test.com:443-12345",
		},
		{
			name:   "Test credentials stripped",
			url:    "https://secret:password@test.com:443",
			expect: "test.com:443-12345",
		},
		{
			name:   "Test path converted",
			url:    "https://test.com/example",
			expect: "test.com-/example-12345",
		},
		{
			name:   "Test query params ignored",
			url:    "https://test.com/example?fake=param&atest=query",
			expect: "test.com-/example-12345",
		},
		{
			name:      "Test fail url parse",
			url:       "httpf:///.invalid.example",
			expectErr: true,
		},
		{
			name:      "Test fail empty",
			url:       "",
			expectErr: true,
		},
	}
	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			u, _ := ParseURL(input.url)
			if u == nil && !input.expectErr {
				t.Errorf("expected to fail on url parsing")
			}

			uid, err := UidGenerator(u, serviceID)
			if err != nil && !input.expectErr {
				t.Errorf(err.Error())
				return
			} else if err == nil && input.expectErr {
				t.Errorf("expected error")
			}

			if uid != input.expect {
				t.Errorf("unexpected result, expected: %s \n got: %s", input.expect, uid)
			}
		})

	}
}

func newTestBuffer(t *testing.T) *bytes.Buffer {
	t.Helper()
	var w bytes.Buffer
	return &w

}

func instanceModifier(t *testing.T, i Instance) ConfigGenerator {
	t.Helper()
	copy := defaultTestFixture
	copy.Instance = i
	return copy
}
