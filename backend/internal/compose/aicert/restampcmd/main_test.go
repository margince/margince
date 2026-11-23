// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

const corpusDir = "../corpus"

func TestRunNeedsTheRuleTheRecordsCarry(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-corpus", corpusDir, "-dir", t.TempDir()}, &out); err == nil || !strings.Contains(err.Error(), "-from") {
		t.Errorf("err = %v, want a refusal naming -from", err)
	}
}

// A rule no reviewer cleared is refused before any record is read.
func TestRunRefusesARuleNoReviewerCleared(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"-from", "grading-rule-unknown", "-corpus", corpusDir, "-dir", t.TempDir()}, &out)
	if err == nil || !strings.Contains(err.Error(), "restampableRules") {
		t.Errorf("err = %v, want a refusal naming restampableRules", err)
	}
}

func TestRunOverNoRecordsReportsNothingChanged(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-from", "grading-rule-13", "-write", "-corpus", corpusDir, "-dir", t.TempDir()}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); got != "0 restamped, 0 left stale (written: true)\n" {
		t.Errorf("report = %q", got)
	}
}

func TestRunRefusesAnUnknownFlag(t *testing.T) {
	if err := run([]string{"-nope"}, &bytes.Buffer{}); err == nil {
		t.Error("an unknown flag was accepted")
	}
}

// The committed records are already re-stamped, so a dry run over them lists
// what stays stale and writes nothing.
func TestRunOverTheCommittedRecordsListsWhatStaysStale(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-from", "grading-rule-13", "-corpus", corpusDir, "-dir", "../records"}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "stale    ") || !strings.Contains(got, "(written: false)") {
		t.Errorf("report does not list stale records and say nothing was written: %.300q", got)
	}
}

func TestRunRefusesAMissingCorpus(t *testing.T) {
	if err := run([]string{"-from", "grading-rule-13", "-corpus", t.TempDir() + "/none", "-dir", t.TempDir()}, &bytes.Buffer{}); err == nil {
		t.Error("a missing corpus was accepted")
	}
}

func TestWrittenRecordsAreReadBackFromTheirDirectory(t *testing.T) {
	dir := t.TempDir()
	rec := aicert.Record{Task: "summarize", Provider: "gemini", ServedModel: "m", EnvClass: "cloud_frontier", Verdict: aicert.VerdictCertified}
	if err := writeRecords(dir, []aicert.Record{rec}); err != nil {
		t.Fatalf("writeRecords: %v", err)
	}
	got, err := aicert.LoadRecords(dir)
	if err != nil || len(got) != 1 || got[0].Verdict != aicert.VerdictCertified {
		t.Errorf("read back %v, %v; want the one record written", got, err)
	}
	if err := writeRecords("/dev/null/none", []aicert.Record{rec}); err == nil {
		t.Error("a directory that cannot be made was written to")
	}
}
