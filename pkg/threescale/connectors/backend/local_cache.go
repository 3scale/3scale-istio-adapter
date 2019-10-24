package backend

import (
	"time"

	"github.com/3scale/3scale-go-client/client"
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

// Report cached entries to 3scale backend
func (l LocalCache) Report() {
	ticker := time.NewTicker(l.reporter.interval)
	// note - it would be nice if these were exposed as iota's to avoid doing this
	ascendingPeriodSequence := []client.LimitPeriod{client.Minute, client.Hour, client.Day, client.Month, client.Eternity}

	go func() {
		for {
			select {
			case <-ticker.C:
				l.reporter.cache.IterCb(func(key string, v interface{}) {
					reportMetrics := client.Metrics{}
					cachedValue := v.(CacheValue)

					for reportableMetric, _ := range cachedValue.LastResponse.GetUsageReports() {
						for localMetric, limitMap := range cachedValue.LimitCounter {
							if reportableMetric != localMetric {
								// we dont need to deal with this particular metric since it was learned and incremented as part of the hierarchy tree
								continue
							}
							for _, ascendingPeriod := range ascendingPeriodSequence {
								if lowestPeriod, ok := limitMap[ascendingPeriod]; ok {
									reportMetrics.Add(reportableMetric, lowestPeriod.current)
									break
								}
							}
						}
					}

					transaction := client.ReportTransactions{
						AppID:   cachedValue.Request.Application.AppID.ID,
						AppKey:  cachedValue.Request.Application.AppID.AppKey,
						UserKey: cachedValue.Request.Application.UserKey,
						Metrics: reportMetrics,
					}
					// TODO - likely want some retry here in case of network failures/ intermittent error??

					cachedValue.ReportWithValues.Client.Report(cachedValue.Request, cachedValue.ServiceID, transaction, nil)

					// todo - note we are now ignoring the hierarchy from the latest request, potentially missing out on config changes - need to figure this part out
					// but the idea is to have the reporting function pluggable so would like to avoid directly injecting the hierarchy here
					resp, err := cachedValue.ReportWithValues.Client.Authorize(cachedValue.Request, cachedValue.ServiceID, nil, map[string]string{"hierarchy": "1"})
					if err != nil {
						//todo - handle failures
					}

					cacheCopy := cloneCacheValue(cachedValue)
					// update the latest state after reporting what we have
					ur := resp.GetUsageReports()
					for metric, report := range ur {
						l := &Limit{
							current:    report.CurrentValue,
							max:        report.MaxValue,
							periodEnds: report.PeriodEnd - report.PeriodStart,
						}
						if _, ok := cacheCopy.LimitCounter[metric]; !ok {
							cacheCopy.LimitCounter[metric] = make(map[client.LimitPeriod]*Limit)
						}
						cacheCopy.LimitCounter[metric][report.Period] = l
					}
					cacheCopy.setLastResponse(resp).setReportWith(cachedValue.ReportWithValues)
					l.Set(key, cacheCopy)
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
