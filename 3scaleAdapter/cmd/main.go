// Copyright 2018 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"github.com/3scale/istio-integration/3scaleAdapter"
	"github.com/spf13/viper"
	"istio.io/istio/pkg/log"
	"errors"
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
	} else {
		return log.InfoLevel, errors.New("invalid log_level")
	}
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