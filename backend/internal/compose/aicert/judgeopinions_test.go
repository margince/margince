// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"fmt"
	"reflect"
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
	return judgedAgainst(t, wideBands, fake, extra...)
}

func judgedAgainst(t *testing.T, bands Bands, fake *ai.FakeClient, extra ...ai.LocalOption) judgement {
	t.Helper()
	router, rec := judgeOver(t, fake, extra...)
	got, err := judgeScore(wsContext(t), router, rec, testScenario("basic", bands), widgetAskTrace(), gradedOutput, quietLogger())
	if err != nil {
		t.Fatalf("judgeScore: %v", err)
	}
	return got
}

// A run is graded once and asked again only where a reading could decide its
// case: within 10 of a bar a second opinion, two more than 5 apart a third. It
// scores at the median of what parsed, the mean of two; a reply that never
// parsed is no opinion and is replaced within the same three.
func TestARunIsReaskedOnlyWhereOneReadingCouldDecideIt(t *testing.T) {
	scored := func(score int, servedBy string) ai.FakeStep {
		return ai.FakeStep{Text: scoreJSON(score), ServedModel: servedBy}
	}
	// An opinion that never parses costs the structured policy's three walks:
	// the try, the told-what-failed retry, and the escalation.
	junk := slices.Repeat([]ai.FakeStep{{Text: unparseable, ServedModel: "judge-junk"}}, 3)
	strict := Bands{CertifiedMin: 90, DegradedMin: 70, Floor: 50}
	for name, tc := range map[string]struct {
		bands        Bands
		replies      []ai.FakeStep
		wantCalls    int
		wantScore    int
		wantScores   []int
		wantUngraded bool
		wantServedBy string
	}{
		"a score 11 from every bar is taken on one reading": {
			replies:   []ai.FakeStep{scored(81, "judge-a"), scored(10, "judge-b")},
			wantCalls: 1, wantScore: 81, wantScores: []int{81}, wantServedBy: "judge-a",
		},
		"a low score far from every bar is taken on one reading too": {
			replies:   []ai.FakeStep{scored(29, "judge-a"), scored(90, "judge-b")},
			wantCalls: 1, wantScore: 29, wantScores: []int{29}, wantServedBy: "judge-a",
		},
		"a score 10 from a bar is asked again, and two that agree are averaged": {
			replies:   []ai.FakeStep{scored(80, "judge-a"), scored(75, "judge-b"), scored(10, "judge-c")},
			wantCalls: 2, wantScore: 77, wantScores: []int{80, 75}, wantServedBy: "judge-b",
		},
		"the case's own bars decide, not a fixed line": {
			bands:     strict,
			replies:   []ai.FakeStep{scored(95, "judge-a"), scored(93, "judge-b"), scored(10, "judge-c")},
			wantCalls: 2, wantScore: 94, wantScores: []int{95, 93}, wantServedBy: "judge-b",
		},
		"two readings more than 5 apart are settled by a third": {
			replies:   []ai.FakeStep{scored(72, "judge-a"), scored(60, "judge-b"), scored(65, "judge-c")},
			wantCalls: 3, wantScore: 65, wantScores: []int{72, 60, 65}, wantServedBy: "judge-c",
		},
		"a low near score is re-asked as a high one is": {
			replies:   []ai.FakeStep{scored(45, "judge-a"), scored(35, "judge-b"), scored(38, "judge-c")},
			wantCalls: 3, wantScore: 38, wantScores: []int{45, 35, 38}, wantServedBy: "judge-c",
		},
		"an unparseable first opinion is replaced": {
			replies:   slices.Concat(junk, []ai.FakeStep{scored(95, "judge-b"), scored(10, "judge-c")}),
			wantCalls: 4, wantScore: 95, wantScores: []int{95}, wantServedBy: "judge-b",
		},
		"an unparseable opinion still counts toward the three": {
			replies:   slices.Concat([]ai.FakeStep{scored(72, "judge-a")}, junk, []ai.FakeStep{scored(60, "judge-c")}),
			wantCalls: 5, wantScore: 66, wantScores: []int{72, 60}, wantServedBy: "judge-c",
		},
		"two unparseable opinions leave the one that parsed": {
			replies:   slices.Concat([]ai.FakeStep{scored(72, "judge-a")}, junk, junk),
			wantCalls: 7, wantScore: 72, wantScores: []int{72}, wantServedBy: "judge-a",
		},
		"no opinion that parses leaves the run ungraded, not 0": {
			replies: slices.Concat(junk, junk, junk), wantCalls: 9, wantUngraded: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			bands := tc.bands
			if bands == (Bands{}) {
				bands = wideBands
			}
			fake := ai.NewFakeClient().ScriptSteps(tc.replies...)
			got := judgedAgainst(t, bands, fake)
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
// third opinion voids the run exactly as a demoted first one would. The scores
// earn all three opinions; the tight budget puts two calls at ~90% utilisation,
// inside the soft-degrade band, so only the third is demoted and none deferred.
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
			fake := ai.NewFakeClient().Script(scoreJSON(72), scoreJSON(60), scoreJSON(65))
			if got := judged(t, fake, ai.WithMonthlyBudget(tc.budget)); got.degraded != tc.wantDegraded {
				t.Fatalf("judge degraded = %v, want %v", got.degraded, tc.wantDegraded)
			}
		})
	}
}

// Every opinion a run was scored from survives the resume journal, however many
// it was asked for, so a reader of a replayed record can still see which grade
// was contested, and a replay reaches the record the live run did.
func TestTheJournalKeepsEveryOpinionARunWasScoredFrom(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate := ai.NewFakeClient().Script(slices.Repeat([]string{containsWidget}, adaptiveMaxRuns)...)
	judge := ai.NewFakeClient().Script(slices.Concat(
		[]string{scoreJSON(72), scoreJSON(60), scoreJSON(65)},
		slices.Repeat([]string{unparseable}, 3*maxJudgeOpinions),
		[]string{scoreJSON(75), scoreJSON(77)},
		opinionsOf(90, adaptiveMaxRuns-3))...)
	live, err := certifyOnce(t, dir, sc, candidate, judge)
	if err != nil {
		t.Fatalf("certifying: %v", err)
	}

	want := map[int]RunResult{
		1: {Score: 65, JudgeScores: []int{72, 60, 65}},
		2: {Ungraded: true},
		3: {Score: 76, JudgeScores: []int{75, 77}},
		4: {Score: 90, JudgeScores: []int{90}},
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

	refusingCandidate, refusingJudge := refusingFakes(t)
	replayed, err := certifyOnce(t, dir, sc, refusingCandidate, refusingJudge)
	if err != nil {
		t.Fatalf("replaying the journaled runs: %v", err)
	}
	if len(refusingJudge.Calls()) != 0 || !reflect.DeepEqual(live, replayed) {
		t.Fatalf("the replay asked the judge %d time(s) and reached\n %+v\nwant the live record\n %+v",
			len(refusingJudge.Calls()), replayed, live)
	}
}

// A judge whose provider withheld its answer gave no opinion, which is what an
// unparseable reply already means: the run stays in the record ungraded rather
// than aborting the task as though the judge were unreachable.
func TestAWithheldJudgementLeavesTheRunUngraded(t *testing.T) {
	waited := recordSleeps(t)
	withheld := ai.FakeStep{Err: fmt.Errorf("%w: SAFETY", model.ErrOutputWithheld)}
	candidate := ai.NewFakeClient().Script(containsWidget, containsWidget, containsWidget)
	// A withheld opinion walks the whole ladder before it is given up on, and a
	// run with none is asked all three times.
	judge := ai.NewFakeClient().ScriptSteps(slices.Repeat([]ai.FakeStep{withheld}, ladderRungs(t)*maxJudgeOpinions)...).Script(opinionsOf(90, 2)...)

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

// A case declaring judge: none is never sent to the judge: its record row is
// graded on its pass count and carries neither bands nor scores.
func TestAJudgelessCaseCertifiesWithoutAskingTheJudge(t *testing.T) {
	sc := testScenario("basic", Bands{})
	sc.Expect.Rubric, sc.Expect.Judge, sc.Expect.JudgeNoneReason = "", judgeNone, "the check reads the whole answer"
	candidate := ai.NewFakeClient().Script(slices.Repeat([]string{containsWidget}, adaptiveMaxRuns)...)
	judge := ai.NewFakeClient()
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t),
		ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judge)},
		})
	if err != nil {
		t.Fatalf("certifying: %v", err)
	}
	if calls := len(judge.Calls()); calls != 0 {
		t.Errorf("the judge was asked %d time(s) about a case that declares none", calls)
	}
	row := rec.Scenarios[0]
	if rec.Verdict != VerdictCertified || !row.JudgeNone || row.Bands != nil || row.JudgeScores != nil {
		t.Errorf("verdict %q, row %+v; want certified on %d passing runs, marked judge_none with no bands or scores",
			rec.Verdict, row, rec.Runs)
	}
}
