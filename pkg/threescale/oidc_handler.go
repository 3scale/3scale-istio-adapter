package threescale

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc"
)

// oauthTypeIdentifier refers to the name by which 3scale config described oauth OpenID connect authentication pattern
const openIDTypeIdentifier = "oauth"
const splitHeaderOn = "Bearer"
const defaultClaimLabel = "azp"

// OIDCHandler manages threescale services integrated with Istio using the OpenID connect authentication option.
// For backwards compatibility the code here will need to match the behaviour of APIcast.
// There are some interesting discussions around this so we need to keep in sync with changes.
// See https://github.com/3scale/APIcast/issues/988 for some discussion related to this.
// APIcast currently only supports RSA* family of signing
type OIDCHandler struct {
	context context.Context
}

// clientCredentials are user:password which can be stripped from a provided url if exist
type clientCredentials struct {
	clientId     string
	clientSecret string
}

// Constructor for OIDCHandler
// Accepts a client and if non provided will default to the http.DefaultClient
func NewOIDCHandler(client *http.Client) *OIDCHandler {
	if client == nil {
		client = http.DefaultClient
	}
	cc := oidc.ClientContext(context.TODO(), client)

	return &OIDCHandler{
		context: cc,
	}
}

// HandleIDToken is responsible for end to end handling handling and validation of a JWT passed in Authorisation header.
// It expects the headers to be passed in the format `Bearer ${TOKEN}` and will parse ${TOKEN} into raw JWT.
// After successfully parsing a JWT it will use the discovery protocol from the provided issuer which will then be used to verify the token.
// Upon successful verification, the claims are parsed for the provided JSON key to retrieve the client id.
// The specific behaviour and parameters required here are intrinsic to the incoming request and the contents of
// the configuration stored in 3scale system.
func (o *OIDCHandler) HandleIDToken(authHeader string, issuer string, claimLabel string) (string, error) {
	var clientId string
	jwt, err := splitToken(authHeader)
	if err != nil {
		return clientId, err
	}

	p, err := o.CreateProvider(issuer)
	if err != nil {
		return clientId, err
	}

	idToken, err := o.VerifyJWT(jwt, o.newDefaultConfig(), p)
	if err != nil {
		return clientId, err
	}

	if claimLabel == "" {
		claimLabel = defaultClaimLabel
	}

	return o.parseClaims(idToken, claimLabel)
}

/*
CreateProvider accepts an issuer URL as a string and strips will strip basic auth credentials from it if set.
Uses OIDC discovery protocol to generate a OIDC provider, giving us access to the underlying JWK - See https://tools.ietf.org/html/rfc7517
The function used here "NewProvider" calls "NewRemoteKeySet" internally which caches based on cache-control headers
TODO - We might want to look at extending the TTL of these keys internally going forward
*/
func (o *OIDCHandler) CreateProvider(issuerUrl string) (*oidc.Provider, error) {
	u, err := url.ParseRequestURI(issuerUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing provided url - %s", issuerUrl)
	}

	_, issuer := stripCredentials(u)
	p, err := oidc.NewProvider(o.context, issuer)
	if err != nil {
		err = fmt.Errorf("error creating OIDC provider " + err.Error())
	}
	return p, err
}

// VerifyJWT verifies the raw JWT against the public key of the provider
func (o *OIDCHandler) VerifyJWT(jwt string, config *oidc.Config, p *oidc.Provider) (*oidc.IDToken, error) {
	var idToken *oidc.IDToken
	verifier := p.Verifier(config)
	idToken, err := verifier.Verify(o.context, jwt)
	if err != nil {
		return idToken, fmt.Errorf("error verifying jwt - %s", err.Error())
	}

	return idToken, nil
}

// newDefaultConfig returns a default configuration as specified by 3scale
// 3scale only supports RS* family and checking of client_id is enforced at the backend so we
// can ignore this when verifying the jwt and client_id will be read from the claims.
func (o *OIDCHandler) newDefaultConfig() *oidc.Config {
	return &oidc.Config{
		SkipClientIDCheck: true,
		SupportedSigningAlgs: []string{
			oidc.RS256,
			oidc.RS384,
			oidc.RS512,
		},
	}
}

// parseClaims takes the id token and attempts to parse it's claims and return the value
// provided by claimLabel key, assuming it exists.
func (o *OIDCHandler) parseClaims(idToken *oidc.IDToken, claimLabel string) (string, error) {
	var clientID string
	var to map[string]interface{}

	err := idToken.Claims(&to)
	if err != nil {
		return clientID, err
	}

	id, ok := to[claimLabel]
	if !ok {
		return clientID, fmt.Errorf("provided label value for client id not present in jwt claims")
	}

	clientID, ok = id.(string)
	if !ok {
		return clientID, fmt.Errorf("error when parsing claims for label %s", claimLabel)
	}

	return clientID, nil
}

// strips basic auth from provided url and returns the credentials and stripped url as string
func stripCredentials(u *url.URL) (clientCredentials, string) {
	var cc clientCredentials
	if u.User.String() != "" {
		cc.clientId = u.User.Username()
		cc.clientSecret, _ = u.User.Password()
		stripped := strings.Replace(u.String(), u.User.String()+"@", "", -1)
		return cc, stripped
	}
	return cc, u.String()
}

// splits a raw Authorisation header based on the Bearer value
func splitToken(bearerToken string) (string, error) {
	var idToken string
	splitHeaderVal := strings.Split(bearerToken, splitHeaderOn)
	if len(splitHeaderVal) < 2 {
		return idToken, fmt.Errorf("unable to parse jwt from provided string")
	}
	return splitHeaderVal[1], nil
}
