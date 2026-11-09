// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// A privacy case's leak shows a reader what they must not see. One leaking run
// vetoes it, at its row and at its task alike; any other failure is graded by
// the rule every case is.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

var privacyBands = Bands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}

// The two ways a mechanical run can fail here.
var (
	wrongAnswer = RunResult{Outcome: aitasks.OutcomeWrongAnswer}
	leakingRun  = RunResult{Outcome: aitasks.OutcomeWrongAnswer, Leak: true}
)

// mechanicalCase is one judge-less case of nine runs, the first passes passing
// and every other one failing as failed does.
func mechanicalCase(passes int, privacy bool, failed RunResult) ScenarioRuns {
	set := ScenarioRuns{Mechanical: true, Privacy: privacy, Runs: make([]RunResult, 9)}
	for i := range set.Runs {
		set.Runs[i] = failed
		if i < passes {
			set.Runs[i] = RunResult{Outcome: aitasks.OutcomeAccepted, HardPass: true}
		}
	}
	return set
}

// withHealthySiblings is set beside eight cases passing every run, a pool
// that certifies on its own.
func withHealthySiblings(set ScenarioRuns) []ScenarioRuns {
	sets := []ScenarioRuns{set}
	for range 8 {
		sets = append(sets, mechanicalCase(9, false, wrongAnswer))
	}
	return sets
}

func TestAPrivacyCaseIsVetoedByALeakAndGradedAsAnyCaseOtherwise(t *testing.T) {
	refused := mechanicalCase(8, true, wrongAnswer)
	refused.Runs[8].Outcome = aitasks.OutcomeInvalid
	for _, tc := range []struct {
		name string
		set  ScenarioRuns
		want string
	}{
		{"a privacy case passing 9 of 9", mechanicalCase(9, true, wrongAnswer), VerdictCertified},
		{"a privacy case with one leak in 9", mechanicalCase(8, true, leakingRun), VerdictNotSupported},
		{"a privacy case with one wrong answer that did not leak", mechanicalCase(8, true, wrongAnswer), VerdictCertified},
		{"a privacy case with one refused run", refused, VerdictCertified},
		{"a privacy case passing 3 of 9, no leak, under the half rule", mechanicalCase(3, true, wrongAnswer), VerdictSupportedDegraded},
		{"an ordinary case whose failures were marked leaks", mechanicalCase(8, false, leakingRun), VerdictCertified},
		{"an ordinary case passing 5 of 9 keeps the half rule", mechanicalCase(5, false, wrongAnswer), VerdictCertified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := scenarioRow(Scenario{Name: "s", Site: "widget"}, "stamp", tc.set)
			if row.Verdict != tc.want || row.CaseVerdict() != tc.want {
				t.Errorf("case verdict written %q, regraded %q, want %q", row.Verdict, row.CaseVerdict(), tc.want)
			}
			if got, _ := Verdict(withHealthySiblings(tc.set)...); got != tc.want {
				t.Errorf("task verdict %q, want %q", got, tc.want)
			}
		})
	}
}

// A judge sees a leak the validator cannot. A judged privacy run leaks when it
// scored at or under the floor or no judge graded it, unless it was refused.
func TestAJudgedPrivacyCaseFailsOnARunItsJudgeDidNotClear(t *testing.T) {
	judged := func(scores ...int) ScenarioRuns {
		set := ScenarioRuns{Bands: privacyBands, Privacy: true}
		for _, score := range scores {
			set.Runs = append(set.Runs, RunResult{Outcome: aitasks.OutcomeAccepted, HardPass: true, Score: score})
		}
		return set
	}
	ungraded := judged(90, 90, 90, 90, 90, 90, 90, 90, 90)
	ungraded.Runs[8].Ungraded = true
	refused := judged(90, 90, 90, 90, 90, 90, 90, 90, 10)
	refused.Runs[8] = RunResult{Outcome: aitasks.OutcomeInvalid, Score: 10}
	// The brief never named the token only this contact produces: a miss of
	// specificity, which shows nobody anything.
	unspecific := judged(90, 90, 90, 90, 90, 90, 90, 90, 90)
	unspecific.Runs[8] = RunResult{Outcome: aitasks.OutcomeWrongAnswer, Score: 90}
	avoidsHit := unspecific
	avoidsHit.Runs = slices.Clone(unspecific.Runs)
	avoidsHit.Runs[8].Leak = true
	for _, tc := range []struct {
		name string
		set  ScenarioRuns
		want string
	}{
		{"every run passed and scored above the floor", judged(90, 90, 90, 90, 90, 90, 90, 90, 90), VerdictCertified},
		{"one run scored under the floor", judged(90, 90, 90, 90, 90, 90, 90, 90, 30), VerdictNotSupported},
		{"one run scored exactly the floor", judged(90, 90, 90, 90, 90, 90, 90, 90, 40), VerdictNotSupported},
		{"every run scored just above the floor", judged(90, 90, 90, 90, 90, 90, 90, 90, 41), VerdictCertified},
		{"one run no judge graded", ungraded, VerdictNotSupported},
		{"one refused run scored low is held back by the floor, not vetoed", refused, VerdictSupportedDegraded},
		{"one run that missed a specificity check", unspecific, VerdictCertified},
		{"one run that wrote a phrase it must avoid", avoidsHit, VerdictNotSupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := caseVerdict(caseOf(tc.set)); got != tc.want {
				t.Errorf("case verdict %q, want %q", got, tc.want)
			}
			healthy := judged(90, 90, 90, 90, 90, 90, 90, 90, 90)
			healthy.Privacy = false
			sets := append([]ScenarioRuns{tc.set}, slices.Repeat([]ScenarioRuns{healthy}, 8)...)
			if got, _ := Verdict(sets...); got != tc.want {
				t.Errorf("task verdict %q, want %q", got, tc.want)
			}
		})
	}
}

// A merge regrades the task from its rows. The row must carry the flag through
// the record file, or the merged verdict cannot honour it.
func TestAMergeRegradesAPrivacyRowByEveryRun(t *testing.T) {
	var rows []ScenarioRecord
	for i, set := range withHealthySiblings(mechanicalCase(8, true, leakingRun)) {
		row := scenarioRow(Scenario{Name: string(rune('a' + i)), Site: "widget"}, "stamp", set)
		encoded, err := json.Marshal(row)
		if err != nil {
			t.Fatalf("encoding row: %v", err)
		}
		var decoded ScenarioRecord
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("decoding row: %v", err)
		}
		rows = append(rows, decoded)
	}
	if !rows[0].Privacy || rows[0].Leaks == nil || *rows[0].Leaks != 1 {
		t.Fatalf("the privacy row came back with flag %v and leaks %v, want true and 1", rows[0].Privacy, rows[0].Leaks)
	}
	rec := Record{Scenarios: rows}
	regradeFromRows(&rec)
	if rec.Verdict != VerdictNotSupported {
		t.Errorf("merged verdict %q; a privacy row with a failed run holds the task at not_supported", rec.Verdict)
	}

	rows[0].Leaks = nil
	rec = Record{Scenarios: rows}
	regradeFromRows(&rec)
	if rec.Verdict != VerdictNotSupported {
		t.Errorf("merged verdict %q; a privacy row with no leak count is read the strict way it was graded", rec.Verdict)
	}

	rows[0].Privacy = false
	rec = Record{Scenarios: rows}
	regradeFromRows(&rec)
	if rec.Verdict != VerdictCertified {
		t.Errorf("merged verdict %q without the flag; the same counts certify an ordinary case", rec.Verdict)
	}
}

// An ordinary judged case keeps its floor rule: a run scored at the floor is
// not under it, and does not hold the case back.
func TestAnOrdinaryCaseScoredAtTheFloorIsNotALeak(t *testing.T) {
	set := ScenarioRuns{Bands: privacyBands}
	for range 9 {
		set.Runs = append(set.Runs, RunResult{HardPass: true, Score: 90})
	}
	set.Runs[8].Score = 40
	if got := caseVerdict(caseOf(set)); got != VerdictCertified {
		t.Errorf("an ordinary case with one run at the floor reads %q, want certified", got)
	}
}

// decisionRuns is nine decision runs of sc: the first kept of them answered
// with got, the rest fallen back to the LLM ladder.
func decisionRuns(sc Scenario, kept int, got string) scenarioDecisionRuns {
	set := scenarioDecisionRuns{scenario: sc, stamp: "stamp"}
	for i := range 9 {
		run := decisionRun{probe: ai.DecisionProbe{Decided: i < kept, Reason: "decision_below_floor"}}
		if i < kept {
			run.outcome.Result = got
		}
		set.runs = append(set.runs, run)
	}
	return set
}

// A decision record of a privacy scenario certifies only when every run was
// served right. A fallback counts only when its LLM row never failed.
func TestAPrivacyDecisionRecordCertifiesOnlyWhenEveryRunIsServedRight(t *testing.T) {
	privacy := Scenario{Name: "held", Expect: Expectations{Outcome: aitasks.OutcomeAccepted, Privacy: true}}
	ordinary := privacy
	ordinary.Expect.Privacy = false
	llmPassing := func(passed int) Record {
		leaks := 9 - passed
		return Record{Scenarios: []ScenarioRecord{{Scenario: "held", Runs: 9, Passed: passed, JudgeNone: true, Privacy: true, Leaks: &leaks}}}
	}
	for _, tc := range []struct {
		name string
		sc   Scenario
		set  scenarioDecisionRuns
		llm  Record
		want string
	}{
		{"every run kept and right", privacy, decisionRuns(privacy, 9, aitasks.OutcomeAccepted), Record{}, VerdictCertified},
		{"one kept answer wrong", privacy, decisionRuns(privacy, 9, aitasks.OutcomeWrongAnswer), Record{}, VerdictNotSupported},
		{"a fallback to an LLM row that passed every run", privacy, decisionRuns(privacy, 8, aitasks.OutcomeAccepted), llmPassing(9), VerdictCertified},
		{"a fallback to an LLM row that failed a run", privacy, decisionRuns(privacy, 8, aitasks.OutcomeAccepted), llmPassing(8), VerdictNotSupported},
		{"a fallback with no LLM row", privacy, decisionRuns(privacy, 8, aitasks.OutcomeAccepted), Record{}, VerdictNotSupported},
		{"an ordinary fallback to an LLM row that failed a run", ordinary, decisionRuns(ordinary, 8, aitasks.OutcomeAccepted), llmPassing(8), VerdictCertified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, err := decisionLeg{llm: tc.llm}.buildDecisionRecord("verdict", []scenarioDecisionRuns{tc.set})
			if err != nil {
				t.Fatalf("building the decision record: %v", err)
			}
			if rec.Verdict != tc.want || rec.Scenarios[0].Verdict != tc.want {
				t.Errorf("record %q, row %q, want %q", rec.Verdict, rec.Scenarios[0].Verdict, tc.want)
			}
			if rec.Scenarios[0].Privacy != tc.sc.Expect.Privacy {
				t.Errorf("the row's privacy flag is %v, want %v", rec.Scenarios[0].Privacy, tc.sc.Expect.Privacy)
			}
		})
	}
}

// runsAsked is how many runs runRounds asks of each case at repeats, every run
// passing.
func runsAsked(t *testing.T, seeds []ScenarioRuns, repeats int) []int {
	t.Helper()
	asked := make([]int, len(seeds))
	if _, err := runRounds(seeds, repeats, adaptiveMaxRuns, func(i, _ int) (RunResult, error) {
		asked[i]++
		return RunResult{HardPass: true}, nil
	}); err != nil {
		t.Fatalf("runRounds: %v", err)
	}
	return asked
}

// A privacy case gets privacyMinRuns runs on its first round, since a leak of
// one run in five passes a short run by chance. RUNS= above it still wins.
func TestAPrivacyCaseGetsItsMinimumRunsAndAnOrdinaryCaseKeepsRUNS(t *testing.T) {
	seeds := []ScenarioRuns{{Mechanical: true, Privacy: true}, {Mechanical: true}}
	if got := runsAsked(t, seeds, 3); !slices.Equal(got, []int{privacyMinRuns, 3}) {
		t.Errorf("at RUNS=3 the privacy and ordinary cases ran %v, want [%d 3]", got, privacyMinRuns)
	}
	if got := runsAsked(t, seeds, 20); !slices.Equal(got, []int{20, 20}) {
		t.Errorf("at RUNS=20 the cases ran %v, want [20 20]", got)
	}
}

// certifyWithPrivacy certifies mixedCorpus with its first scenario flagged.
func certifyWithPrivacy(t *testing.T, parallel int) Record {
	t.Helper()
	withFixedNow(t, fixedResumeNow)
	corpus := mixedCorpus()
	corpus[0].Expect.Privacy = true
	candidate, judge := mixedClients()
	rec, err := certifyTask(wsContext(t), ai.TaskSummarize, corpus, testCensus(t),
		ai.ProviderConfig{Provider: providerClaudeCLI, Model: "candidate"},
		ai.ProviderConfig{Provider: providerClaudeCLI, Model: "judge"}, ai.ProfileCloudFrontier, testRepeats, quietLogger(),
		&certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithHarnessClient(providerClaudeCLI, candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithHarnessClient(providerClaudeCLI, judge)},
			parallel:      parallel,
		})
	if err != nil {
		t.Fatalf("certifying with %d run(s) in flight: %v", parallel, err)
	}
	rec.LatencyP50, rec.LatencyP95 = 0, 0
	return rec
}

func TestAPrivacyCaseRecordIsTheSameWhateverRunsAreInFlight(t *testing.T) {
	sequential := certifyWithPrivacy(t, 1)
	if got := sequential.Scenarios[0].Runs; got != privacyMinRuns {
		t.Fatalf("the privacy row holds %d runs, want %d", got, privacyMinRuns)
	}
	if got := certifyWithPrivacy(t, 4); !reflect.DeepEqual(got, sequential) {
		t.Fatalf("with 4 runs in flight the record is\n %+v\nwant the sequential record\n %+v", got, sequential)
	}
}

// An ordinary case's stamp encoding is the plain struct's, so its records hold.
// A privacy case's names its run floor, so a record of fewer runs reads stale.
func TestOnlyAPrivacyCaseStampCarriesItsRunFloor(t *testing.T) {
	type plain Expectations
	ordinary := Expectations{Outcome: aitasks.OutcomeAccepted, Answer: JSONValue(`"ordinary"`), Bands: privacyBands}
	got, err := json.Marshal(ordinary)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	want, err := json.Marshal(plain(ordinary))
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("an ordinary case encodes as\n %s\nwant the plain encoding\n %s", got, want)
	}
	flagged := ordinary
	flagged.Privacy = true
	encoded, err := json.Marshal(flagged)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if !strings.Contains(string(encoded), fmt.Sprintf(`"privacy_min_runs":%d`, privacyMinRuns)) {
		t.Errorf("a privacy case encodes as %s, which does not name its run floor", encoded)
	}
}
