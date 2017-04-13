// Copyright (C) 2015-Present Pivotal Software, Inc. All rights reserved.
// This program and the accompanying materials are made available under the terms of the under the Apache License,
// Version 2.0 (the "License”); you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and limitations under the License.

package integration_test

import (
	"log"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var execPath string

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	srcPath := "github.com/pivotal-cf/service-metrics"
	var err error
	execPath, err = gexec.Build(srcPath)
	if err != nil {
		log.Fatalf("executable %s could not be built: %s", srcPath, err)
	}
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})

func runCmd(origin string, debugLog bool, metronAddress, metricsInterval, metricsCmd string, metricsCmdArgs ...string) *gexec.Session {
	cmdArgs := []string{
		"--origin", origin,
		"--metron-addr", metronAddress,
		"--metrics-interval", metricsInterval,
		"--metrics-cmd", metricsCmd,
	}

	if debugLog {
		cmdArgs = append(cmdArgs, "--debug")
	}

	for _, arg := range metricsCmdArgs {
		cmdArgs = append(cmdArgs, "--metrics-cmd-arg", arg)
	}

	cmd := exec.Command(execPath, cmdArgs...)

	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	return session
}
