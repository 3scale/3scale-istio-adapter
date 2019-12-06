package authorizer

import (
	"fmt"
	"time"

	"github.com/3scale/3scale-authorizer/pkg/system/v1/cache"
	"github.com/3scale/3scale-porta-go-client/client"
)

// Manager manages connections and interactions between the adapter and 3scale (system and backend)
// Supports managing interactions between multiple hosts and can optionally leverage available caching implementations
// Capable of Authorizing a request to 3scale and providing the required functionality to pull from the sources to do so
type Manager struct {
	clientBuilder Builder
	systemCache   SystemCache
}

// SystemCache wraps the caching implementation and its configuration for 3scale system
type SystemCache struct {
	cache  cache.ConfigurationCache
	config SystemCacheConfig
}

// SystemCacheConfig holds the configuration for the cache
type SystemCacheConfig struct {
	numRetryFailedRefresh int
	refreshInterval       time.Duration
	ttlSeconds            time.Duration
}

// SystemRequest provides the required input to request the latest configuration from 3scale system
type SystemRequest struct {
	AccessToken string
	ServiceID   string
	Environment string
}

// GetSystemConfiguration returns the configuration from 3scale system which can be used to fulfill and Auth request
func (m Manager) GetSystemConfiguration(systemURL string, request SystemRequest) (client.ProxyConfig, error) {
	var config client.ProxyConfig
	var err error

	if err = m.validateSystemRequest(request); err != nil {
		return config, err
	}

	if m.systemCache.cache != nil {
		config, err = m.fetchSystemConfigFromCache(systemURL, request)

	} else {
		config, err = m.fetchSystemConfigRemotely(systemURL, request)
	}

	if err != nil {
		return config, fmt.Errorf("cannot get 3scale system config - %s", err.Error())
	}

	return config, nil
}

// validateSystemRequest to avoid wasting compute time on invalid request
func (m Manager) validateSystemRequest(request SystemRequest) error {
	if request.Environment == "" || request.ServiceID == "" || request.AccessToken == "" {
		return fmt.Errorf("invalid arguements provided")
	}
	return nil
}

func (m Manager) generateSystemCacheKey(systemURL, svcID string) string {
	return fmt.Sprintf("%s_%s", systemURL, svcID)
}

func (m Manager) fetchSystemConfigFromCache(systemURL string, request SystemRequest) (client.ProxyConfig, error) {
	var config client.ProxyConfig
	var err error

	cacheKey := m.generateSystemCacheKey(systemURL, request.ServiceID)
	cachedValue, found := m.systemCache.cache.Get(cacheKey)
	if !found {
		config, err = m.fetchSystemConfigRemotely(systemURL, request)
		if err != nil {
			return config, err
		}

		itemToCache := &cache.Value{Item: config}
		itemToCache = m.setValueFromConfig(systemURL, request, itemToCache)
		m.systemCache.cache.Set(cacheKey, *itemToCache)

	} else {
		config = cachedValue.Item
	}

	return config, err
}

func (m Manager) fetchSystemConfigRemotely(systemURL string, request SystemRequest) (client.ProxyConfig, error) {
	var config client.ProxyConfig

	systemClient, err := m.clientBuilder.BuildSystemClient(systemURL, request.AccessToken)
	if err != nil {
		return config, fmt.Errorf("unable to build system client for %s - %s", systemURL, err.Error())
	}

	proxyConfElement, err := systemClient.GetLatestProxyConfig(request.ServiceID, request.Environment)
	if err != nil {
		return config, fmt.Errorf("unable to fetch required data from 3scale system - %s", err.Error())
	}

	return proxyConfElement.ProxyConfig, nil
}

func (m Manager) refreshCallback(systemURL string, request SystemRequest, retryAttempts int) func() (client.ProxyConfig, error) {
	return func() (client.ProxyConfig, error) {
		config, err := m.fetchSystemConfigRemotely(systemURL, request)
		if err != nil {
			if retryAttempts > 0 {
				retryAttempts--
				return m.refreshCallback(systemURL, request, retryAttempts)()
			}
		}
		return config, err
	}
}

func (m Manager) setValueFromConfig(systemURL string, request SystemRequest, value *cache.Value) *cache.Value {
	value.SetRefreshCallback(m.refreshCallback(systemURL, request, m.systemCache.config.numRetryFailedRefresh))
	value.SetExpiry(time.Now().Add(time.Second * m.systemCache.config.ttlSeconds))
	return value
}
