package threescaleClient

import (
	"github.com/3scale/istio-integration/3scaleAdapter/config"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"encoding/json"
	"istio.io/istio/mixer/template/authorization"
	"istio.io/istio/pkg/log"
	"time"
	"github.com/patrickmn/go-cache"
)

type report struct {
	MetricName string
	Delta      int64
}

const (
	apiServicePath  = "/admin/api/services/"
	proxyConfigPath = "/proxy/configs/production/latest.json"
	authRepPath     = "/transactions/authrep.xml"
)

// Cache is a global cache for storing the ProxyConfig.
var Cache = cache.New(5*time.Minute, 5*time.Minute)


// HandleRequest returns true or false depending on the request and the 3scale config.
func HandleRequest(cfg *config.Params, request authorization.InstanceMsg) (bool, error) {

	log.Debugf("HandleRequest got: %v",request)

	parsedRequest, err := url.ParseRequestURI(request.Action.Path)
	if err != nil {
		return false, err
	}

	threescaleUserKey := parsedRequest.Query().Get("user_key")

	// If the UserKey is empty we don't even need to try to authenticate and report
	if threescaleUserKey == "" {
		return false, nil
	}

	proxyConfig, err := getSystemProxyConfig(cfg)
	if err != nil {
		return false, err
	}

	proxyRules := proxyConfig.ProxyConfig.Content.Proxy.ProxyRules

	reports, err := generateReports(request.Action.Path, proxyRules)
	ok, err := authrep(cfg, reports, threescaleUserKey)

	if err != nil {
		return false, err
	}

	return ok, err
}

// This should be part of an external 3scale Client
func authrep(cfg *config.Params, reports []report, threescaleUserKey string) (bool, error) {
	ok := false

	u, err := url.Parse(cfg.BackendUrl)
	if err != nil {
		return false, err
	}

	u.Path = authRepPath

	q := u.Query()
	q.Set("service_token", cfg.ServiceToken)
	q.Set("service_id", cfg.ServiceId)
	q.Set("user_key", threescaleUserKey)
	for _, report := range reports {
		q.Set("usage["+report.MetricName+"]", strconv.FormatInt(report.Delta, 10))
	}
	u.RawQuery = q.Encode()
	resp, err := http.Get(u.String())
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		ok = true
	}
	return ok, nil
}

// generateReports matches the current request with the mapping rules in the Proxy Rules.
func generateReports(request string, ProxyRules []ProxyRule) ([]report, error) {
	var reports []report
	for _, ProxyRule := range ProxyRules {
		match, err := regexp.MatchString(ProxyRule.Pattern, request)
		if err != nil {
			return nil, err
		}
		if match {
			var report report
			report.Delta = ProxyRule.Delta
			report.MetricName = ProxyRule.MetricSystemName
			reports = addReport(reports, report)
		}
	}
	return reports, nil
}

// addReport when adding a new report, addReport looks for already existing reports with the same metrics and increments the delta accordingly
func addReport(reports []report, newReport report) []report {
	existing := false
	newReports := reports

	for i, report := range reports {
		if report.MetricName == newReport.MetricName {
			report.Delta = newReport.Delta + report.Delta
			newReports[i] = report
			existing = true
		}
	}
	if !existing {
		newReports = append(newReports, newReport)
	}

	return newReports
}

// getSystemProxyConfig get's the 3scale configuration from System url.
func getSystemProxyConfig(cfg *config.Params) (Config, error) {
	var proxyConfig Config

	if cacheConfig, found := Cache.Get("proxyConfig"); found {

		log.Debugf("Cached config: %v", proxyConfig)

		proxyConfig = cacheConfig.(Config)

	} else {

		log.Debugf("Config not cached")

		u, err := url.Parse(cfg.SystemUrl)
		if err != nil {
			return proxyConfig, err
		}
		u.Path = apiServicePath + cfg.ServiceId + proxyConfigPath
		u.User = url.UserPassword("", cfg.AccessToken)
		resp, err := http.Get(u.String())
		if err != nil {
			return proxyConfig, err
		}
		defer resp.Body.Close()
		var bodyString string

		if resp.StatusCode == http.StatusOK {
			bodyBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return proxyConfig, err
			}
			bodyString = string(bodyBytes)
		}

		err = json.Unmarshal([]byte(bodyString), &proxyConfig)
		if err != nil {
			return proxyConfig, err
		}

		Cache.Set("proxyConfig", proxyConfig, cache.DefaultExpiration)
	}

	return proxyConfig, nil
}
