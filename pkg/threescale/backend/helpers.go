package backend

import (
	"fmt"
	"strings"

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
	limitCounter := make(LimitCounter)

	for metric, report := range resp.UsageReports {
		l := &Limit{
			current: report.CurrentValue,
			max:     report.MaxValue,
		}
		limitCounter[metric] = make(map[api.Period]*Limit)
		limitCounter[metric][report.PeriodWindow.Period] = l
	}

	return Application{
		RemoteState:      limitCounter.deepCopy(),
		LimitCounter:     limitCounter,
		UnlimitedCounter: make(map[string]int),
		metricHierarchy:  resp.AuthorizeExtensions.Hierarchy,
	}
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

func parseCacheKey(cacheKey string) (service string, application string, err error) {
	parsed := strings.Split(cacheKey, "_")

	if len(parsed) != 2 {
		return service, application, fmt.Errorf("error parsing key")
	}

	service, application = parsed[0], parsed[1]

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
		LimitCounter:     make(LimitCounter),
		UnlimitedCounter: make(map[string]int),
	}
}
