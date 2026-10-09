// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
	"time"
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

// TestObservePprofIsParsedStrictlyAndNeedsAListener pins --observe-pprof's
// three boot outcomes: off by default, on only alongside a listener to mount
// on, and a value that is not a boolean refused rather than read as off.
func TestObservePprofIsParsedStrictlyAndNeedsAListener(t *testing.T) {
	base := []string{"--dsn", "postgres://localhost/x"}

	cfg, err := parseWorkerFlags(base)
	if err != nil {
		t.Fatalf("parseWorkerFlags with no pprof setting: %v", err)
	}
	if cfg.observePprof {
		t.Error("--observe-pprof is on with nothing set; off must be the default")
	}

	cfg, err = parseWorkerFlags(append(append([]string{}, base...), "--observe-addr=127.0.0.1:9101", "--observe-pprof=true"))
	if err != nil {
		t.Fatalf("--observe-pprof=true alongside --observe-addr: %v", err)
	}
	if !cfg.observePprof {
		t.Error("--observe-pprof=true alongside --observe-addr parsed to off")
	}

	// From the environment too, which is how a deployment sets it.
	t.Setenv("MARGINCE_OBSERVE_ADDR", "127.0.0.1:9101")
	t.Setenv("MARGINCE_OBSERVE_PPROF", "true")
	cfg, err = parseWorkerFlags(base)
	if err != nil {
		t.Fatalf("MARGINCE_OBSERVE_PPROF=true alongside MARGINCE_OBSERVE_ADDR: %v", err)
	}
	if !cfg.observePprof {
		t.Error("MARGINCE_OBSERVE_PPROF=true parsed to off")
	}

	// A typo is a boot error naming the variable, not a silent off.
	t.Setenv("MARGINCE_OBSERVE_PPROF", "ture")
	if _, err := parseWorkerFlags(base); err == nil {
		t.Error("MARGINCE_OBSERVE_PPROF=ture was accepted; a value that is not a boolean must fail the boot")
	} else if !strings.Contains(err.Error(), "MARGINCE_OBSERVE_PPROF") {
		t.Errorf("the error does not name the variable that failed: %v", err)
	}

	// On with no listener would do nothing while the operator believes it
	// did something, so it is refused.
	t.Setenv("MARGINCE_OBSERVE_ADDR", "")
	t.Setenv("MARGINCE_OBSERVE_PPROF", "true")
	if _, err := parseWorkerFlags(base); err == nil {
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
	base := []string{"--dsn", "postgres://localhost/x"}
	cfg, err := parseWorkerFlags(base)
	if err != nil {
		t.Fatalf("parseWorkerFlags: %v", err)
	}
	if cfg.jobDrainWindow != defaultJobDrainWindow {
		t.Errorf("default drain window = %s, want %s", cfg.jobDrainWindow, defaultJobDrainWindow)
	}
	// The declared item spells the default as a literal, because the docs gate
	// reads it from source; it has to be the constant the flag uses.
	for _, item := range workerUnflaggedItems() {
		if item.Name == jobDrainWindowEnv && item.Default != defaultJobDrainWindow.String() {
			t.Errorf("%s declares default %q, the flag defaults to %s", jobDrainWindowEnv, item.Default, defaultJobDrainWindow)
		}
	}
	if budget := defaultJobDrainWindow + jobCancelWindow; budget > 25*time.Second {
		t.Errorf("the default drain plus cancel windows come to %s, which leaves no teardown room inside a 30s termination grace period", budget)
	}

	t.Setenv(jobDrainWindowEnv, "45s")
	cfg, err = parseWorkerFlags(base)
	if err != nil {
		t.Fatalf("%s=45s: %v", jobDrainWindowEnv, err)
	}
	if cfg.jobDrainWindow != 45*time.Second {
		t.Errorf("%s=45s produced %s", jobDrainWindowEnv, cfg.jobDrainWindow)
	}

	t.Setenv(jobDrainWindowEnv, "a while")
	if _, err := parseWorkerFlags(base); err == nil || !strings.Contains(err.Error(), jobDrainWindowEnv) {
		t.Errorf("an unparseable %s must fail the boot naming the variable, got %v", jobDrainWindowEnv, err)
	}

	t.Setenv(jobDrainWindowEnv, "")
	for _, v := range []string{"0s", "-1s"} {
		if _, err := parseWorkerFlags(append(append([]string{}, base...), "--job-drain-window="+v)); err == nil {
			t.Errorf("--job-drain-window=%s was accepted; it would make every shutdown cancel running jobs at once", v)
		}
	}
}
