// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/report.json from testdata/dataset.json")

var preregistered = filepath.Join("testdata", "preregistration.json")

var corpusArgs = []string{
	"-dataset", filepath.Join("testdata", "dataset.json"),
	"-train-end", "2027-02-07", "-validation-end", "2027-04-18", "-horizon-edges", "30,60", "-settle-days", "5",
}

func runOnCorpus(t *testing.T, args ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	noEnv := func(string) string { return "" }
	if err := run(context.Background(), slices.Concat(corpusArgs, args), noEnv, &out); err != nil {
		t.Fatalf("running on the corpus: %v", err)
	}
	return out.Bytes()
}

// The committed dataset is a versioned input, and the committed report is what
// this build prints for it. A change to either the scoring or the dataset
// shape shows here as a diff to review.
func TestTheCommittedDatasetPrintsTheCommittedReport(t *testing.T) {
	got := runOnCorpus(t, "-preregistration", preregistered)
	golden := filepath.Join("testdata", "report.json")
	if *update {
		if err := os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading the committed report: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("the corpus no longer prints the committed report; run with -update and review the diff:\n%s", got)
	}
	if bytes.Contains(bytes.ToLower(got), []byte("accuracy")) {
		t.Fatal("the report states an accuracy figure")
	}
}

func TestAWrittenDatasetScoresAsTheOneItWasReadFrom(t *testing.T) {
	copied := filepath.Join(t.TempDir(), "dataset.json")
	first := runOnCorpus(t, "-write-dataset", copied)
	var out bytes.Buffer
	args := slices.Concat(slicesWithout(corpusArgs, "-dataset"), []string{"-dataset", copied})
	if err := run(context.Background(), args, func(string) string { return "" }, &out); err != nil {
		t.Fatalf("scoring the written dataset: %v", err)
	}
	if !bytes.Equal(first, out.Bytes()) {
		t.Fatal("the written dataset scored differently from the one it was copied from")
	}
}

func TestADatabaseReadWithNoDSNIsRefused(t *testing.T) {
	err := run(context.Background(), []string{"-train-end", "2026-12-15", "-validation-end", "2027-02-17"},
		func(string) string { return "" }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "MARGINCE_DSN") {
		t.Fatalf("a run with neither a dataset nor a database answered %v", err)
	}
}

// slicesWithout drops a flag and its value.
func slicesWithout(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == name {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func TestWithoutAPreregistrationTheCandidateIsNotScored(t *testing.T) {
	var report struct {
		NotScored string `json:"candidate_not_scored"`
		Rows      []struct {
			Method string `json:"method"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(runOnCorpus(t), &report); err != nil {
		t.Fatal(err)
	}
	if report.NotScored == "" || len(report.Rows) == 0 {
		t.Fatalf("a run with no pre-registration reads %+v", report)
	}
	for _, row := range report.Rows {
		if row.Method == "candidate_open_closure" {
			t.Fatal("the candidate was scored with no pre-registration")
		}
	}
}

// A pre-registration with a value left unset, fixed for another dataset, or
// fixed for other windows does not open the candidate comparison.
func TestAPreregistrationMustBeCompleteAndForThisDataset(t *testing.T) {
	body, err := os.ReadFile(preregistered)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(map[string]any){
		"unset threshold": func(p map[string]any) { delete(p, "improvement_threshold") },
		"other dataset":   func(p map[string]any) { p["dataset_digest"] = strings.Repeat("0", 64) },
		"other windows":   func(p map[string]any) { p["settle_days"] = 6 },
		"one maintainer":  func(p map[string]any) { p["reviewed_by"] = p["filled_by"] },
	}
	for name, change := range cases {
		var pre map[string]any
		if err := json.Unmarshal(body, &pre); err != nil {
			t.Fatal(err)
		}
		change(pre)
		changed, err := json.Marshal(pre)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "pre.json")
		if err := os.WriteFile(path, changed, 0o600); err != nil {
			t.Fatal(err)
		}
		args := slices.Concat(corpusArgs, []string{"-preregistration", path})
		if err := run(context.Background(), args, func(string) string { return "" }, &bytes.Buffer{}); err == nil {
			t.Errorf("%s: the candidate was scored", name)
		}
	}
}
