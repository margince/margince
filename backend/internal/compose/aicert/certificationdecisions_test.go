// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

// The certification page's "Decision models" section: every committed decision
// record, whether it still describes the decision question this build asks,
// and whether it earns the row the runtime serves the lane by. Read through
// aicert.DecisionCertTable — the same call the generator makes — so the page
// and the generated table cannot disagree about which records serve.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

// aiCertDecision is one decision record as the document carries it. Binding's
// model is the CONFIGURED lane model, which is what the runtime keys on.
type aiCertDecision struct {
	Task    string           `json:"task"`
	Site    string           `json:"site"`
	Binding aiCertBindingRef `json:"binding"`
	State   string           `json:"state"`
	Verdict string           `json:"verdict"`
	// Serves says whether the record earns a row in the generated table: the
	// lane answers this site only when it does.
	Serves           bool           `json:"serves"`
	Runs             int            `json:"runs"`
	Kept             int            `json:"kept"`
	KeptWrong        int            `json:"kept_wrong"`
	FallbackRate     float64        `json:"fallback_rate"`
	FallbackByReason map[string]int `json:"fallback_by_reason"`
	ServedPassRate   float64        `json:"served_pass_rate"`
	StaleReason      string         `json:"stale_reason"`
}

// buildAICertDecisions folds each decision row into the document, in the
// order readiness yields them (the records' own load order). Serves is the
// row's own answer, the one the generator reads.
func buildAICertDecisions(readiness []aicert.DecisionRow) []aiCertDecision {
	decisions := []aiCertDecision{}
	for _, row := range readiness {
		rec := row.Record
		entry := aiCertDecision{
			Task: rec.Task, Site: rec.Site,
			Binding: aiCertBindingRef{Provider: rec.Provider, Model: rec.Model, Env: rec.EnvClass},
			State:   row.Status(), Verdict: rec.Verdict,
			Serves: row.Serves(),
			Runs:   rec.Runs, FallbackByReason: map[string]int{}, StaleReason: row.Standing.Reason(),
		}
		if stats := rec.Decision; stats != nil {
			entry.Kept, entry.KeptWrong = stats.Kept, stats.KeptWrong
			entry.FallbackRate, entry.ServedPassRate = stats.FallbackRate, stats.ServedPassRate
			for reason, n := range stats.FallbackByReason {
				entry.FallbackByReason[reason] = n
			}
		}
		decisions = append(decisions, entry)
	}
	return decisions
}

// decisionRecordKey is the record key a document entry stands for, spelled by
// aicert.RecordKey itself so the accounting assertion cannot key it otherwise.
func decisionRecordKey(d aiCertDecision) string {
	return aicert.RecordKey(aicert.Record{
		Task: d.Task, Kind: aicert.KindDecision, Site: d.Site,
		Provider: d.Binding.Provider, Model: d.Binding.Model, EnvClass: d.Binding.Env,
	})
}

// writeAICertDecisions renders the section. With no decision record it says
// so, because an empty table under a heading reads like a rendering fault.
func writeAICertDecisions(page *strings.Builder, decisions []aiCertDecision) {
	page.WriteString("### Decision models\n\n")
	page.WriteString("A decision model answers a site's closed question first. The site's LLM ladder\n")
	page.WriteString("answers instead whenever the lane may not be asked (a local-only task), fails,\n")
	page.WriteString("or answers below the site's own floor. A decision record comes from a\n")
	page.WriteString("`make e2e-ai ROUTING=` run whose config binds `decisions:`, and is certified\n")
	page.WriteString("per site with no judge: `certified` means no answer the site kept was wrong in\n")
	page.WriteString("any run, and at least one was kept. The lane serves a site only while its\n")
	page.WriteString("record is certified and not stale (*Serves*); `make gen` writes those rows into\n")
	page.WriteString("`internal/modules/ai/decisioncert_gen.go`.\n\n")
	if len(decisions) == 0 {
		page.WriteString("No decision record is committed, so the decision lane serves no site.\n\n")
		return
	}
	page.WriteString("| Site | Binding | State | Verdict | Serves | Runs | Kept | Kept wrong | Fallback rate | Fallbacks by reason | Served pass rate |\n")
	page.WriteString("|---|---|---|---|---|---:|---:|---:|---:|---|---:|\n")
	for _, d := range decisions {
		serves := "no"
		if d.Serves {
			serves = "yes"
		}
		state := "`" + d.State + "`"
		if d.StaleReason != "" {
			state += " — " + d.StaleReason
		}
		fmt.Fprintf(page, "| `%s/%s` | `%s` | %s | `%s` | %s | %d | %d | %d | %.2f | %s | %.2f |\n",
			d.Task, d.Site, d.Binding.label(), state, d.Verdict, serves, d.Runs, d.Kept, d.KeptWrong,
			d.FallbackRate, fallbackReasonsCell(d.FallbackByReason), d.ServedPassRate)
	}
	page.WriteString("\n")
}

// fallbackReasonsCell spells the reasons in a stable order.
func fallbackReasonsCell(byReason map[string]int) string {
	if len(byReason) == 0 {
		return aicert.Unmeasured
	}
	reasons := make([]string, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	cells := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		cells = append(cells, fmt.Sprintf("`%s` %d", reason, byReason[reason]))
	}
	return strings.Join(cells, ", ")
}

// A decision record reaches the page with its numbers and whether it serves: a
// certified current one does, and a stale one says why it does not.
func TestTheDecisionSectionShowsEachRecordAndWhetherItServes(t *testing.T) {
	stats := &aicert.DecisionStats{
		Kept: 14, KeptCorrect: 14, Fallbacks: 1, FallbackRate: 1.0 / 15,
		FallbackByReason: map[string]int{"below_floor": 1}, ServedPassRate: 1,
	}
	current := aicert.Record{
		Task: "site_triage", Kind: aicert.KindDecision, Site: "triage", Provider: "openrouter_decision",
		Model: "typesafe/jev-1.13", EnvClass: "cloud_frontier", Verdict: aicert.VerdictCertified, Runs: 15,
		Decision: stats, Scenarios: []aicert.ScenarioRecord{{Scenario: "a", Site: "triage", Stamp: "s-a"}},
	}
	stale := current
	stale.EnvClass = "eu_hosted"
	readiness := []aicert.DecisionRow{
		{Site: "site_triage/triage", Record: current, Standing: aicert.Standing{Measured: 1, Total: 1}},
		{Site: "site_triage/triage", Record: stale, Standing: aicert.Standing{
			Stale: true, Moved: []string{"a"},
			MovedParts: map[string]aicert.StampParts{"a": {Prompt: true}}, Total: 1,
		}},
	}
	decisions := buildAICertDecisions(readiness)
	if len(decisions) != 2 || !decisions[0].Serves || decisions[1].Serves || decisions[1].State != aicert.StatusStale {
		t.Fatalf("decisions = %+v, want the current record serving and the stale one stale", decisions)
	}

	var page strings.Builder
	writeAICertDecisions(&page, decisions)
	for _, want := range []string{
		"| `site_triage/triage` | `openrouter_decision · typesafe/jev-1.13 · cloud_frontier` | `current` | `certified` | yes | 15 | 14 | 0 | 0.07 | `below_floor` 1 | 1.00 |",
		"`stale` — the prompt this build sends changed under scenario a",
	} {
		if !strings.Contains(page.String(), want) {
			t.Errorf("the section lacks %q:\n%s", want, page.String())
		}
	}
}
