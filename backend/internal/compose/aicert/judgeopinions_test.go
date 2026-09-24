// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
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

// One judge's word decides nothing below the bar: it scored one correct answer
// 0, 100 and 20 across three runs. A low score is weighed against two fresh
// opinions, a reply that never parsed is no opinion at all, and a score at the
// bar costs exactly the one call it always did. wideBands' certified_min is 70.
func TestALowScoreIsWeighedAgainstFreshOpinions(t *testing.T) {
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
		"a score at certified_min is one opinion": {
			replies: []ai.FakeStep{scored(70, "judge-a")}, wantCalls: 1,
			wantScore: 70, wantScores: []int{70}, wantServedBy: "judge-a",
		},
		"a low score is scored at the median of three": {
			replies:   []ai.FakeStep{scored(10, "judge-a"), scored(80, "judge-b"), scored(30, "judge-c")},
			wantCalls: 3, wantScore: 30, wantScores: []int{10, 80, 30}, wantServedBy: "judge-c",
		},
		"an unparseable re-judge drops out of the median": {
			replies:   slices.Concat([]ai.FakeStep{scored(10, "judge-a"), scored(20, "judge-b")}, junk),
			wantCalls: 5, wantScore: 15, wantScores: []int{10, 20}, wantServedBy: "judge-b",
		},
		"two unparseable re-judges leave the first opinion standing": {
			replies:   slices.Concat([]ai.FakeStep{scored(10, "judge-a")}, junk, junk),
			wantCalls: 7, wantScore: 10, wantScores: []int{10}, wantServedBy: "judge-a",
		},
		"a first reply that never parses leaves the run ungraded, not 0": {
			replies: junk, wantCalls: 3, wantUngraded: true,
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

// The degrade is folded over every call the run's grading made, so a re-judge
// demoted onto a cheaper tier voids the run exactly as a demoted first call
// would. The budget puts two calls at ~90% utilisation, inside the soft-degrade
// band, so only the last re-judge is demoted and no call is deferred.
func TestADemotedRejudgeMarksTheRunJudgeDegraded(t *testing.T) {
	sc := testScenario("basic", wideBands)
	probe, err := ai.NewFakeClient().Script(scoreJSON(10)).
		Complete(context.Background(), compose.JudgeRequest(sc.Expect.Rubric, widgetAsk, gradedOutput))
	if err != nil {
		t.Fatalf("probing the judge's first-call token cost: %v", err)
	}
	budget := 2 * int64(probe.InputTokens+probe.OutputTokens) * 10 / 9

	for name, tc := range map[string]struct {
		first        int
		wantDegraded bool
	}{
		"a score at the bar makes no second call to be demoted": {first: 90, wantDegraded: false},
		"a low score's re-judge is demoted":                     {first: 10, wantDegraded: true},
	} {
		t.Run(name, func(t *testing.T) {
			fake := ai.NewFakeClient().Script(scoreJSON(tc.first), scoreJSON(10), scoreJSON(10))
			if got := judged(t, fake, ai.WithMonthlyBudget(budget)); got.degraded != tc.wantDegraded {
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
	candidate := ai.NewFakeClient().Script(slices.Repeat([]string{containsWidget}, testRepeats)...)
	judge := ai.NewFakeClient().Script(scoreJSON(10), scoreJSON(80), scoreJSON(30), unparseable, unparseable, unparseable, scoreJSON(90))
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("certifying: %v", err)
	}

	want := map[int]RunResult{
		1: {Score: 30, JudgeScores: []int{10, 80, 30}},
		2: {Ungraded: true},
		3: {Score: 90, JudgeScores: []int{90}},
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
