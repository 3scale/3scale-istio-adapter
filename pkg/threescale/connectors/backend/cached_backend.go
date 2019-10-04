package backend

import (
	"fmt"
	"net/http"
	"sync"

	backend "github.com/3scale/3scale-go-client/client"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale/metrics"
	cmap "github.com/orcaman/concurrent-map"
)

const (
	cacheKeySeparator = "_"
	cacheKey          = "%s" + cacheKeySeparator + "%s"
)

// Cacheable - defines the required behaviour of a Backend cache
type Cacheable interface {
	Get(cacheKey string) (CacheValue, bool)
	Set(cacheKey string, cv CacheValue)
}

// Limit captures the current state of the rate limit for a particular time period
type Limit struct {
	current int
	max     int
	// unix timestamp for end period
	periodEnds int64
}

// CacheValue which should be stored in cache implementation
type CacheValue map[string]map[backend.LimitPeriod]*Limit

// CachedBackend provides a pluggable cache enabled 'Backend' implementation
// Supports reporting metrics for non-cached responses.
type CachedBackend struct {
	ReportFn      metrics.ReportMetricsFn
	cache         Cacheable
	mutex         sync.RWMutex
	hierarchyTree hierarchyTree
}

type LocalCache struct {
	ds cmap.ConcurrentMap
}

// map to represent parent --> children metric relationship
type metricParentToChildren map[string][]string

// hierarchyTree - represents the relationship between service metrics and their children
type hierarchyTree map[string]metricParentToChildren

// NewCachedBackend returns a pointer to a CachedBackend with a default LocalCache if no custom implementation has been provided
func NewCachedBackend(cache Cacheable, rfn metrics.ReportMetricsFn) *CachedBackend {
	if cache == nil {
		cache = NewLocalCache()
	}
	return &CachedBackend{
		ReportFn:      rfn,
		cache:         cache,
		hierarchyTree: make(hierarchyTree),
	}
}

// NewLocalCache returns a pointer to a LocalCache with an initialised empty data structure
func NewLocalCache() *LocalCache {
	return &LocalCache{ds: cmap.New()}
}

// Get entries for LocalCache
func (l LocalCache) Get(cacheKey string) (CacheValue, bool) {
	var cv CacheValue
	v, ok := l.ds.Get(cacheKey)
	if !ok {
		return cv, ok
	}

	cv = v.(CacheValue)
	return cv, ok
}

// Set entries for LocalCache
func (l LocalCache) Set(cacheKey string, cv CacheValue) {
	l.ds.Set(cacheKey, cv)
}

// AuthRep provides a combination of authorizing a request and reporting metrics to 3scale
func (cb CachedBackend) AuthRep(req AuthRepRequest, c *backend.ThreeScaleClient) (Response, error) {
	// compute the cache key for this request
	hierarchyKey := generateHierarchyKey(req.ServiceID, c.GetPeer())
	appIdentifier := generateAppIdentifier(req)
	cacheKey := generateCacheKey(hierarchyKey, appIdentifier)

	var affectedMetrics backend.Metrics
	// gather the metrics affected by this request using the hierarchy information we might have gathered
	affectedMetrics, ok := cb.computeAffectedMetrics(hierarchyKey, req.Params.Metrics)

	// instrumentation
	mc := metricsConfig{ReportFn: cb.ReportFn, Endpoint: "AuthRep", Target: "Backend"}

	if !ok {
		// !! CACHE MISS - we don't know about this service id and host combination!!
		// we need to build hierarchy map blanket call to authRep with extensions enabled and no usage reported
		resp, err := callRemote(req, map[string]string{"hierarchy": "1"}, c, mc)
		if err != nil {
			fmt.Println("error calling 3scale with blanket request")
			// need to do some handling here and potentially mark this hierarchyKey as fatal to avoid calling 3scale with repeated invalid/failing requests
			return Response{}, err
		}
		// populate the hierarchy tree
		cb.setTreeEntry(hierarchyKey, resp.GetHierarchy())
		// recompute new metrics
		affectedMetrics, _ = cb.computeAffectedMetrics(hierarchyKey, req.Params.Metrics)

		cv := make(CacheValue)
		ur := resp.GetUsageReports()
		for metric, report := range ur {
			l := &Limit{
				current:    report.CurrentValue,
				max:        report.MaxValue,
				periodEnds: report.PeriodEnd - report.PeriodStart,
			}
			cv[metric] = make(map[backend.LimitPeriod]*Limit)
			cv[metric][report.Period] = l
		}
		// set the cache values for this entry
		cb.cache.Set(cacheKey, cv)
	}

	cv, ok := cb.cache.Get(cacheKey)
	if !ok {
		return Response{}, fmt.Errorf("error fetching cached value")
	}

	copyCache := copyCacheValue(cv)
	commitChanges := true

out:
	for metric, incrementBy := range affectedMetrics {
		if cachedValue, ok := copyCache[metric]; ok {
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

	if !commitChanges {
		return Response{
			Reason:     "Limits exceeded",
			StatusCode: http.StatusTooManyRequests,
			Success:    false,
		}, nil
	}

	cb.cache.Set(cacheKey, copyCache)
	resp := Response{
		StatusCode: http.StatusOK,
		Success:    true,
	}

	return resp, nil
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

func copyCacheValue(existing CacheValue) CacheValue {
	copy := make(CacheValue, len(existing))
	for k, v := range existing {
		copyNested := make(map[backend.LimitPeriod]*Limit)
		for nestedK, nestedV := range v {
			var limit Limit
			limit = *nestedV
			copyNested[nestedK] = &limit
		}
		copy[k] = copyNested
	}
	return copy
}
