// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"math"
	"slices"
	"sort"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// Assembling one task's Record from a finished run set.
//
// Split from record.go, which now holds the record's SHAPE — the struct, what
// each field means and who graded it, and the read/write of the files on disk.
// This holds the arithmetic that fills it: the pooling, the percentiles, the
// per-bucket means and the cost estimate.
//
// The seam is worth the file. A reader asking "what is judge_score_p50" wants
// the struct and its doc; a reader asking "how was it computed" wants this —
// and the two questions were answered by scrolling past each other while the
// grader documentation this file's split paid for went in above.

// buildRecord folds one task's pooled runs (across every scenario, every
// repeat) and its already-folded taskVerdict into the on-disk Record
// shape. Score/latency percentiles are computed directly here (not via
// Verdict, which is scoped to one scenario's odd-N run set and would
// panic on a multi-scenario task's pooled, possibly-even count).
//
// The run set arrives as the accumulation certifyTask folded it into, rather
// than as one parameter per number: the record IS that accumulation written
// down, and a dozen same-typed positional arguments is a transposition waiting
// to happen that no test would catch.
func buildRecord(task ai.Task, taskVerdict string, acc *taskAccumulation, profile ai.Profile, promptVersion string) Record {
	results := acc.allResults
	// Only what a judge graded: a run skipped for truncation carries Score 0
	// because nobody scored it, and averaging that in reports the absence as a
	// verdict.
	scores := judgeScores(results)
	sort.Ints(scores)

	sortedLatencies := append([]int64(nil), acc.latencies...)
	sort.Slice(sortedLatencies, func(i, j int) bool { return sortedLatencies[i] < sortedLatencies[j] })

	n := len(results)
	reliability := 0.0
	if n > 0 {
		reliability = float64(acc.passed) / float64(n)
	}
	meanTokens, usage := meanUsage(acc, n)

	// ranAt is captured once and reused for both RanAt and the pricing
	// snapshot date so buildRecord never calls nowFunc twice — the record's
	// timestamp and the rate sheet it priced against are always the same
	// instant.
	ranAt := nowFunc().UTC()
	estCostMicroUSD := int64(0)
	if rate, ok := seedRateFor(acc.provider, acc.servedModel, ranAt); ok {
		estCostMicroUSD = ai.PriceCall(usage, rate)
	}

	tally := tallyOutcomes(results)
	return Record{
		Task:                 string(task),
		Provider:             acc.provider,
		ServedModel:          acc.servedModel,
		EnvClass:             string(profile),
		PromptVersion:        promptVersion,
		CorpusVersion:        corpusVersionV1,
		Verdict:              taskVerdict,
		Runs:                 n,
		Passed:               acc.passed,
		Reliability:          reliability,
		ReportedAccepted:     tally.accepted,
		ReportedWrongAnswer:  tally.wrongAnswer,
		ReportedInvalid:      tally.invalid,
		ReportedAbstained:    tally.abstained,
		CertifiedScope:       acc.certifiedScope,
		ContextApplied:       certLaneAppliesCompanyContext,
		ContextScopes:        declaredCompanyContextScopes(task),
		JudgeScoreP50:        scores[len(scores)/2],
		JudgeScoreMin:        scores[0],
		LatencyP50:           percentile(sortedLatencies, 0.50),
		LatencyP95:           percentile(sortedLatencies, 0.95),
		MeanTokens:           meanTokens,
		MeanTokensIn:         usage.TokensIn,
		MeanTokensOut:        usage.TokensOut,
		MeanCachedTokens:     usage.CachedTokens,
		MeanCacheWriteTokens: usage.CacheWriteTokens,
		// EstCostMicroUSD prices the pooled per-bucket means against the
		// cert lane's in-memory seed rate sheet (ai.SeedModelRates): the
		// cert lane runs outside any DB workspace tx, so there is no
		// ai_model_rate table to read RateStore.RateFor's own way — this is
		// the closest analogue available here. No matching (provider,
		// served model) row keeps it an honest 0, exactly like an unpriced
		// RateStore.RateFor call (price-on-read; never fabricate a price).
		EstCostMicroUSD:      estCostMicroUSD,
		JudgeServedModel:     acc.judgeServedModel,
		SelfJudged:           acc.selfJudgedEveryRun,
		ServedIdentitySource: acc.identitySource,
		RanAt:                ranAt.Format(time.RFC3339),
		Scenarios:            acc.scenarios,
	}
}

// meanUsage is the pooled run set's per-bucket mean, alongside the flat
// MeanTokens the record has always carried.
//
// MeanTokens divides the exact summed total (tokens in + tokens out, an exact
// sum of two exact sums) — bit-for-bit the value that field held before the
// per-bucket split existed. Each bucket divides its OWN total independently, so
// the four need not add back up to MeanTokens after truncation.
func meanUsage(acc *taskAccumulation, n int) (int, ai.Usage) {
	if n == 0 {
		return 0, ai.Usage{}
	}
	return (acc.tokensInTotal + acc.tokensOutTotal) / n, ai.Usage{
		TokensIn:         acc.tokensInTotal / n,
		TokensOut:        acc.tokensOutTotal / n,
		CachedTokens:     acc.cachedTokensTotal / n,
		CacheWriteTokens: acc.cacheWriteTokensTotal / n,
	}
}

// declaredCompanyContextScopes reads the task contract's own answer to what
// production prepends for this task. Every task declares a policy — the ai
// module's contract holds that — so an absent one is a build that could not
// have run this task at all, and the empty set is the honest reading of it
// rather than a scope list invented here.
//
// The slice is copied because the contract's own is package state: a record
// handed the original would let anything holding it edit what the next record
// claims.
func declaredCompanyContextScopes(task ai.Task) []string {
	policy, declared := ai.CompanyContextFor(task)
	if !declared {
		return nil
	}
	return slices.Clone(policy.Scopes)
}

// certLaneAppliesCompanyContext is false because this lane has no database.
// The company context production prepends is assembled from stored workspace
// facts, and every certification run here is DB-less — so the requests scored
// are the site's own prompt without it. Spelled as a named constant so the
// record's claim is a stated fact of the lane rather than a literal a reader
// has to interpret.
const certLaneAppliesCompanyContext = false

// outcomeTally is the per-outcome run count a record carries. It is a struct
// rather than four returns so the four numbers cannot be transposed at a call
// site, which is the one way this arithmetic can be wrong without failing.
type outcomeTally struct {
	accepted, wrongAnswer, invalid, abstained int
}

// tallyOutcomes counts what each pooled run's validator reported. Every
// RunResult reaching here carries one of the four (the runner refuses a run
// whose case reported anything else), so the counts always sum to len(results).
func tallyOutcomes(results []RunResult) outcomeTally {
	var t outcomeTally
	for _, r := range results {
		switch r.Outcome {
		case aitasks.OutcomeAccepted:
			t.accepted++
		case aitasks.OutcomeWrongAnswer:
			t.wrongAnswer++
		case aitasks.OutcomeInvalid:
			t.invalid++
		case aitasks.OutcomeAbstained:
			t.abstained++
		}
	}
	return t
}

// seedRateFor resolves the exact (provider, servedModel) rate row from
// ai.SeedModelRates(day) — an exact-key lookup, not RateStore.RateFor's
// as-of-date walk, because every row SeedModelRates returns for one day
// carries that same single EffectiveDate. False means no rate is seeded
// for this exact provider/model pair: the call is unpriced, never priced
// at a fabricated 0.
func seedRateFor(provider, servedModel string, day time.Time) (ai.ModelRate, bool) {
	for _, r := range ai.SeedModelRates(day) {
		if r.Provider == provider && r.ModelID == servedModel {
			return r, true
		}
	}
	return ai.ModelRate{}, false
}

// percentile returns the nearest-rank pth percentile (p in [0,1]) of
// sorted, which must already be sorted ascending.
func percentile(sorted []int64, p float64) int64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}
