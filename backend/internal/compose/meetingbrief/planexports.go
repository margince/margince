// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// What the certification lane needs to run this site the way production runs
// it.
//
// Each of these is one line over an unexported function. They exist because the
// alternative is a cert case that rebuilds the floor, the projection or the
// reading of a plan — and a case measuring its own copy stays green through
// exactly the change that breaks the original, which is the failure the
// certification lane exists to catch.

import "strings"

// DeterministicPlanFor is the floor for one assembled meeting.
func DeterministicPlanFor(in Input) Plan {
	return DeterministicPlan(in, rankClaims(in))
}

// PlanPromptFor is what the model is shown for one meeting and its floor.
func PlanPromptFor(in Input, floor Plan) PlanPrompt {
	return planPromptOf(in, floor)
}

// PlanCites reports whether any claim in the plan rests on one record.
//
// This is the question a deterministic filter cannot answer. Grounding checks
// that a citation names a record the caller can OPEN; it never checks that the
// record is the one the sentence is about, so a confident claim hung on an
// unrelated-but-known conversation passes every gate in the tree. Whether the
// plan reached the conversation that matters is measured, not asserted.
func PlanCites(plan Plan, activityID string) bool {
	for _, sentence := range planSentences(plan) {
		for _, cited := range sentence.Evidence {
			if cited.EntityID == activityID {
				return true
			}
		}
	}
	for _, question := range plan.Questions {
		for _, cited := range question.Evidence {
			if cited.EntityID == activityID {
				return true
			}
		}
	}
	for _, scenario := range plan.Scenarios {
		for _, cited := range scenario.Evidence {
			if cited.EntityID == activityID {
				return true
			}
		}
	}
	return false
}

// PlanProse is every word of the plan a reader sees, for asking whether it
// names anything only this account would have produced.
//
// Held by: TestTheCertExportsReachTheWholePlan
// (backend/internal/compose/meetingbrief/planexports_test.go)
func PlanProse(plan Plan) string {
	var prose strings.Builder
	write := func(parts ...string) {
		for _, part := range parts {
			prose.WriteString(part)
			prose.WriteString("\n")
		}
	}
	for _, sentence := range planSentences(plan) {
		write(sentence.Text)
	}
	if plan.TopRisk != nil {
		write(plan.TopRisk.Response.Say, plan.TopRisk.Response.Show, plan.TopRisk.Response.Avoid)
	}
	for _, ask := range plan.LikelyAsks {
		write(ask.Question, ask.Prepare)
	}
	for _, question := range plan.Questions {
		write(question.Ask, question.Why, question.ListenFor)
	}
	for _, scenario := range plan.Scenarios {
		write(scenario.Label, scenario.Play)
	}
	for _, unknown := range plan.Unknowns {
		write(unknown.Question)
	}
	return prose.String()
}

// planSentences is every cited claim in the plan, in one walk.
//
// Held by: TestTheCertExportsReachTheWholePlan
// (backend/internal/compose/meetingbrief/planexports_test.go)
func planSentences(plan Plan) []Sentence {
	var out []Sentence
	if plan.Objective != nil {
		out = append(out, plan.Objective.Sentence)
	}
	if plan.Opening != nil {
		out = append(out, *plan.Opening)
	}
	if plan.TopRisk != nil {
		out = append(out, plan.TopRisk.Text)
	}
	for _, ask := range plan.LikelyAsks {
		out = append(out, ask.Basis)
	}
	for _, moment := range plan.Arc {
		out = append(out, moment.Summary)
	}
	out = append(out, plan.Advance.Minimum, plan.Advance.Best, plan.Advance.Fallback)
	return out
}
