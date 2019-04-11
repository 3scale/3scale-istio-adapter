package threescale

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/jwk"
)

var globalPK *rsa.PrivateKey

func init() {
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Errorf("failed to generate private key: %s", err))
	}
	globalPK = pk
}

func TestOIDCHandler_CreateProvider(t *testing.T) {
	const listener = "127.0.0.1:8090"
	const issuer = "http://" + listener

	ts := createTestServer(listener, t)
	o := NewOIDCHandler(nil)

	// create provider aainst the test server and expect it to work without error and provider not be nil
	p, err := o.CreateProvider(issuer)
	if err != nil || p == nil {
		t.Errorf("unexpected error when creating provider - %v", err)
	}
	ts.Close()

	//retry after closing the server
	o = NewOIDCHandler(&http.Client{
		Timeout: time.Millisecond,
	})
	p, err = o.CreateProvider(issuer)
	if err == nil || p != nil {
		t.Errorf("expected error when creating provider - %v", err)
	}

	//create with invalid url
	_, err = o.CreateProvider("~invalid")
	if err == nil {
		t.Errorf("expected error when creating provide with invalid url - %v", err)
	}

}

// This is testing an internal function and can potentially be integrated into the CreateProviderTest
func TestStripCredentials(t *testing.T) {
	inputs := []struct {
		name   string
		in     string
		expect string
	}{
		{
			name:   "Test removing basic auth",
			in:     "https://fake:user@somewhere.com",
			expect: "https://somewhere.com",
		},
		{
			name:   "Test removing basic auth with path",
			in:     "https://fake:user@somewhere.com/with/some/path",
			expect: "https://somewhere.com/with/some/path",
		},
		{
			name:   "Test removing basic auth with and query",
			in:     "https://fake:user@somewhere.com/with/some/path?test=yes",
			expect: "https://somewhere.com/with/some/path?test=yes",
		},
		{
			name:   "Test removing basic auth no password",
			in:     "https://fake:@somewhere.com",
			expect: "https://somewhere.com",
		},
		{
			name:   "Test removing basic auth no user",
			in:     "https://:user@somewhere.com",
			expect: "https://somewhere.com",
		},
		{
			name:   "Test no basic auth behaves correctly",
			in:     "https://somewhere.com",
			expect: "https://somewhere.com",
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			u, err := url.Parse(input.in)
			if err != nil {
				t.Error("invalid input provided")
			}

			_, s := stripCredentials(u)
			if s != input.expect {
				t.Errorf("parsed result does not match expected output. Got %s but want %s\n", s, input.expect)
			}
		})

	}
}

func getDiscoveryResponse(providerUrl string) []byte {
	return []byte(fmt.Sprintf(`{  
   "issuer":"%s",
   "authorization_endpoint":"%s/auth/realms/3scale-keycloak/protocol/openid-connect/auth",
   "token_endpoint":"%s/auth/realms/3scale-keycloak/protocol/openid-connect/token",
   "userinfo_endpoint":"%s/auth/realms/3scale-keycloak/protocol/openid-connect/userinfo",
   "jwks_uri":"%s/auth/realms/3scale-keycloak/protocol/openid-connect/certs",
   "grant_types_supported":[  
      "authorization_code",
      "implicit",
      "refresh_token",
      "password",
      "client_credentials"
   ],
   "response_types_supported":[  
      "code",
      "none",
      "id_token",
      "token",
      "id_token token",
      "code id_token",
      "code token",
      "code id_token token"
   ],
   "subject_types_supported":[  
      "public",
      "pairwise"
   ],
   "id_token_signing_alg_values_supported":[  
      "ES384",
      "RS384",
      "HS256",
      "HS512",
      "ES256",
      "RS256",
      "HS384",
      "ES512",
      "RS512"
   ],
   "userinfo_signing_alg_values_supported":[  
      "ES384",
      "RS384",
      "HS256",
      "HS512",
      "ES256",
      "RS256",
      "HS384",
      "ES512",
      "RS512",
      "none"
   ],
   "request_object_signing_alg_values_supported":["none","RS256"],
   "response_modes_supported":["query"],
   "registration_endpoint":"http://keycloak-keycloak.34.242.107.254.nip.io/auth/realms/3scale-keycloak/clients-registrations/openid-connect",
   "token_endpoint_auth_methods_supported":[  
      "private_key_jwt",
      "client_secret_basic",
      "client_secret_post",
      "client_secret_jwt"
   ],
   "token_endpoint_auth_signing_alg_values_supported":["RS256","HS256"],
   "claims_supported":[  
      "sub",
      "iss",
      "name"
   ],
   "claim_types_supported":["normal"],
   "claims_parameter_supported":false,
   "scopes_supported":["openid"],
   "request_parameter_supported":true,
   "request_uri_parameter_supported":true,
   "code_challenge_methods_supported":["plain","S256"]
}`, providerUrl, providerUrl, providerUrl, providerUrl, providerUrl))
}

func buildJwk(t *testing.T) []byte {
	t.Helper()

	key, err := jwk.New(&globalPK.PublicKey)
	if err != nil {
		t.Fatalf("failed to create JWK: %s", err)
	}

	key.Set(jwk.KeyUsageKey, "enc")
	key.Set(jwk.KeyIDKey, "enc1")

	b, err := json.MarshalIndent(key, "", "  ")
	if err != nil {
		t.Fatalf("failed to generate JSON: %s", err)
	}

	str := fmt.Sprintf(`{"keys":[%s]}`, string(b))
	b = []byte(str)

	return b
}

// creates and starts a test server which listens on provided ip:port
// this acts as a fake OIDC provider. This starts the server and the caller is responsible for closing
// the connection when done.
func createTestServer(listenOn string, t *testing.T) *httptest.Server {
	t.Helper()
	issuer := fmt.Sprintf("http://%s", listenOn)
	l, err := net.Listen("tcp", listenOn)
	if err != nil {
		t.Fatalf("error listening on port for test data")
	}

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "auth/realms/3scale-keycloak/protocol/openid-connect/certs") {
			w.Write(buildJwk(t))
		} else {
			w.Write(getDiscoveryResponse(issuer))
		}
	}))
	ts.Listener = l
	ts.Start()
	return ts
}
