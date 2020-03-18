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
	for _, report := range reports {
		if lowestGranularity := report[0]; lowestGranularity.PeriodWindow.Period == api.Minute {
			timestamp = lowestGranularity.PeriodWindow.Start
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

// newApplication creates a new, empty application with maps initialised
func newApplication() *Application {
	return &Application{
		RemoteState:      nil,
		LocalState:       make(LimitCounter),
		UnlimitedCounter: make(map[string]int),
	}
}

func hasSurpassedDeadline(timestamp int64, granularity api.Period, deadline time.Time) bool {
	switch granularity {
	case api.Minute:
		return time.Unix(timestamp, 0).Add(time.Minute).After(deadline)
	case api.Hour:
		return time.Unix(timestamp, 0).Add(time.Hour).After(deadline)
	case api.Day:
		return time.Unix(timestamp, 0).AddDate(0, 0, 1).After(deadline)
	case api.Week:
		// todo, here be dragons spawned by the forefathers
		//    __        _
		//  _/  \    _(\(o
		//  /     \  /  _  ^^^o
		//  /   !   \/  ! '!!!v'
		//  !  !  \ _' ( \____
		//  ! . \ _!\   \===^\)
		//  \ \_!  / __!
		//  \!   /    \
		//  (\_      _/   _\ )
		//  \ ^^--^^ __-^ /(__
		//  ^^----^^    "^--v'
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

// statesAreComparable allows us to work with an applications counter and ensure that it makes sense to do calculations
// on the provided index and metric. It also ensures we aren't duplicating code and checking for panics in the calling funcs
// We can safely call this function with any index and know we wont panic. If the function returns false, the provided
// args are unusable and we should handle in the caller appropriately
// TODO - this may end up returning an error instead to provide more context if we need it for others purposes (eg logging)
func statesAreComparable(metric string, index int, state, compareTo LimitCounter) bool {
	localCounters, ok := state[metric]
	if !ok || (len(localCounters)-1) < index {
		return false
	}

	remoteCounter, ok := compareTo[metric]
	if !ok || (len(remoteCounter)-1) < index {
		return false
	}

	if localCounters[index].PeriodWindow.Period != remoteCounter[index].PeriodWindow.Period {
		return false
	}
	return true
}

func getDifferenceAsMetrics(subtract, from LimitCounter) api.Metrics {
	metrics := make(api.Metrics)
	// ensure lists are sorted prior to calculation
	api.UsageReports(from).OrderByAscendingGranularity()
	api.UsageReports(subtract).OrderByAscendingGranularity()

	for metric, counters := range from {
		// we want to handle the lowest granularity only here
		if canHandle := statesAreComparable(metric, 0, subtract, from); !canHandle {
			// TODO - add logging
			continue
		}

		lowestFromGranularity := counters[0]
		lowestSubtractGranularity := subtract[metric][0]

		if lowestFromGranularity.PeriodWindow.Period != lowestSubtractGranularity.PeriodWindow.Period {
			continue
		}

		toReport := lowestFromGranularity.CurrentValue - lowestSubtractGranularity.CurrentValue
		metrics.Add(metric, toReport)
	}
	return metrics
}

func incrementCountersCurrentValue(counter *api.UsageReport, incrementBy int) {
	counter.CurrentValue += incrementBy
}
