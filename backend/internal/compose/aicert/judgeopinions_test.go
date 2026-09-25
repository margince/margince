// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	gradedOutput = "the widget is blue and durable"
	unparseable  = "I would describe the widget as blue."
	widgetAsk    = "a widget"
)

// judgeOver builds the judge router the way certifyTask does, served by fake.
func judgeOver(t *testing.T, fake *ai.FakeClient, extra ...ai.LocalOption) (*ai.Router, *traceRecorder) {
	t.Helper()
	cfg, err := ladderForTask("judge", testJudgeBinding, ai.ProfileEUHosted, ai.TaskSummarize)
	if err != nil {
		t.Fatalf("binding the judge ladder: %v", err)
	}
	rec := newTraceRecorder()
	opts := append([]ai.LocalOption{ai.WithoutResultCache(), ai.WithCallStore(rec), ai.WithFakeClient(fake)}, extra...)
	router, err := compose.NewLocalRouterForCert(cfg, opts...)
	if err != nil {
		t.Fatalf("building the judge router: %v", err)
	}
	return router, rec
}

func widgetAskTrace() aitasks.Trace {
	return aitasks.Trace{Requests: []model.Request{{Messages: []model.Message{{Role: roleUser, Content: widgetAsk}}}}}
}

func judged(t *testing.T, fake *ai.FakeClient, extra ...ai.LocalOption) judgement {
	t.Helper()
	router, rec := judgeOver(t, fake, extra...)
	got, err := judgeScore(wsContext(t), router, rec, testScenario("basic", wideBands), widgetAskTrace(), gradedOutput, quietLogger())
	if err != nil {
		t.Fatalf("judgeScore: %v", err)
	}
	return got
}

// No run is scored on one judge's word: one judge scored a single correct answer
// 0, 100 and 20 across three runs. Every run — a high first score included — is
// graded three times and scored at the median of what parsed; a reply that
// never parsed is no opinion at all.
func TestEveryRunIsGradedThreeTimesAndScoredAtTheMedian(t *testing.T) {
	scored := func(score int, servedBy string) ai.FakeStep {
		return ai.FakeStep{Text: scoreJSON(score), ServedModel: servedBy}
	}
	// An opinion that never parses costs the structured policy's three walks:
	// the try, the told-what-failed retry, and the escalation.
	junk := slices.Repeat([]ai.FakeStep{{Text: unparseable, ServedModel: "judge-junk"}}, 3)
	for name, tc := range map[string]struct {
		replies      []ai.FakeStep
		wantCalls    int
		wantScore    int
		wantScores   []int
		wantUngraded bool
		wantServedBy string
	}{
		"a high first score is weighed against two more": {
			replies:   []ai.FakeStep{scored(95, "judge-a"), scored(60, "judge-b"), scored(70, "judge-c")},
			wantCalls: 3, wantScore: 70, wantScores: []int{95, 60, 70}, wantServedBy: "judge-c",
		},
		"a low first score is weighed against two more": {
			replies:   []ai.FakeStep{scored(10, "judge-a"), scored(80, "judge-b"), scored(30, "judge-c")},
			wantCalls: 3, wantScore: 30, wantScores: []int{10, 80, 30}, wantServedBy: "judge-c",
		},
		"an unparseable opinion drops out of the median": {
			replies:   slices.Concat([]ai.FakeStep{scored(10, "judge-a"), scored(20, "judge-b")}, junk),
			wantCalls: 5, wantScore: 15, wantScores: []int{10, 20}, wantServedBy: "judge-b",
		},
		"two unparseable opinions leave the one that parsed": {
			replies:   slices.Concat([]ai.FakeStep{scored(10, "judge-a")}, junk, junk),
			wantCalls: 7, wantScore: 10, wantScores: []int{10}, wantServedBy: "judge-a",
		},
		"no opinion that parses leaves the run ungraded, not 0": {
			replies: slices.Concat(junk, junk, junk), wantCalls: 9, wantUngraded: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			fake := ai.NewFakeClient().ScriptSteps(tc.replies...)
			got := judged(t, fake)
			if calls := len(fake.Calls()); calls != tc.wantCalls {
				t.Errorf("the judge was called %d times, want %d", calls, tc.wantCalls)
			}
			if got.ungraded != tc.wantUngraded || got.score != tc.wantScore || !slices.Equal(got.scores, tc.wantScores) {
				t.Errorf("judgement = ungraded %v, score %d, opinions %v; want ungraded %v, score %d, opinions %v",
					got.ungraded, got.score, got.scores, tc.wantUngraded, tc.wantScore, tc.wantScores)
			}
			if got.servedModel != tc.wantServedBy {
				t.Errorf("judge served model = %q, want the last graded opinion's %q", got.servedModel, tc.wantServedBy)
			}
		})
	}
}

func TestOpinionsNoneOfWhichParsedLeaveTheRunUngraded(t *testing.T) {
	got := foldOpinions([]opinion{{servedModel: "a"}, {servedModel: "b"}, {servedModel: "c"}})
	if !got.ungraded || got.scores != nil || got.servedModel != "" {
		t.Fatalf("three absent opinions folded to %+v, want an ungraded run naming no grader", got)
	}
}

// The degrade is folded over every call the run's grading made, so a demoted
// third opinion voids the run exactly as a demoted first one would. The tight
// budget puts two calls at ~90% utilisation, inside the soft-degrade band, so
// only the third opinion is demoted and no call is deferred.
func TestADemotedOpinionMarksTheRunJudgeDegraded(t *testing.T) {
	sc := testScenario("basic", wideBands)
	probeIn, err := graderInput(sc, widgetAskTrace(), gradedOutput)
	if err != nil {
		t.Fatalf("assembling the grader's input: %v", err)
	}
	probe, err := ai.NewFakeClient().Script(scoreJSON(10)).
		Complete(context.Background(), compose.JudgeRequest(probeIn))
	if err != nil {
		t.Fatalf("probing the judge's first-call token cost: %v", err)
	}
	perCall := int64(probe.InputTokens + probe.OutputTokens)

	for name, tc := range map[string]struct {
		budget       int64
		wantDegraded bool
	}{
		"a budget every opinion fits in demotes nothing": {budget: 100 * perCall, wantDegraded: false},
		"a third opinion past the soft line is demoted":  {budget: 2 * perCall * 10 / 9, wantDegraded: true},
	} {
		t.Run(name, func(t *testing.T) {
			fake := ai.NewFakeClient().Script(scoreJSON(90), scoreJSON(10), scoreJSON(10))
			if got := judged(t, fake, ai.WithMonthlyBudget(tc.budget)); got.degraded != tc.wantDegraded {
				t.Fatalf("judge degraded = %v, want %v", got.degraded, tc.wantDegraded)
			}
		})
	}
}

// Every opinion a run was scored from survives the resume journal, so a reader
// of a replayed record can still see which grade was contested.
func TestTheJournalKeepsEveryOpinionARunWasScoredFrom(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate := ai.NewFakeClient().Script(slices.Repeat([]string{containsWidget}, adaptiveMaxRuns)...)
	judge := ai.NewFakeClient().Script(slices.Concat(
		[]string{scoreJSON(10), scoreJSON(80), scoreJSON(30)},
		slices.Repeat([]string{unparseable}, 3*judgeOpinions),
		opinionsOf(90, adaptiveMaxRuns-2))...)
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("certifying: %v", err)
	}

	want := map[int]RunResult{
		1: {Score: 30, JudgeScores: []int{10, 80, 30}},
		2: {Ungraded: true},
		3: {Score: 90, JudgeScores: []int{90, 90, 90}},
	}
	withJournal(t, dir, fixedResumeNow, func(j *runJournal) {
		for run, w := range want {
			got, ok := j.forTask(ai.TaskSummarize, testCandidateBinding, testJudgeBinding).lookup(sc, stampFor(t, sc), run)
			if !ok {
				t.Fatalf("run %d was not journaled", run)
			}
			if got.Score != w.Score || got.Ungraded != w.Ungraded || !slices.Equal(got.JudgeScores, w.JudgeScores) {
				t.Errorf("run %d journaled as score %d, ungraded %v, opinions %v; want score %d, ungraded %v, opinions %v",
					run, got.Score, got.Ungraded, got.JudgeScores, w.Score, w.Ungraded, w.JudgeScores)
			}
		}
	})
}

// A judge whose provider withheld its answer gave no opinion, which is what an
// unparseable reply already means: the run stays in the record ungraded rather
// than aborting the task as though the judge were unreachable.
func TestAWithheldJudgementLeavesTheRunUngraded(t *testing.T) {
	waited := recordSleeps(t)
	withheld := ai.FakeStep{Err: fmt.Errorf("%w: SAFETY", model.ErrOutputWithheld)}
	candidate := ai.NewFakeClient().Script(containsWidget, containsWidget, containsWidget)
	// A withheld opinion walks the whole ladder before it is given up on.
	judge := ai.NewFakeClient().ScriptSteps(slices.Repeat([]ai.FakeStep{withheld}, ladderRungs(t)*judgeOpinions)...).Script(opinionsOf(90, 2)...)

	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{testScenario("basic", wideBands)}, testCensus(t),
		ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judge)},
			maxRuns:       3,
		})
	if err != nil {
		t.Fatalf("a judge that declined to grade one run aborted the task: %v", err)
	}
	if rec.Runs != 3 || rec.Reliability != 1 {
		t.Errorf("runs=%d reliability=%v, want 3 passing runs — the candidate answered every one", rec.Runs, rec.Reliability)
	}
	if got := rec.Scenarios[0].JudgeScores; !slices.Equal(got, []int{90, 90}) {
		t.Errorf("graded scores = %v, want the two runs a judge answered and none for the withheld one", got)
	}
	if len(*waited) != 0 {
		t.Errorf("waited %v — a withheld judgement is not an outage", *waited)
	}
}
