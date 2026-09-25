// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testScenario builds a Scenario for TaskSummarize — the one task every
// certifyTask test in this file certifies (the task under test is
// always passed to certifyTask directly; sc.Task here is just the
// corpus record's own descriptive field, never consulted for routing).
func testScenario(name string, bands Bands) Scenario {
	return testScenarioOnSite(name, widgetVariant, bands)
}

// testScenarioOnSite is testScenario for a named site, which is what a task
// whose sites differ in kind needs: the record's certified scope is read off
// the site each scenario names.
func testScenarioOnSite(name, variant string, bands Bands) Scenario {
	return Scenario{
		Name:        name,
		Task:        string(ai.TaskSummarize),
		Site:        variant,
		Source:      sourceHandAuthored,
		SanitizedBy: "tester",
		Fixture:     JSONValue(`{"subject":"a widget"}`),
		Expect: Expectations{
			Outcome: aitasks.OutcomeAccepted,
			Answer:  JSONValue(`"` + containsWidget + `"`),
			Rubric:  "Score higher for a longer, on-topic, concrete answer; lower for a vague or off-topic one.",
			Bands:   bands,
		},
	}
}

func scoreJSON(score int) string {
	return `{"score": ` + strconv.Itoa(score) + `, "reason": "test-scripted"}`
}

var wideBands = Bands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}

// opinionsOf scripts every opinion runs graded runs ask the judge for, each
// scoring score.
func opinionsOf(score, runs int) []string {
	return slices.Repeat([]string{scoreJSON(score)}, runs*judgeOpinions)
}

// --- certifyTask: the real router pipeline over the offline fake ---

func TestCertifyTaskCertifiesWhenEveryRunPassesAndScoresHigh(t *testing.T) {
	// One case right three times of three is not yet evidence of 90%: the pool
	// is undecided, so the case is extended to its cap before it certifies.
	candidateFake := ai.NewFakeClient().Script(slices.Repeat([]string{"the widget is blue and durable"}, adaptiveMaxRuns)...)
	judgeFake := ai.NewFakeClient().Script(opinionsOf(90, adaptiveMaxRuns)...)

	sc := testScenario("basic", wideBands)
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
	})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if rec.Verdict != VerdictCertified {
		t.Fatalf("verdict = %q, want %q (record: %+v)", rec.Verdict, VerdictCertified, rec)
	}
	if rec.Runs != adaptiveMaxRuns || rec.Reliability != 1 {
		t.Fatalf("runs=%d reliability=%v, want %d and 1", rec.Runs, rec.Reliability, adaptiveMaxRuns)
	}
	if rec.JudgeScoreP50 != 90 || rec.JudgeScoreMin != 90 {
		t.Fatalf("judge_score_p50=%d judge_score_min=%d, want 90 and 90", rec.JudgeScoreP50, rec.JudgeScoreMin)
	}
	if !rec.SelfJudged {
		t.Fatalf("both candidate and judge served through the fake provider — want self_judged true, record: %+v", rec)
	}
	if rec.Provider != ai.ProviderFake || rec.ServedModel != ai.ProviderFake {
		t.Fatalf("provider/served_model = %q/%q, want %q/%q", rec.Provider, rec.ServedModel, ai.ProviderFake, ai.ProviderFake)
	}
}

func TestCertifyTaskSupportedDegradedOnPartialReliability(t *testing.T) {
	candidateFake := ai.NewFakeClient().Script(
		"the widget is blue and durable",
		"the widget is blue and durable",
		"off topic, no keyword here",
	)
	judgeFake := ai.NewFakeClient().Script(opinionsOf(70, 3)...)

	sc := testScenario("basic", wideBands)
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
		maxRuns:       3,
	})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if rec.Verdict != VerdictSupportedDegraded {
		t.Fatalf("verdict = %q, want %q (record: %+v)", rec.Verdict, VerdictSupportedDegraded, rec)
	}
	if got := rec.Reliability; got < 0.66 || got > 0.67 {
		t.Fatalf("reliability = %v, want ~2/3", got)
	}
}

func TestCertifyTaskNotSupportedOnLowScores(t *testing.T) {
	candidateFake := ai.NewFakeClient().Script("the widget is blue", "the widget is blue", "the widget is blue")
	judgeFake := ai.NewFakeClient().Script(opinionsOf(10, 3)...)

	sc := testScenario("basic", wideBands)
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
		maxRuns:       3,
	})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if rec.Verdict != VerdictNotSupported {
		t.Fatalf("verdict = %q, want %q — every run passed structurally but the score never clears the floor", rec.Verdict, VerdictNotSupported)
	}
}

// TestCertifyTaskDegradedCandidateAttemptYieldsNoRecord covers the
// spec's hard rule: a budget-forced degrade on ANY run refuses the whole
// task's certification rather than certifying a demoted answer.
// WithMonthlyBudget(1) guarantees the second call already sees a spent
// balance many multiples of the ceiling, so TaskSummarize's ladder
// (interactive, so it pins rather than queues) demotes to local_small —
// still bound and servable under ai.FakeRoutingConfig(), so this is a
// genuine soft-degrade, never a hard failure.
func TestCertifyTaskDegradedCandidateAttemptYieldsNoRecord(t *testing.T) {
	sc := testScenario("basic", wideBands)
	_, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithMonthlyBudget(1)},
	})
	if err == nil {
		t.Fatal("want an error — no record for a task with a degraded candidate attempt")
	}
	if !strings.Contains(err.Error(), "degraded") {
		t.Fatalf("error should name the degrade, got %v", err)
	}
}

// TestCertifyTaskDegradedJudgeAttemptYieldsNoRecord covers the judge-side
// half of the spec's hard rule (§5: "any Degraded attempt ⇒ no record for
// that task"), which historically was only checked on the candidate: a
// budget-forced demotion on the JUDGE's own trace must also void the
// task's record, because a demoted judge silently grades every run with
// a weaker model and nothing in the Record would ever show it.
//
// The judge's task (cert_judge) queues rather than degrades once its
// budget is fully exhausted for background work, so a naively tiny budget
// would surface a hard ErrBudgetDeferred, not the soft in-band
// [80%,100%) demotion this test needs. Instead: probe the exact token
// cost of the judge's first call (request and response text are fixed,
// so the fake client's deterministic "4 bytes per token" arithmetic
// makes this exact, not approximate), then size the budget so the
// THIRD call — the run's last opinion, still against the same judge
// router/meter as the first two — lands at ~90% utilization: squarely
// inside the soft-degrade band regardless of small estimation error.
func TestCertifyTaskDegradedJudgeAttemptYieldsNoRecord(t *testing.T) {
	const candidateOutput = "the widget is blue and durable"
	sc := testScenario("basic", wideBands)

	candidateReq, err := firstBuiltRequest(context.Background(), sc, testCensus(t))
	if err != nil {
		t.Fatalf("building the candidate's request: %v", err)
	}
	probeIn, err := graderInput(sc, aitasks.Trace{Requests: []model.Request{candidateReq}}, candidateOutput)
	if err != nil {
		t.Fatalf("assembling the grader's input: %v", err)
	}
	probeReq := compose.JudgeRequest(probeIn)
	probeResp, err := ai.NewFakeClient().Script(scoreJSON(90)).Complete(context.Background(), probeReq)
	if err != nil {
		t.Fatalf("probing the judge's first-call token cost: %v", err)
	}
	call1Tokens := int64(probeResp.InputTokens + probeResp.OutputTokens)
	budget := 2 * call1Tokens * 10 / 9 // ~90% utilization after call 2 — inside [80%,100%)

	candidateFake := ai.NewFakeClient().Script(candidateOutput)
	judgeFake := ai.NewFakeClient().Script(opinionsOf(90, 1)...)

	_, err = certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 1, quietLogger(), &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake), ai.WithMonthlyBudget(budget)},
	})
	if err == nil {
		t.Fatal("want an error — no record for a task with a degraded judge attempt")
	}
	if !strings.Contains(err.Error(), "judge") || !strings.Contains(err.Error(), "degraded") {
		t.Fatalf("error should name the judge degrade, got %v", err)
	}
}

// TestCertifyTaskJudgeRetriesOnceOnAParseFailureThenScores proves the
// judge's one-retry contract: a first reply that fails strict JSON
// parsing is retried once, and the retry's score is what the run keeps.
func TestCertifyTaskJudgeRetriesOnceOnAParseFailureThenScores(t *testing.T) {
	candidateFake := ai.NewFakeClient().Script("the widget is blue and durable")
	judgeFake := ai.NewFakeClient().Script("not valid json at all", scoreJSON(80), scoreJSON(80), scoreJSON(80))

	sc := testScenario("basic", wideBands)
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 1, quietLogger(), &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
		maxRuns:       1,
	})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if rec.JudgeScoreP50 != 80 {
		t.Fatalf("judge_score_p50 = %d, want 80 (the retry's score)", rec.JudgeScoreP50)
	}
	if got := rec.Scenarios[0].JudgeScores; !slices.Equal(got, []int{80}) {
		t.Fatalf("graded scores = %v, want the one run scored 80 from its three opinions", got)
	}
}

// A judge that never produced a verdict gave no opinion, so the run it failed
// to grade is left out of the judge's numbers instead of dragging them to 0.
func TestCertifyTaskLeavesARunUngradedWhenEveryJudgeAttemptFailsToParse(t *testing.T) {
	candidateFake := ai.NewFakeClient().Script(slices.Repeat([]string{"the widget is blue and durable"}, adaptiveMaxRuns)...)
	// Run 2's three opinions each walk the structured policy's three attempts.
	judgeFake := ai.NewFakeClient().Script(slices.Concat(opinionsOf(90, 1),
		slices.Repeat([]string{"still not json"}, 3*judgeOpinions), opinionsOf(90, adaptiveMaxRuns-2))...)

	sc := testScenario("basic", wideBands)
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
	})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if rec.JudgeScoreP50 != 90 || rec.JudgeScoreMin != 90 {
		t.Fatalf("judge_score_p50=%d judge_score_min=%d, want 90 and 90 — the unparsed run must not count as a 0", rec.JudgeScoreP50, rec.JudgeScoreMin)
	}
	if rec.Verdict != VerdictCertified {
		t.Fatalf("verdict = %q, want %q", rec.Verdict, VerdictCertified)
	}
}

// A run passes when what happened is what the scenario said should happen, and
// nothing in the runner privileges "accepted". This is what lets a scenario
// whose right answer is silence exist at all: the same three replies pass a
// scenario expecting an abstention and fail one expecting an answer, so the
// comparison is doing the work rather than a hardcoded word.
func TestCertifyTaskPassesTheRunsAScenarioSaysShouldAbstain(t *testing.T) {
	cases := []struct {
		name            string
		expectedOutcome string
		wantReliability float64
	}{
		{"the scenario expects the abstention", aitasks.OutcomeAbstained, 1},
		{"the scenario expects an answer", aitasks.OutcomeAccepted, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidateFake := ai.NewFakeClient().Script(widgetAbstention, widgetAbstention, widgetAbstention)
			judgeFake := ai.NewFakeClient().Script(opinionsOf(90, 3)...)

			sc := testScenario("abstains", wideBands)
			sc.Expect.Outcome = tc.expectedOutcome

			rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t),
				ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
					candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
					judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
					maxRuns:       3,
				})
			if err != nil {
				t.Fatalf("certifyTask: %v", err)
			}
			if rec.Reliability != tc.wantReliability {
				t.Fatalf("reliability = %v, want %v (record: %+v)", rec.Reliability, tc.wantReliability, rec)
			}
		})
	}
}

// One scenario grading below its floor sinks a task, however well its sibling does.
func TestCertifyTaskIsNotSupportedWhenOneScenarioGradesBelowItsBands(t *testing.T) {
	candidateFake := ai.NewFakeClient().Script(
		"the widget is blue", "the widget is blue", "the widget is blue", // scenario 1
		"the widget is blue", "the widget is blue", "the widget is blue", // scenario 2
	)
	judgeFake := ai.NewFakeClient().Script(slices.Concat(
		opinionsOf(90, 3), // scenario 1: certified-quality
		opinionsOf(10, 3), // scenario 2: below its floor
	)...)

	scenarios := []Scenario{
		testScenario("good", wideBands),
		testScenario("bad", wideBands),
	}
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, scenarios, testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
		maxRuns:       3,
	})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if rec.Runs != 6 {
		t.Fatalf("runs = %d, want 6 (2 scenarios x 3 repeats, pooled)", rec.Runs)
	}
	if rec.Verdict != VerdictNotSupported {
		t.Fatalf("verdict = %q, want %q — one scenario's scores sit below its floor", rec.Verdict, VerdictNotSupported)
	}
}

// The task verdict is the pass rate over every scenario's runs: 58 of 60
// certifies, and so does each row by the per-case gates. Two passed 2 of 3, which is
// within one standard error of half, so each runs one more round and no other
// case does.
func TestCertifyTaskCertifiesAPoolWithTwoScatteredMisses(t *testing.T) {
	const scenarioCount = 20
	answers := make([]string, 0, scenarioCount*3)
	scores := make([]string, 0, scenarioCount*3)
	scenarios := make([]Scenario, 0, scenarioCount)
	for i := 0; i < scenarioCount; i++ {
		scenarios = append(scenarios, testScenario(fmt.Sprintf("case_%02d", i), wideBands))
		for run := 0; run < 3; run++ {
			answer := "the widget is blue"
			if run == 0 && i < 2 {
				answer = "it is blue"
			}
			answers = append(answers, answer)
		}
	}
	// The extension round: cases 0 and 1, three more runs each, all passing.
	answers = append(answers, slices.Repeat([]string{"the widget is blue"}, 2*adaptiveRound)...)
	scores = append(scores, opinionsOf(90, scenarioCount*3+2*adaptiveRound)...)
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, scenarios, testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(ai.NewFakeClient().Script(answers...))},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(ai.NewFakeClient().Script(scores...))},
	})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if rec.Passed != 58+2*adaptiveRound || rec.Verdict != VerdictCertified {
		t.Fatalf("passed=%d verdict=%q, want %d and %q", rec.Passed, rec.Verdict, 58+2*adaptiveRound, VerdictCertified)
	}
	for i, row := range rec.Scenarios {
		want := 3
		if i < 2 {
			want = 3 + adaptiveRound
		}
		if row.Runs != want {
			t.Errorf("row %s ran %d times, want %d", row.Scenario, row.Runs, want)
		}
	}
	if row := rec.Scenarios[0]; row.Verdict != VerdictCertified || row.JudgeBand != VerdictCertified {
		t.Fatalf("row %s: verdict=%q judge_band=%q, want its 5 of 6 at 90 to clear every per-case gate",
			row.Scenario, row.Verdict, row.JudgeBand)
	}
}

// TestCertifyTaskVoidsARecordWhenALaterRunIsServedByADifferentModel
// covers I2: TaskSummarize's ladder is [cheap_cloud, premium]; cheap_cloud
// serves run 1 as "model-a", then fails transiently on run 2 so premium
// serves it instead as "model-b" — a genuine mid-set ladder fallback
// (mirroring the ai package's own TestLadderFallbackBuffersOneLogicalCall-
// WithTwoAttempts at the router level, replayed here through certifyTask's
// pooled accounting). The task must void its record rather than certify
// scores partly produced by one model and partly by another.
func TestCertifyTaskVoidsARecordWhenALaterRunIsServedByADifferentModel(t *testing.T) {
	candidateFake := ai.NewFakeClient().ScriptSteps(
		ai.FakeStep{Text: "the widget is blue and durable", ServedModel: "model-a"}, // run 1: cheap_cloud serves
		ai.FakeStep{Err: errors.New("cheap_cloud: transient provider error")},       // run 2: cheap_cloud fails
		ai.FakeStep{Text: "the widget is blue and durable", ServedModel: "model-b"}, // run 2: premium falls back and serves
	)
	judgeFake := ai.NewFakeClient().Script(opinionsOf(90, 2)...)

	sc := testScenario("basic", wideBands)
	_, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{sc}, testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 2, quietLogger(), &certifyHooks{
		candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
		judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
	})
	if err == nil {
		t.Fatal("want an error — no record for a task whose runs were served by more than one model")
	}
	if !strings.Contains(err.Error(), "model-a") || !strings.Contains(err.Error(), "model-b") {
		t.Fatalf("error should name both identities, got %v", err)
	}
}

// TestCertifyTaskRecordsTheOutcomeEachRunProduced proves the record reports
// the validators' own verdicts rather than a re-derivation of them. The three
// runs below share one pass/fail column — two failed, one passed — and differ
// in what actually happened: an accepted answer, a wrong one, and an
// abstention. A record that inferred its counts from HardPass could not name
// any of the three.
func TestCertifyTaskRecordsTheOutcomeEachRunProduced(t *testing.T) {
	candidateFake := ai.NewFakeClient().Script(
		"the widget is blue and durable", // accepted
		"off topic, no keyword here",     // wrong answer
		widgetAbstention,                 // abstained
	)
	judgeFake := ai.NewFakeClient().Script(opinionsOf(90, 3)...)

	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{testScenario("basic", wideBands)},
		testCensus(t), ai.ProviderConfig{Provider: ai.ProviderFake, Model: "candidate"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"}, ai.ProfileEUHosted, 3, quietLogger(), &certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithFakeClient(candidateFake)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
			maxRuns:       3,
		})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if rec.ReportedAccepted != 1 || rec.ReportedWrongAnswer != 1 || rec.ReportedInvalid != 0 || rec.ReportedAbstained != 1 {
		t.Fatalf("reported outcome counts = accepted=%d wrong_answer=%d invalid=%d abstained=%d, want 1/1/0/1 (record: %+v)",
			rec.ReportedAccepted, rec.ReportedWrongAnswer, rec.ReportedInvalid, rec.ReportedAbstained, rec)
	}
	if rec.Passed != 1 {
		t.Fatalf("passed = %d, want 1 — only the accepted run did what the scenario asked", rec.Passed)
	}
	if got := rec.Reliability; got < 0.33 || got > 0.34 {
		t.Fatalf("reliability = %v, want ~1/3 — only the accepted run matched the scenario's expected outcome", got)
	}
	if rec.ContextApplied {
		t.Fatal("context_applied is true, but no run here read a company context the product's own path would have applied")
	}
}
