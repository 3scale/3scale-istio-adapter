package backend

import (
	"testing"

	"github.com/3scale/3scale-go-client/threescale"
	"github.com/3scale/3scale-go-client/threescale/api"
)

func Test_EmptyTransactionFrom(t *testing.T) {
	params := api.Params{
		AppID:   "any",
		UserKey: "user",
	}

	transaction := api.Transaction{
		Metrics: api.Metrics{
			"hits": 5,
		},
		Params:    params,
		Timestamp: 100,
	}

	result := emptyTransactionFrom(transaction)

	if result.Params != params {
		t.Errorf("expected params to have been copied")
	}

	if result.Metrics != nil {
		t.Errorf("expected metrics to be empty")
	}

	if result.Timestamp != 100 {
		t.Errorf("expected timestamp to be copied")
	}

}

func Test_GetApplicationFromResponse(t *testing.T) {
	response := &threescale.AuthorizeResult{
		Authorized: true,
		UsageReports: map[string][]api.UsageReport{
			"hits": {
				{
					PeriodWindow: api.PeriodWindow{
						Period: api.Day,
						Start:  1000,
						End:    2000,
					},
					MaxValue:     5,
					CurrentValue: 10,
				},
			},
			"other": {
				{
					PeriodWindow: api.PeriodWindow{
						Period: api.Minute,
						Start:  1000,
						End:    2000,
					},
					MaxValue:     1000,
					CurrentValue: 100,
				},
			},
		},
	}

	expect := Application{
		RemoteState: LimitCounter{
			"hits": []api.UsageReport{
				{
					PeriodWindow: api.PeriodWindow{
						Period: api.Day,
						Start:  1000,
						End:    2000,
					},
					MaxValue:     5,
					CurrentValue: 10,
				},
			},
			"other": {
				{
					PeriodWindow: api.PeriodWindow{
						Period: api.Minute,
						Start:  1000,
						End:    2000,
					},
					MaxValue:     1000,
					CurrentValue: 100,
				},
			},
		},
		LocalState: LimitCounter{
			"hits": []api.UsageReport{
				{
					PeriodWindow: api.PeriodWindow{
						Period: api.Day,
						Start:  1000,
						End:    2000,
					},
					MaxValue:     5,
					CurrentValue: 10,
				},
			},
			"other": {
				{
					PeriodWindow: api.PeriodWindow{
						Period: api.Minute,
						Start:  1000,
						End:    2000,
					},
					MaxValue:     1000,
					CurrentValue: 100,
				},
			},
		},
		timestamp: 1000,
	}

	result := getApplicationFromResponse(response)
	equals(t, expect.RemoteState, result.RemoteState)
	equals(t, expect.LocalState, result.LocalState)
	equals(t, expect.timestamp, result.timestamp)
}

func Test_GetAppIDFromTransaction(t *testing.T) {
	tests := []struct {
		name   string
		input  api.Transaction
		expect string
	}{
		{
			name: "Test App ID from User Key",
			input: api.Transaction{
				Params: api.Params{
					UserKey: "user",
				},
			},
			expect: "user",
		},
		{
			name: "Test App ID from AppID",
			input: api.Transaction{
				Params: api.Params{
					AppID: "app",
				},
			},
			expect: "app",
		},
		{
			name: "Test App ID from User Key prioritised",
			input: api.Transaction{
				Params: api.Params{
					UserKey: "user",
					AppID:   "app",
				},
			},
			expect: "user",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if result := getAppIDFromTransaction(test.input); result != test.expect {
				t.Errorf("unexpected result, wanted %s, but got %s", test.expect, result)
			}
		})
	}
}

func Test_GenerateCacheKeyFromRequest(t *testing.T) {
	const expect = "svc_id"
	request := threescale.Request{
		Service: "svc",
		Transactions: []api.Transaction{
			{
				Params: api.Params{
					AppID: "id",
				},
			},
		},
	}

	result := generateCacheKeyFromRequest(request, 0)
	if result != expect {
		t.Errorf("unexpected result, wanted %s, but got %s", expect, result)
	}
}

func Test_ParseCacheKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		expectService string
		expectApp     string
		expectErr     bool
	}{
		{
			name:      "Test expect error when cache key is incorrectly formatted",
			key:       "invalid",
			expectErr: true,
		},
		{
			name:      "Test expect error when cache key has no service",
			key:       "_app",
			expectErr: true,
		},
		{
			name:      "Test expect error when cache key has no app",
			key:       "svc_",
			expectErr: true,
		},
		{
			name:          "Test happy path",
			key:           "svc_app",
			expectService: "svc",
			expectApp:     "app",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, app, err := parseCacheKey(test.key)

			if err != nil {
				if test.expectErr {
					return
				}
				t.Errorf("unexpected error when parsing the key")
			}

			if test.expectErr && err == nil {
				t.Errorf("test that expected error failed to return an error")
			}

			if svc != api.Service(test.expectService) {
				t.Errorf("unexpected service after parsing")
			}

			if app != test.expectApp {
				t.Errorf("unexpected app after parsing")
			}

		})
	}
}

func Test_ValidateTransactions(t *testing.T) {
	tests := []struct {
		name         string
		transactions []api.Transaction
		expectErr    bool
	}{
		{
			name:      "Expect error when transactions are nil",
			expectErr: true,
		},
		{
			name:         "Expect error when transactions are empty",
			transactions: []api.Transaction{},
			expectErr:    true,
		},
		{
			name: "Expect ok",
			transactions: []api.Transaction{
				{
					Metrics: make(api.Metrics),
					Params: api.Params{
						AppID: "any",
					},
					Timestamp: 1,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTransactions(test.transactions)
			if err != nil {
				if test.expectErr {
					return
				}
				t.Errorf("unexpected error - %v", err)
			}

			if test.expectErr && err == nil {
				t.Errorf("test that expected error failed to return an error")
			}
		})
	}
}

func TestSynchronizeState(t *testing.T) {
	original := LimitCounter{
		"hits": []api.UsageReport{
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Day,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Week,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Month,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Year,
				},
			},
		},
	}

	new := LimitCounter{
		"hits": []api.UsageReport{
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Minute,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Day,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Month,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Year,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Eternity,
				},
			},
		},
	}

	expect := LimitCounter{
		"hits": []api.UsageReport{
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Minute,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Day,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Month,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Year,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Eternity,
				},
			},
		},
	}

	expectChanges := make(map[string]stateChanges)
	expectChanges["hits"] = stateChanges{
		added: []api.UsageReport{
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Minute,
				},
			},
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Eternity,
				},
			},
		},
		removed: []api.UsageReport{
			{
				PeriodWindow: api.PeriodWindow{
					Period: api.Week,
				},
			},
		},
	}

	got, changes := synchronizeStates(original, new)
	equals(t, expect, got)
	equals(t, expectChanges, changes)
}

func TestGetDifferenceBetweenSets(t *testing.T) {
	sourceReports := []api.UsageReport{
		{
			PeriodWindow: api.PeriodWindow{
				Period: api.Minute,
			},
		},
		{
			PeriodWindow: api.PeriodWindow{
				Period: api.Day,
			},
		},
		{
			PeriodWindow: api.PeriodWindow{
				Period: api.Year,
			},
		},
		{
			PeriodWindow: api.PeriodWindow{
				Period: api.Eternity,
			},
		},
	}

	destinationReports := []api.UsageReport{
		{
			PeriodWindow: api.PeriodWindow{
				Period: api.Minute,
			},
		},
		{
			PeriodWindow: api.PeriodWindow{
				Period: api.Month,
			},
		},
		{
			PeriodWindow: api.PeriodWindow{
				Period: api.Year,
			},
		},
	}

	expect := []int{1, 3}
	got := getDifferenceBetweenSets(sourceReports, destinationReports)
	equals(t, expect, got)

	expect = []int{1}
	got = getDifferenceBetweenSets(destinationReports, sourceReports)
	equals(t, expect, got)
}
