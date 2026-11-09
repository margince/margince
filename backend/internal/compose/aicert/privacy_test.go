// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// A privacy case's wrong answer shows a reader what they must not see. It
// certifies only when every run passes, at its row and at its task alike.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
)

var privacyBands = Bands{CertifiedMin: 70, DegradedMin: 50, Floor: 40}

// mechanicalCase is one judge-less case of nine runs, the first passes passing.
func mechanicalCase(passes int, privacy bool) ScenarioRuns {
	set := ScenarioRuns{Mechanical: true, Privacy: privacy, Runs: make([]RunResult, 9)}
	for i := range passes {
		set.Runs[i].HardPass = true
	}
	return set
}

// withHealthySiblings is set beside eight cases passing every run, a pool
// that certifies on its own.
func withHealthySiblings(set ScenarioRuns) []ScenarioRuns {
	sets := []ScenarioRuns{set}
	for range 8 {
		sets = append(sets, mechanicalCase(9, false))
	}
	return sets
}

func TestAPrivacyCaseCertifiesOnlyWhenEveryRunPasses(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  ScenarioRuns
		want string
	}{
		{"a privacy case passing 9 of 9", mechanicalCase(9, true), VerdictCertified},
		{"a privacy case passing 8 of 9", mechanicalCase(8, true), VerdictNotSupported},
		{"a privacy case passing 5 of 9", mechanicalCase(5, true), VerdictNotSupported},
		{"an ordinary case passing 8 of 9", mechanicalCase(8, false), VerdictCertified},
		{"an ordinary case passing 5 of 9 keeps the half rule", mechanicalCase(5, false), VerdictCertified},
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

// A judge sees a leak the validator cannot. A judged privacy run fails when it
// scored at or under the floor or no judge graded it, though it HardPassed.
func TestAJudgedPrivacyCaseFailsOnARunItsJudgeDidNotClear(t *testing.T) {
	judged := func(scores ...int) ScenarioRuns {
		set := ScenarioRuns{Bands: privacyBands, Privacy: true}
		for _, score := range scores {
			set.Runs = append(set.Runs, RunResult{HardPass: true, Score: score})
		}
		return set
	}
	ungraded := judged(90, 90, 90, 90, 90, 90, 90, 90, 90)
	ungraded.Runs[8].Ungraded = true
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := caseVerdict(caseOf(tc.set)); got != tc.want {
				t.Errorf("case verdict %q, want %q", got, tc.want)
			}
			if got, _ := Verdict(tc.set); got != tc.want {
				t.Errorf("task verdict %q, want %q", got, tc.want)
			}
		})
	}
}

// A merge regrades the task from its rows. The row must carry the flag through
// the record file, or the merged verdict cannot honour it.
func TestAMergeRegradesAPrivacyRowByEveryRun(t *testing.T) {
	var rows []ScenarioRecord
	for i, set := range withHealthySiblings(mechanicalCase(8, true)) {
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
	if !rows[0].Privacy {
		t.Fatal("the privacy row lost its flag through the record file")
	}
	rec := Record{Scenarios: rows}
	regradeFromRows(&rec)
	if rec.Verdict != VerdictNotSupported {
		t.Errorf("merged verdict %q; a privacy row with a failed run holds the task at not_supported", rec.Verdict)
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

// decisionRuns is n decision runs of sc: the first kept of them answered with
// got, the rest fallen back to the LLM ladder.
func decisionRuns(sc Scenario, kept, n int, got string) scenarioDecisionRuns {
	set := scenarioDecisionRuns{scenario: sc, stamp: "stamp"}
	for i := range n {
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
		return Record{Scenarios: []ScenarioRecord{{Scenario: "held", Runs: 9, Passed: passed}}}
	}
	for _, tc := range []struct {
		name string
		sc   Scenario
		set  scenarioDecisionRuns
		llm  Record
		want string
	}{
		{"every run kept and right", privacy, decisionRuns(privacy, 9, 9, aitasks.OutcomeAccepted), Record{}, VerdictCertified},
		{"one kept answer wrong", privacy, decisionRuns(privacy, 9, 9, aitasks.OutcomeWrongAnswer), Record{}, VerdictNotSupported},
		{"a fallback to an LLM row that passed every run", privacy, decisionRuns(privacy, 8, 9, aitasks.OutcomeAccepted), llmPassing(9), VerdictCertified},
		{"a fallback to an LLM row that failed a run", privacy, decisionRuns(privacy, 8, 9, aitasks.OutcomeAccepted), llmPassing(8), VerdictNotSupported},
		{"a fallback with no LLM row", privacy, decisionRuns(privacy, 8, 9, aitasks.OutcomeAccepted), Record{}, VerdictNotSupported},
		{"an ordinary fallback to an LLM row that failed a run", ordinary, decisionRuns(ordinary, 8, 9, aitasks.OutcomeAccepted), llmPassing(8), VerdictCertified},
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
