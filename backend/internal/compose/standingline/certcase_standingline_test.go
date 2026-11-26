// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package standingline

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// lineStub answers every request with one fixed reply.
type lineStub struct{ reply string }

func (s lineStub) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: s.reply}, nil
}

func lineScenario(t *testing.T, want standingLineExpectation) aitasks.PreparedCase {
	t.Helper()
	fixture, err := json.Marshal(standingLineFixture{
		RecordKind: KindContact, Previous: "Dana asked for a revised offer.",
		Activities: []standingLineActivity{
			{Kind: "email", DaysAgo: 1, Author: "Dana Buyer", Side: sideCustomer, Subject: "Re: Offer", Text: "The revised offer works. Please send the contract draft by Friday."},
			{Kind: "email", DaysAgo: 3, Author: "Lena Hoffmann", Side: sideOurs, Subject: "Offer", Text: "Attached is the revised offer."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := CertCases{}.Prepare(fixture, expected)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

// The case tells a line that says what was said from one that only says who
// wrote, from a refused reply and from a line that was kept.
func TestTheStandingLineCaseGradesWhatTheLineSays(t *testing.T) {
	said := standingLineExpectation{NamesToken: "contract draft", Avoids: []string{"both sides"}}
	cases := []struct {
		name  string
		want  standingLineExpectation
		reply string
		grade string
	}{
		{"says it", said, `{"keep_previous":false,"sentences":[{"text":"Dana accepted the revised offer and asked for the contract draft by Friday.","evidence":[{"entity_type":"activity","entity_id":"a1"}]}]}`, aitasks.OutcomeAccepted},
		{"misses it", said, `{"keep_previous":false,"sentences":[{"text":"Dana replied to the revised offer on Monday.","evidence":[{"entity_type":"activity","entity_id":"a1"}]}]}`, aitasks.OutcomeWrongAnswer},
		{"cites nothing shown", said, `{"keep_previous":false,"sentences":[{"text":"Dana signed the contract draft.","evidence":[{"entity_type":"activity","entity_id":"a9"}]}]}`, aitasks.OutcomeAbstained},
		{"not json", said, "I cannot help.", aitasks.OutcomeInvalid},
		{"keeps when it should", standingLineExpectation{Keep: true}, `{"keep_previous":true,"sentences":[]}`, aitasks.OutcomeAccepted},
		{"rewrites when it should keep", standingLineExpectation{Keep: true}, `{"keep_previous":false,"sentences":[{"text":"Dana asked for the contract draft by Friday.","evidence":[{"entity_type":"activity","entity_id":"a1"}]}]}`, aitasks.OutcomeWrongAnswer},
	}
	for _, tc := range cases {
		prepared := lineScenario(t, tc.want)
		trace, err := prepared.Run(context.Background(), lineStub{reply: tc.reply})
		if err != nil {
			t.Fatal(err)
		}
		if got := prepared.Evaluate(trace); got.Result != tc.grade {
			t.Errorf("%s: graded %s (%s), want %s", tc.name, got.Result, got.Detail, tc.grade)
		}
	}
}

func TestAScenarioThatMeasuresNothingIsRefused(t *testing.T) {
	if _, err := (CertCases{}).Prepare(json.RawMessage(`{"activities":[]}`), json.RawMessage(`{}`)); err == nil {
		t.Error("prepared a scenario with neither a kept line nor a token to name")
	}
	if _, err := (CertCases{}).Prepare(json.RawMessage(`[]`), json.RawMessage(`{}`)); err == nil {
		t.Error("prepared a fixture of the wrong shape")
	}
	if CertCases.Site(CertCases{}).Variant != "standing_line" {
		t.Error("the case certifies another site")
	}
}
