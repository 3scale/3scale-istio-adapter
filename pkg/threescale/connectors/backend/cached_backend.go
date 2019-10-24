package backend

import (
	"fmt"
	"net/http"
	"sync"

	backend "github.com/3scale/3scale-go-client/client"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale/metrics"
)

const (
	cacheKeySeparator = "_"
	cacheKey          = "%s" + cacheKeySeparator + "%s"
)

// Cacheable - defines the required behaviour of a Backend cache
type Cacheable interface {
	Get(cacheKey string) (CacheValue, bool)
	Set(cacheKey string, cv CacheValue)
	Report()
}

// Limit captures the current state of the rate limit for a particular time period
type Limit struct {
	current int
	max     int
	// unix timestamp for end period
	periodEnds int64
}

// CacheValue which should be stored in cache implementation
type CacheValue struct {
	LastResponse backend.ApiResponse
	LimitCounter map[string]map[backend.LimitPeriod]*Limit
	ReportWithValues
}

// Allows a reporting function to replay a request to report asynchronously
type ReportWithValues struct {
	Client    *backend.ThreeScaleClient
	Request   backend.Request
	ServiceID string
}

// CachedBackend provides a pluggable cache enabled 'Backend' implementation
// Supports reporting metrics for non-cached responses.
type CachedBackend struct {
	ReportFn      metrics.ReportMetricsFn
	cache         Cacheable
	mutex         sync.RWMutex
	hierarchyTree hierarchyTree
}

// map to represent parent --> children metric relationship
type metricParentToChildren map[string][]string

// hierarchyTree - represents the relationship between service metrics and their children
type hierarchyTree map[string]metricParentToChildren

// NewCachedBackend returns a pointer to a CachedBackend with a default LocalCache if no custom implementation has been provided
// and a channel which can be closed to stop the background process from reporting.
func NewCachedBackend(cache Cacheable, rfn metrics.ReportMetricsFn) (*CachedBackend, chan struct{}) {
	stop := make(chan struct{})
	if cache == nil {
		cache = NewLocalCache(nil, stop)
	}
	return &CachedBackend{
		ReportFn:      rfn,
		cache:         cache,
		hierarchyTree: make(hierarchyTree),
	}, stop
}

// AuthRep provides a combination of authorizing a request and reporting metrics to 3scale
func (cb CachedBackend) AuthRep(req AuthRepRequest, c *backend.ThreeScaleClient) (Response, error) {
	// compute the cache key for this request
	hierarchyKey := generateHierarchyKey(req.ServiceID, c.GetPeer())
	appIdentifier := generateAppIdentifier(req)
	cacheKey := generateCacheKey(hierarchyKey, appIdentifier)

	var affectedMetrics backend.Metrics
	// gather the metrics affected by this request using the hierarchy information we might have gathered
	// if bool is false we know this is a new entry and must handle the cache miss here
	affectedMetrics, metricsOk := cb.computeAffectedMetrics(hierarchyKey, req.Params.Metrics)
	if !metricsOk {
		if err := cb.handleCacheMiss(req, hierarchyKey, cacheKey, c); err != nil {
			return Response{}, err
		}
	}

	//expect items to be in the cache at this point but check for presence anyway
	affectedMetrics, metricsOk = cb.computeAffectedMetrics(hierarchyKey, req.Params.Metrics)
	cv, ok := cb.cache.Get(cacheKey)
	if !metricsOk || !ok {
		return cb.getCacheFetchErrorResponse(), fmt.Errorf("error fetching cached value")
	}

	newCacheValue, shouldCommit := cb.handleCachedRead(cv, affectedMetrics)
	if !shouldCommit {
		return Response{
			Reason:     "Limits exceeded",
			StatusCode: http.StatusTooManyRequests,
			Success:    false,
		}, nil
	}

	cb.cache.Set(cacheKey, newCacheValue)
	resp := Response{
		StatusCode: http.StatusOK,
		Success:    true,
	}

	return resp, nil
}

// handleCacheMiss for provided request
// this function is responsible for doing a remote lookup, computing the value and setting the value in the cache
// on successful call to remote backend, the value (that was cached) is returned
// any errors calling backend will result in a non-nil error value
func (cb CachedBackend) handleCacheMiss(req AuthRepRequest, hierarchyKey string, cacheKey string, c *backend.ThreeScaleClient) error {
	// instrumentation
	mc := metricsConfig{ReportFn: cb.ReportFn, Endpoint: "Auth", Target: "Backend"}
	resp, err := remoteAuth(req, map[string]string{"hierarchy": "1"}, c, mc)
	if err != nil {
		fmt.Println("error calling 3scale with blanket request")
		// need to do some handling here and potentially mark this hierarchyKey as fatal to avoid calling 3scale with repeated invalid/failing requests
		return err
	}
	// populate the hierarchy tree
	cb.setTreeEntry(hierarchyKey, resp.GetHierarchy())
	cv := createEmptyCacheValue().
		setReportWith(ReportWithValues{Client: c, Request: req.Request, ServiceID: req.ServiceID}).
		setLastResponse(resp)

	ur := resp.GetUsageReports()
	for metric, report := range ur {
		l := &Limit{
			current:    report.CurrentValue,
			max:        report.MaxValue,
			periodEnds: report.PeriodEnd - report.PeriodStart,
		}
		cv.LimitCounter[metric] = make(map[backend.LimitPeriod]*Limit)
		cv.LimitCounter[metric][report.Period] = l
	}
	// set the cache values for this entry
	cb.cache.Set(cacheKey, cv)
	return nil
}

// handleCachedRead should be called when we are convinced that an entry has been cached
// an error will be returned in case of cache miss
func (cb CachedBackend) handleCachedRead(value CacheValue, affectedMetrics backend.Metrics) (CacheValue, bool) {
	copyCache := cloneCacheValue(value)
	commitChanges := true

out:
	for metric, incrementBy := range affectedMetrics {
		if cachedValue, ok := copyCache.LimitCounter[metric]; ok {
			for _, limit := range cachedValue {
				newValue := limit.current + incrementBy
				if newValue > limit.max {
					commitChanges = false
					break out
				}
				limit.current = newValue
			}
		}
	}
	return copyCache, commitChanges
}

func (cb CachedBackend) getCacheFetchErrorResponse() Response {
	return Response{
		Reason:     "Cache lookup failed",
		StatusCode: http.StatusInternalServerError,
		Success:    false,
	}
}

// computeAffectedMetrics - accepts a list of metrics and computes any additional metrics that will be affected
// If the informer is not aware of this entry, then bool value will be false.
// Internally this function is responsible for walking the hierarchy tree and incrementing the parent value
// of any children that are affected by the provided metrics, in turn modifying the provided metrics
func (cb CachedBackend) computeAffectedMetrics(cacheKey string, m backend.Metrics) (backend.Metrics, bool) {
	affectedMetrics := cb.getTreeEntry(cacheKey)
	if affectedMetrics == nil {
		return m, false
	}

	for parent, children := range affectedMetrics {
		for k, v := range m {
			if contains(k, children) {
				if _, known := m[parent]; known {
					m[parent] += v
				} else {
					m.Add(parent, v)
				}
			}
		}
	}
	return m, true
}

// returns a copy of the informers hierarchy tree for a particular cache key
// safe for concurrent use
// returns nil if no logged entry for provided cache key
func (cb CachedBackend) getTreeEntry(cacheKey string) metricParentToChildren {
	var tree metricParentToChildren

	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	if len(cb.hierarchyTree[cacheKey]) > 0 {
		tree = make(metricParentToChildren)
	}
	for k, v := range cb.hierarchyTree[cacheKey] {
		tree[k] = v
	}

	return tree
}

// sets an entry in the informers hierarchy tree for a particular cache key
// safe for concurrent use
func (cb CachedBackend) setTreeEntry(cacheKey string, tree metricParentToChildren) {
	cb.mutex.Lock()
	cb.hierarchyTree[cacheKey] = tree
	cb.mutex.Unlock()
}

func (cv CacheValue) setLastResponse(resp backend.ApiResponse) CacheValue {
	cv.LastResponse = resp
	return cv
}

func (cv CacheValue) setReportWith(rw ReportWithValues) CacheValue {
	cv.ReportWithValues = rw
	return cv
}

func createEmptyCacheValue() CacheValue {
	limitMap := make(map[string]map[backend.LimitPeriod]*Limit)
	return CacheValue{
		LimitCounter: limitMap,
	}
}

func cloneCacheValue(existing CacheValue) CacheValue {
	copyVal := createEmptyCacheValue().
		setLastResponse(existing.LastResponse).
		setReportWith(existing.ReportWithValues)

	for metric, periodMap := range existing.LimitCounter {
		copyNested := make(map[backend.LimitPeriod]*Limit)
		for period, limit := range periodMap {
			var limitClone Limit
			limitClone = *limit
			copyNested[period] = &limitClone
		}
		copyVal.LimitCounter[metric] = copyNested
	}
	return copyVal
}

// utility function which returns true of an element is in a slice
func contains(key string, in []string) bool {
	for _, i := range in {
		if key == i {
			return true
		}
	}
	return false
}

// computes a unique key from a given service id and host in the format <id>_<host>
func generateHierarchyKey(svcID string, host string) string {
	return fmt.Sprintf(cacheKey, svcID, host)
}

// generates an identifier for app id based on the incoming request authn info
func generateAppIdentifier(req AuthRepRequest) string {
	if req.Request.Application.UserKey != "" {
		return req.Request.Application.UserKey
	}
	return req.Request.Application.AppID.ID + req.Request.Application.AppID.AppKey
}

// computes a unique key from a given service id and host in the format <hierarchyKey>_<appIdentifier>
func generateCacheKey(hierarchyKey string, appIdentifier string) string {
	return fmt.Sprintf(cacheKey, hierarchyKey, appIdentifier)
}
