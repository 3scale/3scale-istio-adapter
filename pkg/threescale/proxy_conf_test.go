package threescale

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogo/googleapis/google/rpc"

	"github.com/3scale/3scale-go-client/fake"
	pb "github.com/3scale/3scale-istio-adapter/config"
	"github.com/3scale/3scale-porta-go-client/client"
	sysFake "github.com/3scale/3scale-porta-go-client/fake"
	"github.com/gogo/protobuf/types"

	"istio.io/api/mixer/adapter/model/v1beta1"
	"istio.io/istio/mixer/template/authorization"
)

type (
	testInput struct {
		name     string
		params   pb.Params
		template authorization.InstanceMsg
	}

	testResult struct {
		result *v1beta1.CheckResult
		err    error
	}
)

func TestProxyConfigCacheFlushing(t *testing.T) {
	const ttl = time.Duration(time.Millisecond * 100)

	var (
		proxyConf         client.ProxyConfigElement
		fetchedFromRemote int
	)

	ctx := context.TODO()
	httpClient := NewTestClient(func(req *http.Request) *http.Response {
		if req.URL.Host == "www.fake-system.3scale.net:443" {
			fetchedFromRemote++
			return sysFake.GetProxyConfigLatestSuccess()
		} else {

			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(fake.GetAuthSuccess())),
				Header:     make(http.Header),
			}
		}
	})

	// Create cache manager and populate
	pc := NewProxyConfigCache(time.Duration(ttl), time.Duration(time.Second*1), DefaultCacheUpdateRetries, 3)
	proxyConf = unmarshalConfig(t)

	cfg := &pb.Params{ServiceId: "123", SystemUrl: "https://www.fake-system.3scale.net"}
	cacheKey := pc.getCacheKeyFromCfg(cfg)
	pc.set(cacheKey, proxyConf, cacheRefreshStore{})
	conf := &AdapterConfig{systemCache: pc}
	c := &Threescale{client: httpClient, conf: conf}

	inputs := []testInput{
		{
			name: "One",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "happy-path",
			},
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Path:   "/?user_key=secret",
					Method: "get",
				},
			},
		},
		{
			name: "Two",
			params: pb.Params{
				ServiceId:   "321",
				SystemUrl:   "https://www.fake-system.3scale.net",
				AccessToken: "happy-path",
			},
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Path:   "/?user_key=secret",
					Method: "get",
				},
			},
		},
	}

	resultOne := make(chan testResult)
	resultTwo := make(chan testResult)
	results := []chan testResult{resultOne, resultTwo}

	for i, input := range inputs {
		copy := testInput{input.name, input.params, input.template}
		index := i
		go func(input testInput, index int) {
			r := &authorization.HandleAuthorizationRequest{
				Instance: &authorization.InstanceMsg{
					Subject: &authorization.SubjectMsg{},
				},
				AdapterConfig: &types.Any{},
				DedupId:       "",
			}

			b, _ := input.params.Marshal()
			r.AdapterConfig.Value = b
			r.Instance = &input.template

			result, err := c.HandleAuthorization(ctx, r)
			results[index] <- testResult{result, err}
		}(copy, index)
	}

	assert := func(msg testResult) {
		if msg.result.Status.Code != 0 {
			t.Fatalf("expected all results to succeed")
		}
	}

	for i := 0; i < len(inputs); i++ {
		select {
		case message := <-resultOne:
			assert(message)
		case message := <-resultTwo:
			assert(message)
		}
	}
	if fetchedFromRemote != 1 {
		t.Fatalf("expected only one result not fetched from cache")
	}

	testStopNotStartedErr := c.conf.systemCache.StopFlushWorker()
	if testStopNotStartedErr == nil {
		t.Fatalf("expected to get error when stopping unstarted worker")
	}

	c.conf.systemCache.StartFlushWorker()

	testStartErr := c.conf.systemCache.StartFlushWorker()
	if testStartErr == nil {
		t.Fatalf("expected only one worker to start")
	}

	<-time.After(time.Second)
	if len(c.conf.systemCache.cache) > 0 {
		t.Fatalf("expected cache to be empty")
	}
	c.conf.systemCache.StopFlushWorker()

	testStartErr = c.conf.systemCache.StartFlushWorker()
	if testStartErr != nil {
		t.Fatalf("expected to be able to restart worker")
	}

}

func TestProxyConfigCacheRefreshing(t *testing.T) {
	const ttl = time.Duration(time.Second * 10)
	const defaultSystemUrl = "https://www.fake-system.3scale.net"

	var (
		proxyConf          client.ProxyConfigElement
		fetchedFromRemote  int32
		fetchedMisbehaving int32
		wasCalled          bool
	)

	ctx := context.TODO()
	httpClient := NewTestClient(func(req *http.Request) *http.Response {
		switch req.URL.Host {
		case "www.fake-system.3scale.net:443":
			atomic.AddInt32(&fetchedFromRemote, 1)
			return sysFake.GetProxyConfigLatestSuccess()
		case "misbehaving-host-1.net:443":
			if !wasCalled {
				wasCalled = true
				return getRespBadGateway(t)
			}
			atomic.AddInt32(&fetchedMisbehaving, 1)
			return sysFake.GetProxyConfigLatestSuccess()
		default:
			return getRespOk(t)
		}
	})

	// Create cache manager
	pc := NewProxyConfigCache(ttl, ttl-(time.Second*1), DefaultCacheUpdateRetries, 3)
	proxyConf = unmarshalConfig(t)
	conf := &AdapterConfig{systemCache: pc}
	c := &Threescale{client: httpClient, conf: conf}

	//Pre-Populate the cache
	cfg := &pb.Params{ServiceId: "123", SystemUrl: defaultSystemUrl}
	cacheKey := pc.getCacheKeyFromCfg(cfg)
	// With valid entry
	pc.set(cacheKey, proxyConf, cacheRefreshStore{cfg: cfg, client: getSysClient(t, c, defaultSystemUrl)})
	// With misbehaving host
	cfgBadHost := &pb.Params{ServiceId: "12345", SystemUrl: "https://misbehaving-host-1.net"}
	cacheKeyBadHost := pc.getCacheKeyFromCfg(cfg)
	pc.set(cacheKeyBadHost, proxyConf, cacheRefreshStore{cfg: cfgBadHost, client: getSysClient(t, c, "https://misbehaving-host-1.net")})
	pc.addMisbehavingHost("misbehaving-host-1.net", fakeNetError{})

	inputs := []testInput{
		{
			name: "Mock Cached Result",
			params: pb.Params{
				ServiceId:   "123",
				SystemUrl:   defaultSystemUrl,
				AccessToken: "happy-path",
			},
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Path:   "/?user_key=secret",
					Method: "get",
				},
			},
		},
		{
			name: "Mock Cache Miss",
			params: pb.Params{
				ServiceId:   "321",
				SystemUrl:   defaultSystemUrl,
				AccessToken: "happy-path",
			},
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Path:   "/?user_key=secret",
					Method: "get",
				},
			},
		},
		{
			name: "Mock Misbehaving Host",
			params: pb.Params{
				ServiceId:   "12345",
				SystemUrl:   "https://misbehaving-host-1.net",
				AccessToken: "host-down-5xx",
			},
			template: authorization.InstanceMsg{
				Name: "",
				Subject: &authorization.SubjectMsg{
					User: "secret",
				},
				Action: &authorization.ActionMsg{
					Path:   "/?user_key=secret",
					Method: "get",
				},
			},
		},
	}

	resultOne := make(chan testResult)
	resultTwo := make(chan testResult)
	resultThree := make(chan testResult)
	results := []chan testResult{resultOne, resultTwo, resultThree}

	for i, input := range inputs {
		copy := testInput{input.name, input.params, input.template}
		index := i
		go func(input testInput, index int) {
			r := &authorization.HandleAuthorizationRequest{
				Instance: &authorization.InstanceMsg{
					Subject: &authorization.SubjectMsg{},
				},
				AdapterConfig: &types.Any{},
				DedupId:       "",
			}

			b, _ := input.params.Marshal()
			r.AdapterConfig.Value = b
			r.Instance = &input.template

			result, err := c.HandleAuthorization(ctx, r)
			results[index] <- testResult{result, err}
		}(copy, index)
	}

	assert := func(msg testResult) {
		if msg.result.Status.Code != 0 {
			t.Fatalf("expected all results to succeed")
		}
	}

	for i := 0; i < len(inputs); i++ {
		select {
		case message := <-resultOne:
			assert(message)
		case message := <-resultTwo:
			assert(message)
		case message := <-resultThree:
			if message.result.Status.Code != int32(rpc.UNAVAILABLE) {
				t.Errorf("expected not authorized")
			}
		}
	}
	if atomic.LoadInt32(&fetchedFromRemote) != 1 {
		t.Fatalf("expected only one result not fetched from cache")
	}

	c.conf.systemCache.refreshMinDuration = time.Nanosecond
	err := c.conf.systemCache.StartRefreshWorker()
	if err != nil {
		t.Fatalf("expected to be able to start the refresh worker")
	}

	<-time.After(time.Second * 3)
	err = c.conf.systemCache.StopRefreshWorker()
	if err != nil {
		t.Fatalf("unexpected error when stopping refresh worker")
	}

	if atomic.LoadInt32(&fetchedFromRemote) < 2 || atomic.LoadInt32(&fetchedMisbehaving) < 1 {
		t.Fatalf("expected cache to have been refreshed and misbehaving host to have been retried and fetched")
	}

	err = c.conf.systemCache.StopRefreshWorker()
	if err == nil {
		t.Fatalf("unexpected error when stopping refresh worker again")
	}
}

func unmarshalConfig(t *testing.T) client.ProxyConfigElement {
	t.Helper()
	var proxyConf client.ProxyConfigElement
	if err := json.Unmarshal([]byte(sysFake.GetProxyConfigLatestJson()), &proxyConf); err != nil {
		t.Fatalf("failed to unmarshal proxy conf")
	}
	return proxyConf
}

func getSysClient(t *testing.T, c *Threescale, sysURL string) *client.ThreeScaleClient {
	t.Helper()
	sysClient, err := c.systemClientBuilder(sysURL)
	if err != nil {
		t.Fatalf("unexpected error building system client")
	}
	return sysClient
}

func getRespOk(t *testing.T) *http.Response {
	t.Helper()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       ioutil.NopCloser(bytes.NewBufferString(fake.GetAuthSuccess())),
		Header:     make(http.Header),
	}
}

func getRespBadGateway(t *testing.T) *http.Response {
	t.Helper()
	return &http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       ioutil.NopCloser(bytes.NewBufferString("Bad Gateway")),
		Header:     make(http.Header),
	}
}

type fakeNetError struct {
	error
}

func (f fakeNetError) Error() string {
	return "fake"
}

func (f fakeNetError) Timeout() bool {
	return true
}

func (f fakeNetError) Temporary() bool {
	return true
}
