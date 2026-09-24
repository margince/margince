// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// One run's model calls, folded into the accounting a record is written from.
//
// Its own file because run.go drives a run and this answers what the run cost
// and who served it — and because a run that several models answered has no
// single heading to be certified under, which is a question about the calls
// rather than about the driving.

import (
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// runCalls is every logical call one run made, folded into the single
// accounting a RunResult keeps.
//
// Each field folds the way its own meaning demands, and none of them is the
// last call's value:
//
//   - Degraded is true when ANY call was served on a budget-degraded route. A
//     demoted first attempt followed by a healthy retry is still a demoted
//     answer inside a certified run, and §5 voids the record for it.
//   - The four token buckets and the latency SUM: they are what the run spent,
//     and a run that spent it over three calls spent it.
//   - Provider/ServedModel/ServedIdentitySource are the FIRST call's, which is
//     the whole run's whenever servedUniformly says so. A caller that certifies
//     a (provider, model) heading asks that question; the judge, whose score is
//     whichever attempt parsed, does not.
type runCalls struct {
	Calls    []ai.Call
	Degraded bool
	// Truncated says a call in this run stopped at the output ceiling rather
	// than at the end of its answer. OR-ed across the run like Degraded, and
	// for its reason: one cut-off attempt leaves the run without an answer to
	// score, whichever attempt it was.
	Truncated bool
	// Unanswered is the provider's own reason there is no answer: it withheld
	// one, or rejected the request. Like Truncated, a measurement of the
	// binding rather than an outage, so the run is recorded and not re-driven.
	// Text rather than an error, because it is a finding to record, not a
	// failure for any caller to handle.
	Unanswered                                  string
	Provider, ServedModel, ServedIdentitySource string
	TokensIn, TokensOut                         int
	CachedTokens, CacheWriteTokens              int
	ReasoningTokens                             int
	LatencyMS                                   int64
}

// poolRunCalls folds one run's calls into that accounting. A run with no call
// at all is refused rather than folded to zeroes: a scored run that made no
// model call is a harness fault, and zeroes would report it as a free, instant,
// healthy one.
func poolRunCalls(calls []ai.Call) (runCalls, error) {
	if len(calls) == 0 {
		return runCalls{}, fmt.Errorf("no model call was recorded, so there is nothing to score")
	}
	first := calls[0]
	pooled := runCalls{
		Calls:                calls,
		Provider:             first.Provider,
		ServedModel:          first.ServedModel,
		ServedIdentitySource: first.ServedIdentitySource,
	}
	for _, c := range calls {
		pooled.Degraded = pooled.Degraded || c.Degraded
		pooled.Truncated = pooled.Truncated || c.FinishReason == model.FinishReasonLength
		pooled.TokensIn += c.TokensIn
		pooled.TokensOut += c.TokensOut
		pooled.CachedTokens += c.CachedTokens
		pooled.CacheWriteTokens += c.CacheWriteTokens
		pooled.ReasoningTokens += c.ReasoningTokens
		pooled.LatencyMS += c.LatencyMS
	}
	return pooled, nil
}

// servedUniformly reports whether one model answered the whole run, naming both
// identities when one did not.
//
// A mid-run ladder fallback is the same defect as a mid-SET one: a record that
// pooled it would report an answer partly produced by one model and partly by
// another under a single (provider, model) heading, and nothing in the record
// would ever show it. The fix is a re-run once the ladder is stable, not an
// edit, so the message names what to compare rather than what to change.
func (r runCalls) servedUniformly() error {
	for i, c := range r.Calls {
		if c.Provider != r.Provider || c.ServedModel != r.ServedModel {
			return fmt.Errorf(
				"call %d of %d was served by %s:%s, but call 1 was served by %s:%s — refusing to certify one run answered by two models",
				i+1, len(r.Calls), c.Provider, c.ServedModel, r.Provider, r.ServedModel,
			)
		}
	}
	return nil
}

// unansweredReason is a failed candidate call's text when the failure is the
// provider's answer about this request — withheld or rejected — and empty when
// it is a failure to reach one.
func unansweredReason(err error) string {
	if errors.Is(err, model.ErrOutputWithheld) || errors.Is(err, model.ErrRequestRejected) {
		return err.Error()
	}
	return ""
}
