package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/3scale/istio-integration/3scaleAdapter/pkg/threescale"
	"github.com/3scale/istio-integration/3scaleAdapter/pkg/threescale/metrics"
	"github.com/spf13/viper"
	"istio.io/istio/pkg/log"
)

func init() {
	viper.SetEnvPrefix("threescale")
	viper.BindEnv("log_level")
	viper.BindEnv("log_json")
	viper.BindEnv("listen_addr")
	viper.BindEnv("report_metrics")
	viper.BindEnv("metrics_port")

	options := log.DefaultOptions()

	if viper.IsSet("log_level") {
		loglevel := viper.GetString("log_level")
		parsedLogLevel, err := stringToLogLevel(loglevel)

		if err != nil {
			fmt.Printf("THREESCALE_LOG_LEVEL is not valid, expected: debug,info,warn,error or none. Got: %v\n", loglevel)
			os.Exit(1)
		}

		options.SetOutputLevel(log.DefaultScopeName, parsedLogLevel)
	}

	if viper.IsSet("log_json") {
		options.JSONEncoding = viper.GetBool("log_json")
	}

	log.Configure(options)
	log.Infof("Logging started")

}

func stringToLogLevel(loglevel string) (log.Level, error) {

	stringToLevel := map[string]log.Level{
		"debug": log.DebugLevel,
		"info":  log.InfoLevel,
		"warn":  log.WarnLevel,
		"error": log.ErrorLevel,
		"none":  log.NoneLevel,
	}

	if val, ok := stringToLevel[loglevel]; ok {
		return val, nil
	}

	return log.InfoLevel, errors.New("invalid log_level")
}

func main() {
	var addr string

	if viper.IsSet("listen_addr") {
		addr = viper.GetString("listen_addr")
	} else {
		addr = "0"
	}

	c := &http.Client{
		// Setting some sensible default here for http timeouts
		// This should probably come from a flag/env
		Timeout: time.Duration(time.Second * 10),
	}

	proxyCache := threescale.NewProxyConfigCache(
		threescale.DefaultCacheTTL, threescale.DefaultCacheRefreshBuffer, threescale.DefaultCacheLimit)

	var metricsConf *threescale.MetricsConfig
	if viper.IsSet("report_metrics") {
		gatherMetrics := viper.GetBool("report_metrics")
		if gatherMetrics {
			port := metrics.DefaultMetricsPort
			if viper.IsSet("metrics_port") {
				port = viper.GetInt("metrics_port")
			}
			metricsConf = threescale.NewMetricsConfig(gatherMetrics, port)
		}
	}

	s, err := threescale.NewThreescale(addr, c, proxyCache, metricsConf)

	if err != nil {
		log.Errorf("Unable to start sever: %v", err)
		os.Exit(1)
	}

	proxyCache.StartRefreshWorker()
	if err != nil {
		log.Errorf("error while starting cache refresh worker %s", err.Error())
	}

	shutdown := make(chan error, 1)
	go func() {
		s.Run(shutdown)
	}()

	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGTERM)

	go func() {
		_ = <-sigC
		fmt.Println("SIGTERM received. Attempting graceful shutdown")
		err := s.Close()
		if err != nil {
			fmt.Println("Error calling graceful shutdown")
			os.Exit(1)
		}
		return
	}()

	_ = <-shutdown
}
