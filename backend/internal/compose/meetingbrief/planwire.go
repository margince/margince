// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The plan, rendered onto the wire and grounded on the way.
//
// Every sentence goes through the same filter the sections use: a claim citing
// a record this caller cannot open, or citing nothing, is dropped whole. What
// differs is what a drop MEANS for each field. A likely ask that fails simply
// vanishes from its list. An objective that fails leaves the plan without one,
// which is honest. The advance cannot vanish — the contract requires all three
// legs — so a leg that fails falls back to the meeting-cited floor rather than
// leaving a reader with two ways to close and no third.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// wirePlan renders the plan, dropping what is not grounded.
func wirePlan(plan Plan, in Input) crmcontracts.MeetingPlan {
	out := crmcontracts.MeetingPlan{
		GeneratedBy: crmcontracts.Deterministic,
		MeetingType: crmcontracts.MeetingPlanType{
			Value:      plan.Type.Value,
			Confidence: plan.Type.Confidence,
		},
		LikelyAsks: wireAsks(plan.LikelyAsks),
		Questions:  wireQuestions(plan.Questions),
		Scenarios:  wireScenarios(plan.Scenarios),
		AccountArc: wireArc(plan.Arc),
		Advance:    wireAdvance(plan.Advance, in),
		Unknowns:   wireUnknowns(plan.Unknowns),
	}
	if plan.Objective != nil {
		if sentence, ok := wireSentence(plan.Objective.Sentence); ok {
			out.Objective = &crmcontracts.MeetingPlanObjective{
				Sentence: sentence,
				Caveat:   plan.Objective.Caveat,
			}
		}
	}
	if plan.Opening != nil {
		if sentence, ok := wireSentence(*plan.Opening); ok {
			out.Opening = &sentence
		}
	}
	if plan.TopRisk != nil {
		if sentence, ok := wireSentence(plan.TopRisk.Text); ok {
			out.TopRisk = &crmcontracts.MeetingPlanRisk{
				Text: sentence,
				ResponsePlan: crmcontracts.MeetingPlanResponse{
					Say:   plan.TopRisk.Response.Say,
					Show:  plan.TopRisk.Response.Show,
					Avoid: plan.TopRisk.Response.Avoid,
				},
			}
		}
	}
	// Readiness is decided on what SURVIVED grounding, not on what was built:
	// a plan whose risk was dropped is an outline however complete it looked
	// a moment ago, and a client leading with it would be leading with less
	// than the sections it displaced.
	out.Readiness = wireReadiness(out)
	return out
}

func wireReadiness(out crmcontracts.MeetingPlan) crmcontracts.MeetingPlanReadiness {
	if out.TopRisk != nil && len(out.LikelyAsks) >= 2 && len(out.Questions) >= 3 {
		return crmcontracts.MeetingPlanReadinessPrepared
	}
	return crmcontracts.MeetingPlanReadinessOutline
}

// wireSentence is wireSentences for exactly one, which is the shape most of
// the plan's fields have.
func wireSentence(sentence Sentence) (crmcontracts.OrganizationBriefSentence, bool) {
	wired := wireSentences([]Sentence{sentence})
	if len(wired) == 0 {
		return crmcontracts.OrganizationBriefSentence{}, false
	}
	return wired[0], true
}

func wireAsks(asks []Ask) []crmcontracts.MeetingPlanAsk {
	out := make([]crmcontracts.MeetingPlanAsk, 0, len(asks))
	for _, ask := range asks {
		basis, ok := wireSentence(ask.Basis)
		if !ok {
			continue
		}
		out = append(out, crmcontracts.MeetingPlanAsk{
			Question:  ask.Question,
			Basis:     basis,
			Relevance: ask.Relevance,
			Prepare:   ask.Prepare,
		})
	}
	return out
}

func wireQuestions(questions []Question) []crmcontracts.MeetingPlanQuestion {
	out := make([]crmcontracts.MeetingPlanQuestion, 0, len(questions))
	for _, question := range questions {
		evidence, ok := wireEvidence(question.Evidence)
		if !ok {
			continue
		}
		out = append(out, crmcontracts.MeetingPlanQuestion{
			Ask:       question.Ask,
			Why:       question.Why,
			ListenFor: question.ListenFor,
			Evidence:  evidence,
		})
	}
	return out
}

func wireScenarios(scenarios []Scenario) []crmcontracts.MeetingPlanScenario {
	out := make([]crmcontracts.MeetingPlanScenario, 0, len(scenarios))
	for _, scenario := range scenarios {
		evidence, ok := wireEvidence(scenario.Evidence)
		if !ok {
			continue
		}
		out = append(out, crmcontracts.MeetingPlanScenario{
			Label: scenario.Label, Play: scenario.Play, Evidence: evidence,
		})
	}
	return out
}

func wireArc(arc []ArcSentence) []crmcontracts.MeetingPlanArcMoment {
	out := make([]crmcontracts.MeetingPlanArcMoment, 0, len(arc))
	for _, moment := range arc {
		summary, ok := wireSentence(moment.Summary)
		if !ok {
			continue
		}
		out = append(out, crmcontracts.MeetingPlanArcMoment{
			From:    moment.Moment.From,
			To:      moment.Moment.To,
			Title:   moment.Moment.Title,
			Summary: summary,
		})
	}
	return out
}

// wireAdvance renders the three legs, replacing any that failed grounding.
//
// The contract requires all three, so a dropped leg cannot simply vanish. The
// replacement cites the meeting — a move whose only support is that this
// meeting is happening, which is true of every advance and is why the floor
// legs cite it in the first place.
func wireAdvance(advance Advance, in Input) crmcontracts.MeetingPlanAdvance {
	return crmcontracts.MeetingPlanAdvance{
		Minimum:  advanceLeg(advance.Minimum, in, "Leave with one named next step, owned and dated."),
		Best:     advanceLeg(advance.Best, in, "Leave with the next meeting booked and its purpose agreed."),
		Fallback: advanceLeg(advance.Fallback, in, "Leave with what you will send and who decides on it."),
	}
}

func advanceLeg(
	leg Sentence, in Input, floor string,
) crmcontracts.OrganizationBriefSentence {
	if wired, ok := wireSentence(leg); ok {
		return wired
	}
	wired, ok := wireSentence(Sentence{
		Text:     floor,
		Nature:   natureRecommendation,
		Evidence: []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}},
	})
	if !ok {
		// The meeting's own id did not parse, which means this brief should not
		// have been assembled at all. An empty leg is the honest rendering of
		// "there is nothing here to stand on".
		return crmcontracts.OrganizationBriefSentence{Text: floor}
	}
	return wired
}

func wireUnknowns(unknowns []Unknown) []crmcontracts.MeetingPlanUnknown {
	out := make([]crmcontracts.MeetingPlanUnknown, 0, len(unknowns))
	for _, unknown := range unknowns {
		out = append(out, crmcontracts.MeetingPlanUnknown{
			Kind: unknown.Kind, Question: unknown.Question,
		})
	}
	return out
}
