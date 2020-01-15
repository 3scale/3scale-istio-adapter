package backend

import (
	"fmt"
	"sync"

	"github.com/3scale/3scale-go-client/threescale"
	"github.com/3scale/3scale-go-client/threescale/api"
)

// Backend defines the connection to a single backend and maintains a cache
// for multiple services and applications per backend. It implements the 3scale Client interface
type Backend struct {
	client threescale.Client
	cache  Cacheable
}

// Application defined under a 3scale service
type Application struct {
	RemoteState      LimitCounter
	LimitCounter     LimitCounter
	UnlimitedCounter UnlimitedCounter
	sync.RWMutex
	metricHierarchy api.Hierarchy
	auth            api.ClientAuth
	params          api.Params
}

// Limit captures the current state of the rate limit for a particular time period
type Limit struct {
	current int
	max     int
}

// LimitCounter keeps a count of limits for a given period
type LimitCounter map[string]map[api.Period]*Limit

// UnlimitedCounter keeps a count of metrics without limits
type UnlimitedCounter map[string]int

// Authorize authorizes a request based on the current cached values
// If the request misses the cache, a remote call to 3scale is made
// Request Transactions must not be nil and must not be empty
// If multiple transactions are provided, all but the first is discarded
func (b *Backend) Authorize(request threescale.Request) (*threescale.AuthorizeResult, error) {
	return b.authorize(request)
}

func (b *Backend) authorize(request threescale.Request) (*threescale.AuthorizeResult, error) {
	var err error

	err = validateTransactions(request.Transactions)
	if err != nil {
		return nil, err
	}

	cacheKey := generateCacheKeyFromRequest(request)
	app := b.getApplicationFromCache(cacheKey)

	if app == nil {
		app, err = b.handleCacheMiss(request, cacheKey)
		if err != nil {
			return nil, fmt.Errorf("unable to process request - %s", err.Error())
		}
	}

	affectedMetrics := computeAffectedMetrics(app, request)
	isAuthorized := b.isAuthorized(app, affectedMetrics)

	result := &threescale.AuthorizeResult{Authorized: isAuthorized}
	if !isAuthorized {
		result.ErrorCode = "usage limits are exceeded"
	}

	return result, nil
}

// handleCacheMiss attempts to call remote 3scale with a blank request and the
// required extensions enabled to learn current state for caching purposes.
// Sets the value in the cache and returns the newly cached value for re-use if required
func (b *Backend) handleCacheMiss(request threescale.Request, cacheKey string) (*Application, error) {
	var app Application

	emptyTransaction := emptyTransactionFrom(request.Transactions[0])
	emptyRequest := getEmptyAuthRequest(request.Service, request.Auth, emptyTransaction.Params)

	resp, err := b.remoteAuth(emptyRequest)
	if err != nil {
		return nil, err
	}
	app = getApplicationFromResponse(resp)
	app.auth = request.Auth
	app.params = request.Transactions[0].Params
	app.metricHierarchy = resp.Hierarchy

	b.cache.Set(cacheKey, &app)
	return &app, nil
}

// isAuthorized takes a read lock on the application and confirms if the request
// should be authorized based on the affected metrics against current state
func (b *Backend) isAuthorized(application *Application, affectedMetrics api.Metrics) bool {
	application.RLock()
	defer application.RUnlock()

	authorized := true

out:
	for metric, incrementBy := range affectedMetrics {
		cachedValue, ok := application.LimitCounter[metric]
		if ok {
			for _, limit := range cachedValue {
				if limit.current+incrementBy > limit.max {
					authorized = false
					break out
				}
			}
		}
	}
	return authorized
}

func (b *Backend) getApplicationFromCache(key string) *Application {
	app, ok := b.cache.Get(key)
	if !ok {
		app = nil
	}
	return app
}

// remoteAuth calls Authorize API of the underlying client
func (b *Backend) remoteAuth(request threescale.Request) (*threescale.AuthorizeResult, error) {
	return b.client.Authorize(request)
}

// GetPeer returns the hostname of the connected backend
func (b *Backend) GetPeer() string {
	return b.client.GetPeer()
}

// deepCopy creates a clone of the LimitCounter 'lc'
func (lc LimitCounter) deepCopy() LimitCounter {
	clone := make(LimitCounter)
	for metric, periods := range lc {
		clone[metric] = make(map[api.Period]*Limit, len(periods))
		for period, limit := range periods {
			clone[metric][period] = &Limit{current: limit.current, max: limit.max}
		}
	}
	return clone
}
