// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// Until a decision certification runs, the report says no site has a verdict,
// and never that a bound lane stops answering.
func TestTheDecisionReportSaysNoSiteIsMeasuredWithoutARecord(t *testing.T) {
	out := renderDecisions(nil)
	if !strings.Contains(out, "no site has a measured verdict") || !strings.Contains(out, "a bound lane answers every site") {
		t.Fatalf("an empty decision report reads:\n%s", out)
	}
}

// The report never says a missing row switches the lane off: Router.Decide
// answers an uncertified site.
func TestTheDecisionReportNeverSaysARowGatesTheLane(t *testing.T) {
	out := renderDecisions([]aicert.DecisionRow{{Site: "site_triage/triage", Record: aicert.Record{Task: "site_triage", Site: "triage"}}})
	for _, gone := range []string{"SERVES", "only where", "serves no site"} {
		if strings.Contains(out, gone) {
			t.Errorf("the decision report still says %q:\n%s", gone, out)
		}
	}
	if !strings.Contains(out, "a bound lane answers every site") {
		t.Errorf("the decision report does not say the lane answers every site:\n%s", out)
	}
}

// A certified current record earns a row; a stale one does not, and says why.
func TestTheDecisionReportNamesWhatIsCertifiedAndWhatWentStale(t *testing.T) {
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
		t.Errorf("the certified current record should earn a row: %q", current)
	}
	if !strings.Contains(staleLine, "stale") || strings.Contains(staleLine, " yes ") {
		t.Errorf("the stale record should earn no row: %q", staleLine)
	}
	if !strings.Contains(out, "the case changed under scenario a") {
		t.Errorf("the stale record's reason is missing:\n%s", out)
	}
}

// A current certified record on too few kept answers prints no as its row.
// It also says why, so a certified verdict never sits beside a bare no.
func TestTheDecisionReportSaysWhyAThinRecordEarnsNoRow(t *testing.T) {
	thin := aicert.Record{
		Task: "site_triage", Kind: aicert.KindDecision, Site: "triage", Provider: "jev_compatible",
		Model: "typesafe/jev-1.13", EnvClass: "cloud_frontier", Verdict: aicert.VerdictCertified, Runs: 9,
		Decision: &aicert.DecisionStats{Kept: 9, KeptCorrect: 9},
	}
	out := renderDecisions([]aicert.DecisionRow{{Site: "site_triage/triage", Record: thin, Standing: aicert.Standing{Measured: 1, Total: 1}}})
	if !strings.Contains(out, fmt.Sprintf("fewer than %d kept", ai.DecisionMinKept)) {
		t.Errorf("a certified record on 9 kept answers gives no reason for its missing row:\n%s", out)
	}
}
