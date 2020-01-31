package backend

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/3scale/3scale-go-client/threescale"
	"github.com/3scale/3scale-go-client/threescale/api"
)

func TestBackend_Authorize(t *testing.T) {
	const cacheKey = "test_application"

	inputs := []struct {
		name         string
		setup        func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend
		request      threescale.Request
		expectError  bool
		expectResult *threescale.AuthorizeResult
	}{
		{
			name: "Test error when no transactions are provided",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
			},
			expectError: true,
		},
		{
			name: "Test cache miss and error from 3scale, can't fetch hierarchy returns error",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.authErr(t, fmt.Errorf("some arbitary error"))
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "Test failure on metrics with no parents that breach the limits - cache miss",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				// we want to ensure the conversion is done here and the cache is populated correctly
				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy: make(api.Hierarchy),
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				remoteClient.setAuthResponse(t, remoteResponse)
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"hits": 2},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectResult: &threescale.AuthorizeResult{Authorized: false},
		},
		{
			name: "Test failure on metrics with no parents that breach the limits - cache hit",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  make(api.Hierarchy),
						RateLimits: nil,
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}
				fullRemoteState := getApplicationFromResponse(remoteResponse)

				lc := make(LimitCounter)
				lc["hits"] = make(map[api.Period]*Limit)
				lc["hits"][api.Minute] = &Limit{
					current: 3,
					max:     4,
				}

				app := &Application{
					RemoteState:      fullRemoteState.RemoteState,
					LimitCounter:     lc,
					UnlimitedCounter: make(map[string]int),
				}

				cacheable.Set(cacheKey, app)

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"hits": 2},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectResult: &threescale.AuthorizeResult{Authorized: false},
		},
		{
			name: "Test failure when hierarchy should cause a breach of limits - cache miss",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				// we want to ensure the conversion is done here and the cache is populated correctly
				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				remoteClient.setAuthResponse(t, remoteResponse)
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"hits_child": 2},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectResult: &threescale.AuthorizeResult{Authorized: false},
		},
		{
			name: "Test success case",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				// we want to ensure the conversion is done here and the cache is populated correctly
				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				remoteClient.setAuthResponse(t, remoteResponse)
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"orphan": 2, "hits": 1},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectResult: &threescale.AuthorizeResult{Authorized: true},
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			remoteClient := &mockRemoteClient{}
			b := input.setup(NewLocalCache(), remoteClient)

			resp, err := b.Authorize(input.request)
			if err != nil {
				if !input.expectError {
					t.Errorf("unexpeced error - %s", err.Error())
				}
				return
			}
			equals(t, resp.Authorized, input.expectResult.Authorized)
			equals(t, resp.Hierarchy, api.Hierarchy(nil))
			equals(t, resp.UsageReports, api.UsageReports(nil))
			equals(t, resp.RateLimits, (*api.RateLimits)(nil))
		})
	}
}

func TestBackend_AuthRep(t *testing.T) {
	const cacheKey = "test_application"

	inputs := []struct {
		name             string
		setup            func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend
		request          threescale.Request
		expectError      bool
		expectResult     *threescale.AuthorizeResult
		expectCacheState Application
	}{
		{
			name: "Test error when no transactions are provided",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
			},
			expectError: true,
		},
		{
			name: "Test cache miss and error from 3scale, can't fetch hierarchy returns error",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.authErr(t, fmt.Errorf("some arbitary error"))
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "Test failure on metrics with no parents that breach the limits - cache miss",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				// we want to ensure the conversion is done here and the cache is populated correctly
				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  make(api.Hierarchy),
						RateLimits: nil,
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				remoteClient.setAuthResponse(t, remoteResponse)
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"hits": 2},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectResult: &threescale.AuthorizeResult{Authorized: false},
		},
		{
			name: "Test failure on metrics with no parents that breach the limits - cache hit",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {

				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  make(api.Hierarchy),
						RateLimits: nil,
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				fullRemoteState := getApplicationFromResponse(remoteResponse)

				lc := make(LimitCounter)
				lc["hits"] = make(map[api.Period]*Limit)
				lc["hits"][api.Minute] = &Limit{
					current: 3,
					max:     4,
				}

				app := &Application{
					RemoteState:      fullRemoteState.LimitCounter,
					LimitCounter:     lc,
					UnlimitedCounter: make(map[string]int),
				}

				cacheable.Set(cacheKey, app)

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"hits": 2},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectResult: &threescale.AuthorizeResult{Authorized: false},
		},
		{
			name: "Test failure when hierarchy should cause a breach of limits - cache miss",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				// we want to ensure the conversion is done here and the cache is populated correctly
				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				remoteClient.setAuthResponse(t, remoteResponse)
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"hits_child": 2},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectResult: &threescale.AuthorizeResult{Authorized: false},
		},
		{
			name: "Test success case cache miss",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				// we want to ensure the conversion is done here and the cache is populated correctly
				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				remoteClient.setAuthResponse(t, remoteResponse)
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"orphan": 2, "hits": 1},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectResult: &threescale.AuthorizeResult{Authorized: true},
			expectCacheState: Application{
				auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				params: api.Params{
					AppID: "application",
				},
				RemoteState: newLimitCounter(t, "hits", api.Minute, &Limit{
					current: 3,
					max:     4,
				}),
				LimitCounter: newLimitCounter(t, "hits", api.Minute, &Limit{
					current: 4,
					max:     4,
				}),
				UnlimitedCounter: map[string]int{"orphan": 2},
			},
		},
		{
			name: "Test success case cache hit",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				fullRemoteState := getApplicationFromResponse(remoteResponse)

				lc := make(LimitCounter)
				lc["hits"] = make(map[api.Period]*Limit)
				lc["hits"][api.Minute] = &Limit{
					current: 3,
					max:     4,
				}

				app := &Application{
					auth: api.ClientAuth{
						Type:  api.ProviderKey,
						Value: "any",
					},
					params: api.Params{
						AppID: "application",
					},
					RemoteState:      fullRemoteState.RemoteState,
					LimitCounter:     lc,
					UnlimitedCounter: map[string]int{"orphan": 1},
				}

				cacheable.Set(cacheKey, app)

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"orphan": 2, "hits": 1},
						Params: api.Params{
							UserKey: "application",
						},
					},
				},
			},
			expectResult: &threescale.AuthorizeResult{Authorized: true},
			expectCacheState: Application{
				auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				params: api.Params{
					AppID: "application",
				},
				RemoteState: newLimitCounter(t, "hits", api.Minute, &Limit{
					current: 3,
					max:     4,
				}),
				LimitCounter: newLimitCounter(t, "hits", api.Minute, &Limit{
					current: 4,
					max:     4,
				}),
				UnlimitedCounter: map[string]int{"orphan": 3},
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			remoteClient := &mockRemoteClient{}
			b := input.setup(NewLocalCache(), remoteClient)

			resp, err := b.AuthRep(input.request)
			if err != nil {
				if !input.expectError {
					t.Errorf("unexpeced error - %s", err.Error())
				}
				return
			}

			equals(t, resp.Authorized, input.expectResult.Authorized)
			equals(t, resp.Hierarchy, api.Hierarchy(nil))
			equals(t, resp.UsageReports, api.UsageReports(nil))
			equals(t, resp.RateLimits, (*api.RateLimits)(nil))

			cachedVal, _ := b.cache.Get(cacheKey)

			if resp.Authorized {
				equals(t, input.expectCacheState.UnlimitedCounter, cachedVal.UnlimitedCounter)
				equals(t, input.expectCacheState.LimitCounter, cachedVal.LimitCounter)
				equals(t, input.expectCacheState.RemoteState, cachedVal.RemoteState)

				equals(t, cachedVal.params, input.expectCacheState.params)
				equals(t, cachedVal.auth, input.expectCacheState.auth)
			}
		})
	}
}

func TestBackend_Report(t *testing.T) {
	const cacheKey = "test_application"

	inputs := []struct {
		name                              string
		setup                             func(cacheable *mockCache, remoteClient *mockRemoteClient) *Backend
		request                           threescale.Request
		expectError                       bool
		expectProcessedTransactionCounter int
		expectCacheState                  Application
	}{
		{
			name: "Test error when no transactions are provided",
			setup: func(cacheable *mockCache, remoteClient *mockRemoteClient) *Backend {
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
			},
			expectError: true,
		},
		{
			name: "Test cache miss and error from 3scale, can't fetch hierarchy",
			setup: func(cacheable *mockCache, remoteClient *mockRemoteClient) *Backend {
				remoteClient.authErr(t, fmt.Errorf("some arbitary error"))
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
		},
		{
			name: "Test cache miss, can fetch hierarchy from 3scale, entry is set",
			setup: func(cacheable *mockCache, remoteClient *mockRemoteClient) *Backend {

				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy: api.Hierarchy{"hits": []string{"hits_child"}},
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				remoteClient.setAuthResponse(t, remoteResponse)
				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"hits": 2, "orphan": 2},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			// we expect two here because we call out for hierarchy which will set the entry in the cache
			expectProcessedTransactionCounter: 2,
			expectCacheState: Application{
				auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				params: api.Params{
					AppID: "application",
				},
				RemoteState: newLimitCounter(t, "hits", api.Minute, &Limit{
					current: 3,
					max:     4,
				}),
				LimitCounter: newLimitCounter(t, "hits", api.Minute, &Limit{
					current: 5,
					max:     4,
				}),
				UnlimitedCounter: map[string]int{"orphan": 2},
			},
		},
		{
			name: "Test cache hit and correct reporting, no hierarchy",
			setup: func(cacheable *mockCache, remoteClient *mockRemoteClient) *Backend {

				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy: api.Hierarchy{"hits": []string{"hits_child"}},
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				fullRemoteState := getApplicationFromResponse(remoteResponse)

				lc := make(LimitCounter)
				lc["hits"] = make(map[api.Period]*Limit)
				lc["hits"][api.Minute] = &Limit{
					current: 3,
					max:     4,
				}

				app := &Application{
					RemoteState:      fullRemoteState.RemoteState,
					LimitCounter:     lc,
					UnlimitedCounter: map[string]int{"orphan": 1},
				}

				cacheable.Set(cacheKey, app)
				// reset the counter
				cacheable.setCounter = 0

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"hits": 2, "orphan": 2},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectProcessedTransactionCounter: 1,
			expectCacheState: Application{
				RemoteState: newLimitCounter(t, "hits", api.Minute, &Limit{
					current: 3,
					max:     4,
				}),
				LimitCounter: newLimitCounter(t, "hits", api.Minute, &Limit{
					current: 5,
					max:     4,
				}),
				UnlimitedCounter: map[string]int{"orphan": 3},
			},
		},
		{
			name: "Test cache hit and correct reporting, with hierarchy",
			setup: func(cacheable *mockCache, remoteClient *mockRemoteClient) *Backend {

				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
						UsageReports: api.UsageReports{
							"hits": api.UsageReport{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
				}

				fullRemoteState := getApplicationFromResponse(remoteResponse)

				lc := make(LimitCounter)
				lc["hits"] = make(map[api.Period]*Limit)
				lc["hits"][api.Minute] = &Limit{
					current: 3,
					max:     4,
				}

				app := &Application{
					RemoteState:      fullRemoteState.RemoteState,
					LimitCounter:     lc,
					UnlimitedCounter: map[string]int{"orphan": 1},
					metricHierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
				}

				cacheable.Set(cacheKey, app)
				// reset the counter
				cacheable.setCounter = 0

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
				}
			},
			request: threescale.Request{
				Auth: api.ClientAuth{
					Type:  api.ProviderKey,
					Value: "any",
				},
				Service: "test",
				Transactions: []api.Transaction{
					{
						Metrics: api.Metrics{"hits": 2, "orphan": 2, "hits_child": 2},
						Params: api.Params{
							AppID: "application",
						},
					},
				},
			},
			expectProcessedTransactionCounter: 1,
			expectCacheState: Application{
				RemoteState: newLimitCounter(t, "hits", api.Minute, &Limit{
					current: 3,
					max:     4,
				}),
				LimitCounter: newLimitCounter(t, "hits", api.Minute, &Limit{
					current: 7,
					max:     4,
				}),
				UnlimitedCounter: map[string]int{"orphan": 3, "hits_child": 2},
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			remoteClient := &mockRemoteClient{
				repRes: &threescale.ReportResult{},
			}

			cache := &mockCache{
				internal:   NewLocalCache(),
				setCounter: 0,
			}
			b := input.setup(cache, remoteClient)

			// ignoring error since we know it will return nil each time and run a background task
			resp, err := b.Report(input.request)
			if err != nil {
				if !input.expectError {
					t.Errorf("unexpeced error - %s", err.Error())
				}
				return
			}

			// expect it to be accepted every call
			equals(t, resp.Accepted, true)

			for cache.setCounter < input.expectProcessedTransactionCounter {
				// wait for job to finish transactions, the cache will adjust the counter each time it calls Set
			}

			// check the final end state matches desired state
			cachedVal, cacheHit := b.cache.Get(cacheKey)

			if input.expectProcessedTransactionCounter == 0 {
				if cacheHit {
					t.Errorf("unexpected entry in cache")
				}
				return
			}

			equals(t, cachedVal.LimitCounter["hits"][api.Minute], input.expectCacheState.LimitCounter["hits"][api.Minute])
			equals(t, cachedVal.UnlimitedCounter, input.expectCacheState.UnlimitedCounter)
			equals(t, cachedVal.RemoteState, input.expectCacheState.RemoteState)
			equals(t, cachedVal.params, input.expectCacheState.params)
			equals(t, cachedVal.params, input.expectCacheState.params)

		})
	}
}

func TestBackend_GetPeer(t *testing.T) {
	mc := &mockRemoteClient{}
	b := &Backend{
		client: mc,
		cache:  nil,
	}
	if b.GetPeer() != mc.GetPeer() {
		t.Errorf("peer should be propogated from dependency")
	}
}

// equals fails the test if exp is not equal to act.
func equals(t *testing.T, exp, act interface{}) {
	t.Helper()
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d:\n\n\texp: %#v\n\n\tgot: %#v\033[39m\n\n", filepath.Base(file), line, exp, act)
		t.Error("unexpected result when calling equals")
	}
}

func newLimitCounter(t *testing.T, metric string, period api.Period, limit *Limit) LimitCounter {
	t.Helper()
	lc := make(LimitCounter)
	lc[metric] = make(map[api.Period]*Limit)
	lc[metric][period] = limit
	return lc
}

// mocks and helpers *****************
type mockRemoteClient struct {
	authzErr       error
	reportErr      error
	authRes        *threescale.AuthorizeResult
	repRes         *threescale.ReportResult
	authCallback   func(request threescale.Request)
	reportCallback func(request threescale.Request)
	err            error
}

func (mc *mockRemoteClient) Authorize(request threescale.Request) (*threescale.AuthorizeResult, error) {
	if mc.authCallback != nil {
		mc.authCallback(request)
	}

	if mc.authzErr != nil {
		return nil, mc.authzErr
	}

	return mc.authRes, mc.err
}

func (mc *mockRemoteClient) AuthRep(request threescale.Request) (*threescale.AuthorizeResult, error) {
	return mc.authRes, mc.err
}

func (mc *mockRemoteClient) Report(request threescale.Request) (*threescale.ReportResult, error) {
	if mc.reportCallback != nil {
		mc.reportCallback(request)
	}

	if mc.reportErr != nil {
		return nil, mc.reportErr
	}

	return &threescale.ReportResult{Accepted: true}, nil
}

func (mc *mockRemoteClient) GetPeer() string {
	return "su1.3scale.net/status"
}

func (mc *mockRemoteClient) authErr(t *testing.T, err error) {
	t.Helper()
	mc.err = err
}

func (mc *mockRemoteClient) setAuthResponse(t *testing.T, authResponse *threescale.AuthorizeResult) {
	t.Helper()
	mc.authRes = authResponse
}

type mockAuthResult struct {
	ok        bool
	reports   api.UsageReports
	hierarchy api.Hierarchy
}

func (mar *mockAuthResult) setUsageReports(t *testing.T, reports api.UsageReports) *mockAuthResult {
	t.Helper()
	mar.reports = reports
	return mar
}

func (mar *mockAuthResult) setHierarchy(t *testing.T, h api.Hierarchy) *mockAuthResult {
	t.Helper()
	mar.hierarchy = h
	return mar
}

type mockCache struct {
	internal   Cacheable
	setCounter int
}

func (mc *mockCache) Get(cacheKey string) (*Application, bool) {
	return mc.internal.Get(cacheKey)
}

func (mc *mockCache) Set(key string, application *Application) {
	mc.internal.Set(key, application)
	mc.setCounter++
}

func (mc *mockCache) Keys() []string {
	return mc.internal.Keys()
}

// ********************************
