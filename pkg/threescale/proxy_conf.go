package threescale

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/3scale/3scale-istio-adapter/config"
	"github.com/3scale/3scale-istio-adapter/pkg/threescale/metrics"
	sysC "github.com/3scale/3scale-porta-go-client/client"
	"istio.io/istio/pkg/log"
)

const (
	// DefaultCacheTTL - Default time to wait before purging expired items from the cache
	DefaultCacheTTL = time.Duration(time.Minute * 5)

	// DefaultCacheRefreshBuffer - Default time difference to refresh the cache element before expiry time
	DefaultCacheRefreshBuffer = time.Duration(time.Minute * 3)

	// DefaultCacheLimit - Default max number of items that can be stored in the cache at any time
	DefaultCacheLimit = 1000

	// DefaultCacheUpdateRetries - Default number of additional attempts made to update cached entries for unreachable hosts
	DefaultCacheUpdateRetries = 1

	cacheKeySeparator = "_"
	cacheKey          = "%s" + cacheKeySeparator + "%s"

	refreshSleepDefault = time.Duration(time.Second * 2)
)

type proxyStore struct {
	element    sysC.ProxyConfigElement
	expiresAt  time.Time
	replayWith cacheRefreshStore
}

type cacheRefreshStore struct {
	cfg    *config.Params
	client *sysC.ThreeScaleClient
}

// ProxyConfigCache provides a mechanism to enable caching of calls to 3scale system
type ProxyConfigCache struct {
	limit                int
	ttl                  time.Duration
	refreshBuffer        time.Duration
	refreshMinDuration   time.Duration
	refreshRetries       int
	flushWorkerRunning   int32
	stopFlushWorker      chan bool
	refreshWorkerRunning int32
	stopRefreshWorker    chan bool
	metricsReporter      *metrics.Reporter
	misbehavingHosts     map[string]bool
	mutex                sync.RWMutex
	cache                map[string]proxyStore
}

// NewProxyConfigCache returns a ProxyConfigCache
// The accepted parameters are cacheTTL - the time between when an entry is added to the
// cache and when it should expire, and limit - the total number of entries that can be
// stored in the cache at any given time.
func NewProxyConfigCache(cacheTTL time.Duration, refreshBuffer time.Duration, refreshRetries int, limit int) *ProxyConfigCache {
	pcc := &ProxyConfigCache{
		limit:                limit,
		ttl:                  cacheTTL,
		refreshBuffer:        refreshBuffer,
		refreshRetries:       refreshRetries,
		cache:                make(map[string]proxyStore, limit),
		flushWorkerRunning:   0,
		refreshWorkerRunning: 0,
	}
	return pcc
}

// FlushCache iterates over the items in the cache and purges any expired items
func (pc *ProxyConfigCache) FlushCache() {
	log.Debugf("starting cache flush for proxy config")
	pc.delete(pc.markForDeletion()...)
}

// StartFlushWorker starts a background process that periodically carries out the
// functionality provided by FlushCache
func (pc *ProxyConfigCache) StartFlushWorker() error {
	if !atomic.CompareAndSwapInt32(&pc.flushWorkerRunning, 0, 1) {
		return errors.New("worker has already been started")
	}

	pc.stopFlushWorker = make(chan bool)
	go pc.flushCache(pc.stopFlushWorker)
	return nil
}

// StopFlushWorker stops a background process started by StartFlushWorker if it has been started
// Returns an error if the background task is not running
func (pc *ProxyConfigCache) StopFlushWorker() error {
	if !atomic.CompareAndSwapInt32(&pc.flushWorkerRunning, 1, 0) {
		return errors.New("worker is not running")
	}

	pc.stopFlushWorker <- true
	close(pc.stopFlushWorker)
	return nil
}

// StartRefreshWorker starts a background process that periodically carries out the
// functionality provided by refreshCache
func (pc *ProxyConfigCache) StartRefreshWorker() error {
	if !atomic.CompareAndSwapInt32(&pc.refreshWorkerRunning, 0, 1) {
		return errors.New("worker has already been started")
	}

	pc.stopRefreshWorker = make(chan bool)
	go pc.refreshCacheWorker(pc.stopRefreshWorker)
	return nil
}

// StopRefreshWorker stops a background process started by StartRefreshWorker if it has been started
// Returns an error if the background task is not running
func (pc *ProxyConfigCache) StopRefreshWorker() error {
	if !atomic.CompareAndSwapInt32(&pc.refreshWorkerRunning, 1, 0) {
		return errors.New("worker is not running")
	}

	pc.stopRefreshWorker <- true
	close(pc.stopRefreshWorker)
	return nil
}

func (pc *ProxyConfigCache) get(cfg *config.Params, c *sysC.ThreeScaleClient) (sysC.ProxyConfigElement, error) {
	var conf sysC.ProxyConfigElement
	var err error

	cacheKey := pc.getCacheKeyFromCfg(cfg)

	pc.mutex.RLock()
	e, ok := pc.cache[cacheKey]
	pc.mutex.RUnlock()

	if !ok {
		conf, err = getFromRemote(cfg, c, pc.metricsReporter.ReportMetrics)
		if err == nil {
			replayWith := cacheRefreshStore{cfg, c}
			go pc.set(cacheKey, conf, replayWith)
		}
	} else {
		log.Debugf("proxy config fetched from cache for service id %s", cfg.ServiceId)
		conf = e.element
		go pc.metricsReporter.IncrementCacheHits()
	}

	return conf, err
}

func (pc *ProxyConfigCache) set(cacheKey string, e sysC.ProxyConfigElement, replayWith cacheRefreshStore) {
	expiresAt := time.Now().Add(pc.ttl)
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	if len(pc.cache) < pc.limit {
		pc.cache[cacheKey] = proxyStore{
			expiresAt:  expiresAt,
			element:    e,
			replayWith: replayWith,
		}
	}
}

func (pc *ProxyConfigCache) delete(key ...string) {
	pc.mutex.Lock()
	for _, k := range key {
		delete(pc.cache, k)
		log.Debugf("proxy config purged from cache for key %s", k)
	}
	pc.mutex.Unlock()
}

func (pc *ProxyConfigCache) getCacheKeyFromCfg(cfg *config.Params) string {
	// safe to ignore error here as this was parsed when building the client
	u, _ := url.Parse(cfg.GetSystemUrl())
	return fmt.Sprintf(cacheKey, u.Host, cfg.ServiceId)
}

func (pc *ProxyConfigCache) splitHostFromCacheKey(cacheKey string) string {
	return strings.Split(cacheKey, cacheKeySeparator)[0]
}

func (pc *ProxyConfigCache) flushCache(exitC chan bool) {
	for {

		select {
		case stop := <-exitC:
			if stop {
				log.Debugf("stopping cache flushing worker")
				return
			}
		default:
			pc.FlushCache()
			<-time.After(pc.ttl)
		}
	}
}

func (pc *ProxyConfigCache) refreshCacheWorker(exitC chan bool) {
	var retryCounter int
	var wait time.Duration
	var refreshAfter <-chan time.Time

	baseWaitDuration := pc.ttl - pc.refreshBuffer
	// this is configurable internally for testing purposes only
	// check if its the default value and set to const value if true
	minRefreshDurationAllowed := pc.refreshMinDuration
	if pc.refreshMinDuration == time.Second*0 {
		minRefreshDurationAllowed = time.Second * 2
	}

	setState := func(waitFor time.Duration, retries int) {
		// Protect the api with some sensible minimum
		// Attempted details below this duration are reset
		if waitFor < minRefreshDurationAllowed {
			waitFor = minRefreshDurationAllowed
		}

		wait = waitFor
		refreshAfter = time.After(wait)
		retryCounter = retries
	}

	resetState := func() {
		setState(baseWaitDuration, pc.refreshRetries)
	}

	resetState()

	for {
		select {
		case stop := <-exitC:
			if stop {
				log.Debugf("stopping cache refresh worker")
				return
			}
		case <-refreshAfter:
			shouldRetry := pc.refreshCache()
			if shouldRetry && retryCounter > 0 {
				setState((baseWaitDuration)/time.Duration(retryCounter+1), retryCounter)
				retryCounter--
				continue
			}
			resetState()
		default:
			sleepFor := refreshSleepDefault
			if wait < sleepFor {
				sleepFor = wait - (time.Millisecond * 5)
			}
			time.Sleep(sleepFor)
		}
	}
}

// refreshCache iterates over the items in the cache and updates their values
// returns true if all comms to remote hosts during refresh succeed
func (pc *ProxyConfigCache) refreshCache() bool {
	log.Debugf("refreshing cache for existing proxy config entries")
	forRefresh := pc.markForRefresh()
	for _, store := range forRefresh {

		cacheKey := pc.getCacheKeyFromCfg(store.replayWith.cfg)
		host := pc.splitHostFromCacheKey(cacheKey)
		if pc.isMisbehaving(host) {
			continue
		}

		pce, err := getFromRemote(store.replayWith.cfg, store.replayWith.client, pc.metricsReporter.ReportMetrics)
		if err != nil {
			log.Infof("error fetching from remote while refreshing cache for service id %s", store.replayWith.cfg.ServiceId)
			pc.addMisbehavingHost(host, err)
			continue
		}
		pc.set(cacheKey, pce, store.replayWith)
	}

	// check if any remote hosts have not been reachable
	shouldRetry := !isEmptySet(pc.misbehavingHosts)
	// reset the set of unreachable hosts
	pc.emptySet(&pc.misbehavingHosts)
	return shouldRetry
}

func (pc *ProxyConfigCache) markForDeletion() []string {
	var forDeletion []string
	now := time.Now()
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()
	for k, v := range pc.cache {
		if isExpired(now, v.expiresAt) {
			forDeletion = append(forDeletion, k)
		}
	}
	return forDeletion
}

func (pc *ProxyConfigCache) markForRefresh() []proxyStore {
	var forRefresh []proxyStore
	pc.mutex.RLock()
	now := time.Now()
	defer pc.mutex.RUnlock()
	for _, v := range pc.cache {
		if pc.shouldRefresh(now, v.expiresAt) {
			forRefresh = append(forRefresh, v)
		}
	}
	return forRefresh
}

func (pc *ProxyConfigCache) shouldRefresh(currentTime time.Time, expiryTime time.Time) bool {
	return currentTime.Add(pc.refreshBuffer).After(expiryTime)
}

func (pc *ProxyConfigCache) addMisbehavingHost(host string, err error) {
	var misbehaving bool

	switch errT := err.(type) {
	case net.Error:
		if errT.Timeout() {
			misbehaving = true
		}
	case sysC.ApiErr:
		if errT.Code() >= 500 && errT.Code() < 600 {
			misbehaving = true
		}
	default:
		log.Infof("unreachable host will not be skipped - error type %T", errT)
	}
	if misbehaving {
		if isEmptySet(pc.misbehavingHosts) {
			allocateSet(&pc.misbehavingHosts)
		}
		pc.misbehavingHosts[host] = true
	}
}

func (pc *ProxyConfigCache) isMisbehaving(host string) bool {
	_, ok := pc.misbehavingHosts[host]
	return ok
}

func (pc *ProxyConfigCache) emptySet(set *map[string]bool) {
	*set = nil
}

func isExpired(currentTime time.Time, expiryTime time.Time) bool {
	return currentTime.After(expiryTime)
}

// Fetch the proxy config from 3scale using the client
func getFromRemote(cfg *config.Params, c *sysC.ThreeScaleClient, report reportMetrics) (sysC.ProxyConfigElement, error) {
	log.Debugf("proxy config for service id %s is being fetching from 3scale", cfg.ServiceId)

	start := time.Now()
	proxyConf, err := c.GetLatestProxyConfig(cfg.AccessToken, cfg.ServiceId, "production")
	elapsed := time.Since(start)

	go func() {
		if apiErr, ok := err.(sysC.ApiErr); ok {
			report(cfg.SystemUrl, metrics.NewLatencyReport("", elapsed, cfg.SystemUrl, metrics.System),
				metrics.NewStatusReport("", apiErr.Code(), cfg.SystemUrl, metrics.System))
		}
	}()

	return proxyConf, err
}

func isEmptySet(set map[string]bool) bool {
	return set == nil
}

func allocateSet(set *map[string]bool) {
	*set = make(map[string]bool)
}
