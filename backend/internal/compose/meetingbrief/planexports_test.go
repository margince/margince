// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// planSentences and PlanProse both claim to reach the whole plan, and the
// certification lane's measurement rests on that: PlanCites asks whether the
// plan read the right conversation, PlanProse asks whether it named anything
// only this account produced. A field either of them does not walk is a field
// the cert run cannot see, so a model could write a plan about the wrong
// account and be graded on the fields that happened to be right.
//
// This plants a distinct string in every prose-bearing field and a distinct
// citation in every cited one, then asks for all of them back.
func TestTheCertExportsReachTheWholePlan(t *testing.T) {
	cite := func(id string) []Evidence {
		return []Evidence{{EntityType: citeActivity, EntityID: id}}
	}
	line := func(text, id string) Sentence {
		return Sentence{Text: text, Evidence: cite(id)}
	}
	plan := Plan{
		Objective: &Objective{Sentence: line("objective-text", "id-objective")},
		Opening:   ptr(line("opening-text", "id-opening")),
		TopRisk: &Risk{
			Text: line("risk-text", "id-risk"),
			Response: Response{
				Say: "say-text", Show: "show-text", Avoid: "avoid-text",
			},
		},
		LikelyAsks: []Ask{{
			Question: "ask-question", Basis: line("ask-basis", "id-ask"),
			Relevance: crmcontracts.MeetingPlanTierHigh, Prepare: "ask-prepare",
		}},
		Questions: []Question{{
			Ask: "question-ask", Why: "question-why", ListenFor: "question-listen",
			Evidence: cite("id-question"),
		}},
		Scenarios: []Scenario{{
			Label: "scenario-label", Play: "scenario-play", Evidence: cite("id-scenario"),
		}},
		Arc: []ArcSentence{{Summary: line("arc-summary", "id-arc")}},
		Advance: Advance{
			Minimum:  line("minimum-text", "id-minimum"),
			Best:     line("best-text", "id-best"),
			Fallback: line("fallback-text", "id-fallback"),
		},
		Unknowns: []Unknown{{
			Kind:     crmcontracts.MeetingPlanUnknownNoOpenDeal,
			Question: "unknown-question",
		}},
	}

	prose := PlanProse(plan)
	for _, planted := range []string{
		"objective-text", "opening-text", "risk-text", "say-text", "show-text",
		"avoid-text", "ask-question", "ask-basis", "ask-prepare", "question-ask",
		"question-why", "question-listen", "scenario-label", "scenario-play",
		"arc-summary", "minimum-text", "best-text", "fallback-text",
		"unknown-question",
	} {
		if !strings.Contains(prose, planted) {
			t.Errorf("PlanProse does not reach %q; a cert run cannot see that field", planted)
		}
	}

	for _, planted := range []string{
		"id-objective", "id-opening", "id-risk", "id-ask", "id-question",
		"id-scenario", "id-arc", "id-minimum", "id-best", "id-fallback",
	} {
		if !PlanCites(plan, planted) {
			t.Errorf("PlanCites does not reach %q; a plan could cite the right record there and be graded as though it had not", planted)
		}
	}
}
