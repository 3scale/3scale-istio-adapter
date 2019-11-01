package backend

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/3scale/3scale-go-client/client"
)

// TestLocalCache_E2E provides a complete test of a cached backend which uses a 'LocalCache' as a data store
func TestLocalCache_E2E(t *testing.T) {
	var called bool
	const cacheKey = "12345_su1.3scale.net:443"
	const appID = "any"
	doneReporting := make(chan client.Metrics)

	stop := make(chan struct{})
	interval := time.Microsecond
	cache := NewLocalCache(&interval, stop)

	cachedBackend := NewCachedBackend(cache, nil)
	// we need to init the cache with some workable values for testing but the required fields are unexported and we can't override the funcs
	// might want to look into using an interface here down the road, but for now this test will provide extra validation of full caching system
	// and is probably more useful when iterating through the design as it mimics e2e
	httpClient := NewTestClient(t, func(req *http.Request) *http.Response {

		if req.Method == http.MethodPost || called {
			// this function also implies a lot of working knowledge of the 3scale api and response format which would be nice to avoid
			go func() { doneReporting <- parseQueryString(req, t) }()
			// this return statement is irrelevant - sending the parsed request back on the channel to evaluate correctness is tested below
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewBufferString(getInitResp(t))),
				Header:     make(http.Header),
			}
		}

		if req.URL.Path == "/transactions/authorize.xml" {
			called = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(bytes.NewBufferString(getInitResp(t))),
				Header:     make(http.Header),
			}
		}
		return nil
	})

	testingClient := client.NewThreeScale(nil, httpClient)

	req := client.Request{
		Application: client.Application{
			AppID: client.AppID{
				ID: appID,
			},
		},
		Credentials: client.TokenAuth{
			Type:  "service_token",
			Value: "any",
		},
	}

	initRequest := AuthRepRequest{
		ServiceID: "12345",
		Request:   req,
		Params:    client.AuthRepParams{},
	}
	_, err := cachedBackend.AuthRep(initRequest, testingClient)
	if err != nil {
		t.Errorf("unexpected error when calling fake client")
	}

	// expecting this to hit the cache
	_, err = cachedBackend.AuthRep(AuthRepRequest{
		Request:   req,
		ServiceID: "12345",
		Params: client.AuthRepParams{
			AuthorizeParams: client.AuthorizeParams{
				Metrics: client.Metrics{
					"m":       1,
					"n":       1,
					"hits":    1,
					"bananas": 1,
				},
			},
		},
	}, testingClient)
	if err != nil {
		t.Errorf("unexpected error when calling cache")
	}

	cv, ok := cache.Get(cacheKey)
	if !ok {
		t.Errorf("missing cache key")
	}

	expect, isSet := cv[appID].UnlimitedHits["bananas"]
	if !isSet {
		t.Errorf("expected unlimited metric to be stored in cache")
	}
	if expect != 1 {
		t.Errorf("unexpected result fro unlimited metric")
	}

	expectHits, isSet := cv[appID].LimitCounter["hits"][client.Hour]
	if !isSet {
		t.Errorf("expected hits metric to be stored in cache")
	}

	if expectHits.current != 153 {
		t.Errorf("unexpceted result for hits metric which is a parent")
	}

	expectM, isSet := cv[appID].LimitCounter["m"][client.Minute]
	if !isSet {
		t.Errorf("expected m metric to be stored in cache")
	}

	if expectM.current != 11 {
		t.Errorf("unexpceted result for m metric which is a child")
	}

	expectN, isSet := cv[appID].LimitCounter["n"][client.Week]
	if !isSet {
		t.Errorf("expected n metric to be stored in cache")
	}

	if expectN.current != 3 {
		t.Errorf("unexpceted result for n metric")
	}

	go cache.Report()
	reported := <-doneReporting
	close(cache.GetStopChan())

	expected := client.Metrics{"hits": 1, "m": 1, "n": 1, "bananas": 1}

	if !reflect.DeepEqual(reported, expected) {
		t.Errorf("unexpected result reported got \n %v \n wanted \n %v", reported, expected)
	}
}

// helper which takes an encoded query string (encoded by 3scale client for correct format for 3scale api)
// and disassembles it back to readable metrics
func parseQueryString(req *http.Request, t *testing.T) client.Metrics {
	t.Helper()
	metrics := make(client.Metrics)
	encodedValue := req.URL.RawQuery
	values, err := url.ParseQuery(encodedValue)
	if err != nil {
		t.Error("error when url decoding provided query string")
	}

	re := regexp.MustCompile(`\[(.*?)\]`)

	for k, v := range values {
		if strings.Contains(k, "usage") {
			match := re.FindStringSubmatch(k)
			intVal, err := strconv.Atoi(v[0])
			if err != nil {
				t.Error("error during str conv")
			}
			metric := match[1]
			metrics.Add(metric, intVal)
		}
	}
	return metrics
}

// helper which populates the cache with a workable example
func getInitResp(t *testing.T) string {
	t.Helper()
	return `<?xml version="1.0" encoding="UTF-8"?>
<status>
   <authorized>true</authorized>
   <plan>Basic</plan>
   <usage_reports>
      <usage_report metric="m" period="minute">
         <period_start>2019-02-22 14:32:00 +0000</period_start>
         <period_end>2019-02-22 14:33:00 +0000</period_end>
         <max_value>50</max_value>
         <current_value>10</current_value>
      </usage_report>
      <usage_report metric="hits" period="hour">
         <period_start>2019-02-22 14:32:00 +0000</period_start>
         <period_end>2019-02-22 14:33:00 +0000</period_end>
         <max_value>1000</max_value>
         <current_value>150</current_value>
      </usage_report>
      <usage_report metric="n" period="week">
         <period_start>2019-02-18 00:00:00 +0000</period_start>
         <period_end>2019-02-25 00:00:00 +0000</period_end>
         <max_value>100</max_value>
         <current_value>2</current_value>
      </usage_report>
   </usage_reports>
   <hierarchy>
      <metric name="hits" children="m bananas" />
   </hierarchy>
</status>`
}
