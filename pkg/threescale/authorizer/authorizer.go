package authorizer

import (
	"fmt"
	"time"

	"github.com/3scale/3scale-authorizer/pkg/system/v1/cache"
	"github.com/3scale/3scale-go-client/threescale"
	"github.com/3scale/3scale-go-client/threescale/api"
	"github.com/3scale/3scale-porta-go-client/client"
)

// Authorizer fetches configuration from 3scale and authorizes requests to 3scale
type Authorizer interface {
	GetSystemConfiguration(systemURL string, request SystemRequest) (client.ProxyConfig, error)
	AuthRep(backendURL string, request BackendRequest) (*BackendResponse, error)
}

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
	stop                  chan struct{}
}

// SystemRequest provides the required input to request the latest configuration from 3scale system
type SystemRequest struct {
	AccessToken string
	ServiceID   string
	Environment string
}

// BackendAuth contains client authorization credentials for apisonator
type BackendAuth struct {
	Type  string
	Value string
}

// BackendRequest contains the data required to make an Auth/AuthRep request to apisonator
type BackendRequest struct {
	Auth         BackendAuth
	Service      string
	Transactions []BackendTransaction
}

// BackendResponse contains the result of an Auth/AuthRep request
type BackendResponse struct {
	Authorized bool
	// RejectedReason should* be set in cases where Authorized is false
	RejectedReason string
}

// BackendTransaction contains the metrics and end user auth required to make an Auth/AuthRep request to apisonator
type BackendTransaction struct {
	Metrics map[string]int
	Params  BackendParams
}

// BackendParams contains the ebd user auth for the various supported authentication patterns
type BackendParams struct {
	AppID   string
	AppKey  string
	UserID  string
	UserKey string
}

// NewManager returns an instance of Manager with some sensible configuration defaults if not explicitly provided
// Starts refreshing background process for underlying system cache if provided
func NewManager(builder Builder, systemCache *SystemCache) (*Manager, error) {
	if builder == nil {
		return nil, fmt.Errorf("manager requires a valid builder")
	}

	if systemCache.cache != nil {
		if systemCache.config.refreshInterval == time.Duration(0) {
			conf := &systemCache.config
			conf.refreshInterval = cache.DefaultCacheRefreshInterval
		}

		if systemCache.config.ttlSeconds == time.Duration(0) {
			conf := &systemCache.config
			conf.ttlSeconds = cache.DefaultCacheTTL
		}

		go func() {
			ticker := time.NewTicker(systemCache.config.refreshInterval)
			for {
				select {
				case <-ticker.C:
					systemCache.cache.Refresh()
				case <-systemCache.config.stop:
					ticker.Stop()
					return
				}
			}
		}()

	}

	return &Manager{
		clientBuilder: builder,
		systemCache:   *systemCache,
	}, nil
}

// NewSystemCache returns a system cache based on the provided interface and config
func NewSystemCache(cache cache.ConfigurationCache, config SystemCacheConfig) *SystemCache {
	return &SystemCache{
		cache:  cache,
		config: config,
	}
}

func NewSystemCacheConfig(refreshRetries int, refreshInterval, ttlSeconds time.Duration, stopRefreshing chan struct{}) *SystemCacheConfig {
	return &SystemCacheConfig{
		numRetryFailedRefresh: refreshRetries,
		refreshInterval:       refreshInterval,
		ttlSeconds:            ttlSeconds,
		stop:                  stopRefreshing,
	}
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

// AuthRep does a Authorize and Report request into 3scale apisonator
func (m Manager) AuthRep(backendURL string, request BackendRequest) (*BackendResponse, error) {
	client, err := m.clientBuilder.BuildBackendClient(backendURL)
	if err != nil {
		return nil, fmt.Errorf("unable to build required client for 3scale backend - %s", err.Error())
	}

	req, err := request.ToAPIRequest()
	if err != nil {
		return nil, fmt.Errorf("unable to build request to 3scale - %s", err)
	}

	res, err := client.AuthRep(*req)
	if err != nil {
		return nil, fmt.Errorf("error calling AuthRep - %s", err)
	}

	return &BackendResponse{Authorized: res.Success()}, nil
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

// ToAPIRequest transforms the BackendRequest into a request that is acceptable for the 3scale Client interface
func (request BackendRequest) ToAPIRequest() (*threescale.Request, error) {
	if request.Transactions == nil || len(request.Transactions) < 1 {
		return nil, fmt.Errorf("cannot process emtpy transaction")
	}

	return &threescale.Request{
		Auth: api.ClientAuth{
			Type:  api.AuthType(request.Auth.Type),
			Value: request.Auth.Value,
		},
		Extensions: nil,
		Service:    api.Service(request.Service),
		Transactions: []api.Transaction{
			{
				Metrics: request.Transactions[0].Metrics,
				Params: api.Params{
					AppID:   request.Transactions[0].Params.AppID,
					AppKey:  request.Transactions[0].Params.AppKey,
					UserID:  request.Transactions[0].Params.UserID,
					UserKey: request.Transactions[0].Params.UserKey,
				},
			},
		},
	}, nil
}
