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

func (b *Backend) remoteReport(request threescale.Request) (*threescale.ReportResult, error) {
	return b.client.Report(request)
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

// Flush the cached entries and report existing state to backend
func (b *Backend) Flush() {
	b.flush()
}

// cachedApp provides a placeholder for information for cache flushing
type cachedApp struct {
	// snapshot stores the app state (copy) as it was when we attempted to flush the cache
	snapshot     Application
	cacheKey     string
	serviceID    api.Service
	appID        string
	reportingErr bool
	authErr      bool
	authResp     *threescale.AuthorizeResult
	// deltas stores the metrics (copy) that were actually reported to 3scale after
	// taking the hierarchy into account
	deltas api.Metrics
}

func (b *Backend) flush() {
	// create a hash of cached applications under a serviceApps ID
	// this may not be required down in future and we can simplify but
	// this gives us the flexibility to do batched reporting
	groupedKeys := b.groupCachedItems()
	for _, serviceApps := range groupedKeys {

		// the first stage of flushing the cache involves calculating the value to be reported to 3scale
		// and reporting remotely
		b.handleFlushReporting(serviceApps)

		// after we have reported for each known app under this service, we assume backend has finished its
		// processing from the queue and authorize the app with a blank request to fetch updated state
		// saving the response in the process
		for i, cachedApp := range serviceApps {
			req := getEmptyAuthRequest(cachedApp.serviceID, cachedApp.snapshot.auth, cachedApp.snapshot.params)
			resp, err := b.remoteAuth(req)
			if err != nil {
				serviceApps[i].authErr = true
			}
			serviceApps[i].authResp = resp
		}

		// update the entry in the cache
		b.handleFlushCacheUpdate(serviceApps)
	}
}

func (b *Backend) groupCachedItems() map[string][]cachedApp {
	keys := b.cache.Keys()
	groupedServices := make(map[string][]cachedApp)

	for _, key := range keys {
		svc, app, err := parseCacheKey(key)
		if err == nil {
			groupedServices[svc] = append(groupedServices[svc], cachedApp{
				cacheKey:  key,
				serviceID: api.Service(svc),
				appID:     app,
			})
		}
	}
	return groupedServices
}

// handleFlushReporting takes a read lock on the cached application and a snapshot of the app state and releases the
// lock before computing the data which must be reported to 3scales backend
func (b *Backend) handleFlushReporting(apps []cachedApp) {
	for i, cachedApp := range apps {
		app, ok := b.cache.Get(cachedApp.cacheKey)
		if !ok {
			continue
		}
		// for each application for this service
		// take a read lock and take a snapshot of the current state
		app.RLock()
		// take a snapshot of the current application
		apps[i].snapshot = app.deepCopy()
		app.RUnlock()
		appClone := &apps[i].snapshot

		// calculate the deltas and report remotely
		deltas := appClone.calculateDeltas()
		// store the deltas for further use
		apps[i].deltas = deltas
		req := threescale.Request{
			Auth:    appClone.auth,
			Service: cachedApp.serviceID,
			Transactions: []api.Transaction{
				{
					Metrics: deltas,
					Params:  app.params,
				},
			},
			Extensions: api.Extensions{
				api.FlatUsageExtension: "1",
			},
		}
		_, err := b.remoteReport(req)
		if err != nil {
			apps[i].reportingErr = true
		}
	}
}

// handleFlushCacheUpdate updates a cached entry (taking a write lock) using the prior flushing steps as a decision tree.
// A description of the terminology used below is as follows:
//     snapshot_hits - the value in local state when the snapshot is taken
//     new_auth_hits - the current value for a metric as fetched from 3scale after successful authorization
//     to_report_hits - the deltas that are calculated prior to reporting = current local state - last known 3scale state
//     actually_reported_hits - is equal 0 (if report fails) || is equal to to_report_hits (if report succeeded)
//     remote_state - local representation of state in remote 3scale
// We need to handle the following cases ("hits" is used as an example metric name for simplification):
// 1. reporting error and authorization error -> nothing to do, no change of state
// 2,3. reporting error and authorization success, report success and authorization success -> hits += new_auth_hits - (snapshot_hits - to_report_hits ) - actually_reported_hits
// 4. report success and authorization error -> remote_state = remote_state + actually_reported_hits. No change to local counters.
// To summarise, if we have a reporting error, we will not amend our local representation of the remote state. if we have an authorization error we will not amend our local state.
// In both of these cases, a successful call instead of an error will result in a change in the mentioned state
func (b *Backend) handleFlushCacheUpdate(apps []cachedApp) {
	for _, cachedApp := range apps {
		// handle case (1)
		if cachedApp.reportingErr && cachedApp.authErr {
			// nothing to do here only continue on
			// todo add logging
			continue
		}

		//now that we have handled the dual error case, we know we need to
		// read from the cache and modify the entry so we take a write lock
		app, ok := b.cache.Get(cachedApp.cacheKey)
		if !ok {
			continue
		}

		// handle case (3)
		if !cachedApp.reportingErr && cachedApp.authErr {
			app.Lock()
			// This is necessary when, during flushing, we have reported correctly but failed to authorize
			// we dont have an updated state from 3scale so in order to avoid re-reporting what we have already
			// successfully reported. We can achieve this safely by modifying our last know local
			// representation of the remote state with the deltas we know that 3scale has processed.
			app.addDeltasToRemoteState(cachedApp.deltas)
			// we need to account for activity in between while adjusting our unlimited counter
			app.pruneUnlimitedCounter(cachedApp.snapshot)
			b.cache.Set(cachedApp.cacheKey, app)
			app.Unlock()
			continue
		}

		// handles cases (2, 4)
		remoteState := getApplicationFromResponse(cachedApp.authResp).RemoteState

		app.Lock()
		app.adjustLocalState(cachedApp, remoteState)
		b.cache.Set(cachedApp.cacheKey, app)
		app.Unlock()
	}
}

// GetPeer returns the hostname of the connected backend
func (b *Backend) GetPeer() string {
	return b.client.GetPeer()
}

// calculateDeltas returns a set of metrics that we can report to 3scale using the lowest period known in the counters
// for metrics with rate limits attached
func (a *Application) calculateDeltas() api.Metrics {
	deltas := make(api.Metrics)

	for metric := range a.LimitCounter {
		lowestPeriod := a.LimitCounter.getLowestKnownPeriodForMetric(metric)
		if lowestPeriod != nil {
			if lastKnownState, ok := a.RemoteState[metric][*lowestPeriod]; ok {
				toReport := a.LimitCounter[metric][*lowestPeriod].current - lastKnownState.current
				deltas.Add(metric, toReport)
			}
		}
	}

	// now add the metrics with no limits to the delta
	for unlimitedMetric, reportWithValue := range a.UnlimitedCounter {
		deltas[unlimitedMetric] = reportWithValue
	}
	return deltas
}

// addDeltasToRemoteState modifies the remote state with the provided deltas
func (a *Application) addDeltasToRemoteState(deltas api.Metrics) *Application {
	for metric, value := range deltas {
		periods, ok := a.RemoteState[metric]
		if !ok {
			continue
		}

		for period, _ := range periods {
			a.RemoteState[metric][period].current += value
		}
	}

	return a
}

// deepCopy creates a clone of the Application 'a'
func (a *Application) deepCopy() Application {
	unlimitedHitsClone := make(map[string]int, len(a.UnlimitedCounter))
	for metric, val := range a.UnlimitedCounter {
		unlimitedHitsClone[metric] = val
	}

	return Application{
		RemoteState:      a.RemoteState,
		LimitCounter:     a.LimitCounter.deepCopy(),
		UnlimitedCounter: unlimitedHitsClone,
		params:           a.params,
		auth:             a.auth,
	}
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

// pruneUnlimitedCounter resets a metrics counter to the difference between the provided old value and the current value
// If the end result is not at least 1, the metric is pruned from the counter
func (a *Application) pruneUnlimitedCounter(snapshot Application) {
	for metric, value := range a.UnlimitedCounter {
		oldValue, ok := snapshot.UnlimitedCounter[metric]
		if !ok {
			delete(a.UnlimitedCounter, metric)
		}

		value -= oldValue
		if value < 1 {
			delete(a.UnlimitedCounter, metric)
		}
	}
}

// adjustLocalState assumes that we have a new remote state (set on a) fetched from 3scale and modifies local state based
// on the state obtained during cache flushing
func (a *Application) adjustLocalState(flushingState cachedApp, remoteState LimitCounter) *Application {
	a.RemoteState = remoteState

	for metric := range a.LimitCounter {
		lowestPeriod := a.LimitCounter.getLowestKnownPeriodForMetric(metric)

		if lowestPeriod == nil {
			continue
		}

		snapped, ok := flushingState.snapshot.LimitCounter[metric][*lowestPeriod]
		if !ok {
			continue
		}

		latest, ok := a.RemoteState[metric][*lowestPeriod]
		if !ok {
			continue
		}

		lowestLimit := a.LimitCounter[metric][*lowestPeriod]

		var reportedHits int
		delta := flushingState.deltas[metric]
		if !flushingState.reportingErr {
			reportedHits = delta
		}
		updated := lowestLimit.current + latest.current - (snapped.current - delta) - reportedHits
		lowestLimit.current = updated
		lowestLimit.max = a.RemoteState[metric][*lowestPeriod].max
	}

	if !flushingState.reportingErr {
		// reset the counter if we have reported already
		a.pruneUnlimitedCounter(flushingState.snapshot)
	}

	return a
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

var ascendingPeriodSequence = []api.Period{api.Minute, api.Hour, api.Day, api.Week, api.Month, api.Eternity}

func (lc LimitCounter) getLowestKnownPeriodForMetric(metric string) *api.Period {
	var period *api.Period

	periodMap, ok := lc[metric]
	if !ok {
		return nil
	}

	for _, lowestKnownPeriod := range ascendingPeriodSequence {
		if _, ok := periodMap[lowestKnownPeriod]; ok {
			period = &lowestKnownPeriod
			break
		}
	}
	return period
}
