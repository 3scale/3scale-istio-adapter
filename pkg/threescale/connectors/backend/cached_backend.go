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

// CacheValue which should be stored in cache implementation maps an
// application identifier to an Application which houses information  required for caching purpose
type CacheValue map[string]*Application

// Application defined under a 3scale service
type Application struct {
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
	appID := cb.getAppIdFromRequest(req)

	cv, ok := cb.cache.Get(cacheKey)
	if !ok || cv[appID] == nil {
		// we consider it a cache miss in two situations, either we know nothing about this host and service id
		// or we have seen this but have no information for this particular application
		newlyCachedValue, err := cb.handleCacheMiss(req, cacheKey, appID, c)
		if err != nil {
			return Response{}, err
		}
		cv = newlyCachedValue
	}

	affectedMetrics := computeAffectedMetrics(cv[appID].LastResponse.GetHierarchy(), req.Params.Metrics)
	copiedCacheValue, shouldCommit := cb.handleCachedRead(cv, appID, affectedMetrics)
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
func (cb CachedBackend) handleCacheMiss(req AuthRepRequest, cacheKey string, appID string, c *backend.ThreeScaleClient) (CacheValue, error) {
	// instrumentation
	mc := metricsConfig{ReportFn: cb.ReportFn, Endpoint: "Auth", Target: "Backend"}
	resp, err := remoteAuth(req, map[string]string{"hierarchy": "1"}, c, mc)
	if err != nil {
		fmt.Println("error calling 3scale with blanket request")
		// need to do some handling here and potentially mark this hierarchyKey as fatal to avoid calling 3scale with repeated invalid/failing requests
		return CacheValue{}, err
	}
	cv := createEmptyCacheValue().
		setApplication(appID, nil).
		setReportWith(appID, ReportWithValues{Client: c, Request: req.Request, ServiceID: req.ServiceID}).
		setLastResponse(appID, resp).
		setLimitsFromUsageReports(appID, resp.GetUsageReports())

	// set the cache values for this entry
	cb.cache.Set(cacheKey, cv)
	return cv, nil
}

// handleCachedRead works on a copy of the passed CacheValue and will inform the caller if limits have
// been breached in the provided affected metrics
func (cb CachedBackend) handleCachedRead(value CacheValue, appID string, affectedMetrics backend.Metrics) (CacheValue, bool) {
	copyCache := cloneCacheValue(value)
	commitChanges := true

out:
	for metric, incrementBy := range affectedMetrics {
		cachedValue, ok := copyCache[appID].LimitCounter[metric]
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
			current, ok := copyCache[appID].UnlimitedHits[metric]
			if ok {
				copyCache[appID].UnlimitedHits[metric] = current + incrementBy
				break
			}
			copyCache[appID].UnlimitedHits[metric] = incrementBy
		}
	}
	return copyCache, commitChanges
}

// getAppIdFromRequest prioritizes and returns a user key if present, defaulting to app id otherwise
func (cb CachedBackend) getAppIdFromRequest(req AuthRepRequest) string {
	if appID := req.Request.Application.UserKey; appID != "" {
		return appID
	}

	return req.Request.Application.AppID.ID
}

// setApplication for a particular appID
// if applications value is nil, a new application will be instantiated
func (cv CacheValue) setApplication(appID string, application *Application) CacheValue {
	if application == nil {
		application = &Application{
			LimitCounter:  make(map[string]map[backend.LimitPeriod]*Limit),
			UnlimitedHits: make(map[string]int),
		}
	}
	cv[appID] = application
	return cv
}

func (cv CacheValue) setLastResponse(appID string, resp backend.ApiResponse) CacheValue {
	cv[appID].LastResponse = resp
	return cv
}

func (cv CacheValue) setReportWith(appID string, rw ReportWithValues) CacheValue {
	cv[appID].ReportWithValues = rw
	return cv
}

func (cv CacheValue) setLimitsFromUsageReports(appID string, ur backend.UsageReports) CacheValue {
	for metric, report := range ur {
		l := &Limit{
			current:    report.CurrentValue,
			max:        report.MaxValue,
			periodEnds: report.PeriodEnd - report.PeriodStart,
		}
		cv[appID].LimitCounter[metric] = make(map[backend.LimitPeriod]*Limit)
		cv[appID].LimitCounter[metric][report.Period] = l
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
	return make(map[string]*Application)
}

func cloneCacheValue(existing CacheValue) CacheValue {
	copyVal := createEmptyCacheValue()
	for appIdentifier, app := range existing {
		copyVal.setApplication(appIdentifier, app)
		copyVal.setLastResponse(appIdentifier, app.LastResponse).setReportWith(appIdentifier, app.ReportWithValues)

		for metric, periodMap := range app.LimitCounter {
			copyNested := make(map[backend.LimitPeriod]*Limit)
			for period, limit := range periodMap {
				var limitClone Limit
				limitClone = *limit
				copyNested[period] = &limitClone
			}
			copyVal[appIdentifier].LimitCounter[metric] = copyNested
		}

		for metric, current := range app.UnlimitedHits {
			copyVal[appIdentifier].UnlimitedHits[metric] = current
		}
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
