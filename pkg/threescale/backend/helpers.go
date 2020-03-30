package backend

import (
	"fmt"
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

func contains(key string, in []string) bool {
	for _, value := range in {
		if value == key {
			return true
		}
	}
	return false
}

// newApplication creates a new, empty application with maps initialised
func newApplication() *Application {
	return &Application{
		RemoteState:      nil,
		LocalState:       make(LimitCounter),
		UnlimitedCounter: make(map[string]int),
	}
}
