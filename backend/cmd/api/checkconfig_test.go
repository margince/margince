// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// --check-config loads what a boot loads and stops. testDSN names a database
// nobody runs, so a pass proves no connection was tried.
func TestCheckConfigPassesAValidConfigurationWithoutADatabase(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	mustWrite(t, path, "version: 1\nworkspace:\n  name: Acme\n  base_currency: EUR\nlog_level: debug\n")
	var out strings.Builder
	checked, err := checkAPIConfig([]string{"--check-config", "--config", path, "--dsn", testDSN}, nil, &out)
	if !checked || err != nil {
		t.Fatalf("checked %v, err %v; want a passing check", checked, err)
	}
	for _, want := range []string{"config: read base_file " + path, "log_level = debug (base_file log_level " + path + ")", "dsn = <secret> (flag --dsn)", "config: valid"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the check output lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "margince_test") {
		t.Errorf("the check printed the DSN:\n%s", out.String())
	}
}

// Only the worker reads job_drain_window. The api check names it as one the
// worker must check, instead of calling the whole file valid.
func TestCheckConfigNamesAWorkerSettingItDoesNotCheck(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	mustWrite(t, path, "version: 1\nworkspace:\n  name: Acme\n  base_currency: EUR\njob_drain_window: soon\n")
	var out strings.Builder
	checked, err := checkAPIConfig([]string{"--check-config", "--config", path, "--dsn", testDSN}, nil, &out)
	if !checked || err != nil {
		t.Fatalf("checked %v, err %v; the api reads no worker setting, so it has no fault to report", checked, err)
	}
	for _, want := range []string{
		"config: not checked here, another program reads it: job_drain_window (base_file " + path + ")",
		"config: valid for this program; the 1 setting(s) another program reads need its own --check-config",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the check output lacks %q:\n%s", want, out.String())
		}
	}
}

// A key written twice is refused with both lines, so the operator sees which
// copy to delete instead of which one silently won.
func TestCheckConfigRefusesARepeatedKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	mustWrite(t, path, "version: 1\nworkspace:\n  name: Acme\n  base_currency: EUR\nlog_level: debug\nlog_level: info\n")
	var out strings.Builder
	checked, err := checkAPIConfig([]string{"--check-config", "--config", path, "--dsn", testDSN}, nil, &out)
	if !checked || err == nil {
		t.Fatalf("checked %v, err %v; want a failing check", checked, err)
	}
	if want := path + ": log_level appears on lines 5 and 6; keep one"; !strings.Contains(err.Error(), want) {
		t.Errorf("the report lacks %q:\n%v", want, err)
	}
}

// The bootstrap roots fault in the same report as the settings, so one run
// names every fault in the file.
func TestCheckConfigReportsBootstrapAndSettingFaultsTogether(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	mustWrite(t, path, "version: 1\nworkspace:\n  base_language: fr\nbootstrap_admin:\n  email: a@acme.test\n  dispaly_name: A\nlog_lvl: debug\n")
	var out strings.Builder
	checked, err := checkAPIConfig([]string{"--check-config", "--config", path}, nil, &out)
	if !checked || err == nil {
		t.Fatalf("checked %v, err %v; want a failing check", checked, err)
	}
	for _, want := range []string{
		"MARGINCE_DSN: required",
		path + ": unknown key log_lvl (line 7); did you mean log_level?",
		path + ": unknown key bootstrap_admin.dispaly_name (line 6); did you mean bootstrap_admin.display_name?",
		`workspace.base_language in ` + path + ` (base_file): "fr" is not a language this build speaks`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the report lacks %q:\n%v", want, err)
		}
	}
	if strings.Contains(out.String(), "config: valid") {
		t.Errorf("a failing check said valid:\n%s", out.String())
	}
}

// Without the flag the boot goes on as before.
func TestWithoutTheFlagNoCheckRuns(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	checked, err := checkAPIConfig([]string{"--dsn", testDSN}, nil, &out)
	if checked || err != nil || out.Len() != 0 {
		t.Fatalf("checked %v, err %v, output %q; want no check", checked, err, out.String())
	}
}
