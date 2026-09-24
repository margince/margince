// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"fmt"
	"slices"
)

// Verdict strings — the three outcomes a set of run sets can land on.
const (
	VerdictCertified         = "certified"
	VerdictSupportedDegraded = "supported_degraded"
	VerdictNotSupported      = "not_supported"
)

// RunResult is one candidate completion already scored and measured by the
// caller (the runner drives the model and the judge; this package never
// calls either). Degraded mirrors the router's own degrade signal (a
// budget-forced tier drop mid-run); HardPass is the run's pass/fail verdict
// against its scenario's expected outcome and caps — the input Verdict folds
// across a whole run set.
//
// Outcome is what the site's own validator reported (aitasks.OutcomeAccepted
// and its siblings), carried alongside HardPass rather than derived from it: a
// run can produce exactly the answer the scenario asked for and still fail its
// latency cap, and one that reports "accepted" for that run would be counting
// something that never happened.
// The json tags are the resume journal's on-disk shape (resume.go): a run
// survives a killed process as one of these, and is replayed into the same
// Record arithmetic a live one feeds.
type RunResult struct {
	Output    string `json:"output"`
	Outcome   string `json:"outcome"`
	LatencyMS int64  `json:"latency_ms"`
	// TokensIn/TokensOut/CachedTokens/CacheWriteTokens are the terminal
	// attempt's own four token buckets, carried through unsummed (ai.Call's
	// contract: TokensIn is cache-inclusive, CachedTokens/CacheWriteTokens
	// are the itemized subsets already counted inside it) so buildRecord can
	// price the run against ai.PriceCall's per-bucket rate arithmetic
	// instead of a pre-collapsed total.
	TokensIn         int  `json:"tokens_in"`
	TokensOut        int  `json:"tokens_out"`
	CachedTokens     int  `json:"cached_tokens"`
	CacheWriteTokens int  `json:"cache_write_tokens"`
	Degraded         bool `json:"degraded"`
	HardPass         bool `json:"hard_pass"`
	Score            int  `json:"score"`
	// Ungraded says no judge gave this run an opinion, so Score is absent rather
	// than zero and must not reach a median or a minimum. A run cut off at the
	// output ceiling has no answer to have an opinion about; a judge whose reply
	// never parsed gave none. Folding either's 0 into the judge's numbers would
	// report a missing opinion as a bad one.
	//
	// It stays in the run, validator and pass counts: it happened, and the
	// mechanical grade still judged it.
	Ungraded bool `json:"ungraded,omitempty"`
	// JudgeScores holds the graded opinions the run was scored from, in the order
	// given; more than one means the first fell below certified_min and Score is
	// their median, so a reader can see a contested grade.
	JudgeScores []int `json:"judge_scores,omitempty"`
	// Withheld is the provider's reason it withheld this run's answer — the
	// filter or stop that fired — and empty for a run that was answered.
	Withheld string `json:"withheld,omitempty"`
}

// judgeMedianAndMin answers the median and minimum of the scores a judge
// actually gave, and whether any were given.
//
// ONE reader for both the verdict and the record, because the guard is the whole
// point: a task whose every run was cut off has an empty score set, and
// indexing it is a panic. Written as two functions it was exactly that — the
// verdict got the empty check and buildRecord, its sibling, did not.
func judgeMedianAndMin(rs []RunResult) (median, minimum int, graded bool) {
	scores := make([]int, 0, len(rs))
	for _, r := range rs {
		if r.Ungraded {
			continue
		}
		scores = append(scores, r.Score)
	}
	if len(scores) == 0 {
		return 0, 0, false
	}
	slices.Sort(scores)
	return medianOf(scores), scores[0], true
}

// medianOf answers the median of a sorted, non-empty score set, averaging the
// two middle values on an even count.
//
// The even case is reachable even though RunnerConfig.Repeats is odd: an
// ungraded run leaves the run set odd and the SCORE set one shorter. Without
// the average, a graded pair {10, 80} reported 80 — the upper middle — which is
// the one direction a certification number must never err in, because it turns
// a set with a failing grade in it into a passing median.
//
// Integer division truncates, so an exact half lands on the lower value. That
// is the conservative side: a median is compared against a floor, and rounding
// down can only withhold a verdict, never grant one.
func medianOf(sorted []int) int {
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

// ScenarioRuns is one scenario's run set and the bands its own judge scores are
// held to; scenarios in one task carry different bands.
type ScenarioRuns struct {
	Runs  []RunResult
	Bands Bands
}

// Verdict folds a set of scenario run sets into one certification outcome — a
// single scenario for its record row, every scenario of a task for the task:
//
//	certified          = pooled passes ≥ 90% of all runs
//	                   ∧ every scenario passes ≥⌈2n/3⌉ of its own n runs
//	                   ∧ every scenario's graded median ≥ its CertifiedMin ∧ graded minimum ≥ its Floor
//	supported_degraded = pooled passes ≥ ⌈2N/3⌉ of all N runs
//	                   ∧ every scenario's graded median ≥ its DegradedMin
//	otherwise          = not_supported, which includes any scenario no judge graded
//
// A rate rather than "every run" so a large pool can absorb a stray miss: at
// 99% per run, 57 of 57 happens barely half the time. The rate rounds up, so a
// pool too small to hold one miss under it needs every run. The per-scenario
// majority keeps one case failing systematically from hiding inside a large pool.
// thresholds.go holds every number here; the page test pins the rate above to it.
//
// reliability is the pooled fraction of runs that HardPassed (0..1), reported
// whichever verdict the set lands on.
//
// Every scenario's run count must be ODD and non-zero, and the set non-empty,
// or Verdict panics: RunnerConfig.Repeats enforces oddness before any run, so a
// call breaking it is a caller bug, not a certification input.
func Verdict(sets ...ScenarioRuns) (verdict string, reliability float64) {
	if len(sets) == 0 {
		panic("aicert: Verdict: no scenario run sets")
	}
	tallies := make([]scenarioTally, 0, len(sets))
	for _, set := range sets {
		n := len(set.Runs)
		if n == 0 || n%2 == 0 {
			panic(fmt.Sprintf("aicert: Verdict: run count must be odd and non-zero, got %d", n))
		}
		tally := scenarioTally{runs: n, judgeBand: judgeBand(set.Runs, set.Bands)}
		for _, r := range set.Runs {
			if r.HardPass {
				tally.passed++
			}
		}
		tallies = append(tallies, tally)
	}
	return verdictOver(tallies)
}

// scenarioTally is what the verdict rule reads of one scenario: its run and
// pass counts, and the best verdict its judge scores alone reach.
type scenarioTally struct {
	runs, passed int
	judgeBand    string
}

// judgeBand is the best verdict rs's graded scores reach against b, ignoring
// pass/fail. No graded run reaches nothing: a band is not met by a score
// nobody gave.
func judgeBand(rs []RunResult, b Bands) string {
	median, minScore, graded := judgeMedianAndMin(rs)
	switch {
	case !graded:
		return VerdictNotSupported
	case median >= b.CertifiedMin && minScore >= b.Floor:
		return VerdictCertified
	case median >= b.DegradedMin:
		return VerdictSupportedDegraded
	default:
		return VerdictNotSupported
	}
}

// verdictOver applies Verdict's rule to tallies; the record's per-site verdict
// reads it too, from the scenario rows it kept.
func verdictOver(tallies []scenarioTally) (verdict string, reliability float64) {
	runs, passed := 0, 0
	certified, degraded := true, true
	for _, t := range tallies {
		runs += t.runs
		passed += t.passed
		if t.judgeBand != VerdictCertified || t.passed < majorityOf(t.runs) {
			certified = false
		}
		if t.judgeBand != VerdictCertified && t.judgeBand != VerdictSupportedDegraded {
			degraded = false
		}
	}
	if runs == 0 {
		return VerdictNotSupported, 0
	}
	reliability = float64(passed) / float64(runs)
	switch {
	case certified && passed*100 >= certifiedPassPercent*runs:
		return VerdictCertified, reliability
	case degraded && passed >= majorityOf(runs):
		return VerdictSupportedDegraded, reliability
	default:
		return VerdictNotSupported, reliability
	}
}

// majorityOf is ⌈n·majorityNumerator/majorityDenominator⌉ in integer arithmetic.
func majorityOf(n int) int {
	return (n*majorityNumerator + majorityDenominator - 1) / majorityDenominator
}
