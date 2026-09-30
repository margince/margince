// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Folding a decision leg's runs into one site's decision record.
//
// The verdict is stricter than the LLM's band rule and needs no judge: the lane
// is certified on a site only when NO answer the site would have acted on was
// wrong, across every run, and at least one answer was kept. A fallback is
// never wrong — the ladder answers it, and the LLM record measures that — so
// the fallback rate is reported beside the verdict and does not decide it.

import (
	"sort"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// scenarioDecisionRuns is one scenario's decision runs and the stamp they were
// asked under.
type scenarioDecisionRuns struct {
	scenario Scenario
	stamp    string
	runs     []decisionRun
}

// decisionReasonPrefix is what every decision attempt reason starts with; a
// record's FallbackByReason keys drop it, since every key there is a decision
// reason.
const decisionReasonPrefix = "decision_"

// decisionVerdict is the rule the leg certifies by: no kept answer wrong, and
// at least one kept.
func decisionVerdict(stats DecisionStats) string {
	if stats.KeptWrong == 0 && stats.Kept > 0 {
		return VerdictCertified
	}
	return VerdictNotSupported
}

// foldDecisionRuns tallies one scenario's runs against what it expects.
func foldDecisionRuns(sc Scenario, runs []decisionRun) DecisionStats {
	var stats DecisionStats
	for _, run := range runs {
		if !run.probe.Decided {
			stats.Fallbacks++
			if stats.FallbackByReason == nil {
				stats.FallbackByReason = map[string]int{}
			}
			stats.FallbackByReason[strings.TrimPrefix(run.probe.Reason, decisionReasonPrefix)]++
			continue
		}
		stats.Kept++
		if run.outcome.Result == sc.Expect.Outcome {
			stats.KeptCorrect++
		} else {
			stats.KeptWrong++
		}
		if stats.Kept == 1 || run.probe.Answer.Confidence < stats.MinKeptConfidence {
			stats.MinKeptConfidence = run.probe.Answer.Confidence
		}
	}
	return stats
}

// addStats folds one scenario's stats into a site's, rates excepted: those are
// computed once, over the whole site, by finishStats.
func addStats(total *DecisionStats, s DecisionStats) {
	if s.Kept > 0 && (total.Kept == 0 || s.MinKeptConfidence < total.MinKeptConfidence) {
		total.MinKeptConfidence = s.MinKeptConfidence
	}
	total.Kept += s.Kept
	total.KeptCorrect += s.KeptCorrect
	total.KeptWrong += s.KeptWrong
	total.Fallbacks += s.Fallbacks
	for reason, n := range s.FallbackByReason {
		if total.FallbackByReason == nil {
			total.FallbackByReason = map[string]int{}
		}
		total.FallbackByReason[reason] += n
	}
}

// finishStats fills the two rates over runs. servedPasses is kept-correct plus
// the LLM's expected passes on the runs that fell back.
func finishStats(s *DecisionStats, runs int, servedPasses float64) {
	if runs == 0 {
		return
	}
	s.FallbackRate = float64(s.Fallbacks) / float64(runs)
	s.ServedPassRate = servedPasses / float64(runs)
}

// llmPassRate is the LLM record's pass rate on one scenario, over the n runs
// its adaptive rounds reached: what a fallen-back run of it is credited with.
// The decision leg asks the scenario that same n times (llmRuns), so both legs'
// rates share a denominator. A scenario the LLM record did not run earns
// nothing, rather than a pass nobody measured.
//
// A rate rather than a pairing by run index: the two legs' runs are independent
// repeats, and run 2 of one says nothing about run 2 of the other.
func llmPassRate(llm Record, scenario string) float64 {
	row, ok := llmRow(llm, scenario)
	if !ok || row.Runs == 0 {
		return 0
	}
	return float64(row.Passed) / float64(row.Runs)
}

// llmRuns is how many runs the LLM record made of one scenario.
func llmRuns(llm Record, scenario string) int {
	row, _ := llmRow(llm, scenario)
	return row.Runs
}

func llmRow(llm Record, scenario string) (ScenarioRecord, bool) {
	for _, row := range llm.Scenarios {
		if row.Scenario == scenario {
			return row, true
		}
	}
	return ScenarioRecord{}, false
}

// buildDecisionRecord folds one site's scenario runs into its decision record.
// The asked calls must all have been served by one model, for the reason a
// completion run's must: a record names one.
func (leg decisionLeg) buildDecisionRecord(site string, sets []scenarioDecisionRuns) (Record, error) {
	var total DecisionStats
	var calls []ai.Call
	runs, served := 0, 0.0
	rows := make([]ScenarioRecord, 0, len(sets))
	stamps := make(map[string]string, len(sets))
	for _, set := range sets {
		stats := foldDecisionRuns(set.scenario, set.runs)
		n := len(set.runs)
		scenarioServed := float64(stats.KeptCorrect) + float64(stats.Fallbacks)*llmPassRate(leg.llm, set.scenario.Name)
		finishStats(&stats, n, scenarioServed)
		addStats(&total, stats)
		runs += n
		served += scenarioServed
		stamps[set.scenario.Name] = set.stamp
		for _, run := range set.runs {
			calls = append(calls, run.calls...)
		}
		row := stats
		rows = append(rows, ScenarioRecord{
			Scenario: set.scenario.Name, Site: site, Stamp: set.stamp, Verdict: decisionVerdict(stats),
			Runs: n, Passed: stats.KeptCorrect, Decision: &row,
		})
	}
	finishStats(&total, runs, served)
	rec, err := leg.decisionUsage(calls, runs)
	if err != nil {
		return Record{}, err
	}
	rec.Task, rec.Kind, rec.Site = string(leg.task), KindDecision, site
	rec.Provider, rec.Model, rec.EnvClass = leg.lane.Provider, leg.lane.Model, string(leg.profile)
	rec.PromptVersion, rec.CorpusVersion = FoldScenarioStamps(stamps), corpusVersionV1
	rec.Verdict, rec.Runs, rec.Passed = decisionVerdict(total), runs, total.KeptCorrect
	if runs > 0 {
		rec.Reliability = float64(total.KeptCorrect) / float64(runs)
	}
	// One call per run: the decision form is one question, one answer.
	rec.CertifiedScope = aitasks.ScopeSingleCall
	// A decision state carries none of the company context a completion
	// prompt is prepended with: production sends the site's state alone.
	rec.ContextScopes = []string{}
	rec.Decision, rec.Scenarios = &total, rows
	return rec, nil
}

// decisionUsage is a record holding the served identity, latency, tokens and
// cost of the asked calls, pooled over every run — a run refused before any
// call counts as a free one, which is what it costs the site.
func (leg decisionLeg) decisionUsage(calls []ai.Call, runs int) (Record, error) {
	ranAt := nowFunc().UTC()
	rec := Record{RanAt: ranAt.Format(time.RFC3339)}
	if len(calls) == 0 || runs == 0 {
		return rec, nil
	}
	pooled, err := poolRunCalls(calls)
	if err != nil {
		return Record{}, err
	}
	if err := pooled.servedUniformly(); err != nil {
		return Record{}, err
	}
	latencies := make([]int64, 0, len(calls))
	for _, c := range calls {
		latencies = append(latencies, c.LatencyMS)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	rec.ServedModel, rec.ServedIdentitySource = pooled.ServedModel, pooled.ServedIdentitySource
	rec.LatencyP50, rec.LatencyP95 = percentile(latencies, 0.50), percentile(latencies, 0.95)
	rec.MeanTokensIn = pooled.TokensIn / runs
	rec.MeanTokens = rec.MeanTokensIn
	if rate, ok := seedRateFor(leg.lane.Provider, leg.lane.Model, ranAt); ok {
		rec.EstCostMicroUSD = ai.PriceCall(ai.Usage{TokensIn: rec.MeanTokensIn}, rate)
	}
	return rec, nil
}
