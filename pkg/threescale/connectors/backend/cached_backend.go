package backend

import (
	"fmt"
	"net/http"

	backend "github.com/3scale/3scale-go-client/client"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale/metrics"
)

const (
	cacheKeySeparator = "_"
	cacheKeyFormat    = "%s" + cacheKeySeparator + "%s"
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
	UnlimitedHits map[string]int
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
	ReportFn metrics.ReportMetricsFn
	cache    Cacheable
}

// map to represent parent --> children metric relationship
type metricParentToChildren map[string][]string

// NewCachedBackend returns a pointer to a CachedBackend with a default LocalCache if no custom implementation has been provided
func NewCachedBackend(cache Cacheable, rfn metrics.ReportMetricsFn) *CachedBackend {
	stop := make(chan struct{})
	if cache == nil {
		cache = NewLocalCache(nil, stop)
	}
	return &CachedBackend{
		ReportFn: rfn,
		cache:    cache,
	}
}

// AuthRep provides a combination of authorizing a request and reporting metrics to 3scale
func (cb CachedBackend) AuthRep(req AuthRepRequest, c *backend.ThreeScaleClient) (Response, error) {
	// compute the cache key for this request
	cacheKey := generateCacheKey(req.ServiceID, c.GetPeer())

	cv, ok := cb.cache.Get(cacheKey)
	if !ok {
		newlyCachedValue, err := cb.handleCacheMiss(req, cacheKey, c)
		if err != nil {
			return Response{}, err
		}
		cv = newlyCachedValue
	}

	affectedMetrics := computeAffectedMetrics(cv.LastResponse.GetHierarchy(), req.Params.Metrics)
	copiedCacheValue, shouldCommit := cb.handleCachedRead(cv, affectedMetrics)
	if !shouldCommit {
		return Response{
			Reason:     "Limits exceeded",
			StatusCode: http.StatusTooManyRequests,
			Success:    false,
		}, nil
	}

	cb.cache.Set(cacheKey, copiedCacheValue)
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
func (cb CachedBackend) handleCacheMiss(req AuthRepRequest, cacheKey string, c *backend.ThreeScaleClient) (CacheValue, error) {
	// instrumentation
	mc := metricsConfig{ReportFn: cb.ReportFn, Endpoint: "Auth", Target: "Backend"}
	resp, err := remoteAuth(req, map[string]string{"hierarchy": "1"}, c, mc)
	if err != nil {
		fmt.Println("error calling 3scale with blanket request")
		// need to do some handling here and potentially mark this hierarchyKey as fatal to avoid calling 3scale with repeated invalid/failing requests
		return CacheValue{}, err
	}
	cv := createEmptyCacheValue().
		setReportWith(ReportWithValues{Client: c, Request: req.Request, ServiceID: req.ServiceID}).
		setLastResponse(resp).
		setLimitsFromUsageReports(resp.GetUsageReports())

	// set the cache values for this entry
	cb.cache.Set(cacheKey, cv)
	return cv, nil
}

// handleCachedRead works on a copy of the passed CacheValue and will inform the caller if limits have
// been breached in the provided affected metrics
func (cb CachedBackend) handleCachedRead(value CacheValue, affectedMetrics backend.Metrics) (CacheValue, bool) {
	copyCache := cloneCacheValue(value)
	commitChanges := true

out:
	for metric, incrementBy := range affectedMetrics {
		cachedValue, ok := copyCache.LimitCounter[metric]
		if ok {
			for _, limit := range cachedValue {
				newValue := limit.current + incrementBy
				if newValue > limit.max {
					commitChanges = false
					break out
				}
				limit.current = newValue
			}
		} else {
			// has no limits so just cache the value for reporting purposes
			current, ok := copyCache.UnlimitedHits[metric]
			if ok {
				copyCache.UnlimitedHits[metric] = current + incrementBy
				break
			}
			copyCache.UnlimitedHits[metric] = incrementBy
		}
	}
	return copyCache, commitChanges
}

func (cv CacheValue) setLastResponse(resp backend.ApiResponse) CacheValue {
	cv.LastResponse = resp
	return cv
}

func (cv CacheValue) setReportWith(rw ReportWithValues) CacheValue {
	cv.ReportWithValues = rw
	return cv
}

func (cv CacheValue) setLimitsFromUsageReports(ur backend.UsageReports) CacheValue {
	for metric, report := range ur {
		l := &Limit{
			current:    report.CurrentValue,
			max:        report.MaxValue,
			periodEnds: report.PeriodEnd - report.PeriodStart,
		}
		cv.LimitCounter[metric] = make(map[backend.LimitPeriod]*Limit)
		cv.LimitCounter[metric][report.Period] = l
	}
	return cv
}

// computeAffectedMetrics accepts a list of metrics and hierarchy structure,
// computing additional metrics that will be affected.
// This function is responsible for walking the hierarchy tree and incrementing the parent value
// of any children that are affected by the provided metrics, in turn modifying the provided metrics
func computeAffectedMetrics(hierarchy metricParentToChildren, m backend.Metrics) backend.Metrics {
	for parent, children := range hierarchy {
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
	return m
}

func createEmptyCacheValue() CacheValue {
	limitMap := make(map[string]map[backend.LimitPeriod]*Limit)
	unlimitedMap := make(map[string]int)
	return CacheValue{
		LimitCounter:  limitMap,
		UnlimitedHits: unlimitedMap,
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

	for metric, current := range existing.UnlimitedHits {
		copyVal.UnlimitedHits[metric] = current
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

// computes a unique key from a given service id and host in the format <hierarchyKey>_<host>
func generateCacheKey(hierarchyKey string, host string) string {
	return fmt.Sprintf(cacheKeyFormat, hierarchyKey, host)
}
