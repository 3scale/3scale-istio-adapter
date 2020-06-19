package backend

import (
	"errors"
	"fmt"
	"net"
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
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{{
							PeriodWindow: api.PeriodWindow{
								Period: api.Minute,
							},
							MaxValue:     4,
							CurrentValue: 3,
						},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy: make(api.Hierarchy),
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
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  make(api.Hierarchy),
						RateLimits: nil,
					},
				}
				fullRemoteState := getApplicationFromResponse(remoteResponse)

				lc := make(LimitCounter)
				lc["hits"] = []api.UsageReport{
					{
						PeriodWindow: api.PeriodWindow{
							Period: api.Minute,
						},
						MaxValue:     4,
						CurrentValue: 3,
					},
				}

				app := &Application{
					RemoteState:      fullRemoteState.RemoteState,
					LocalState:       lc,
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
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{{
							PeriodWindow: api.PeriodWindow{
								Period: api.Minute,
							},
							MaxValue:     4,
							CurrentValue: 3,
						},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
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
			name: "Test the application of policy, fail closed case - timeout error",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.err = &net.DNSError{
					IsTimeout: true,
				}

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
					policy: FailClosedPolicy,
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
			expectError: true,
		},
		{
			name: "Test the application of policy, fail closed case - temporary error",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.err = &net.DNSError{
					IsTemporary: true,
				}

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
					policy: FailClosedPolicy,
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
			expectError: true,
		},
		{
			name: "Test the application of policy, nil policy default to fail closed",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.err = &net.DNSError{
					IsTemporary: true,
					IsTimeout:   true,
				}

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
			expectError: true,
		},
		{
			name: "Test the application of policy, fail open case - timeout error",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.err = &net.DNSError{
					IsTimeout: true,
				}

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
					policy: FailOpenPolicy,
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
		{
			name: "Test the application of policy, fail open case - temporary error",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.err = &net.DNSError{
					IsTemporary: true,
				}

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
					policy: FailOpenPolicy,
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
		{
			name: "Test success case",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				// we want to ensure the conversion is done here and the cache is populated correctly
				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
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
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  make(api.Hierarchy),
						RateLimits: nil,
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
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  make(api.Hierarchy),
						RateLimits: nil,
					},
				}

				fullRemoteState := getApplicationFromResponse(remoteResponse)

				lc := make(LimitCounter)
				lc["hits"] = []api.UsageReport{
					{
						PeriodWindow: api.PeriodWindow{
							Period: api.Minute,
						},
						MaxValue:     4,
						CurrentValue: 3,
					},
				}

				app := &Application{
					RemoteState:      fullRemoteState.LocalState,
					LocalState:       lc,
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
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
					Authorized: true,
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
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
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
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
				RemoteState:      newLimitCounter(t, "hits", api.Minute, 3, 4),
				LocalState:       newLimitCounter(t, "hits", api.Minute, 4, 4),
				UnlimitedCounter: map[string]int{"orphan": 2},
			},
		},
		{
			name: "Test success case cache hit",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
					},
				}

				fullRemoteState := getApplicationFromResponse(remoteResponse)

				lc := make(LimitCounter)
				lc["hits"] = []api.UsageReport{
					{
						PeriodWindow: api.PeriodWindow{
							Period: api.Minute,
						},
						MaxValue:     4,
						CurrentValue: 3,
					},
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
					LocalState:       lc,
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
				RemoteState:      newLimitCounter(t, "hits", api.Minute, 3, 4),
				LocalState:       newLimitCounter(t, "hits", api.Minute, 4, 4),
				UnlimitedCounter: map[string]int{"orphan": 3},
			},
		},
		{
			name: "Test the application of policy, fail closed case - timeout error",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.err = &net.DNSError{
					IsTimeout: true,
				}

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
					policy: FailClosedPolicy,
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
			expectError: true,
		},
		{
			name: "Test the application of policy, fail closed case - temporary error",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.err = &net.DNSError{
					IsTemporary: true,
				}

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
					policy: FailClosedPolicy,
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
			expectError: true,
		},
		{
			name: "Test the application of policy, nil policy default to fail closed",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.err = &net.DNSError{
					IsTemporary: true,
					IsTimeout:   true,
				}

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
			expectError: true,
		},
		{
			name: "Test the application of policy, fail open case - timeout error",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.err = &net.DNSError{
					IsTimeout: true,
				}

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
					policy: FailOpenPolicy,
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
		{
			name: "Test the application of policy, fail open case - temporary error",
			setup: func(cacheable Cacheable, remoteClient *mockRemoteClient) *Backend {
				remoteClient.err = &net.DNSError{
					IsTemporary: true,
				}

				return &Backend{
					client: remoteClient,
					cache:  cacheable,
					policy: FailOpenPolicy,
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

			// we need to disregard these additional checks in the case where we have fail open policy as
			// we cant expect anything to be set in the cache
			if resp.Authorized && input.expectCacheState.LocalState != nil {
				equals(t, input.expectCacheState.UnlimitedCounter, cachedVal.UnlimitedCounter)
				equals(t, input.expectCacheState.LocalState, cachedVal.LocalState)
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
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy: api.Hierarchy{"hits": []string{"hits_child"}},
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
				RemoteState:      newLimitCounter(t, "hits", api.Minute, 3, 4),
				LocalState:       newLimitCounter(t, "hits", api.Minute, 5, 4),
				UnlimitedCounter: map[string]int{"orphan": 2},
			},
		},
		{
			name: "Test cache hit and correct reporting, no hierarchy",
			setup: func(cacheable *mockCache, remoteClient *mockRemoteClient) *Backend {

				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy: api.Hierarchy{"hits": []string{"hits_child"}},
					},
				}

				fullRemoteState := getApplicationFromResponse(remoteResponse)

				lc := make(LimitCounter)
				lc["hits"] = []api.UsageReport{
					{
						PeriodWindow: api.PeriodWindow{
							Period: api.Minute,
						},
						MaxValue:     4,
						CurrentValue: 3,
					},
				}

				app := &Application{
					RemoteState:      fullRemoteState.RemoteState,
					LocalState:       lc,
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
				RemoteState:      newLimitCounter(t, "hits", api.Minute, 3, 4),
				LocalState:       newLimitCounter(t, "hits", api.Minute, 5, 4),
				UnlimitedCounter: map[string]int{"orphan": 3},
			},
		},
		{
			name: "Test cache hit and correct reporting, with hierarchy",
			setup: func(cacheable *mockCache, remoteClient *mockRemoteClient) *Backend {

				remoteResponse := &threescale.AuthorizeResult{
					Authorized: true,
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Minute,
								},
								MaxValue:     4,
								CurrentValue: 3,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{
						Hierarchy:  api.Hierarchy{"hits": []string{"hits_child"}},
						RateLimits: nil,
					},
				}

				fullRemoteState := getApplicationFromResponse(remoteResponse)

				lc := make(LimitCounter)
				lc["hits"] = []api.UsageReport{
					{
						PeriodWindow: api.PeriodWindow{
							Period: api.Minute,
						},
						MaxValue:     4,
						CurrentValue: 3,
					},
				}

				app := &Application{
					RemoteState:      fullRemoteState.RemoteState,
					LocalState:       lc,
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
				RemoteState:      newLimitCounter(t, "hits", api.Minute, 3, 4),
				LocalState:       newLimitCounter(t, "hits", api.Minute, 7, 4),
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

			equals(t, cachedVal.LocalState["hits"], input.expectCacheState.LocalState["hits"])
			equals(t, cachedVal.UnlimitedCounter, input.expectCacheState.UnlimitedCounter)
			equals(t, cachedVal.RemoteState, input.expectCacheState.RemoteState)
			equals(t, cachedVal.params, input.expectCacheState.params)
			equals(t, cachedVal.params, input.expectCacheState.params)

		})
	}
}

func TestBackend_Flush(t *testing.T) {
	const service = api.Service("testService")
	const application = "testApplication"
	const authValue = "any"

	const cacheKey = string(service + "_" + application)

	type augmenter struct {
		unlimitedMetrics api.Metrics
		limitedMetrics   api.Metrics
	}

	tests := []struct {
		name  string
		setup func(cache Cacheable)
		// augmenter will modify the cache used in the setup function above with its metrics
		// this augmentation will occur after the initial snapshot is taken and prior to calling the authorize
		// endpoint at the end of the flushing process, therefore simulating a set of requests that will be
		// seen as intermediate activity, allowing us to test the concurrent nature of the cycle.
		// provided metrics should be known to the cache in advance for limited metrics
		augmenter        augmenter
		remoteClient     threescale.Client
		expectFinalState *Application
	}{
		{
			name: "Test reporting error and authorization error leaves cached item untouched",
			setup: func(cache Cacheable) {
				app := newApplication()
				app.RemoteState = newLimitCounter(t, "hits", api.Hour, 30, 0)
				app.LocalState = newLimitCounter(t, "hits", api.Hour, 50, 0)
				app.UnlimitedCounter["orphan"] = 10

				cache.Set(cacheKey, app)
			},
			remoteClient: &mockRemoteClient{
				authzErr:  errors.New("err"),
				reportErr: errors.New("err"),
			},
			expectFinalState: &Application{
				RemoteState:      newLimitCounter(t, "hits", api.Hour, 30, 0),
				LocalState:       newLimitCounter(t, "hits", api.Hour, 50, 0),
				UnlimitedCounter: map[string]int{"orphan": 10},
			},
		},
		{
			name: "Test reporting success and authorization error - remote_state = remote_state + actually_reported_hits. No change to local counters.",
			setup: func(cache Cacheable) {
				app := newApplication()
				app.RemoteState = newLimitCounter(t, "hits", api.Hour, 30, 0)
				app.LocalState = newLimitCounter(t, "hits", api.Hour, 50, 0)
				app.UnlimitedCounter["orphan"] = 10
				app.timestamp = 1000

				cache.Set(cacheKey, app)
			},
			remoteClient: &mockRemoteClient{
				reportCallback: func(request threescale.Request) {
					// verify that the metrics that we report for metrics with rate limits
					// are the current state minus the last known state 50 - 30
					// as well as the counters for unlimited metrics
					equals(t, api.Metrics{"orphan": 10, "hits": 20}, request.Transactions[0].Metrics)
					equals(t, int64(1000), request.Transactions[0].Timestamp)
				},
				authzErr: errors.New("err"),
			},
			expectFinalState: &Application{
				// we expect the local representation of the remote state to be updated with the deltas
				RemoteState: newLimitCounter(t, "hits", api.Hour, 50, 0),
				// we expect the counter for rate limited metrics to remain untouched
				LocalState: newLimitCounter(t, "hits", api.Hour, 50, 0),
				// we expect the metrics that remain  unreported to be empty
				UnlimitedCounter: make(UnlimitedCounter),
			},
		},
		{
			name: "Test reporting error and authorization success - hits += new_auth_hits - snapshot_hits + to_report - actually_reported",
			setup: func(cache Cacheable) {
				app := newApplication()
				app.RemoteState = newLimitCounter(t, "hits", api.Hour, 80, 0)
				app.LocalState = newLimitCounter(t, "hits", api.Hour, 90, 0)
				app.UnlimitedCounter["orphan"] = 10
				app.timestamp = 1000

				cache.Set(cacheKey, app)
			},
			remoteClient: &mockRemoteClient{
				reportErr: errors.New("err"),
				authRes: &threescale.AuthorizeResult{
					Authorized: true,
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Hour,
								},
								CurrentValue: 80,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{},
				},
				reportCallback: func(request threescale.Request) {
					// verify that the metrics that we report for metrics with rate limits
					// are the current state minus the last known state 90 - 80
					// as well as the counters for unlimited metrics
					equals(t, api.Metrics{"orphan": 10, "hits": 10}, request.Transactions[0].Metrics)
					equals(t, int64(1000), request.Transactions[0].Timestamp)
				},
			},
			expectFinalState: &Application{
				RemoteState:      newLimitCounter(t, "hits", api.Hour, 80, 0),
				LocalState:       newLimitCounter(t, "hits", api.Hour, 90, 0),
				UnlimitedCounter: map[string]int{"orphan": 10},
			},
		},
		{
			name: "Test reporting success and authorization success - hits += new_auth_hits - snapshot_hits + to_report - actually_reported",
			setup: func(cache Cacheable) {
				app := newApplication()
				app.RemoteState = newLimitCounter(t, "hits", api.Hour, 80, 0)
				app.LocalState = newLimitCounter(t, "hits", api.Hour, 90, 0)
				app.UnlimitedCounter["orphan"] = 10
				app.timestamp = 500
				cache.Set(cacheKey, app)
			},
			remoteClient: &mockRemoteClient{
				authRes: &threescale.AuthorizeResult{
					Authorized: true,
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Hour,
								},
								CurrentValue: 90,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{},
				},
				reportCallback: func(request threescale.Request) {
					// verify that the metrics that we report for metrics with rate limits
					// are the current state minus the last known state 90 - 80
					// as well as the counters for unlimited metrics
					equals(t, api.Metrics{"orphan": 10, "hits": 10}, request.Transactions[0].Metrics)
					equals(t, int64(500), request.Transactions[0].Timestamp)
				},
			},
			expectFinalState: &Application{
				RemoteState:      newLimitCounter(t, "hits", api.Hour, 90, 0),
				LocalState:       newLimitCounter(t, "hits", api.Hour, 90, 0),
				UnlimitedCounter: make(UnlimitedCounter),
			},
		},
		{
			name: "Test reporting error and authorization error with intermediate activity leaves cached updated",
			setup: func(cache Cacheable) {
				app := newApplication()
				app.RemoteState = newLimitCounter(t, "hits", api.Hour, 30, 0)
				app.LocalState = newLimitCounter(t, "hits", api.Hour, 50, 0)
				app.UnlimitedCounter["orphan"] = 10

				cache.Set(cacheKey, app)
			},
			augmenter: augmenter{
				unlimitedMetrics: api.Metrics{"orphan": 5},
				limitedMetrics:   api.Metrics{"hits": 5},
			},
			remoteClient: &mockRemoteClient{
				authzErr:  errors.New("err"),
				reportErr: errors.New("err"),
			},
			expectFinalState: &Application{
				RemoteState:      newLimitCounter(t, "hits", api.Hour, 30, 0),
				LocalState:       newLimitCounter(t, "hits", api.Hour, 55, 0),
				UnlimitedCounter: map[string]int{"orphan": 15},
			},
		},
		{
			name: "Test reporting success and authorization error with intermediate activity",
			setup: func(cache Cacheable) {
				app := newApplication()
				app.RemoteState = newLimitCounter(t, "hits", api.Hour, 30, 0)
				app.LocalState = newLimitCounter(t, "hits", api.Hour, 50, 0)
				app.UnlimitedCounter["orphan"] = 10
				app.timestamp = 1000
				cache.Set(cacheKey, app)
			},
			augmenter: augmenter{
				unlimitedMetrics: api.Metrics{"orphan": 5},
				limitedMetrics:   api.Metrics{"hits": 5},
			},
			remoteClient: &mockRemoteClient{
				reportCallback: func(request threescale.Request) {
					// verify that the metrics that we report for metrics with rate limits
					// are the current state minus the last known state 50 - 30
					// as well as the counters for unlimited metrics
					equals(t, api.Metrics{"orphan": 10, "hits": 20}, request.Transactions[0].Metrics)
					equals(t, int64(1000), request.Transactions[0].Timestamp)
				},
				authzErr: errors.New("err"),
			},
			expectFinalState: &Application{
				// we expect the local representation of the remote state to be updated with the deltas
				RemoteState: newLimitCounter(t, "hits", api.Hour, 50, 0),
				// we expect the counter for rate limited metrics to include intermediate activity
				LocalState: newLimitCounter(t, "hits", api.Hour, 55, 0),
				// we expect intermediate metrics to be known
				UnlimitedCounter: map[string]int{"orphan": 5},
			},
		},
		{
			name: "Test reporting error and authorization success with intermediate activity",
			setup: func(cache Cacheable) {
				app := newApplication()
				app.RemoteState = newLimitCounter(t, "hits", api.Hour, 80, 0)
				app.LocalState = newLimitCounter(t, "hits", api.Hour, 90, 0)
				app.UnlimitedCounter["orphan"] = 10
				app.timestamp = 200
				cache.Set(cacheKey, app)
			},
			augmenter: augmenter{
				unlimitedMetrics: api.Metrics{"orphan": 5},
				limitedMetrics:   api.Metrics{"hits": 5},
			},
			remoteClient: &mockRemoteClient{
				reportErr: errors.New("err"),
				authRes: &threescale.AuthorizeResult{
					Authorized: true,
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Hour,
								},
								CurrentValue: 80,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{},
				},
				reportCallback: func(request threescale.Request) {
					// verify that the metrics that we report for metrics with rate limits
					// are the current state minus the last known state 90 - 80
					// as well as the counters for unlimited metrics
					equals(t, api.Metrics{"orphan": 10, "hits": 10}, request.Transactions[0].Metrics)
					equals(t, int64(200), request.Transactions[0].Timestamp)
				},
			},
			expectFinalState: &Application{
				RemoteState:      newLimitCounter(t, "hits", api.Hour, 80, 0),
				LocalState:       newLimitCounter(t, "hits", api.Hour, 95, 0),
				UnlimitedCounter: map[string]int{"orphan": 15},
			},
		},
		{
			name: "Test reporting success and authorization success with intermediate activity",
			setup: func(cache Cacheable) {
				app := newApplication()
				app.RemoteState = newLimitCounter(t, "hits", api.Hour, 80, 0)
				app.LocalState = newLimitCounter(t, "hits", api.Hour, 90, 0)
				app.UnlimitedCounter["orphan"] = 10
				app.timestamp = 500
				cache.Set(cacheKey, app)
			},
			augmenter: augmenter{
				unlimitedMetrics: api.Metrics{"orphan": 5},
				limitedMetrics:   api.Metrics{"hits": 5},
			},
			remoteClient: &mockRemoteClient{
				authRes: &threescale.AuthorizeResult{
					Authorized: true,
					UsageReports: api.UsageReports{
						"hits": []api.UsageReport{
							{
								PeriodWindow: api.PeriodWindow{
									Period: api.Hour,
								},
								CurrentValue: 90,
							},
						},
					},
					AuthorizeExtensions: threescale.AuthorizeExtensions{},
				},
				reportCallback: func(request threescale.Request) {
					// verify that the metrics that we report for metrics with rate limits
					// are the current state minus the last known state 90 - 80
					// as well as the counters for unlimited metrics
					equals(t, api.Metrics{"orphan": 10, "hits": 10}, request.Transactions[0].Metrics)
					equals(t, int64(500), request.Transactions[0].Timestamp)
				},
			},
			expectFinalState: &Application{
				RemoteState: newLimitCounter(t, "hits", api.Hour, 90, 0),
				LocalState:  newLimitCounter(t, "hits", api.Hour, 95, 0),
				// we expect intermediate metrics to be known
				UnlimitedCounter: map[string]int{"orphan": 5},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache := NewLocalCache()
			test.setup(cache)

			reportNotifier := make(chan bool)
			client := test.remoteClient.(*mockRemoteClient)
			client.reportDone = reportNotifier

			b := &Backend{
				client: test.remoteClient,
				cache:  cache,
				queue:  newQueue(10),
				logger: &noOpLogger{},
			}

			b.Flush()

			<-reportNotifier

			app, _ := b.cache.Get(cacheKey)
			app.Lock()
			for metric, value := range test.augmenter.unlimitedMetrics {
				app.UnlimitedCounter[metric] += value
			}

			for metric, value := range test.augmenter.limitedMetrics {
				counters, ok := app.LocalState[metric]
				if ok {
					for i, _ := range counters {
						counters[i].CurrentValue += value
					}
				}
			}
			app.Unlock()

			equals(t, test.expectFinalState.RemoteState, app.RemoteState)
			for metric, periods := range test.expectFinalState.LocalState {
				for period := range periods {
					equals(t, app.LocalState[metric][period], test.expectFinalState.LocalState[metric][period])
				}

			}
			equals(t, test.expectFinalState.UnlimitedCounter, app.UnlimitedCounter)
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

func newLimitCounter(t *testing.T, metric string, period api.Period, current, max int) LimitCounter {
	t.Helper()
	lc := make(LimitCounter)
	lc[metric] = []api.UsageReport{
		{
			PeriodWindow: api.PeriodWindow{
				Period: period,
			},
			CurrentValue: current,
			MaxValue:     max,
		},
	}
	return lc
}

// mocks and helpers *****************
type mockRemoteClient struct {
	// error from authorization call, default nil
	authzErr error
	// error from report call, default nil
	reportErr error
	// the response the client should respond to an authorization call with
	authRes *threescale.AuthorizeResult
	// the response the client should respond to an report call with
	repRes *threescale.ReportResult
	// can be used for verification of the information sent to 3scale at authz stage
	authCallback func(request threescale.Request)
	// can be used for verification of the information sent to 3scale at report stage
	reportCallback func(request threescale.Request)
	// *don't set* - this is used internally where required
	reportDone chan bool
	err        error
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

	if mc.reportDone != nil {
		go func() {
			mc.reportDone <- true
		}()
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
