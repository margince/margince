// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/platform/config"
)

// TestFxBootstrapCurrenciesFallsBackToTheDefault pins the fresh-install
// contract: an operator who configures no candidate set still gets the
// USD/GBP/CHF default, so "Refresh from sources" bootstraps an empty FX sheet
// rather than being a dead button. A configured set is used verbatim.
func TestFxBootstrapCurrenciesFallsBackToTheDefault(t *testing.T) {
	if got := strings.Join(fxBootstrapCurrencies(nil), ","); got != "USD,GBP,CHF" {
		t.Fatalf("unset config = %q, want the USD,GBP,CHF default", got)
	}
	if got := strings.Join(fxBootstrapCurrencies([]string{}), ","); got != "USD,GBP,CHF" {
		t.Fatalf("empty config = %q, want the USD,GBP,CHF default", got)
	}
	if got := strings.Join(fxBootstrapCurrencies([]string{"JPY", "SEK"}), ","); got != "JPY,SEK" {
		t.Fatalf("configured set = %q, want it used verbatim", got)
	}
}

const workerTestDSN = "MARGINCE_DSN=postgres://localhost/x"

// TestObservePprofIsParsedStrictlyAndNeedsAListener pins --observe-pprof's
// three boot outcomes: off by default, on only alongside a listener to mount
// on, and a value that is not a boolean refused rather than read as off.
func TestObservePprofIsParsedStrictlyAndNeedsAListener(t *testing.T) {
	t.Parallel()
	base := []string{workerTestDSN}

	cfg, _, err := loadWorkerConfig(nil, base)
	if err != nil {
		t.Fatalf("loadWorkerConfig with no pprof setting: %v", err)
	}
	if cfg.observePprof {
		t.Error("--observe-pprof is on with nothing set; off must be the default")
	}

	cfg, _, err = loadWorkerConfig([]string{"--observe-addr=127.0.0.1:9101", "--observe-pprof=true"}, base)
	if err != nil {
		t.Fatalf("--observe-pprof=true alongside --observe-addr: %v", err)
	}
	if !cfg.observePprof {
		t.Error("--observe-pprof=true alongside --observe-addr parsed to off")
	}

	// From the environment too, which is how a deployment sets it.
	cfg, _, err = loadWorkerConfig(nil, append([]string{"MARGINCE_OBSERVE_ADDR=127.0.0.1:9101", "MARGINCE_OBSERVE_PPROF=true"}, base...))
	if err != nil {
		t.Fatalf("MARGINCE_OBSERVE_PPROF=true alongside MARGINCE_OBSERVE_ADDR: %v", err)
	}
	if !cfg.observePprof {
		t.Error("MARGINCE_OBSERVE_PPROF=true parsed to off")
	}

	// A typo is a boot error naming the variable, not a silent off.
	if _, _, err := loadWorkerConfig(nil, append([]string{"MARGINCE_OBSERVE_ADDR=127.0.0.1:9101", "MARGINCE_OBSERVE_PPROF=ture"}, base...)); err == nil {
		t.Error("MARGINCE_OBSERVE_PPROF=ture was accepted; a value that is not a boolean must fail the boot")
	} else if !strings.Contains(err.Error(), "MARGINCE_OBSERVE_PPROF") {
		t.Errorf("the error does not name the variable that failed: %v", err)
	}

	// On with no listener would do nothing while the operator believes it
	// did something, so it is refused.
	if _, _, err := loadWorkerConfig(nil, append([]string{"MARGINCE_OBSERVE_PPROF=true"}, base...)); err == nil {
		t.Error("MARGINCE_OBSERVE_PPROF=true with no MARGINCE_OBSERVE_ADDR was accepted; there is no listener to serve it on")
	} else if !strings.Contains(err.Error(), "MARGINCE_OBSERVE_ADDR") {
		t.Errorf("the error does not name the missing listener: %v", err)
	}
}

// The drain window is part of a budget the operator sizes the pod's grace
// period against, so its default is pinned, the environment can move it, a
// value that does not parse fails the boot, and zero — River's hard stop — is
// refused rather than taken.
func TestTheJobDrainWindowIsBoundedAndConfigurable(t *testing.T) {
	t.Parallel()
	const drainEnv = "MARGINCE_JOB_DRAIN_WINDOW"
	base := []string{workerTestDSN}
	cfg, _, err := loadWorkerConfig(nil, base)
	if err != nil {
		t.Fatalf("loadWorkerConfig: %v", err)
	}
	if cfg.jobDrainWindow != programsettings.DefaultJobDrainWindow {
		t.Errorf("default drain window = %s, want %s", cfg.jobDrainWindow, programsettings.DefaultJobDrainWindow)
	}
	if budget := programsettings.DefaultJobDrainWindow + jobCancelWindow; budget > 25*time.Second {
		t.Errorf("the default drain plus cancel windows come to %s, which leaves no teardown room inside a 30s termination grace period", budget)
	}

	cfg, _, err = loadWorkerConfig(nil, append([]string{drainEnv + "=45s"}, base...))
	if err != nil {
		t.Fatalf("%s=45s: %v", drainEnv, err)
	}
	if cfg.jobDrainWindow != 45*time.Second {
		t.Errorf("%s=45s produced %s", drainEnv, cfg.jobDrainWindow)
	}

	if _, _, err := loadWorkerConfig(nil, append([]string{drainEnv + "=a while"}, base...)); err == nil || !strings.Contains(err.Error(), drainEnv) {
		t.Errorf("an unparseable %s must fail the boot naming the variable, got %v", drainEnv, err)
	}

	for _, v := range []string{"0s", "-1s"} {
		if _, _, err := loadWorkerConfig([]string{"--job-drain-window=" + v}, base); err == nil {
			t.Errorf("--job-drain-window=%s was accepted; it would make every shutdown cancel running jobs at once", v)
		}
	}
}

// The worker joins every fault of one boot into one error, as the api does.
func TestTheWorkerReportsEveryConfigurationFaultTogether(t *testing.T) {
	t.Parallel()
	_, _, err := loadWorkerConfig([]string{"--observe-pprof", "--job-drain-window=0s"}, []string{"MARGINCE_AI_FAKE=true", "MARGINCE_ENV=production"})
	for _, want := range []string{"MARGINCE_DSN: required", "MARGINCE_OBSERVE_ADDR is unset", "--job-drain-window", "MARGINCE_AI_FAKE"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

// A misspelled variable reaches the boot report, and one a sibling program
// reads does not.
func TestAMisspelledVariableReachesTheBootReport(t *testing.T) {
	t.Parallel()
	// Assembled rather than written whole: the documentation gate reads a
	// quoted MARGINCE_* literal as a variable somebody must document.
	const misspelled = "MARGINCE_" + "REDDIS"
	_, loaded, err := loadWorkerConfig(nil, []string{workerTestDSN, misspelled + "=localhost:6379", "MARGINCE_METRICS_TOKEN=the-apis"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !slices.Contains(loaded.Warnings(), misspelled) {
		t.Errorf("warnings = %v; the misspelling never reached the boot report", loaded.Warnings())
	}
	for _, read := range []string{"MARGINCE_DSN", "MARGINCE_METRICS_TOKEN"} {
		if slices.Contains(loaded.Warnings(), read) {
			t.Errorf("%s is read by a program and was reported as unread", read)
		}
	}
}

func TestTheWorkerMarksItsCredentials(t *testing.T) {
	t.Parallel()
	reg, err := compose.ProgramRegistry(config.RoleWorker)
	if err != nil {
		t.Fatal(err)
	}
	secret := map[string]bool{}
	for _, s := range reg.All() {
		secret[s.Env()] = s.Secret()
	}
	for _, name := range []string{
		"MARGINCE_DSN", "MARGINCE_GMAIL_CLIENT_SECRET", "MARGINCE_GRAPH_CLIENT_SECRET",
		"MARGINCE_WEBHOOK_KEY", "MARGINCE_KEYVAULT_ROOT_KEY", "MARGINCE_LICENSE",
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY",
		// The Graph notification URL carries the push token in its query.
		"MARGINCE_GRAPH_NOTIFICATION_URL",
	} {
		if !secret[name] {
			t.Errorf("%s authenticates something and is not marked Secret", name)
		}
	}
	// A value that authenticates nobody stays visible, because an operator
	// debugging a boot wants to read it.
	for _, name := range []string{"MARGINCE_LOG_LEVEL", "MARGINCE_PUBLIC_BASE_URL", "MARGINCE_OBSERVE_ADDR"} {
		if secret[name] {
			t.Errorf("%s is marked Secret but authenticates nothing; hiding it only makes a boot harder to debug", name)
		}
	}
}
