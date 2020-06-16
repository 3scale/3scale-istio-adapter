package authorizer

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/3scale/3scale-authorizer/pkg/system/v1/cache"
	"github.com/3scale/3scale-go-client/threescale"
	"github.com/3scale/3scale-go-client/threescale/api"
	http2 "github.com/3scale/3scale-go-client/threescale/http"
	"github.com/3scale/3scale-porta-go-client/client"
)

func TestNewManager(t *testing.T) {
	manager, err := NewManager(nil, nil, BackendConfig{})
	if err == nil {
		t.Errorf("expected error as no builder provided")
	}

	manager, err = NewManager(
		NewClientBuilder(http.DefaultClient),
		NewSystemCache(SystemCacheConfig{}, nil),
		BackendConfig{},
	)
	if err != nil {
		t.Error("unexpected error")
	}

	if manager.systemCache.TTL.Seconds() != cache.DefaultCacheTTL.Seconds() {
		t.Error("unexpected defaults set")
	}
}

func TestManager_GetSystemConfiguration(t *testing.T) {
	const systemURL = "test"
	const token = "any"
	const svcID = "any"
	const env = "test"

	var cacheKey = generateSystemCacheKey(systemURL, svcID)

	validRequest := SystemRequest{
		AccessToken: token,
		ServiceID:   svcID,
		Environment: env,
	}

	inputs := []struct {
		name    string
		request SystemRequest
		builder Builder
		/// where tests use a cache we inject a real cache which acts as a form of integration testing here
		cache     SystemCache
		setup     func(configurationCache cache.ConfigurationCache)
		expectErr bool
		inspect   func(returnedConfig client.ProxyConfig, configurationCache cache.ConfigurationCache, t *testing.T)
	}{
		{
			name: "Test expect fail empty access token",
			request: SystemRequest{
				ServiceID:   svcID,
				Environment: env,
			},
			expectErr: true,
		},
		{
			name: "Test expect fail empty service",
			request: SystemRequest{
				AccessToken: token,
				Environment: env,
			},
			expectErr: true,
		},
		{
			name: "Test expect fail empty environment",
			request: SystemRequest{
				AccessToken: token,
				ServiceID:   svcID,
			},
			expectErr: true,
		},
		{
			name:      "Test no cache and failed to build client returns error",
			request:   validRequest,
			builder:   newMockBuilderWithSystemClientBuildError(t),
			expectErr: true,
		},
		{
			name:    "Test cache miss and failed to build client returns error",
			request: validRequest,
			builder: newMockBuilderWithSystemClientBuildError(t),
			cache: SystemCache{
				cache: cache.NewDefaultConfigCache(),
			},
			expectErr: true,
		},
		{
			name:    "Test no cache and failed remote call",
			request: validRequest,
			builder: mockBuilder{
				withBuildSystemClientErr: false,
				withSystemClient: mockSystemClient{
					withErr:    true,
					withConfig: client.ProxyConfigElement{},
				},
			},
			expectErr: true,
		},
		{
			name:    "Test cache miss and failed remote call",
			request: validRequest,
			builder: mockBuilder{
				withBuildSystemClientErr: false,
				withSystemClient: mockSystemClient{
					withErr:    true,
					withConfig: client.ProxyConfigElement{},
				},
			},
			cache: SystemCache{
				cache: cache.NewDefaultConfigCache(),
			},
			expectErr: true,
		},
		{
			name:    "Test no cache and success remote call",
			request: validRequest,
			builder: mockBuilder{
				withBuildSystemClientErr: false,
				withSystemClient: mockSystemClient{
					withConfig: client.ProxyConfigElement{},
				},
			},
			expectErr: false,
		},
		{
			name:    "Test cache miss and success remote call",
			request: validRequest,
			builder: mockBuilder{
				withBuildSystemClientErr: false,
				withSystemClient: mockSystemClient{
					withErr: false,
					withConfig: client.ProxyConfigElement{
						ProxyConfig: client.ProxyConfig{
							Environment: "shouldBeCached",
							Content:     client.Content{},
						},
					},
				},
			},
			cache: SystemCache{
				cache: cache.NewDefaultConfigCache(),
			},
			expectErr: false,
			inspect: func(returnedConfig client.ProxyConfig, configurationCache cache.ConfigurationCache, t *testing.T) {
				hasVal, ok := configurationCache.Get(cacheKey)
				if !ok {
					t.Errorf("expected provided cache key to be present")
				}

				if hasVal.Item.Environment != "shouldBeCached" {
					t.Errorf("cached value not set as expected")
				}
			},
		},
		{
			name:    "Test cache hit and success",
			request: validRequest,
			builder: mockBuilder{
				withBuildSystemClientErr: false,
				withSystemClient: mockSystemClient{
					withErr: false,
					withConfig: client.ProxyConfigElement{
						ProxyConfig: client.ProxyConfig{
							Environment: "shouldBeFetchedFromCached",
							Content:     client.Content{},
						},
					},
				},
			},
			cache: SystemCache{
				cache: cache.NewDefaultConfigCache(),
			},
			setup: func(configurationCache cache.ConfigurationCache) {

				value := cache.Value{
					Item: client.ProxyConfig{
						Environment: "shouldBeFetchedFromCached",
					},
				}
				configurationCache.Set(cacheKey, value)
			},
			expectErr: false,
			inspect: func(returnedConfig client.ProxyConfig, configurationCache cache.ConfigurationCache, t *testing.T) {
				hasVal, ok := configurationCache.Get(cacheKey)
				if !ok {
					t.Errorf("expected provided cache key to be present")
				}

				if hasVal.Item.Environment != "shouldBeFetchedFromCached" {
					t.Errorf("cached value not set as expected")
				}
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			m := Manager{
				clientBuilder: input.builder,
				systemCache:   input.cache,
			}

			if input.setup != nil {
				input.setup(input.cache.cache)
			}

			config, err := m.GetSystemConfiguration(systemURL, input.request)
			if err != nil {
				if !input.expectErr {
					t.Errorf("unexpected err %v", err)
				}
				return
			}

			if input.inspect != nil {
				input.inspect(config, input.cache.cache, t)
			}

		})

	}
}

// This tests some internal behaviour but since it is critical it warrants its own test
func TestManager_CacheRefreshCallback(t *testing.T) {
	const systemURL = "test"
	const token = "any"
	const svcID = "any"
	const env = "test"

	m := Manager{}
	cacheKey := generateSystemCacheKey(systemURL, svcID)

	sc := SystemCache{cache: cache.NewDefaultConfigCache()}
	value := cache.Value{
		Item: client.ProxyConfig{
			Environment: "cached",
		},
	}

	validRequest := SystemRequest{
		AccessToken: token,
		ServiceID:   svcID,
		Environment: env,
	}

	var refreshedConf client.ProxyConfig
	var err error
	// wrap the callback we know we use internally to provide a hook into whats returned
	refreshWith := func() (client.ProxyConfig, error) {
		refreshedConf, err = m.refreshCallback(systemURL, validRequest, 1)()
		return refreshedConf, err
	}

	value.SetRefreshCallback(refreshWith)
	sc.cache.Set(cacheKey, value)

	// inject a builder that fails when the client should be built and expect it to error
	m.clientBuilder = newMockBuilderWithSystemClientBuildError(t)
	sc.cache.Refresh()

	if err != nil {
		fmt.Errorf("expected an error to have been returned")
	}

	// inject a builder that fails to make a remote connection and expect it to error
	m.clientBuilder = mockBuilder{
		withBuildSystemClientErr: false,
		withSystemClient: mockSystemClient{
			withErr: true,
		},
	}
	sc.cache.Refresh()
	if err != nil {
		fmt.Errorf("expected an error to have been returned")
	}

	// inject a builder that makes a remote connection and expect no error
	m.clientBuilder = mockBuilder{
		withBuildSystemClientErr: false,
		withSystemClient: mockSystemClient{
			withErr: false,
		},
	}
	sc.cache.Refresh()
	if err == nil {
		fmt.Errorf("unexpected error")
	}

}

func TestManager_AuthRep(t *testing.T) {
	inputs := []struct {
		name             string
		url              string
		request          BackendRequest
		builder          Builder
		expectErr        bool
		expectAuthorized bool
	}{
		{
			name: "Test expect fail when fail to build a client",
			// we know we cannot build a client if we dont provide a valid URL
			builder:   NewClientBuilder(http.DefaultClient),
			expectErr: true,
		},
		{
			name: "Test expect fail when a bad transaction is provided",
			url:  "https://somewhere-valid.com",
			// we can build a client here but expect not to use it
			builder: NewClientBuilder(http.DefaultClient),
			request: BackendRequest{
				Auth: BackendAuth{
					Type:  "any",
					Value: "any",
				},
				Service:      "any",
				Transactions: nil,
			},
			expectErr: true,
		},
		{
			name: "Test expect error when the client that gets built throws an error",
			builder: mockBuilder{
				withBackendClient: mockBackendClient{
					withAuthRepErr: true,
				},
			},
			request: BackendRequest{
				Auth: BackendAuth{
					Type:  "any",
					Value: "any",
				},
				Service: "any",
				Transactions: []BackendTransaction{
					{
						Metrics: map[string]int{"hits": 5},
						Params: BackendParams{
							AppID:  "yes",
							AppKey: "no",
							UserID: "maybe",
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "Test expect end-to-end success and failed auth",
			builder: mockBuilder{
				withBackendClient: mockBackendClient{
					withAuthRepErr: false,
					withAuthResponse: &threescale.AuthorizeResult{
						Authorized:          false,
						ErrorCode:           "some_error",
						AuthorizeExtensions: threescale.AuthorizeExtensions{},
					},
				},
			},
			request: BackendRequest{
				Auth: BackendAuth{
					Type:  "any",
					Value: "any",
				},
				Service: "any",
				Transactions: []BackendTransaction{
					{
						Metrics: map[string]int{"hits": 5},
						Params: BackendParams{
							AppID:  "yes",
							AppKey: "no",
							UserID: "maybe",
						},
					},
				},
			},
			expectErr:        false,
			expectAuthorized: false,
		},
		{
			name: "Test expect end-to-end success",
			builder: mockBuilder{
				withBackendClient: mockBackendClient{
					withAuthRepErr: false,
					withAuthResponse: &threescale.AuthorizeResult{
						Authorized: true,
					},
				},
			},
			request: BackendRequest{
				Auth: BackendAuth{
					Type:  "any",
					Value: "any",
				},
				Service: "any",
				Transactions: []BackendTransaction{
					{
						Metrics: map[string]int{"hits": 5},
						Params: BackendParams{
							AppID:  "yes",
							AppKey: "no",
							UserID: "maybe",
						},
					},
				},
			},
			expectAuthorized: true,
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			m := Manager{
				clientBuilder: input.builder,
			}

			response, err := m.AuthRep(input.url, input.request)
			if err != nil {
				if !input.expectErr {
					t.Errorf("unexpected error %v", err)
				}
				return
			}

			if response.Authorized != input.expectAuthorized {
				t.Errorf("unexpected auth result")
			}
		})

	}
}

func TestBackendRequest_ToAPIRequest(t *testing.T) {
	badRequestWithNilTransaction := BackendRequest{
		Auth: BackendAuth{
			Type:  "any",
			Value: "any",
		},
		Service:      "any",
		Transactions: nil,
	}

	_, err := badRequestWithNilTransaction.ToAPIRequest()
	if err == nil {
		t.Errorf("expected an error due to nil transaction")
	}

	badRequestWithEmptyTransaction := BackendRequest{
		Auth: BackendAuth{
			Type:  "any",
			Value: "any",
		},
		Service:      "any",
		Transactions: []BackendTransaction{},
	}
	_, err = badRequestWithEmptyTransaction.ToAPIRequest()
	if err == nil {
		t.Errorf("expected an error due to empty transactions")
	}

	validRequest := BackendRequest{
		Auth: BackendAuth{
			Type:  "any",
			Value: "any",
		},
		Service: "any",
		Transactions: []BackendTransaction{
			{
				Metrics: map[string]int{"hits": 5},
				Params: BackendParams{
					AppID:  "yes",
					AppKey: "no",
					UserID: "maybe",
				},
			},
		},
	}

	apiReq, err := validRequest.ToAPIRequest()
	if err != nil {
		t.Errorf("unexpected error when transforming request")
	}

	expect := &threescale.Request{
		Auth: api.ClientAuth{
			Type:  api.AuthType("any"),
			Value: "any",
		},
		Extensions: api.Extensions{
			http2.RejectionReasonHeaderExtension: "1",
		},
		Service: "any",
		Transactions: []api.Transaction{
			{
				Metrics: api.Metrics{"hits": 5},
				Params: api.Params{
					AppID:    "yes",
					AppKey:   "no",
					Referrer: "",
					UserID:   "maybe",
					UserKey:  "",
				},
			},
		},
	}

	if !reflect.DeepEqual(expect, apiReq) {
		t.Errorf("expected pointer values to be deeply equal")
	}
}

type mockBuilder struct {
	withBuildSystemClientErr bool
	withSystemClient         mockSystemClient
	withBackendClient        mockBackendClient
}

func (m mockBuilder) BuildSystemClient(systemURL, accessToken string) (SystemClient, error) {
	if m.withBuildSystemClientErr {
		return mockSystemClient{}, fmt.Errorf("arbitary error")
	}
	return m.withSystemClient, nil
}

func (m mockBuilder) BuildBackendClient(backendURL string) (threescale.Client, error) {
	return m.withBackendClient, nil
}

func newMockBuilderWithSystemClientBuildError(t *testing.T) mockBuilder {
	t.Helper()
	return mockBuilder{withBuildSystemClientErr: true}
}

type mockSystemClient struct {
	withErr    bool
	withConfig client.ProxyConfigElement
}

func (m mockSystemClient) GetLatestProxyConfig(serviceID, environment string) (client.ProxyConfigElement, error) {
	if m.withErr {
		return client.ProxyConfigElement{}, fmt.Errorf("arbitrary error")
	}
	return m.withConfig, nil
}

type mockBackendClient struct {
	withAuthRepErr   bool
	withAuthResponse *threescale.AuthorizeResult
}

func (mbc mockBackendClient) Authorize(request threescale.Request) (*threescale.AuthorizeResult, error) {
	panic("implement me")
}

func (mbc mockBackendClient) AuthRep(request threescale.Request) (*threescale.AuthorizeResult, error) {
	if mbc.withAuthRepErr {
		return nil, fmt.Errorf("arbitrary error")
	}
	return mbc.withAuthResponse, nil
}

func (mockBackendClient) Report(request threescale.Request) (*threescale.ReportResult, error) {
	panic("implement me")
}

func (mockBackendClient) GetPeer() string {
	panic("implement me")
}
