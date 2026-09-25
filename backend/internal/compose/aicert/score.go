// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import "slices"

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
	// given, and Score is their median, so a reader can see a contested grade.
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
// The even case is reachable from both sides: an extended case runs six times,
// and an ungraded run leaves the SCORE set one shorter than the run set. Without
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

// Verdict folds a set of scenario run sets into one certification outcome —
// every scenario of a task, or of one site; a record row reads caseVerdict.
// K of N is the pooled pass count, k of n one case's, and a score is one run's
// median judge opinion:
//
//	certified          = K/N ≥ 90% ∧ WilsonLower(K, N) ≥ 80%
//	                   ∧ every case passes ≥ 50% of its own n runs
//	                   ∧ TLower(score − its case's certified_min, over every graded run) ≥ 0
//	                   ∧ every case's TUpper(scores) ≥ its certified_min
//	                   ∧ every graded run's score ≥ its case's floor
//	                   ∧ no case vetoed
//	supported_degraded = K ≥ ⌈2N/3⌉
//	                   ∧ TLower(score − its case's degraded_min, over every graded run) ≥ 0
//	                   ∧ no case's WilsonUpper(k, n) < 50% ∧ no case's TUpper(scores) < its floor
//	otherwise          = not_supported, which includes any case no judge graded
//
//	vetoed             = WilsonUpper(k, n) < 50% ∨ TUpper(its scores) < its degraded_min
//	WilsonLower/Upper  = the one-sided 90% Wilson score bounds (z = 1.2816)
//	TLower/TUpper      = mean ∓ t(0.90, m−1)·max(sd, 5)/√m over m values; the mean itself when m = 1
//
// Pooled so that a task of many cases, each right nine times in ten, is graded
// on its whole evidence instead of failing on whichever case drew the unlucky
// run. Bounded so that a pool too small to tell good from lucky cannot certify.
// Vetoed so that one clearly broken case — failing every run, or scored as
// inventing facts every run — is never averaged away by its healthy siblings.
// The judge criterion pools each run's distance from its OWN case's band, since
// cases in one task are held to different bars. thresholds.go holds every
// number here; the page test pins this block to it.
//
// reliability is the pooled fraction of runs that HardPassed (0..1), reported
// whichever verdict the set lands on. Every scenario's run count must be
// non-zero and the set non-empty, or Verdict panics: the runner never builds
// either, so a call breaking it is a caller bug, not a certification input.
func Verdict(sets ...ScenarioRuns) (verdict string, reliability float64) {
	if len(sets) == 0 {
		panic("aicert: Verdict: no scenario run sets")
	}
	cases := make([]caseStats, 0, len(sets))
	for _, set := range sets {
		if len(set.Runs) == 0 {
			panic("aicert: Verdict: a scenario with no runs")
		}
		cases = append(cases, caseOf(set))
	}
	return verdictOver(cases)
}

// caseVerdict is one case's standing against the per-case gates Verdict applies
// to it inside a pool: not_supported when it would veto the task, certified when
// it clears every gate a certified task holds each case to, degraded otherwise.
//
// It never re-runs the pooled bounds on the case alone. A task's evidence is
// its pool, and three passing runs held to a pool's Wilson bound read as weak
// however good they were, so every case of a certified task looked it.
func caseVerdict(c caseStats) string {
	return lowerVerdict(caseMechanicalBand(c), caseJudgeBand(c))
}

// caseMechanicalBand is the pass-count half of caseVerdict.
func caseMechanicalBand(c caseStats) string {
	switch {
	case passVetoed(c):
		return VerdictNotSupported
	case c.passed*100 < casePassPercent*c.runs:
		return VerdictSupportedDegraded
	default:
		return VerdictCertified
	}
}

// caseJudgeBand is the judge-score half of caseVerdict; a case no judge graded
// vetoes, as it does in the pool.
func caseJudgeBand(c caseStats) string {
	if len(c.scores) == 0 {
		return VerdictNotSupported
	}
	upper := judgeUpper(c)
	switch {
	case upper < float64(c.bands.DegradedMin) || upper < float64(c.bands.Floor):
		return VerdictNotSupported
	case upper < float64(c.bands.CertifiedMin) || underFloor(c):
		return VerdictSupportedDegraded
	default:
		return VerdictCertified
	}
}

// caseStats is what the verdict rule reads of one case: its run and pass
// counts, the scores its judge gave, and the bands those scores are held to.
type caseStats struct {
	runs, passed int
	scores       []int
	bands        Bands
}

// caseOf reads a run set into the counts the rule needs; an ungraded run is
// counted as a run and never as a score.
func caseOf(set ScenarioRuns) caseStats {
	c := caseStats{runs: len(set.Runs), bands: set.Bands}
	for _, r := range set.Runs {
		if r.HardPass {
			c.passed++
		}
		if !r.Ungraded {
			c.scores = append(c.scores, r.Score)
		}
	}
	return c
}

// passVetoed says even the most generous reading of c's pass rate is below
// vetoPassPercent.
func passVetoed(c caseStats) bool {
	_, upper := wilsonBounds(c.passed, c.runs)
	return upper*100 < vetoPassPercent
}

// judgeUpper is the upper bound on c's mean judge score.
func judgeUpper(c caseStats) float64 {
	_, upper := meanBounds(asFloats(c.scores))
	return upper
}

// underFloor says one of c's graded runs scored under its floor: the worst
// single run, which Bands.Floor gates.
func underFloor(c caseStats) bool {
	return len(c.scores) > 0 && slices.Min(c.scores) < c.bands.Floor
}

// rowCase reads a record's scenario row back into what the rule needs of its
// case; a row graded before the pooled rule has no bands and no scores.
func rowCase(sc ScenarioRecord) caseStats {
	c := caseStats{runs: sc.Runs, passed: sc.Passed, scores: sc.JudgeScores}
	if sc.Bands != nil {
		c.bands = Bands{CertifiedMin: sc.Bands.CertifiedMin, DegradedMin: sc.Bands.DegradedMin, Floor: sc.Bands.Floor}
	}
	return c
}

// verdictOver applies Verdict's rule to cases; the record's per-site verdict
// reads it too, from the scenario rows it kept. A grade is the lower of what
// the mechanical check and the judge each reach, which is Verdict's rule
// exactly, because each upper grade's conditions contain the lower's.
func verdictOver(cases []caseStats) (verdict string, reliability float64) {
	runs, passed := 0, 0
	for _, c := range cases {
		runs += c.runs
		passed += c.passed
	}
	if runs == 0 {
		return VerdictNotSupported, 0
	}
	reliability = float64(passed) / float64(runs)
	return lowerVerdict(mechanicalBand(cases, passed, runs), judgeBand(cases)), reliability
}

// mechanicalBand is the best grade the pass counts alone reach.
func mechanicalBand(cases []caseStats, passed, runs int) string {
	lower, _ := wilsonBounds(passed, runs)
	certified := passed*100 >= certifiedPassPercent*runs && lower*100 >= certifiedPassBoundPercent
	for _, c := range cases {
		if passVetoed(c) {
			return VerdictNotSupported
		}
		if c.passed*100 < casePassPercent*c.runs {
			certified = false
		}
	}
	switch {
	case certified:
		return VerdictCertified
	case passed >= majorityOf(runs):
		return VerdictSupportedDegraded
	default:
		return VerdictNotSupported
	}
}

// judgeBand is the best grade the judge scores alone reach, ignoring pass/fail;
// a scenario row carries caseJudgeBand. A case no judge graded reaches nothing: a
// band is not met by a score nobody gave.
func judgeBand(cases []caseStats) string {
	vetoed, belowCertified := false, false
	for _, c := range cases {
		if len(c.scores) == 0 {
			return VerdictNotSupported
		}
		upper := judgeUpper(c)
		if upper < float64(c.bands.Floor) {
			return VerdictNotSupported
		}
		if upper < float64(c.bands.DegradedMin) {
			vetoed = true
		}
		if upper < float64(c.bands.CertifiedMin) || underFloor(c) {
			belowCertified = true
		}
	}
	certifiedLower, _ := meanBounds(marginsOver(cases, func(b Bands) int { return b.CertifiedMin }))
	degradedLower, _ := meanBounds(marginsOver(cases, func(b Bands) int { return b.DegradedMin }))
	switch {
	case !vetoed && !belowCertified && certifiedLower >= 0:
		return VerdictCertified
	case degradedLower >= 0:
		return VerdictSupportedDegraded
	default:
		return VerdictNotSupported
	}
}

// marginsOver answers each graded run's distance above the bar picked from its
// own case's bands, pooled across cases.
func marginsOver(cases []caseStats, bar func(Bands) int) []float64 {
	var margins []float64
	for _, c := range cases {
		for _, s := range c.scores {
			margins = append(margins, float64(s-bar(c.bands)))
		}
	}
	return margins
}

// lowerVerdict is the worse of two grades.
func lowerVerdict(a, b string) string {
	if verdictRank(a) < verdictRank(b) {
		return a
	}
	return b
}

// verdictRank orders the grades worst first; anything unrecognised ranks with
// not_supported, so an unknown word can never lift a grade.
func verdictRank(v string) int {
	switch v {
	case VerdictCertified:
		return 2
	case VerdictSupportedDegraded:
		return 1
	default:
		return 0
	}
}

func asFloats(xs []int) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = float64(x)
	}
	return out
}

// majorityOf is ⌈n·majorityNumerator/majorityDenominator⌉ in integer arithmetic.
func majorityOf(n int) int {
	return (n*majorityNumerator + majorityDenominator - 1) / majorityDenominator
}
