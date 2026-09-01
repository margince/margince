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
	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// wirePlan renders the plan, dropping what is not grounded.
//
// `known` is the same allowlist the sections are filtered against: the records
// this input actually carried. Parsing a citation as a UUID is not grounding —
// it proves the string is an id, not that it names a record this caller can
// open — and every field below is checked against the set rather than against
// the syntax.
func wirePlan(plan Plan, in Input) crmcontracts.MeetingPlan {
	known := knownRecords(in)
	out := crmcontracts.MeetingPlan{
		GeneratedBy: crmcontracts.Deterministic,
		MeetingType: crmcontracts.MeetingPlanType{
			Value:      plan.Type.Value,
			Confidence: plan.Type.Confidence,
		},
		LikelyAsks: wireAsks(plan.LikelyAsks, known),
		Questions:  wireQuestions(plan.Questions, known),
		Scenarios:  wireScenarios(plan.Scenarios, known),
		AccountArc: wireArc(plan.Arc, known),
		Advance:    wireAdvance(plan.Advance, in, known),
		Unknowns:   wireUnknowns(plan.Unknowns),
	}
	if plan.Objective != nil {
		if sentence, ok := groundedSentence(plan.Objective.Sentence, known); ok {
			out.Objective = &crmcontracts.MeetingPlanObjective{
				Sentence: sentence,
				Caveat:   plan.Objective.Caveat,
			}
		}
	}
	if plan.Opening != nil {
		if sentence, ok := groundedSentence(*plan.Opening, known); ok {
			out.Opening = &sentence
		}
	}
	if plan.TopRisk != nil {
		if sentence, ok := groundedSentence(plan.TopRisk.Text, known); ok {
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

// groundedSentence renders one sentence, refusing it unless every record it
// cites is one this input carried.
//
// Two checks, and both are load-bearing. claims.Grounded asks whether the
// citation names a record the reader can open; wireSentences asks whether the
// id parses at all. Either alone admits something: a syntactically valid UUID
// for a record in another workspace passes the second, and the first cannot run
// on a string the wire layer will later reject.
func groundedSentence(
	sentence Sentence, known map[Evidence]bool,
) (crmcontracts.OrganizationBriefSentence, bool) {
	if !claims.Grounded(sentence, known) {
		return crmcontracts.OrganizationBriefSentence{}, false
	}
	wired := wireSentences([]Sentence{sentence})
	if len(wired) == 0 {
		return crmcontracts.OrganizationBriefSentence{}, false
	}
	return wired[0], true
}

// groundedEvidence is the same rule for a field that carries citations without
// prose of its own.
func groundedEvidence(
	cited []Evidence, known map[Evidence]bool,
) ([]crmcontracts.OrganizationBriefEvidence, bool) {
	for _, one := range cited {
		if !known[Evidence{EntityType: one.EntityType, EntityID: one.EntityID}] {
			return nil, false
		}
	}
	return wireEvidence(cited)
}

func wireAsks(asks []Ask, known map[Evidence]bool) []crmcontracts.MeetingPlanAsk {
	out := make([]crmcontracts.MeetingPlanAsk, 0, len(asks))
	for _, ask := range asks {
		basis, ok := groundedSentence(ask.Basis, known)
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

func wireQuestions(questions []Question, known map[Evidence]bool) []crmcontracts.MeetingPlanQuestion {
	out := make([]crmcontracts.MeetingPlanQuestion, 0, len(questions))
	for _, question := range questions {
		evidence, ok := groundedEvidence(question.Evidence, known)
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

func wireScenarios(scenarios []Scenario, known map[Evidence]bool) []crmcontracts.MeetingPlanScenario {
	out := make([]crmcontracts.MeetingPlanScenario, 0, len(scenarios))
	for _, scenario := range scenarios {
		evidence, ok := groundedEvidence(scenario.Evidence, known)
		if !ok {
			continue
		}
		out = append(out, crmcontracts.MeetingPlanScenario{
			Label: scenario.Label, Play: scenario.Play, Evidence: evidence,
		})
	}
	return out
}

func wireArc(arc []ArcSentence, known map[Evidence]bool) []crmcontracts.MeetingPlanArcMoment {
	out := make([]crmcontracts.MeetingPlanArcMoment, 0, len(arc))
	for _, moment := range arc {
		summary, ok := groundedSentence(moment.Summary, known)
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
func wireAdvance(advance Advance, in Input, known map[Evidence]bool) crmcontracts.MeetingPlanAdvance {
	return crmcontracts.MeetingPlanAdvance{
		Minimum:  advanceLeg(advance.Minimum, in, known, "Leave with one named next step, owned and dated."),
		Best:     advanceLeg(advance.Best, in, known, "Leave with the next meeting booked and its purpose agreed."),
		Fallback: advanceLeg(advance.Fallback, in, known, "Leave with what you will send and who decides on it."),
	}
}

func advanceLeg(
	leg Sentence, in Input, known map[Evidence]bool, floor string,
) crmcontracts.OrganizationBriefSentence {
	if wired, ok := groundedSentence(leg, known); ok {
		return wired
	}
	wired, ok := groundedSentence(Sentence{
		Text:     floor,
		Nature:   natureRecommendation,
		Evidence: []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}},
	}, known)
	if !ok {
		// The meeting's own id did not ground, which means this brief should
		// not have been assembled at all. An EMPTY leg rather than an uncited
		// one: the contract requires three legs, and "cited or dropped" is the
		// stronger rule of the two — a sentence shown without its receipts is
		// the one thing this surface must never do.
		return crmcontracts.OrganizationBriefSentence{}
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
