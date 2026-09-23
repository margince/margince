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

// An ungraded run's absent score never reaches a median or a minimum.
//
// Skipping the judge left Score at zero, and both aggregations read every
// run's Score — so refusing to grade a truncated answer REPORTED it as a
// zero-quality one, which is the ambiguity the skip exists to remove, entered
// from the other side. The run still counts everywhere a run should: the
// mechanical grade judged the same incomplete text production would have.
func TestAnUngradedRunIsLeftOutOfTheJudgesNumbers(t *testing.T) {
	t.Parallel()
	graded := func(score int) RunResult { return RunResult{HardPass: true, Score: score} }
	cutOff := RunResult{HardPass: true, Ungraded: true}

	scores := judgeScores([]RunResult{graded(90), cutOff, graded(100)})
	if len(scores) != 2 {
		t.Fatalf("judgeScores kept %d of 3 runs, want the 2 a judge graded: %v", len(scores), scores)
	}
	for _, s := range scores {
		if s == 0 {
			t.Error("an ungraded run's zero reached the judge's numbers")
		}
	}
}

// And it still counts as a run: reliability is mechanical and says so.
func TestAnUngradedRunStillCountsTowardReliability(t *testing.T) {
	t.Parallel()
	bands := Bands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}

	_, reliability := Verdict([]RunResult{
		{HardPass: true, Score: 100},
		{HardPass: true, Ungraded: true},
		{HardPass: false, Score: 20},
	}, bands)
	if reliability != 2.0/3.0 {
		t.Errorf("reliability = %v, want 2/3 — the ungraded run passed its validator and must count", reliability)
	}
}

// Every run ungraded leaves nothing for the judge's half of the bands, and a
// verdict is not certified on a median nobody produced.
func TestATaskWhoseEveryRunWasCutOffIsNotCertified(t *testing.T) {
	t.Parallel()
	verdict, reliability := Verdict([]RunResult{
		{HardPass: true, Ungraded: true},
		{HardPass: true, Ungraded: true},
		{HardPass: true, Ungraded: true},
	}, Bands{CertifiedMin: 70, DegradedMin: 50, Floor: 40})
	if verdict != VerdictNotSupported {
		t.Errorf("verdict = %q, want %q — no judge saw any of these runs", verdict, VerdictNotSupported)
	}
	if reliability != 1.0 {
		t.Errorf("reliability = %v, want 1.0 — they all passed their validator", reliability)
	}
}
