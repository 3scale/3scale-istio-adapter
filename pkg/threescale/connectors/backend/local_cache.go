package backend

import (
	"time"

	"github.com/3scale/3scale-go-client/client"
	"github.com/orcaman/concurrent-map"
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
	ascendingPeriodSequence := []client.LimitPeriod{client.Minute, client.Hour, client.Day, client.Week, client.Month, client.Eternity}
	reportMetrics := client.Metrics{}
	for {
		select {
		case <-ticker.C:

			l.reporter.cache.IterCb(func(key string, v interface{}) {
				cachedValueClone := cloneCacheValue(v.(CacheValue))

				for appID, application := range cachedValueClone {
					// report unlimited metrics without checking hierarchy
					for unlimitedMetric, reportWithValue := range application.UnlimitedHits {
						reportMetrics[unlimitedMetric] = reportWithValue
					}

					// walk over our metrics with limits attached and reduce the reporting value by our last previous saved state
					lastReports := application.LastResponse.GetUsageReports()
					reportMetrics = l.subtractUsageReportFromState(application, ascendingPeriodSequence, reportMetrics, lastReports)

					// now we have almost correct state but to avoid over reporting, we need to take the hierarchy into account
					parentsChildren := application.LastResponse.GetHierarchy()
					reportMetrics = l.subtractChildHitsFromParent(reportMetrics, parentsChildren)

					transaction := client.ReportTransactions{
						AppID:   application.Request.Application.AppID.ID,
						AppKey:  application.Request.Application.AppID.AppKey,
						UserKey: application.Request.Application.UserKey,
						Metrics: reportMetrics,
					}
					// TODO - likely want some retry here in case of network failures/ intermittent error??
					_, err := application.ReportWithValues.Client.Report(application.Request, application.ServiceID, transaction, nil)
					if err != nil {
						//todo logging
						return
					}

					resp, err := application.ReportWithValues.Client.Authorize(application.Request, application.ServiceID, nil, map[string]string{"hierarchy": "1"})
					if err != nil {
						//todo logging
						return
					}

					// reset the state of the cache
					cv := createEmptyCacheValue().
						setApplication(appID, application).
						setReportWith(appID, application.ReportWithValues).
						setLastResponse(appID, resp).
						setLimitsFromUsageReports(appID, resp.GetUsageReports())

					go l.Set(key, cv)
				}

			})
		case <-l.reporter.stop:
			ticker.Stop()
			return
		}
	}
}

// GetStopChan returns the channel which can be closed to stop the reporting background process
func (l LocalCache) GetStopChan() chan struct{} {
	return l.reporter.stop
}

// subtractUsageReportFromState of a preexisting application taking into account a given sequence which should be prioritised
// walks over our metrics with limits attached and reduce the reporting value by our last previous saved state
func (l LocalCache) subtractUsageReportFromState(application *Application, sequence []client.LimitPeriod, metrics client.Metrics, reports client.UsageReports) client.Metrics {
	for limitedMetric, limitMap := range application.LimitCounter {
		for _, nextPeriod := range sequence {
			if lowestPeriod, ok := limitMap[nextPeriod]; ok {
				metrics[limitedMetric] = lowestPeriod.current - reports[limitedMetric].CurrentValue
				break
			}
		}
	}
	return metrics
}

// subtractUsageReportFromState of a preexisting application taking into account a given sequence which should be prioritised
// walks over our metrics with limits attached and reduce the reporting value by our last previous saved state
func (l LocalCache) subtractChildHitsFromParent(metrics client.Metrics, hierarchy metricParentToChildren) client.Metrics {
	for metric, _ := range metrics {
		// check if each metric is a parent
		if children, hasChildren := hierarchy[metric]; hasChildren {
			// if its a parent pull out its children's values, if any
			for _, child := range children {
				if childValue, reportExists := metrics[child]; reportExists {
					// negate the child value from parent
					metrics[metric] -= childValue
				}
			}
		}
	}
	return metrics
}
