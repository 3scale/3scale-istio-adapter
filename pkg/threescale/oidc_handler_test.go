package threescale

import (
	"context"
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

	"github.com/coreos/go-oidc"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jwt"
)

var globalPK *rsa.PrivateKey

func init() {
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Errorf("failed to generate private key: %s", err))
	}
	globalPK = pk
}

func TestOIDCHandler_HandleIDToken(t *testing.T) {
	const listener = "127.0.0.1:8090"
	const issuer = "http://" + listener

	ts := createTestServer(listener, t)
	defer ts.Close()

	o := NewOIDCHandler(nil)

	inputs := []struct {
		name         string
		headerVal    string
		issuer       string
		claimLabel   string
		expectErr    bool
		expectResult string
	}{
		{
			name:      "Test unexpected header format",
			headerVal: "any",
			expectErr: true,
		},
		{
			name:      "Test provider unavailable",
			headerVal: "Bearer " + buildJwt(issuer, "test", time.Now().Add(time.Hour), map[string]string{"azp": "fake-client"}, jwa.RS256, t),
			issuer:    "unreachable",
			expectErr: true,
		},
		{
			name:      "Test invalid jwt",
			headerVal: "Bearer " + buildJwt(issuer, "test", time.Now().AddDate(0, 0, -1), map[string]string{"azp": "fake-client"}, jwa.RS256, t),
			issuer:    issuer,
			expectErr: true,
		},
		{
			name:       "Test parsing claim label",
			headerVal:  "Bearer " + buildJwt(issuer, "test", time.Now().Add(time.Hour), map[string]string{"azp": "fake-client"}, jwa.RS256, t),
			claimLabel: "not-azp",
			issuer:     issuer,
			expectErr:  true,
		},
		{
			name:         "Test happy path with default claim",
			headerVal:    "Bearer " + buildJwt(issuer, "test", time.Now().Add(time.Hour), map[string]string{"azp": "fake-client"}, jwa.RS256, t),
			issuer:       issuer,
			expectErr:    true,
			expectResult: "fake-client",
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			token, err := o.HandleIDToken(input.headerVal, input.issuer, input.claimLabel)
			if err != nil {
				if !input.expectErr {
					t.Errorf("unexpected error - %s", err.Error())
				}
				return
			}

			if token != input.expectResult {
				t.Errorf("unexpected client id  wanted %s but got %s", input.expectResult, token)
			}
		})
	}
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

func TestOIDCHandler_VerifyJWT(t *testing.T) {
	const listener = "127.0.0.1:8090"
	const issuer = "http://" + listener
	const invalidJWt = "oidc: malformed jwt"

	ctx := context.TODO()

	ts := createTestServer(listener, t)
	defer ts.Close()

	o := NewOIDCHandler(nil)
	p, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		t.Errorf("failed to create new provider - %s/n", err.Error())
	}

	inputs := []struct {
		name              string
		jwt               string
		c                 *oidc.Config
		expectErr         bool
		expectErrContains string
	}{
		{
			name:              "Test fail invalid jwt",
			jwt:               "",
			c:                 nil,
			expectErr:         true,
			expectErrContains: invalidJWt,
		},
		{
			name: "Test fail - aud mismatch",
			jwt:  buildJwt(issuer, "not-test", time.Now().Add(time.Hour), nil, jwa.RS256, t),
			c: &oidc.Config{
				ClientID: "test",
			},
			expectErr:         true,
			expectErrContains: `expected audience "test"`,
		},
		{
			name: "Test fail - unsupported algorithm",
			jwt:  buildJwt(issuer, "test", time.Now().Add(time.Hour), nil, jwa.HS256, t),
			c: &oidc.Config{
				ClientID:             "test",
				SupportedSigningAlgs: []string{},
			},
			expectErr:         true,
			expectErrContains: `id token signed with unsupported algorithm`,
		},
		{
			name: "Test expired token",
			jwt:  buildJwt(issuer, "test", time.Now().AddDate(0, 0, -1), nil, jwa.RS256, t),
			c: &oidc.Config{
				ClientID:             "test",
				SupportedSigningAlgs: []string{string(jwa.RS256)},
			},
			expectErr:         true,
			expectErrContains: "token is expired",
		},
		{
			name: "Test happy path",
			jwt:  buildJwt(issuer, "test", time.Now().Add(time.Hour), nil, jwa.RS256, t),
			c:    o.newDefaultConfig(),
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			_, err = o.VerifyJWT(input.jwt, input.c, p)
			if input.expectErr {
				if err == nil {
					t.Error("expected validation to fail but error is nil")
				}

				if !strings.Contains(err.Error(), input.expectErrContains) {
					t.Errorf("unexpected error returned - got %s\n", err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("failed to validate jwt - %s", err.Error())
			}
		})
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

func buildJwt(issuer string, aud string, expiresAt time.Time, opts map[string]string, alg jwa.SignatureAlgorithm, t *testing.T) string {
	t.Helper()
	const secret = "12345"

	token := jwt.New()
	token.Set(jwt.IssuerKey, issuer)
	token.Set(jwt.IssuedAtKey, time.Now().Unix())
	token.Set(jwt.ExpirationKey, expiresAt.Unix())
	token.Set(jwt.SubjectKey, "f9651481-bbbb-4710-9722-0f81c5803c6d")
	token.Set(jwt.AudienceKey, aud)

	for k, v := range opts {
		token.Set(k, v)
	}

	var b []byte
	var err error
	if alg == jwa.HS256 {
		b, err = token.Sign(jwa.HS256, []byte(secret))
	} else {
		b, err = token.Sign(jwa.RS256, globalPK)
	}

	if err != nil {
		t.Fatalf("Could not sign token - %v", err)
	}

	return string(b)
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
