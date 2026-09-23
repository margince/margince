// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"fmt"
	"slices"
)

// Verdict strings — the three §5 outcomes a run set can land on.
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
	// Ungraded says no judge saw this run, so Score is absent rather than zero
	// and must not reach a median or a minimum. A run cut off at the output
	// ceiling is the case: there is no answer to have an opinion about, and
	// folding its 0 into the judge's numbers would report the skipped opinion
	// as a bad one — the same ambiguity, entered from the other side.
	//
	// It stays in the run, validator and pass counts: it happened, and the
	// mechanical grade judged the same incomplete text production would have.
	Ungraded bool `json:"ungraded,omitempty"`
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

// Verdict folds N runs of one scenario into a certification outcome per
// spec §5, literally:
//
//	Certified          = every run HardPass ∧ median(Score) ≥ b.CertifiedMin ∧ min(Score) ≥ b.Floor
//	Supported-degraded = ≥⌈2N/3⌉ runs HardPass ∧ median(Score) ≥ b.DegradedMin
//	otherwise          = Not-supported
//
// reliability is the fraction of runs that HardPassed (0..1), reported
// regardless of which verdict the run set lands on — it is the number a
// dashboard trends over time, not just the pass/fail label.
//
// Verdict requires an ODD run count (N=0 or even panics): ⌈2N/3⌉ is a
// threshold on the RUN set, and the runner's own config (RunnerConfig.Repeats)
// already enforces oddness before any run happens, so a call here with an
// even N is a caller bug, not a certification input to report gracefully.
// The median is NOT what the oddness buys — it comes from the graded subset,
// which an ungraded run leaves even, and medianOf defines that case.
func Verdict(rs []RunResult, b Bands) (verdict string, reliability float64) {
	n := len(rs)
	if n == 0 || n%2 == 0 {
		panic(fmt.Sprintf("aicert: Verdict: run count must be odd and non-zero, got %d", n))
	}

	passed := 0
	for _, r := range rs {
		if r.HardPass {
			passed++
		}
	}
	reliability = float64(passed) / float64(n)

	// Every run ungraded leaves no opinion to band on. The mechanical grade
	// still decides pass/fail, and the judge's half of the bands cannot be met
	// by a score nobody gave — so the verdict falls to not-supported rather
	// than certifying on an empty median.
	median, minScore, graded := judgeMedianAndMin(rs)
	if !graded {
		return VerdictNotSupported, reliability
	}

	if passed == n && median >= b.CertifiedMin && minScore >= b.Floor {
		return VerdictCertified, reliability
	}
	// ceil(2N/3) via integer arithmetic: (2N + 2) / 3.
	degradedThreshold := (2*n + 2) / 3
	if passed >= degradedThreshold && median >= b.DegradedMin {
		return VerdictSupportedDegraded, reliability
	}
	return VerdictNotSupported, reliability
}
