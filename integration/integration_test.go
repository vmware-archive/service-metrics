// Copyright (C) 2015-Present Pivotal Software, Inc. All rights reserved.
// This program and the accompanying materials are made available under the terms of the under the Apache License,
// Version 2.0 (the "License”); you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and limitations under the License.

package integration_test

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/pivotal-cf/service-metrics/metrics"
	. "github.com/st3v/glager"
)

var _ = Describe("service-metrics", func() {

	var (
		origin          string
		metronAddr      string
		metricsCmd      string
		metricsCmdArgs  []string
		metricsInterval string
		debugLog        bool
		session         *gexec.Session
		metronPort      = ":3777"
	)

	var metricsJson = `
		[
			{
				"key": "loadMetric",
				"value": 4,
				"unit": "Load"
			},
			{
				"key": "temperatureMetric",
				"value": 99,
				"unit": "Temperature"
			}
		]
	`

	BeforeEach(func() {
		origin = "p-service-origin"
		debugLog = false
		metronAddr = "localhost" + metronPort
		metricsCmd = "/bin/echo"
		metricsCmdArgs = []string{"-n", metricsJson}
		metricsInterval = "10ms"
	})

	JustBeforeEach(func() {
		session = runCmd(origin, debugLog, metronAddr, metricsInterval, metricsCmd, metricsCmdArgs...)
	})

	AfterEach(func() {
		Eventually(session.Interrupt()).Should(gexec.Exit())
	})

	It("never exits", func() {
		Consistently(session.ExitCode).Should(Equal(-1))
	})

	It("logs the call to metrics command to stdout", func() {
		Eventually(func() *gbytes.Buffer {
			return session.Out
		}).Should(ContainSequence(
			Info(
				Message("service-metrics.executing-metrics-cmd"),
				Data("event", "starting"),
			),
			Info(
				Message("service-metrics.executing-metrics-cmd"),
				Data("event", "done"),
			),
		))
	})

	Context("when the metrics command exits with 1", func() {
		BeforeEach(func() {
			metricsCmd = "/bin/bash"
			metricsCmdArgs = []string{"-c", "echo -n failed to obtain metrics; exit 1"}
		})

		It("exits with an exit code of zero", func() {
			Eventually(session.ExitCode).Should(Equal(0))
		})

		It("logs the error to stdout", func() {
			Eventually(func() *gbytes.Buffer {
				return session.Out
			}).Should(ContainSequence(
				Error(
					AnyErr,
					Message("service-metrics.executing-metrics-cmd"),
					Data("event", "failed"),
					Data("output", "failed to obtain metrics"),
				),
			))
		})
	})

	Context("when the metrics command exits with 10", func() {
		BeforeEach(func() {
			metricsCmd = "/bin/bash"
			metricsCmdArgs = []string{"-c", "echo -n failed to obtain metrics; exit 10"}
		})

		It("never exits", func() {
			Consistently(session.ExitCode).Should(Equal(-1))
		})

		It("logs not ready to emit metrics and the metrics command output to stdout", func() {
			Eventually(func() *gbytes.Buffer {
				return session.Out
			}).Should(ContainSequence(
				Info(
					Message("service-metrics.executing-metrics-cmd"),
					Data("event", "not yet ready to emit metrics"),
					Data("output", "failed to obtain metrics"),
				),
			))
		})
	})

	Context("when the metrics command returns invalid JSON", func() {
		BeforeEach(func() {
			metricsCmd = "/bin/echo"
			metricsCmdArgs = []string{"-n", "invalid"}
		})

		It("exits with a non-zero exit code", func() {
			Eventually(session.ExitCode).Should(BeNumerically(">", 0))
		})

		It("logs a fatal error to stdout", func() {
			Eventually(func() *gbytes.Buffer {
				return session.Out
			}).Should(ContainSequence(
				Error(
					errors.New("invalid character 'i' looking for beginning of value"),
					Message("service-metrics.parsing-metrics-output"),
					Data("event", "failed"),
					Data("output", "invalid"),
				),
			))
		})
	})

	Context("when the metrics command does not exist", func() {
		BeforeEach(func() {
			metricsCmd = "/your/system/wont/have/this/yet"
			metricsCmdArgs = []string{}
		})

		It("exits with an exit code of 1", func() {
			Eventually(session.ExitCode).Should(Equal(1))
		})

		It("logs the error to stdout", func() {
			Eventually(func() *gbytes.Buffer {
				return session.Out
			}).Should(ContainSequence(
				Error(
					AnyErr,
					Message("service-metrics.executing-metrics-cmd"),
					Data("event", "failed"),
					Data("output", "no metrics command has been configured, cannot collect metrics"),
				),
			))
		})
	})

	Context("when the --origin param is not provided", func() {
		BeforeEach(func() {
			origin = ""
		})

		It("returns with a non-zero exit code", func() {
			Eventually(session.ExitCode).Should(BeNumerically(">", 0))
		})

		It("provides a meaningful error message", func() {
			Eventually(func() *gbytes.Buffer {
				return session.Err
			}).Should(gbytes.Say("Must provide --origin"))
		})
	})

	Context("when the --metrics-cmd param is not provided", func() {
		BeforeEach(func() {
			metricsCmd = ""
			metricsCmdArgs = []string{}
		})

		It("returns with a non-zero exit code", func() {
			Eventually(session.ExitCode).Should(BeNumerically(">", 0))
		})

		It("provides a meaningful error message", func() {
			Eventually(func() *gbytes.Buffer {
				return session.Err
			}).Should(gbytes.Say("Must provide --metrics-cmd"))
		})
	})

	Context("when the --metron-address param is not provided", func() {
		BeforeEach(func() {
			metronAddr = ""
		})

		It("exits with a non-zero exit code", func() {
			Eventually(session.ExitCode).Should(BeNumerically(">", 0))
		})

		It("provides a meaningful error message", func() {
			Eventually(func() *gbytes.Buffer {
				return session.Err
			}).Should(gbytes.Say("Must provide --metron-addr"))
		})
	})

	Context("when the --metrics-interval param is invalid", func() {
		BeforeEach(func() {
			metricsInterval = "10x"
		})

		It("returns with a non-zero exit code", func() {
			Eventually(session.ExitCode).Should(BeNumerically(">", 0))
		})

		It("provides a meaningful error message", func() {
			Eventually(func() *gbytes.Buffer {
				return session.Err
			}).Should(gbytes.Say("invalid value \"10x\" for flag -metrics-interval"))
		})
	})

	Context("when Metron is running", func() {
		var (
			lock            sync.RWMutex
			udpListener     net.PacketConn
			receivedOrigins []string
			receivedMetrics metrics.Metrics
		)

		var listenForEvents = func() {
			for {
				buffer := make([]byte, 1024)
				n, _, err := udpListener.ReadFrom(buffer)
				if err != nil {
					return
				}

				if n == 0 {
					panic("Received empty packet")
				}

				envelope := new(events.Envelope)
				err = proto.Unmarshal(buffer[0:n], envelope)
				if err != nil {
					panic(err)
				}

				if envelope.GetEventType() == events.Envelope_ValueMetric {
					lock.Lock()

					receivedOrigins = append(receivedOrigins, envelope.GetOrigin())

					m := envelope.GetValueMetric()
					receivedMetrics = append(
						receivedMetrics,
						metrics.Metric{
							Key:   m.GetName(),
							Value: m.GetValue(),
							Unit:  m.GetUnit(),
						},
					)
					lock.Unlock()
				}
			}
		}

		var assertMetricReceived = func(m metrics.Metric, count int) {
			var assertion = func() int {
				count := 0

				lock.RLock()
				defer lock.RUnlock()

				for _, received := range receivedMetrics {
					if received == m {
						count += 1
					}
				}

				return count
			}

			Eventually(assertion, 1*time.Second).Should(BeNumerically(">=", count))
		}

		BeforeEach(func() {
			var err error
			udpListener, err = net.ListenPacket("udp4", metronPort)
			Expect(err).ToNot(HaveOccurred())

			go listenForEvents()
		})

		AfterEach(func() {
			udpListener.Close()
		})

		It("repeatedly emits metrics to Metron", func() {
			assertMetricReceived(
				metrics.Metric{
					Key:   "loadMetric",
					Value: 4,
					Unit:  "Load",
				},
				50,
			)

			assertMetricReceived(
				metrics.Metric{
					Key:   "temperatureMetric",
					Value: 99,
					Unit:  "Temperature",
				},
				50,
			)

			for _, origin := range receivedOrigins {
				Expect(origin).To(Equal("p-service-origin"))
			}
		})
	})
})
