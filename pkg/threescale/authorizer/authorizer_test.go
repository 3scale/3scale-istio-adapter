package authorizer

import (
	"fmt"
	"testing"

	"github.com/3scale/3scale-authorizer/pkg/system/v1/cache"
	"github.com/3scale/3scale-go-client/threescale"
	"github.com/3scale/3scale-porta-go-client/client"
)

func TestManager_GetSystemConfiguration(t *testing.T) {
	const systemURL = "test"
	const token = "any"
	const svcID = "any"
	const env = "test"

	var cacheKey = Manager{}.generateSystemCacheKey(systemURL, svcID)

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
	cacheKey := m.generateSystemCacheKey(systemURL, svcID)

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

type mockBuilder struct {
	withBuildSystemClientErr bool
	withSystemClient         mockSystemClient
}

func (m mockBuilder) BuildSystemClient(systemURL, accessToken string) (SystemClient, error) {
	if m.withBuildSystemClientErr {
		return mockSystemClient{}, fmt.Errorf("arbitary error")
	}
	return m.withSystemClient, nil
}

func (m mockBuilder) BuildBackendClient(backendURL string) (threescale.Client, error) {
	panic("implement me")
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
		return client.ProxyConfigElement{}, fmt.Errorf("arbitary error")
	}
	return m.withConfig, nil
}
