// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

// Until a decision certification runs, the report says the lane serves
// nothing rather than printing an empty table.
func TestTheDecisionReportSaysTheLaneServesNothingWithoutARecord(t *testing.T) {
	if out := renderDecisions(nil); !strings.Contains(out, "serves no site") {
		t.Fatalf("an empty decision report reads:\n%s", out)
	}
}

// A certified current record serves; a stale one does not, and says why.
func TestTheDecisionReportNamesWhatServesAndWhatWentStale(t *testing.T) {
	rec := aicert.Record{
		Task: "site_triage", Kind: aicert.KindDecision, Site: "triage", Provider: "jev_compatible",
		Model: "typesafe/jev-1.13", EnvClass: "cloud_frontier", Verdict: aicert.VerdictCertified, Runs: 15,
		Decision: &aicert.DecisionStats{Kept: 15, KeptCorrect: 15, ServedPassRate: 1},
	}
	stale := rec
	stale.EnvClass = "eu_hosted"
	rows := []aicert.DecisionRow{
		{Site: "site_triage/triage", Record: rec, Standing: aicert.Standing{Measured: 1, Total: 1}},
		{Site: "site_triage/triage", Record: stale, Standing: aicert.Standing{
			Stale: true, Moved: []string{"a"},
			MovedParts: map[string]aicert.StampParts{"a": {Case: true}}, Total: 1,
		}},
	}
	out := renderDecisions(rows)
	lines := strings.Split(out, "\n")
	var current, staleLine string
	for _, line := range lines {
		switch {
		case strings.Contains(line, "cloud_frontier") && strings.HasPrefix(line, "site_triage/triage"):
			current = line
		case strings.Contains(line, "eu_hosted") && strings.HasPrefix(line, "site_triage/triage"):
			staleLine = line
		}
	}
	if !strings.Contains(current, "current") || !strings.Contains(current, "yes") {
		t.Errorf("the certified current record should serve: %q", current)
	}
	if !strings.Contains(staleLine, "stale") || strings.Contains(staleLine, " yes ") {
		t.Errorf("the stale record should not serve: %q", staleLine)
	}
	if !strings.Contains(out, "the case changed under scenario a") {
		t.Errorf("the stale record's reason is missing:\n%s", out)
	}
}
