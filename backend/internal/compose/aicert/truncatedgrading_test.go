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
	cutOff := RunResult{Ungraded: true}

	median, minimum, anyGraded := judgeMedianAndMin([]RunResult{graded(90), cutOff, graded(100)})
	if !anyGraded {
		t.Fatal("two graded runs reported as ungraded")
	}
	if minimum == 0 {
		t.Error("an ungraded run's zero reached the judge's minimum")
	}
	// EXACT, because the ungraded run is what makes the score set even, and an
	// even median is the average of the two middles. Asserting only "one of
	// the scores a judge gave" accepted the upper middle, which is the error
	// below, so it is spelled out rather than bounded.
	if median != 95 {
		t.Errorf("median = %d, want 95 — the average of the two graded scores", median)
	}
}

// An even graded set takes the AVERAGE of its two middles, never the upper one.
//
// This is the direction a certification number must not err in. RunnerConfig
// repeats an odd number of times, so the run set is odd and a median looked
// safe; an ungraded run leaves the SCORE set even, and upper-middle selection
// then reports a pair {10, 80} as 80. That clears a DegradedMin of 60 on a set
// half of whose grades are a 10.
func TestAnEvenGradedSetTakesTheAverageOfItsMiddles(t *testing.T) {
	t.Parallel()
	graded := func(score int) RunResult { return RunResult{HardPass: true, Score: score} }

	median, _, anyGraded := judgeMedianAndMin([]RunResult{graded(10), {Ungraded: true}, graded(80)})
	if !anyGraded {
		t.Fatal("two graded runs reported as ungraded")
	}
	if median != 45 {
		t.Errorf("median = %d, want 45 — the upper middle certifies a set it should not", median)
	}
}

// A cut-off run counts as a RUN and never as a pass.
//
// Both halves matter. It has to count, or a binding that truncates half its
// answers reports a denominator it did not earn. And it must not pass: a run
// that contributed to reliability while withholding its score would lift a
// certified median above what the binding earned, and the model is the only
// actor in that arithmetic.
func TestACutOffRunCountsAsARunAndNeverAsAPass(t *testing.T) {
	t.Parallel()
	_, reliability := Verdict(ScenarioRuns{Runs: []RunResult{
		{HardPass: true, Score: 100},
		{HardPass: false, Ungraded: true},
		{HardPass: true, Score: 90},
	}, Bands: Bands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}})
	if reliability != 2.0/3.0 {
		t.Errorf("reliability = %v, want 2/3 — the cut-off run is one of three runs", reliability)
	}
}

// Every run ungraded leaves nothing for the judge's half of the bands, and a
// verdict is not certified on a median nobody produced.
func TestATaskWhoseEveryRunWasCutOffIsNotCertified(t *testing.T) {
	t.Parallel()
	runs := []RunResult{
		{HardPass: false, Ungraded: true},
		{HardPass: false, Ungraded: true},
		{HardPass: false, Ungraded: true},
	}
	bands := Bands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}

	verdict, _ := Verdict(ScenarioRuns{Runs: runs, Bands: bands})
	if verdict != VerdictNotSupported {
		t.Errorf("verdict = %q, want %q — no judge saw any of these runs", verdict, VerdictNotSupported)
	}
	// And the RECORD's own percentiles, which read the same set through the
	// same helper. Indexing an empty score set here was a panic, and it was a
	// panic because the guard had been added to Verdict and not to its sibling.
	median, minimum, graded := judgeMedianAndMin(runs)
	if graded || median != 0 || minimum != 0 {
		t.Errorf("judgeMedianAndMin = (%d, %d, %v), want (0, 0, false) for an all-ungraded task",
			median, minimum, graded)
	}
}

// A candidate cut off on every attempt still yields a record: each run counts,
// fails and goes ungraded, and none is re-driven as though the provider were
// down. What each adapter must hand over for this to hold — the partial text
// and model.FinishReasonLength rather than an error — is held per wire by the
// ai package's finish-reason parity tests; the cert lane refuses a loopback
// vendor endpoint, so the real wire cannot be served here.
func TestATaskCutOffOnEveryAttemptIsRecordedNotAborted(t *testing.T) {
	waited := recordSleeps(t)
	cutOff := ai.FakeStep{Text: "the widget is", FinishReason: model.FinishReasonLength}
	candidate := ai.NewFakeClient().ScriptSteps(cutOff, cutOff, cutOff)
	// Left unscripted: a cut-off answer is never sent to the judge.
	judge := ai.NewFakeClient()

	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{testScenario("basic", wideBands)}, testCensus(t),
		ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judge)},
		})
	if err != nil {
		t.Fatalf("a candidate that answered, if too long, aborted the task with no record: %v", err)
	}
	if rec.Runs != 3 || rec.Reliability != 0 || rec.Verdict != VerdictNotSupported {
		t.Errorf("runs=%d reliability=%v verdict=%q, want 3 failed runs and %q", rec.Runs, rec.Reliability, rec.Verdict, VerdictNotSupported)
	}
	if got := len(candidate.Calls()); got != 3 {
		t.Errorf("the candidate was called %d times, want 3 — one per run, with no ladder walk and no re-drive", got)
	}
	if len(*waited) != 0 {
		t.Errorf("waited %v — a truncation is an answer, not an outage to back off from", *waited)
	}
	if got := len(judge.Calls()); got != 0 {
		t.Errorf("the judge was called %d time(s) on answers that never finished", got)
	}
}
