// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// A replay keeps what the models said and grades it with the code that runs
// now. These tests change the grading code or the request between a run and
// its replay, and read what each replay concluded.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// fixedAnswer is a completer that gives one answer to every request.
type fixedAnswer string

func (a fixedAnswer) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: string(a)}, nil
}

// journaledAnswer is a journal line for one run of sc on the widget site. Its
// candidate said answer and its judge gave score, and it carries the evidence a
// live run files.
func journaledAnswer(t *testing.T, sc Scenario, answer string, score int) journaledRun {
	t.Helper()
	census := testCensus(t)
	_, prepared, err := prepareCase(census, ai.TaskSummarize, sc)
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	recorder := &recordingCompleter{next: fixedAnswer(answer)}
	caseTrace, err := prepared.Run(wsContext(t), recorder)
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	evidence, err := recorder.evidence(runCalls{})
	if err != nil {
		t.Fatalf("building the evidence: %v", err)
	}
	if evidence.GraderDigest, err = graderDigest(asGraded(sc, census), caseTrace, answer); err != nil {
		t.Fatalf("digesting the grader request: %v", err)
	}
	return journaledRun{
		Outcome:  runOutcome{RunResult: RunResult{Outcome: aitasks.OutcomeAccepted, HardPass: true, Score: score, JudgeScores: []int{score}}},
		Evidence: evidence,
	}
}

// strictWidgetCases asks what widgetCases asks, so every stamp is the same,
// and accepts only a description that also says "durable".
type strictWidgetCases struct{ widgetCases }

func (c strictWidgetCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	prepared, err := c.widgetCases.Prepare(fixture, expected)
	if err != nil {
		return nil, err
	}
	return strictWidgetCase{prepared}, nil
}

type strictWidgetCase struct{ aitasks.PreparedCase }

func (c strictWidgetCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	if !strings.Contains(trace.Output, "durable") {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: "the description never says durable"}
	}
	return c.PreparedCase.Evaluate(trace)
}

func TestAReplayIsGradedByTheValidatorThatRunsNow(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	first, err := certifyOnce(t, dir, sc, candidate, judge)
	if err != nil {
		t.Fatalf("first certification: %v", err)
	}
	if first.Passed != first.Runs {
		t.Fatalf("the first certification passed %d of %d runs; the test needs every run passing", first.Passed, first.Runs)
	}

	strict := aitasks.NewRegistry()
	strict.Register(widgetSite())
	strict.BindCase(widgetSite(), strictWidgetCases{widgetCases{site: widgetSite()}})
	if stampFor(t, sc) != stampsUnder(t, sc, strict) {
		t.Fatal("the stricter case changed the scenario stamp, so this test would pass by paying for every run")
	}

	refusingCandidate, refusingJudge := refusingFakes(t)
	var replayed Record
	withJournal(t, dir, fixedResumeNow, func(j *runJournal) {
		replayed, err = certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, strict,
			testCandidateBinding, testJudgeBinding, ai.ProfileEUHosted, testRepeats, quietLogger(), &certifyHooks{
				candidateOpts: []ai.LocalOption{ai.WithFakeClient(refusingCandidate)},
				judgeOpts:     []ai.LocalOption{ai.WithFakeClient(refusingJudge)},
				journal:       j.forTask(ai.TaskSummarize, testCandidateBinding, testJudgeBinding),
			})
	})
	if err != nil {
		t.Fatalf("replaying under the stricter validator: %v", err)
	}
	if len(refusingCandidate.Calls()) != 0 || len(refusingJudge.Calls()) != 0 {
		t.Fatalf("the replay paid for %d candidate and %d judge call(s)", len(refusingCandidate.Calls()), len(refusingJudge.Calls()))
	}
	if replayed.Passed != 0 || replayed.ReportedWrongAnswer != replayed.Runs || replayed.Verdict != VerdictNotSupported {
		t.Fatalf("replayed record passed %d, wrong answers %d of %d runs, verdict %q; want every run graded wrong by the stricter validator",
			replayed.Passed, replayed.ReportedWrongAnswer, replayed.Runs, replayed.Verdict)
	}
}

// stampsUnder is sc's stamp with its site bound to census's case.
func stampsUnder(t *testing.T, sc Scenario, census *aitasks.Registry) string {
	t.Helper()
	stamps, err := ScenarioStamps(wsContext(t), []Scenario{sc}, census)
	if err != nil {
		t.Fatalf("ScenarioStamps: %v", err)
	}
	return stamps[sc.Name]
}

func TestAJournaledAnswerWithoutEvidenceIsPaidForAgain(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}
	rewriteEveryJournaledRun(t, dir, func(line *journaledRun) { line.Evidence = nil })

	refusingCandidate, refusingJudge := refusingFakes(t)
	if _, err := certifyOnce(t, dir, sc, refusingCandidate, refusingJudge); err == nil {
		t.Fatal("an answer with nothing to re-grade it from was replayed")
	}
	if len(refusingCandidate.Calls()) == 0 {
		t.Fatal("the candidate was never asked — an answer the current code cannot re-grade stood in for a fresh one")
	}
}

func TestAReplayRenamesTheIDsThisPreparationMinted(t *testing.T) {
	ctx := context.Background()
	oldID, newID := ids.NewV7().String(), ids.NewV7().String()
	ask := func(id string) model.Request {
		return model.Request{System: "Name the row.", Messages: []model.Message{{Role: "user", Content: "row " + id}}}
	}
	recorder := &recordingCompleter{next: fixedAnswer("the row is " + oldID)}
	if _, err := recorder.Complete(ctx, ask(oldID)); err != nil {
		t.Fatalf("recording: %v", err)
	}
	evidence, err := recorder.evidence(runCalls{})
	if err != nil {
		t.Fatalf("building the evidence: %v", err)
	}

	replay := &replayCompleter{evidence: evidence}
	resp, err := replay.Complete(ctx, ask(newID))
	if err != nil {
		t.Fatalf("a request that differs only in its minted id was refused: %v", err)
	}
	if resp.Text != "the row is "+newID {
		t.Fatalf("replayed answer %q, want the id this preparation minted (%s)", resp.Text, newID)
	}
	if err := replay.finished(nil); err != nil {
		t.Fatalf("a replay that asked exactly the journaled call reported %v", err)
	}

	moved := &replayCompleter{evidence: evidence}
	if _, err := moved.Complete(ctx, model.Request{System: "Name the column.", Messages: ask(newID).Messages}); err == nil {
		t.Fatal("a request the journaled answer was never given to was answered from the journal")
	}
	if err := moved.finished(nil); err == nil {
		t.Fatal("a diverged replay reported itself finished")
	}

	unasked := &replayCompleter{evidence: evidence}
	if err := unasked.finished(nil); err == nil || !strings.Contains(err.Error(), "0 of the 1") {
		t.Fatalf("a replay that asked nothing reported %v, want it to name the call it skipped", err)
	}
}

func TestARunServedUnderAnotherRoutingIsPaidForAgain(t *testing.T) {
	dir := t.TempDir()
	sc := testScenario("basic", wideBands)
	candidate, judge := answeringFakes()
	if _, err := certifyOnce(t, dir, sc, candidate, judge); err != nil {
		t.Fatalf("first certification: %v", err)
	}
	rewriteEveryJournaledRun(t, dir, func(line *journaledRun) { line.Evidence.Routing = "replay-routing-0" })

	refusingCandidate, refusingJudge := refusingFakes(t)
	if _, err := certifyOnce(t, dir, sc, refusingCandidate, refusingJudge); err == nil {
		t.Fatal("a run the router served under another version was replayed")
	}
	if len(refusingCandidate.Calls()) == 0 {
		t.Fatal("the candidate was never asked — an answer routed by older code stood in for a fresh one")
	}
}

// A withheld run carries no answer, but its refusal is still the routing's. It
// replays only from a line that says which routing refused it.
func TestAWithheldRunReplaysOnlyUnderItsOwnRouting(t *testing.T) {
	sc := testScenario("basic", wideBands)
	withheld := runOutcome{}
	withheld.Outcome, withheld.Withheld, withheld.Ungraded = aitasks.OutcomeInvalid, "SAFETY", true
	for name, tc := range map[string]struct {
		evidence *replayEvidence
		replays  bool
	}{
		"no evidence":        {nil, false},
		"another routing":    {&replayEvidence{Routing: "replay-routing-0"}, false},
		"this build's route": {&replayEvidence{Routing: replayRouting}, true},
	} {
		got, err := reevaluate(wsContext(t), testCensus(t), ai.TaskSummarize, sc, journaledRun{Outcome: withheld, Evidence: tc.evidence}, quietLogger())
		if replayed := err == nil; replayed != tc.replays {
			t.Errorf("%s: replayed = %v (err %v), want %v", name, replayed, err, tc.replays)
		}
		if tc.replays && got.Withheld != "SAFETY" {
			t.Errorf("%s: replayed as %+v, want the withheld run kept whole", name, got)
		}
	}
}

func TestAReplayRenamesAnIDTheModelWroteInCapitals(t *testing.T) {
	ctx := context.Background()
	oldID, newID := ids.NewV7().String(), ids.NewV7().String()
	ask := func(id string) model.Request {
		return model.Request{System: "Name the row.", Messages: []model.Message{{Role: "user", Content: "row " + id}}}
	}
	recorder := &recordingCompleter{next: fixedAnswer("the row is " + strings.ToUpper(oldID))}
	if _, err := recorder.Complete(ctx, ask(oldID)); err != nil {
		t.Fatalf("recording: %v", err)
	}
	evidence, err := recorder.evidence(runCalls{})
	if err != nil {
		t.Fatalf("building the evidence: %v", err)
	}
	resp, err := (&replayCompleter{evidence: evidence}).Complete(ctx, ask(newID))
	if err != nil {
		t.Fatalf("replaying: %v", err)
	}
	if resp.Text != "the row is "+newID {
		t.Fatalf("replayed answer %q, want the capitalised id renamed to %s", resp.Text, newID)
	}
}
