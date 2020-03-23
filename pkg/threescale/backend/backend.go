package backend

import (
	"fmt"
	"sync"
	"time"

	"github.com/3scale/3scale-go-client/threescale"
	"github.com/3scale/3scale-go-client/threescale/api"
)

// Backend defines the connection to a single backend and maintains a cache
// for multiple services and applications per backend. It implements the 3scale Client interface
type Backend struct {
	client threescale.Client
	cache  Cacheable
	// a queue to enqueue cached applications whose counters need to be reported in a older period
	// queue must not be nil
	queue *dequeue
}

// Application defined under a 3scale service
// It is the responsibility of creator of an application to ensure that the counters for both remote and local state
// is sorted by ascending granularity. The internals of the cache relies on these semantics.
type Application struct {
	RemoteState      LimitCounter
	LocalState       LimitCounter
	UnlimitedCounter UnlimitedCounter
	sync.RWMutex
	metricHierarchy api.Hierarchy
	auth            api.ClientAuth
	params          api.Params
	timestamp       int64
	// id as recorded by 3scale
	id string
	// ownedBy this service id
	ownedBy api.Service
}

// LimitCounter keeps a count of limits for a given period
type LimitCounter api.UsageReports

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
	app.annotateWithRequestDetails(request)

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
		cachedValue, ok := application.LocalState[metric]
		if !ok {
			continue
		}

		for _, granularity := range cachedValue {
			if granularity.CurrentValue+incrementBy > granularity.MaxValue {
				authorized = false
				break out
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
		cachedValue, ok := application.LocalState[metric]

		if ok {
			for index := range cachedValue {
				incrementCountersCurrentValue(&cachedValue[index], incrementBy)
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

// handledApp represents an application that is going through the flushing process
// It holds the state of the application at the time of the remote report as well
// as some metadata which gives insight into the events that occurred during flushing
type handledApp struct {
	// snapshot stores the app state (copy) as it was when we attempted to flush the cache
	snapshot     Application
	reportingErr bool
	authErr      bool
	authResp     *threescale.AuthorizeResult
	// deltas stores the metrics (copy) that were actually reported to 3scale after
	// taking the hierarchy into account
	deltas api.Metrics
}

func (b *Backend) flush() {
	// read the cache and write the items to the queue
	b.enqueueCachedApplications()

	// report the metrics for all known applications
	handledApps := b.handleFlushReporting()

	// after we have reported for each known app under this service, we assume backend has finished its
	// processing from the queue and authorize the app with a blank request to fetch updated state
	// saving the response in the process
	handledApps = b.handleFlushAuthorization(handledApps)
	// update the entry in the cache
	b.handleFlushCacheUpdate(handledApps)
}

// enqueueCachedApplications takes a snapshot of each application currently stored in the cache
// and writes it to the back of the queue
func (b *Backend) enqueueCachedApplications() {
	keys := b.cache.Keys()

	for _, key := range keys {
		if app, ok := b.cache.Get(key); ok {
			app.RLock()
			clone := app.deepCopy()
			app.RUnlock()
			svc, appID, _ := parseCacheKey(key)
			clone.ownedBy = svc
			clone.id = appID
			b.queue.append(&clone)
		}
	}
}

// handleFlushReporting removes items from the deque and sorts them under relevant services
// reports in batches as it pops apps off the queue
func (b *Backend) handleFlushReporting() []*handledApp {
	var groupedApps []*Application
	var handledApps []*handledApp
	var serviceID api.Service

	for !b.queue.isEmpty() {
		// take the item off the front of the queue
		queuedApp := b.queue.shift()
		if queuedApp == nil {
			continue
		}

		if serviceID == "" {
			serviceID = queuedApp.ownedBy
		}
		// if we have are currently handling an app from the same service, add to the list and move on
		if queuedApp.ownedBy == serviceID {
			groupedApps = append(groupedApps, queuedApp)
			continue
		}
		// we have hit a different service, put it back on the front of the queue
		b.queue.prepend(queuedApp)
		// report this batch and mark these items as handled
		handledApps = append(handledApps, b.reportGroupedApps(serviceID, groupedApps)...)
		serviceID = ""
		groupedApps = nil

	}
	handledApps = append(handledApps, b.reportGroupedApps(serviceID, groupedApps)...)
	return handledApps
}

// batch report the applications for the given service id
func (b *Backend) reportGroupedApps(service api.Service, apps []*Application) []*handledApp {
	var handledApps []*handledApp
	if len(apps) < 1 {
		return handledApps
	}
	auth := apps[0].auth

	var transactions []api.Transaction
	for _, app := range apps {
		// calculate the deltas and report remotely
		deltas := app.calculateDeltas()

		transactions = append(transactions, api.Transaction{
			Metrics:   deltas,
			Params:    app.params,
			Timestamp: app.timestamp,
		})

		handledApps = append(handledApps, &handledApp{
			snapshot: *app,
			deltas:   deltas,
		})
	}
	req := threescale.Request{
		Auth:         auth,
		Service:      service,
		Transactions: transactions,
		Extensions: api.Extensions{
			api.FlatUsageExtension: "1",
		},
	}
	_, err := b.remoteReport(req)
	if err != nil {
		// TODO Log error
		for _, app := range handledApps {
			app.reportingErr = true
		}
	}
	return handledApps
}

// authorize an application with an empty request
func (b *Backend) handleFlushAuthorization(apps []*handledApp) []*handledApp {
	for _, app := range apps {
		req := getEmptyAuthRequest(app.snapshot.ownedBy, app.snapshot.auth, app.snapshot.params)
		resp, err := b.remoteAuth(req)
		if err != nil {
			app.authErr = true
		}
		app.authResp = resp
	}
	return apps
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
func (b *Backend) handleFlushCacheUpdate(apps []*handledApp) {
	for _, app := range apps {
		cacheKey := app.snapshot.getCacheKey()

		// handle case (1, 2)
		if app.reportingErr {
			b.handleFailedFlushReporting(app, cacheKey)
			continue
		}

		cachedApp, ok := b.cache.Get(cacheKey)
		if !ok {
			continue
		}

		// handle case (4)
		if !app.reportingErr && app.authErr {
			cachedApp.Lock()
			// This is necessary when, during flushing, we have reported correctly but failed to authorize
			// we dont have an updated state from 3scale so in order to avoid re-reporting what we have already
			// successfully reported. We can achieve this safely by modifying our last know local
			// representation of the remote state with the deltas we know that 3scale has processed.
			cachedApp.addDeltasToRemoteState(app.deltas)
			// we need to account for activity in between while adjusting our unlimited counter
			cachedApp.pruneUnlimitedCounter(app.snapshot)
			b.cache.Set(cacheKey, cachedApp)
			cachedApp.Unlock()
			continue
		}

		// handles cases (3)
		updatedApp := getApplicationFromResponse(app.authResp)
		cachedApp.Lock()
		cachedApp.adjustLocalState(app, updatedApp.RemoteState)
		b.cache.Set(cacheKey, cachedApp)
		cachedApp.Unlock()
	}
}

func (b *Backend) handleFailedFlushReporting(app *handledApp, cacheKey string) {
	cachedApp, ok := b.cache.Get(cacheKey)
	if !ok {
		return
	}
	// handling a reporting error and auth error is simple since there are no adjustments we can make
	// so we just take the system time and take a lock on the cache at this point
	deadline := time.Now()
	cachedApp.Lock()
	defer cachedApp.Unlock()

	// handle reporting error and authorization success
	if !app.authErr {
		// if we have authorised correctly, attempt to glean a timestamp from the response
		updatedApp := getApplicationFromResponse(app.authResp)
		deadline = time.Unix(updatedApp.timestamp, 0)
		// alter the applications state based on the response from 3scale
		cachedApp.adjustLocalState(app, updatedApp.RemoteState)
	}

	// we need to check if any of the metrics are under a period that has elapsed and if this is the case
	// add the snapshot back onto the queue to be reported
	var deadlineExceeded bool
out:
	for _, metric := range cachedApp.LocalState {
		for _, counter := range metric {
			if hasSurpassedDeadline(app.snapshot.timestamp, counter.PeriodWindow.Period, deadline) {
				// add the report to the queue once again since we know this is for an elapsed period
				b.queue.append(&app.snapshot)
				deadlineExceeded = true
				break out
			}
		}
	}

	// if no period has elapsed, treat the timestamp as current and write to the cache
	if !deadlineExceeded {
		b.cache.Set(cacheKey, cachedApp)
		return
	}

	// if we have had an elapsed period we need to handle it accordingly. This means making a further adjustment
	// and reducing our local state by the values related to the periods that have elapsed and been re-queued
	for metric, usageReports := range cachedApp.LocalState {
		for _, counter := range usageReports {
			if !hasSurpassedDeadline(app.snapshot.timestamp, counter.PeriodWindow.Period, deadline) {
				// we can break the loop here since we are sure the counters are sorted by ascending periods
				break
			}
			// since it has surpassed the deadline, we lookup the values we have re-queued and make a reduction
			// we can safely take the zero value if the map lookup fails
			counter.CurrentValue -= app.deltas[metric]
		}
	}

}

// GetPeer returns the hostname of the connected backend
func (b *Backend) GetPeer() string {
	return b.client.GetPeer()
}

func (a *Application) annotateWithRequestDetails(request threescale.Request) {
	if len(request.Transactions) > 0 {
		transaction := request.Transactions[0]
		a.id = getAppIDFromTransaction(transaction)
		a.params = transaction.Params
	}
	a.ownedBy = request.GetServiceID()
	a.auth = request.Auth
}

// calculateDeltas returns a set of metrics that we can report to 3scale using the lowest period known in the counters
// for metrics with rate limits attached
func (a *Application) calculateDeltas() api.Metrics {
	deltas := getDifferenceAsMetrics(a.RemoteState, a.LocalState)

	// now add the metrics with no limits to the delta
	for unlimitedMetric, reportWithValue := range a.UnlimitedCounter {
		deltas[unlimitedMetric] = reportWithValue
	}
	return deltas
}

// addDeltasToRemoteState modifies the remote state with the provided deltas
func (a *Application) addDeltasToRemoteState(deltas api.Metrics) *Application {
	for metric, value := range deltas {
		counters, ok := a.RemoteState[metric]
		if !ok {
			continue
		}

		for index := range counters {
			incrementCountersCurrentValue(&counters[index], value)
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
		RemoteState:      a.RemoteState.deepCopy(),
		LocalState:       a.LocalState.deepCopy(),
		UnlimitedCounter: unlimitedHitsClone,
		params:           a.params,
		auth:             a.auth,
		timestamp:        a.timestamp,
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
func (a *Application) adjustLocalState(flushingState *handledApp, remoteState LimitCounter) *Application {
	a.RemoteState = remoteState

	for metric, counters := range a.LocalState {

		if canAdjust := statesAreComparable(metric, 0, a.RemoteState, flushingState.snapshot.LocalState); !canAdjust {
			continue
		}

		lowestLocalGranularity := counters[0]
		lowestSnappedGranularity := flushingState.snapshot.LocalState[metric][0]
		lowestRemoteGranularity := a.RemoteState[metric][0]

		var reportedHits int
		delta := flushingState.deltas[metric]
		if !flushingState.reportingErr {
			reportedHits = delta
		}

		updated := lowestLocalGranularity.CurrentValue + lowestRemoteGranularity.CurrentValue -
			(lowestSnappedGranularity.CurrentValue - delta) - reportedHits

		lowestLocalGranularity.CurrentValue = updated
		lowestLocalGranularity.MaxValue = lowestRemoteGranularity.MaxValue
	}

	if !flushingState.reportingErr {
		// reset the counter if we have reported already
		a.pruneUnlimitedCounter(flushingState.snapshot)
	}

	return a
}

func (a *Application) compareLocalToRemoteState(metric string, index int) bool {
	return statesAreComparable(metric, index, a.LocalState, a.RemoteState)
}

func (a *Application) getCacheKey() string {
	return fmt.Sprintf("%s_%s", a.ownedBy, a.id)
}

// deepCopy creates a clone of the LocalState 'lc'
func (lc LimitCounter) deepCopy() LimitCounter {
	clone := make(LimitCounter)
	for metric, counters := range lc {
		clone[metric] = append([]api.UsageReport(nil), counters...)
	}
	return clone
}
