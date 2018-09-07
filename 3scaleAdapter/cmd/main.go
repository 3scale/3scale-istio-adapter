package main

import (
	"errors"
	"fmt"
	"github.com/3scale/istio-integration/3scaleAdapter"
	"github.com/spf13/viper"
	"istio.io/istio/pkg/log"
	"os"
)

func init() {
	viper.SetEnvPrefix("threescale")
	viper.BindEnv("log_level")
	viper.BindEnv("log_json")
	viper.BindEnv("listen_addr")

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

	s, err := threescale.NewThreescale(addr)

	if err != nil {
		log.Errorf("Unable to start sever: %v", err)
		os.Exit(1)
	}

	shutdown := make(chan error, 1)
	go func() {
		s.Run(shutdown)
	}()
	_ = <-shutdown
}
