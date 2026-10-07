// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

func TestACommitmentBandNeedsAnAcceptedReplyThatStatesACommitment(t *testing.T) {
	accepted := Expectations{Outcome: aitasks.OutcomeAccepted, Answer: JSONValue(`[{"line":2}]`)}
	cases := map[string]struct {
		band    string
		expect  Expectations
		wantErr bool
	}{
		"no band":               {band: "", expect: Expectations{Outcome: aitasks.OutcomeAbstained}},
		"firm and accepted":     {band: CommitmentBandFirm, expect: accepted},
		"hedged and accepted":   {band: CommitmentBandHedged, expect: accepted},
		"a band nobody reads":   {band: "lukewarm", expect: accepted, wantErr: true},
		"an abstention":         {band: CommitmentBandFirm, expect: Expectations{Outcome: aitasks.OutcomeAbstained}, wantErr: true},
		"an empty answer":       {band: CommitmentBandHedged, expect: Expectations{Outcome: aitasks.OutcomeAccepted, Answer: JSONValue(`[]`)}, wantErr: true},
		"a wrong-answer expect": {band: CommitmentBandFirm, expect: Expectations{Outcome: aitasks.OutcomeWrongAnswer}, wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := validateCommitmentBand(Scenario{CommitmentBand: tc.band, Expect: tc.expect}, "x.yaml")
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateCommitmentBand error = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

// confidentCase reports the confidence a reply carries as its whole output.
type confidentCase struct{ widgetCase }

func (confidentCase) AnswerConfidence(trace aitasks.Trace) (low, high float64, ok bool) {
	var span ConfidenceRange
	if err := json.Unmarshal([]byte(trace.Output), &span); err != nil {
		return 0, 0, false
	}
	return span.Min, span.Max, true
}

func TestOnlyAPassedRunContributesItsConfidence(t *testing.T) {
	trace := aitasks.Trace{Output: `{"min":0.6,"max":0.9}`}

	if got := keptConfidence(confidentCase{}, trace, false); got != nil {
		t.Fatalf("a run that did not pass contributed %+v", got)
	}
	if got := keptConfidence(widgetCase{}, trace, true); got != nil {
		t.Fatalf("a case that reports no confidence contributed %+v", got)
	}
	if got := keptConfidence(confidentCase{}, aitasks.Trace{Output: "garbled"}, true); got != nil {
		t.Fatalf("an unreadable reply contributed %+v", got)
	}
	got := keptConfidence(confidentCase{}, trace, true)
	if got == nil || got.Min != 0.6 || got.Max != 0.9 {
		t.Fatalf("a passed run contributed %+v, want 0.6-0.9", got)
	}
}

func TestAScenarioRowSpansTheConfidenceOfItsKeptRuns(t *testing.T) {
	sc := Scenario{Name: "s", Site: "widget", CommitmentBand: CommitmentBandFirm}
	runs := []RunResult{
		{HardPass: true, AnswerConfidence: &ConfidenceRange{Min: 0.9, Max: 0.95}},
		{HardPass: true, AnswerConfidence: &ConfidenceRange{Min: 0.8, Max: 1}},
		{HardPass: true},
	}

	row := scenarioRow(sc, "stamp", ScenarioRuns{Runs: runs, Mechanical: true})

	if row.CommitmentBand != CommitmentBandFirm || row.AnswerConfidenceMin == nil || row.AnswerConfidenceMax == nil ||
		*row.AnswerConfidenceMin != 0.8 || *row.AnswerConfidenceMax != 1 {
		t.Fatalf("row = band %q, min %v, max %v, want firm 0.8-1", row.CommitmentBand, row.AnswerConfidenceMin, row.AnswerConfidenceMax)
	}
}
