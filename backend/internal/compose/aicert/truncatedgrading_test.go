// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A run whose answer was cut off is not an answer to grade.
//
// The judge was asked anyway, and what came back said the problem out loud:
// draft_reply/replying_to_a_thread_eight_months_old scored 0 with an entirely
// POSITIVE reason — "the draft correctly addresses the recipient, names the
// earlier conversation topic… and asks whether the project is still live" —
// because the judge was reading the fragment that arrived before the output
// ceiling cut it, and scoring the absence of the rest.
//
// A score means a model's opinion of an answer. There is no answer here, so
// there is nothing for the opinion to be about, and paying a second model to
// form one buys a number that reads like quality and is not.
//
// This lane sends the candidate call BARE — certcase_replydraft says so: the
// site's CompleteValidated wrapper is production's, and the case issues the
// request underneath it. So the §5.2 retry that now tells a truncated model to
// be briefer never runs here, and a cut-off reply reaches the judge intact.
func TestATruncatedRunIsNotSentToTheJudge(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		finish    string
		gradeable bool
	}{
		"a complete answer":    {"stop", true},
		"cut off at the limit": {model.FinishReasonLength, false},
		"no reason reported":   {"", true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pooled, err := poolRunCalls([]ai.Call{{
				Provider: "openai_compatible", ServedModel: "m", FinishReason: tc.finish,
			}})
			if err != nil {
				t.Fatalf("pooling one call: %v", err)
			}
			if pooled.Truncated == tc.gradeable {
				t.Errorf("finish_reason %q: Truncated=%v, want %v", tc.finish, pooled.Truncated, !tc.gradeable)
			}
		})
	}
}

// Truncation anywhere in a pooled run counts, not only on the last call.
//
// A run is several calls folded into one accounting, and Degraded is already
// OR-ed across them for the same reason: one spoiled attempt spoils the run.
func TestTruncationAnywhereInARunCounts(t *testing.T) {
	t.Parallel()
	pooled, err := poolRunCalls([]ai.Call{
		{Provider: "openai_compatible", ServedModel: "m", FinishReason: model.FinishReasonLength},
		{Provider: "openai_compatible", ServedModel: "m", FinishReason: "stop"},
	})
	if err != nil {
		t.Fatalf("pooling: %v", err)
	}
	if !pooled.Truncated {
		t.Error("a run carrying a cut-off call was pooled as a complete one")
	}
}
