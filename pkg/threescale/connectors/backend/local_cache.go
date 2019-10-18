package backend

import (
	"time"

	cmap "github.com/orcaman/concurrent-map"
)

var now = time.Now

// LocalCache is an implementation of Cacheable providing an in-memory cache
// and asynchronous reporting for 3scale backend
type LocalCache struct {
	ds       cmap.ConcurrentMap
	reporter *reporter
}

type reporter struct {
	interval time.Duration
	stop     chan struct{}
	cache    cmap.ConcurrentMap
}

// NewLocalCache returns a pointer to a LocalCache with an initialised empty data structure
func NewLocalCache(reportInterval *time.Duration, reportChan chan struct{}) *LocalCache {
	var defaultInterval = time.Second * 15

	if reportInterval == nil {
		reportInterval = &defaultInterval
	}

	cache := cmap.New()

	return &LocalCache{
		ds: cache,
		reporter: &reporter{
			interval: *reportInterval,
			stop:     reportChan,
			cache:    cache,
		},
	}
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

func (l LocalCache) Report() {
	ticker := time.NewTicker(l.reporter.interval)

	go func() {
		for {
			select {
			case <-ticker.C:
				l.reporter.cache.IterCb(func(key string, v interface{}) {
					//TODO - walk the cache and handle the reporting
				})
			case <-l.reporter.stop:
				ticker.Stop()
				return
			}
		}
	}()
}

func isExpired(limit Limit) bool {
	return now().After(time.Unix(limit.periodEnds, 0))
}
