// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

func TestTheCheckFlagIsRequestedOnlyWhenGiven(t *testing.T) {
	reg, err := config.NewRegistry([]config.Any{level})
	if err != nil {
		t.Fatal(err)
	}
	for args, want := range map[string]bool{
		"--check-config":                  true,
		"--check-config --log-level=warn": true,
		"--check-config=false":            false,
		"--log-level=warn":                false,
		"--check-config --no-such-flag":   false,
	} {
		if got := config.CheckRequested(reg, strings.Fields(args)); got != want {
			t.Errorf("CheckRequested(%s) = %v, want %v", args, got, want)
		}
	}
}

// A valid configuration prints its trace, a secret by name only, and says so.
func TestACheckOfAValidConfigurationPrintsTheTrace(t *testing.T) {
	reg, src := sources(t, fixture{
		args: []string{"--check-config"},
		env:  []string{"MARGINCE_DSN=postgres://user:hunter2@db/crm"},
	}, level, dsn)
	var out strings.Builder
	if err := config.Check(&out, reg, src); err != nil {
		t.Fatalf("a valid configuration failed the check: %v", err)
	}
	for _, want := range []string{"config: read env 1", "log.level = info (default)", "dsn = <secret> (env MARGINCE_DSN)", "config: valid"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the check output lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "hunter2") {
		t.Errorf("the check printed a secret:\n%s", out.String())
	}
}

// A broken configuration returns every fault and still prints what it read,
// with each variable no program reads.
func TestACheckOfABrokenConfigurationReturnsEveryFault(t *testing.T) {
	reg, src := sources(t, fixture{
		args:  []string{"--check-config", "--log-level=loud"},
		env:   []string{ns + "LOG_LVL=debug"},
		files: map[string]string{"margince.yaml": "lgo:\n  level: debug\n"},
	}, level, dsn)
	src.Faults = func(*config.Config) []error { return []error{errors.New("a bootstrap fault")} }
	var out strings.Builder
	err := config.Check(&out, reg, src)
	if err == nil {
		t.Fatal("a broken configuration passed the check")
	}
	for _, want := range []string{"--log-level (flag): want one of", "MARGINCE_DSN: required", "unknown key lgo (line 1); did you mean log?", "a bootstrap fault"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the faults lack %q:\n%v", want, err)
		}
	}
	for _, want := range []string{"config: read base_file", "set but read by no Margince program: " + ns + "LOG_LVL (did you mean " + ns + "LOG_LEVEL?)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the check output lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "config: valid") {
		t.Errorf("a broken configuration was called valid:\n%s", out.String())
	}
}

// A setting only a sibling program reads is never parsed here. The check names
// it, wherever it is set, instead of calling the whole file valid.
func TestACheckNamesTheSettingsAnotherProgramReads(t *testing.T) {
	reg, src := sources(t, fixture{
		args:  []string{"--check-config"},
		env:   []string{ns + "DSN=postgres://db/crm", ns + "JOB_DRAIN_WINDOW=later"},
		files: map[string]string{"margince.yaml": "log:\n  level: warn\njob_drain_window: soon\n"},
	}, level, dsn)
	src.Known = config.Names{Keys: []string{"job_drain_window"}, Envs: []string{ns + "JOB_DRAIN_WINDOW"}}
	var out strings.Builder
	if err := config.Check(&out, reg, src); err != nil {
		t.Fatalf("a sibling's setting failed this program's check: %v", err)
	}
	for _, want := range []string{
		"config: not checked here, another program reads it: job_drain_window (base_file ",
		"config: not checked here, another program reads it: " + ns + "JOB_DRAIN_WINDOW (env)",
		"config: valid for this program; the 2 setting(s) another program reads need its own --check-config",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the check output lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "another program reads it: log.level") {
		t.Errorf("the check named one of its own settings as another program's:\n%s", out.String())
	}
}

// One misspelt parent is one fault naming the section it meant, beside the
// suggestion a misspelt leaf gets.
func TestAnUnknownYAMLKeyNamesTheKeyItLikelyMeant(t *testing.T) {
	_, err := load(t, fixture{files: map[string]string{
		"margince.yaml": "lgo:\n  level: debug\n  format: json\nlog:\n  lvl: debug\n",
	}}, level)
	if err == nil {
		t.Fatal("want faults")
	}
	msg := err.Error()
	for _, want := range []string{"unknown key lgo (line 1); did you mean log?", "unknown key log.lvl (line 5); did you mean log.level?"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "lgo.level") || strings.Contains(msg, "lgo.format") {
		t.Errorf("a misspelt section was reported once per child:\n%s", msg)
	}
}

// YAML forbids a key twice in one mapping, and the parser keeps the last. A
// repeat is a fault naming both lines, in a setting section or a bootstrap one.
func TestARepeatedYAMLKeyIsAFaultNamingBothLines(t *testing.T) {
	reg, src := sources(t, fixture{
		args: []string{"--check-config"},
		env:  []string{ns + "DSN=postgres://db/crm"},
		files: map[string]string{"margince.yaml": "log:\n  level: debug\n" +
			"workspace:\n  name: Acme\n  name: Beta\n" +
			"log:\n  level: info\n"},
	}, level, dsn)
	src.BootstrapRoots = map[string]bool{"workspace": true}
	var out strings.Builder
	err := config.Check(&out, reg, src)
	if err == nil {
		t.Fatalf("a file with repeated keys passed the check:\n%s", out.String())
	}
	for _, want := range []string{"log appears on lines 1 and 6; keep one", "workspace.name appears on lines 4 and 5; keep one"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the faults lack %q:\n%v", want, err)
		}
	}
}

func TestARefusalNamesItsProgramOnce(t *testing.T) {
	for err, want := range map[error]string{
		errors.New("api: log_level in margince.yaml: want one of debug, info"): "api: log_level in margince.yaml: want one of debug, info",
		errors.New("log_level in margince.yaml: want one of debug, info"):      "api: log_level in margince.yaml: want one of debug, info",
		errors.New("apiary: closed"):                                           "api: apiary: closed",
	} {
		if got := config.RoleAPI.Refusal(err); got != want {
			t.Errorf("Refusal(%q) = %q, want %q", err, got, want)
		}
	}
}
