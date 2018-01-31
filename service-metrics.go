// Copyright (C) 2015-Present Pivotal Software, Inc. All rights reserved.
// This program and the accompanying materials are made available under the terms of the under the Apache License,
// Version 2.0 (the "License"); you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and limitations under the License.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/dropsonde"
	dmetrics "github.com/cloudfoundry/dropsonde/metrics"
	"github.com/pivotal-cf/service-metrics/metrics"
)

var (
	origin          string
	metronAddr      string
	metricsInterval time.Duration
	metricsCmd      string
	metricsCmdArgs  multiFlag
	debug           bool
)

type multiFlag []string

func (m *multiFlag) String() string {
	return fmt.Sprint(metricsCmdArgs)
}

func (m *multiFlag) Set(value string) error {
	if metricsCmdArgs == nil {
		metricsCmdArgs = multiFlag{}
	}

	metricsCmdArgs = append(metricsCmdArgs, value)

	return nil
}

func assertFlag(name, value string) {
	if value == "" {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "\nMust provide --%s", name)
		os.Exit(1)
	}
}

func main() {
	flag.StringVar(&origin, "origin", "", "Required. Source name for metrics emitted by this process, e.g. service-name")
	flag.StringVar(&metronAddr, "metron-addr", "", "Required. Metron address, e.g. localhost:2346")
	flag.StringVar(&metricsCmd, "metrics-cmd", "", "Required. Path to metrics command")
	flag.Var(&metricsCmdArgs, "metrics-cmd-arg", "Argument to pass on to metrics-cmd (multi-valued)")
	flag.DurationVar(&metricsInterval, "metrics-interval", time.Minute, "Interval to run metrics-cmd")
	flag.BoolVar(&debug, "debug", false, "Output debug logging")

	flag.Parse()

	assertFlag("origin", origin)
	assertFlag("metron-addr", metronAddr)
	assertFlag("metrics-cmd", metricsCmd)

	stdoutLogLevel := lager.INFO
	if debug {
		stdoutLogLevel = lager.DEBUG
	}

	logger := lager.NewLogger("service-metrics")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, stdoutLogLevel))
	logger.RegisterSink(lager.NewWriterSink(os.Stderr, lager.ERROR))

	err := dropsonde.Initialize(metronAddr, origin)
	if err != nil {
		logger.Error("Dropsonde failed to initialize", err)
		os.Exit(1)
	}

	process(logger)
	for {
		select {
		case <-time.After(metricsInterval):
			process(logger)
		}
	}
}

func process(logger lager.Logger) {
	action := "executing-metrics-cmd"

	logger.Info(action, lager.Data{
		"event": "starting",
	})

	cmd := exec.Command(metricsCmd, metricsCmdArgs...)
	out, err := cmd.CombinedOutput()

	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			logger.Error(action, err, lager.Data{
				"event":  "failed",
				"output": "no metrics command has been configured, cannot collect metrics",
			})
			os.Exit(1)
		}

		exitStatus := cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus()
		if exitStatus == 10 {
			logger.Info(action, lager.Data{
				"event":  "not yet ready to emit metrics",
				"output": string(out),
			})
			return
		}

		logger.Error(action, err, lager.Data{
			"event":  "failed",
			"output": string(out),
		})
		os.Exit(0)
	}

	logger.Info(action, lager.Data{
		"event": "done",
	})

	var parsedMetrics metrics.Metrics

	decoder := json.NewDecoder(bytes.NewReader(out))
	err = decoder.Decode(&parsedMetrics)
	if err != nil {
		logger.Error("parsing-metrics-output", err, lager.Data{
			"event":  "failed",
			"output": string(out),
		})
		os.Exit(1)
	}

	for _, m := range parsedMetrics {
		err := dmetrics.SendValue(m.Key, m.Value, m.Unit)
		if err != nil {
			logger.Error("sending metric value failed", err, lager.Data{
				"event":        "failed",
				"metric.key":   m.Key,
				"metric.value": m.Value,
				"metric.unit":  m.Unit,
			})
		}
	}
}
