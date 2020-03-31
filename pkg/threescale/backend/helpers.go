package backend

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/3scale/3scale-go-client/threescale"
	"github.com/3scale/3scale-go-client/threescale/api"
)

// emptyTransactionFrom is used when we need to gather state from the remote backend
// it parses the transactions for their auth and augments the request to provide nil metrics
func emptyTransactionFrom(transaction api.Transaction) api.Transaction {
	return api.Transaction{
		Metrics:   nil,
		Params:    transaction.Params,
		Timestamp: transaction.Timestamp,
	}
}

func computeAffectedMetrics(app *Application, request threescale.Request) api.Metrics {
	return request.Transactions[0].Metrics.AddHierarchyToMetrics(app.metricHierarchy)
}

// getApplicationFromResponse builds an Application from a response from 3scale client
func getApplicationFromResponse(resp *threescale.AuthorizeResult) Application {
	resp.UsageReports.OrderByAscendingGranularity()

	counters := LimitCounter(resp.UsageReports)

	return Application{
		RemoteState:      counters,
		LocalState:       counters.deepCopy(),
		UnlimitedCounter: make(map[string]int),
		metricHierarchy:  resp.AuthorizeExtensions.Hierarchy,
		timestamp:        deriveTimestamp(resp.UsageReports),
	}
}

func deriveTimestamp(reports api.UsageReports) int64 {
	var timestamp int64
	var lowestKnownGranularity *api.UsageReport

	for _, report := range reports {
		lowestGranularity := report[0]
		if lowestKnownGranularity == nil {
			lowestKnownGranularity = &lowestGranularity
		}

		if lowestGranularity.PeriodWindow.Period < lowestKnownGranularity.PeriodWindow.Period {
			lowestKnownGranularity = &lowestGranularity
		}

		timestamp = lowestGranularity.PeriodWindow.Start

		if lowestGranularity.PeriodWindow.Period == api.Minute {
			break
		}
	}

	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	return timestamp
}

// getAppIdFromTransaction prioritizes and returns a user key if present, defaulting to app id otherwise
func getAppIDFromTransaction(transaction api.Transaction) string {
	appID := transaction.Params.UserKey

	if appID != "" {
		return appID
	}
	return transaction.Params.AppID
}

func generateCacheKeyFromRequest(request threescale.Request, transactionIndex int) string {
	return fmt.Sprintf("%s_%s", request.GetServiceID(), getAppIDFromTransaction(request.Transactions[transactionIndex]))
}

// getEmptyAuthRequest is a helper method to return a request suitable for a blanket auth request
func getEmptyAuthRequest(service api.Service, auth api.ClientAuth, params api.Params) threescale.Request {
	return threescale.Request{
		Auth:       auth,
		Extensions: api.Extensions{api.HierarchyExtension: "1"},
		Service:    service,
		Transactions: []api.Transaction{
			{
				Params: params,
			},
		},
	}
}

func parseCacheKey(cacheKey string) (service api.Service, application string, err error) {
	parsed := strings.Split(cacheKey, "_")

	if len(parsed) != 2 {
		return service, application, fmt.Errorf("error parsing key")
	}

	service, application = api.Service(parsed[0]), parsed[1]

	if service == "" || application == "" {
		return service, application, fmt.Errorf("error parsing key. empty service or application")
	}

	return service, application, nil
}

func validateTransactions(transactions []api.Transaction) error {
	if transactions == nil || len(transactions) == 0 {
		return fmt.Errorf("transaction must be non-nil and non-empty")
	}
	return nil
}

func computeAddedAndRemovedMetrics(src, dst LimitCounter) (added, removed []string) {
	for metric, _ := range dst {
		if _, wasKnown := src[metric]; !wasKnown {
			added = append(added, metric)
			continue
		}
	}

	for metric, _ := range src {
		if _, stillExists := dst[metric]; !stillExists {
			removed = append(removed, metric)
			continue
		}
	}

	return added, removed
}

// synchronizeState updates the original limit counter based on the current state of the new counter
// any additional limits in the new counter will be added to the original and any existing limits in the
// original that have been removed from the new state, will be removed from the source.
func synchronizeStates(original, new LimitCounter) LimitCounter {
	for metric, reports := range original {
		if newReports, contains := new[metric]; contains {
			// we first grab what has been removed from the new state because we need to prune these
			getRemovals := getDifferenceBetweenSets(reports, newReports)
			// do a reverse of the sorted slice so we can safely prune from the back of the list without
			// affecting the order of our original state, those entries may be valid so take from the tail
			sort.Reverse(sort.IntSlice(getRemovals))
			for _, i := range getRemovals {
				// remove the element at index i
				original[metric] = append(original[metric][:i], original[metric][i+1:]...)
			}

			// we npw can grab what was added
			getAdditions := getDifferenceBetweenSets(newReports, reports)
			for _, i := range getAdditions {
				original[metric] = append(original[metric], new[metric][i])
			}

		}
	}
	api.UsageReports(original).OrderByAscendingGranularity()
	return original
}

// getDifferenceBetweenSets returns the index of the usage reports whose time periods exist
// in the source, that are not present in the destination
func getDifferenceBetweenSets(src, dst []api.UsageReport) []int {
	var indexes []int

	for index, report := range src {
		found := false
		for _, dstReport := range dst {
			if report.PeriodWindow.Period == dstReport.PeriodWindow.Period {
				found = true
				break
			}
		}

		if !found {
			indexes = append(indexes, index)
		}
	}

	return indexes
}

func contains(key string, in []string) bool {
	for _, value := range in {
		if value == key {
			return true
		}
	}
	return false
}

// todo - this function is currently safe to use because we only provide it with timestamps and
// deadlines that have been learned from 3scale backend. This means they are already correctly set
// at the beginning of a specific period. That is to say, for example f(hour, 12:23:39) => 12:00:00 to 12:59:59
// an hour has been passed through func f and its timestamp set to 12:00:00 and deadline to 12:59:59
// This works because we only check deadlines on successful auth responses. *When* we provide adjustments for other
// use cases, f(granularity api.Period, timestamp int64) will need to be created.
func hasSurpassedDeadline(timestamp int64, granularity api.Period, deadline time.Time) bool {
	switch granularity {
	case api.Minute:
		return time.Unix(timestamp, 0).Add(time.Minute).After(deadline)
	case api.Hour:
		return time.Unix(timestamp, 0).Add(time.Hour).After(deadline)
	case api.Day:
		return time.Unix(timestamp, 0).AddDate(0, 0, 1).After(deadline)
	case api.Week:
		// todo - needs revisiting as may not be technically correct with the way 3scale handles weeks
		// time.Unix(timestamp, 0).AddDate(0, 0, 7).After(deadline)
		return false
	case api.Month:
		return time.Unix(timestamp, 0).AddDate(0, 1, 0).After(deadline)
	case api.Year:
		return time.Unix(timestamp, 0).AddDate(1, 0, 0).After(deadline)
	case api.Eternity:
		return false
	default:
		return false
	}
}

// newApplication creates a new, empty application with maps initialised
func newApplication() *Application {
	return &Application{
		RemoteState:      nil,
		LocalState:       make(LimitCounter),
		UnlimitedCounter: make(map[string]int),
	}
}
