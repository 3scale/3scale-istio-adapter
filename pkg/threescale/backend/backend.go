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

	cacheKey := generateCacheKeyFromRequest(request, 0)
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

// AuthRep authorizes a request based on the current cached values and reports
// the new state to the cache in cases where the request has been authorized
// If the request misses the cache, a remote call to 3scale is made
// Request Transactions must not be nil and must not be empty
// If multiple transactions are provided, all but the first are discarded
func (b *Backend) AuthRep(request threescale.Request) (*threescale.AuthorizeResult, error) {
	return b.authRep(request)
}

func (b *Backend) authRep(request threescale.Request) (*threescale.AuthorizeResult, error) {
	var err error

	err = validateTransactions(request.Transactions)
	if err != nil {
		return nil, err
	}

	cacheKey := generateCacheKeyFromRequest(request, 0)
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
	if isAuthorized {
		b.localReport(cacheKey, affectedMetrics)

	} else {
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

// Report the provided request transactions to the cache
// Report always returns with 'Accepted' as bool 'true' and the error will only ever be nil
// in cases where  the provided transaction are empty or nil
// The background task will abort in cases where we have a cache miss and we also fail to get the hierarchy from 3scale
// Typically, we can handle cache miss and report to cache once we are aware of a hierarchy for the service
// Request Transactions must not be nil and must not be empty
// Supports multiple transactions
func (b *Backend) Report(request threescale.Request) (*threescale.ReportResult, error) {
	return b.report(request)
}

func (b *Backend) report(request threescale.Request) (*threescale.ReportResult, error) {
	var hierarchy api.Hierarchy
	var err error

	err = validateTransactions(request.Transactions)
	if err != nil {
		return nil, err
	}

	go func() {
		for index, transaction := range request.Transactions {
			// we support reporting in batches so for every transaction, grab the cache key and see if we
			// have a match. if not, we can report locally regardless once we know the hierarchy
			cacheKey := generateCacheKeyFromRequest(request, index)

			app, ok := b.cache.Get(cacheKey)
			if !ok {
				// if we missed, configure a blank app
				app = newApplication()
			}

			if hierarchy == nil {
				if app.RemoteState == nil {
					// while reporting locally, we don't care about cache misses, however we do need
					// to deal with the exception of cases where we have no hierarchy
					// which could lead to incorrect reports
					app, err = b.handleCacheMiss(request, cacheKey)
					if err != nil {
						// we can continue here and try to deal with the rest of the transactions
						// it may fail if 3scale was down and likely a transient error
						continue
					}
				}
				hierarchy = app.metricHierarchy
			}

			affectedMetrics := transaction.Metrics.AddHierarchyToMetrics(app.metricHierarchy)
			b.localReport(cacheKey, affectedMetrics)
		}
	}()

	return &threescale.ReportResult{Accepted: true}, nil
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

// localReport takes a write lock on the application and reports to the cache
func (b *Backend) localReport(cacheKey string, metrics api.Metrics) {
	// note - this should not miss since we will have returned an error prior to this if
	// we have failed to fetch and build an application and populate the cache with it
	// that is, since the Set func on the cache currently does not return an error then this is ok
	// if we end up supporting external caches and the write can fail then this may need updating.
	application, _ := b.cache.Get(cacheKey)

	application.Lock()
	defer application.Unlock()

	for metric, incrementBy := range metrics {
		cachedValue, ok := application.LimitCounter[metric]
		if ok {
			for _, limit := range cachedValue {
				limit.current += incrementBy
			}
		} else {
			application.updateUnlimitedCounter(metric, incrementBy)
		}
	}
	b.cache.Set(cacheKey, application)
}

// GetPeer returns the hostname of the connected backend
func (b *Backend) GetPeer() string {
	return b.client.GetPeer()
}

// updateUnlimitedCounter modifies the Applications 'UnlimitedCounter' field
func (a *Application) updateUnlimitedCounter(metric string, incrementBy int) {
	// has no limits so just cache the value for reporting purposes
	current, ok := a.UnlimitedCounter[metric]
	if !ok {
		a.UnlimitedCounter[metric] = 0
	}

	a.UnlimitedCounter[metric] = current + incrementBy
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
