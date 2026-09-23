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

// judgeScores is the scores a judge actually gave, which is what a median and a
// minimum may be taken over.
func judgeScores(rs []RunResult) []int {
	scores := make([]int, 0, len(rs))
	for _, r := range rs {
		if r.Ungraded {
			continue
		}
		scores = append(scores, r.Score)
	}
	return scores
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
// Verdict requires an ODD run count (N=0 or even panics): a median needs a
// single middle element, and the runner's own config (RunnerConfig.Repeats)
// already enforces oddness before any run happens, so a call here with an
// even N is a caller bug, not a certification input to report gracefully.
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

	scores := judgeScores(rs)
	slices.Sort(scores)
	// Every run ungraded leaves no opinion to band on. The mechanical grade
	// still decides pass/fail, and the judge's half of the bands cannot be met
	// by a score nobody gave — so the verdict falls to not-supported rather
	// than certifying on an empty median.
	if len(scores) == 0 {
		return VerdictNotSupported, reliability
	}
	median := scores[len(scores)/2]
	minScore := scores[0]

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
