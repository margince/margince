// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const salesAssessmentFixtureJSON = `{
  "frame": {"scope": "Whole company", "period": "this_quarter", "window": "2026-10-01 to 2026-12-31", "currency": "EUR", "today": "2026-11-03"},
  "cells": [
    {"label": "bookings_won", "value": "216000 EUR", "unit": "EUR", "window": "2026-10-01 to 2026-11-03", "coverage": "ok"},
    {"label": "open_pipeline", "value": "370000 EUR", "unit": "EUR", "window": "2026-10-01 to 2026-11-03", "coverage": "ok"}
  ],
  "candidate_deals": [
    {"label": "linden", "name": "Linden press", "amount": "250000 EUR"},
    {"label": "tern", "name": "Tern portal", "amount": "120000 EUR"}
  ],
  "decisions": []
}`

// minted names the ids one preparation minted, so a reply is written
// against the prepared input rather than typed by hand.
type minted struct{ c *salesAssessmentCase }

func (m minted) cell(label string) string { return m.c.labels[label] }
func (m minted) deal(label string) string { return m.c.labels[label] }
func (m minted) owner(label string) string {
	for _, candidate := range m.c.in.Candidates {
		if candidate.ID == m.c.labels[label] {
			return candidate.OwnerID
		}
	}
	return ""
}

func runSalesAssessment(t *testing.T, expected string, build func(ids minted) string) aitasks.Outcome {
	t.Helper()
	prepared, err := salesAssessmentCases{}.Prepare(json.RawMessage(salesAssessmentFixtureJSON), json.RawMessage(expected))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	c, ok := prepared.(*salesAssessmentCase)
	if !ok {
		t.Fatalf("prepared a %T, want the sales assessment case", prepared)
	}
	trace, err := prepared.Run(context.Background(), &replyBrainStub{response: model.Response{Text: build(minted{c})}})
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	return prepared.Evaluate(trace)
}

func TestTheSalesAssessmentCaseKeepsOnlyGroundedSentencesAndCandidateDecisions(t *testing.T) {
	expected := `{"cites":["bookings_won","open_pipeline"],"next_decision":"linden"}`
	for _, tc := range []struct {
		name  string
		build func(ids minted) string
		want  string
	}{
		{
			name: "a cited reading with a decision on a candidate deal",
			build: func(ids minted) string {
				return `{"sentences":[{"text":"Sales won stand at 216,000 EUR against an open pipeline of 370,000 EUR.","kind":"observation","cells":["` + ids.cell("bookings_won") + `","` + ids.cell("open_pipeline") + `"]}],"next_decision":{"deal_id":"` + ids.deal("linden") + `","text":"Ask the buyer for the budget owner this week."},"confidence":"medium"}`
			},
			want: aitasks.OutcomeAccepted,
		},
		{
			name: "a number no cited cell carries drops its sentence",
			build: func(ids minted) string {
				return `{"sentences":[{"text":"Sales won stand at 216,000 EUR.","kind":"observation","cells":["` + ids.cell("bookings_won") + `"]},{"text":"The pipeline grew by 48 percent.","kind":"hypothesis","cells":["` + ids.cell("open_pipeline") + `"]}],"next_decision":{"deal_id":"` + ids.deal("linden") + `","text":"Ask for the budget owner."},"confidence":"high"}`
			},
			want: aitasks.OutcomeWrongAnswer,
		},
		{
			name: "a decision on a deal outside the candidates is dropped",
			build: func(ids minted) string {
				return `{"sentences":[{"text":"Sales won stand at 216,000 EUR against an open pipeline of 370,000 EUR.","kind":"observation","cells":["` + ids.cell("bookings_won") + `","` + ids.cell("open_pipeline") + `"]}],"next_decision":{"deal_id":"01a19fad-0000-7000-8000-000000000000","text":"Call them."},"confidence":"low"}`
			},
			want: aitasks.OutcomeWrongAnswer,
		},
		{
			name: "a reply that is not the shape asked for is invalid",
			build: func(ids minted) string {
				return `Sales look fine.`
			},
			want: aitasks.OutcomeInvalid,
		},
		{
			name: "a reply citing nothing is invalid",
			build: func(ids minted) string {
				return `{"sentences":[{"text":"Sales look fine.","kind":"observation","cells":[]}],"next_decision":null,"confidence":"low"}`
			},
			want: aitasks.OutcomeInvalid,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome := runSalesAssessment(t, expected, tc.build)
			if outcome.Result != tc.want {
				t.Fatalf("outcome %s (%s), want %s", outcome.Result, outcome.Detail, tc.want)
			}
		})
	}
}

func TestTheSalesAssessmentCaseRefusesAnExpectationTheFixtureCannotCarry(t *testing.T) {
	_, err := salesAssessmentCases{}.Prepare(json.RawMessage(salesAssessmentFixtureJSON), json.RawMessage(`{"cites":["win_rate"],"next_decision":"none"}`))
	if err == nil || !strings.Contains(err.Error(), "win_rate") {
		t.Fatalf("an expected citation outside the fixture was prepared: %v", err)
	}
	_, err = salesAssessmentCases{}.Prepare(json.RawMessage(salesAssessmentFixtureJSON), json.RawMessage(`{"cites":[],"next_decision":"globex"}`))
	if err == nil || !strings.Contains(err.Error(), "globex") {
		t.Fatalf("an expected decision outside the candidates was prepared: %v", err)
	}
}

func TestTheSalesAssessmentDecisionKeepsTheCandidatesOwner(t *testing.T) {
	expected := `{"cites":["bookings_won"],"next_decision":"tern"}`
	outcome := runSalesAssessment(t, expected, func(ids minted) string {
		return `{"sentences":[{"text":"Sales won stand at 216,000 EUR.","kind":"observation","cells":["` + ids.cell("bookings_won") + `"]}],"next_decision":{"deal_id":"` + ids.deal("tern") + `","owner_id":"` + ids.owner("tern") + `","text":"Qualify the portal deal with its sponsor."},"confidence":"medium"}`
	})
	if outcome.Result != aitasks.OutcomeAccepted {
		t.Fatalf("outcome %s (%s)", outcome.Result, outcome.Detail)
	}
}
