// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The api's usage output and boot errors must never carry a value the
// environment supplied. That environment holds DSNs, OAuth client secrets and
// signing keys, and `-h` or one mistyped argument writes to the pod's log.
//
// The variable set comes from APISettings itself, so a setting added later is
// covered the day it is added.

import (
	"io"
	"os"
	"strings"
	"testing"
)

const leakSentinel = "SENTINEL_VALUE_MUST_NOT_APPEAR"

// sentinelEnviron sets every credential the api's own settings read to the
// sentinel. A setting that is not Secret may quote its value in a fault.
func sentinelEnviron(t *testing.T) []string {
	t.Helper()
	var environ []string
	for _, s := range APISettings() {
		if s.Secret() {
			environ = append(environ, s.Env()+"="+leakSentinel)
		}
	}
	if len(environ) == 0 {
		t.Fatal("APISettings declares no secret, so this gate checks nothing")
	}
	return environ
}

// captureStderr runs fn with os.Stderr redirected, returning what was written.
// loadAPIConfig writes its usage there, and that writer is the one that matters.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		_, copyErr := io.Copy(&b, r)
		if copyErr != nil {
			done <- "READ FAILED: " + copyErr.Error()
			return
		}
		done <- b.String()
	}()

	fn()

	os.Stderr = saved
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("closing the capture pipe: %v", closeErr)
	}
	return <-done
}

func TestAPIUsageOutputCarriesNoEnvironmentValue(t *testing.T) {
	environ := sentinelEnviron(t)
	var helpErr, unknownErr error
	usage := captureStderr(t, func() {
		_, _, helpErr = loadAPIConfig([]string{"-h"}, environ)
		_, _, unknownErr = loadAPIConfig([]string{"--nosuchflag"}, environ)
	})
	if helpErr == nil || unknownErr == nil {
		t.Fatalf("-h answered %v and an undefined flag %v, want both refused", helpErr, unknownErr)
	}
	for line := range strings.SplitSeq(usage, "\n") {
		if strings.Contains(line, leakSentinel) {
			t.Errorf("the usage text echoes an environment value: %s", strings.TrimSpace(line))
		}
	}
	for _, err := range []error{helpErr, unknownErr} {
		if strings.Contains(err.Error(), leakSentinel) {
			t.Errorf("the parse error echoes an environment value: %v", err)
		}
	}
	// An empty usage block would satisfy every assertion above by containing
	// nothing at all.
	if !strings.Contains(usage, "-dsn") || !strings.Contains(usage, "-config-file") {
		t.Fatalf("no usage text was captured, so nothing above was checked:\n%s", usage)
	}
}

// The faults of a boot with every credential set name no credential.
func TestAPIBootFaultsCarryNoEnvironmentValue(t *testing.T) {
	t.Parallel()
	environ := append(sentinelEnviron(t), "MARGINCE_METRICS_ACCESS=open", "MARGINCE_GRAPH_TENANT=a,b")
	_, _, err := loadAPIConfig(nil, environ)
	if err == nil {
		t.Fatal("a boot with two contradicting settings was accepted")
	}
	if strings.Contains(err.Error(), leakSentinel) {
		t.Errorf("a boot fault echoes an environment value:\n%v", err)
	}
}

// The environment reaches the settings, the base file's path included.
func TestAPIStillReadsTheEnvironment(t *testing.T) {
	t.Parallel()
	cfg, _, err := loadAPIConfig(nil, []string{
		"MARGINCE_DSN=postgres://u@127.0.0.1:1/from-env",
		"MARGINCE_CONFIG=/from/env.yaml",
	})
	if err != nil {
		t.Fatalf("loading with no arguments: %v", err)
	}
	if cfg.dsn != "postgres://u@127.0.0.1:1/from-env" {
		t.Errorf("--dsn did not come from the environment: %q", cfg.dsn)
	}
	if cfg.configPath != "/from/env.yaml" {
		t.Errorf("--config did not come from the environment: %q", cfg.configPath)
	}
}

// And an explicit flag still beats the environment.
func TestAPIFlagBeatsTheEnvironment(t *testing.T) {
	t.Parallel()
	cfg, _, err := loadAPIConfig([]string{"--config", "/from/flag.yaml", "--addr", ":9"}, []string{
		"MARGINCE_DSN=postgres://u@127.0.0.1:1/db",
		"MARGINCE_CONFIG=/from/env.yaml",
		"MARGINCE_ADDR=:7",
	})
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if cfg.configPath != "/from/flag.yaml" || cfg.addr != ":9" {
		t.Errorf("the environment overrode an explicit flag: config %q, addr %q", cfg.configPath, cfg.addr)
	}
}
