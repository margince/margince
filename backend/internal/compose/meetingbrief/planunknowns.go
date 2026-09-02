// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// What the record does not say, what they are likely to ask, and what to ask
// them.
//
// The three read the same evidence from different sides, which is why they are
// one file: an unknown is a gap, a likely ask is a gap THEY will notice, and a
// question is how we close one.
//
// THE RULE THAT MATTERS HERE. A question must name something only this account
// would produce — a person, a promise, a subject somebody actually wrote. A
// question that would read identically on any other prospect is not a prepared
// question, it is a questionnaire, and printing five of them is how a
// preparation surface teaches a rep it has nothing to say. So a template that
// cannot be filled with a real fact is not emitted at all: fewer, specific
// questions beat five generic ones, and an empty list is honest.

import (
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// questionCap is what a rep can hold. The plan prints at most five and tells
// them to ask three.
const questionCap = 5

// askCap bounds the hypotheses about what THEY will ask.
const askCap = 5

// unknownsFor reads the gaps off the ABSENCE of records.
//
// Derived from absence rather than from a writer's omission, which is the
// distinction the contract names: "nobody captured the decision route" is a
// fact about the record, and "the model did not mention it" is a fact about the
// model. Only the first belongs in front of a reader.
func unknownsFor(in Input, typ MeetingType, arc []ArcMoment) []Unknown {
	var out []Unknown
	add := func(kind crmcontracts.MeetingPlanUnknownKind, question string) {
		out = append(out, Unknown{Kind: kind, Question: question})
	}
	if typ.Value == crmcontracts.MeetingPlanTypeUnknown {
		add(crmcontracts.MeetingPlanUnknownIntentNotCaptured,
			"What would make this hour worth your time?")
	}
	if in.Deal == nil {
		add(crmcontracts.MeetingPlanUnknownNoOpenDeal,
			"Is there a piece of work here you are trying to get funded, or is this still exploratory?")
	}
	if !hasClaimKind(in, kindDecisionProcess) {
		add(crmcontracts.MeetingPlanUnknownDecisionRouteNotCaptured,
			"Who else has to agree before this can go ahead?")
	}
	if len(in.PriorMeetings) == 0 {
		add(crmcontracts.MeetingPlanUnknownNoPriorMeeting,
			"Have you spoken to anyone else on our side before today?")
	}
	if len(in.Commitments) == 0 {
		add(crmcontracts.MeetingPlanUnknownNoCommitmentsCaptured,
			"Is anything outstanding from either side that I should pick up?")
	}
	if len(arc) == 0 {
		add(crmcontracts.MeetingPlanUnknownNoHistory,
			"How much of the background do you already have from your side?")
	}
	if len(in.Attendees) == 0 {
		add(crmcontracts.MeetingPlanUnknownAttendeesNotVisible,
			"Who else is joining, and what do they need from this?")
	}
	return out
}

func hasClaimKind(in Input, kind string) bool {
	for _, claim := range in.Commitments {
		if claim.Kind == kind {
			return true
		}
	}
	return false
}

// likelyAsksFor is what THEY are likely to put to US.
//
// Every one is grounded in something they actually said: an open question they
// asked, a promise we made them. A hypothesis with no record behind it is a
// guess, and a guess printed beside cited facts borrows their credibility.
func likelyAsksFor(in Input, ranked *rankedClaims) []Ask {
	var out []Ask
	for _, claim := range ranked.all(ofKind(kindOpenQuestion, kindCommitmentOurs, kindObjection)) {
		if len(out) == askCap {
			break
		}
		question, prepare := askLines(claim)
		if question == "" {
			continue
		}
		out = append(out, Ask{
			Question: question,
			Basis: Sentence{
				Text:     askBasis(claim),
				Nature:   natureAssessment,
				Evidence: []Evidence{{EntityType: citeActivity, EntityID: claim.SourceID}},
			},
			Relevance: relevanceOf(claim),
			Prepare:   prepare,
		})
	}
	return out
}

func askLines(claim ClaimIn) (string, string) {
	switch claim.Kind {
	case kindOpenQuestion:
		return fmt.Sprintf("%q", claim.Body),
			"Answer it directly, or say who will and by when."
	case kindCommitmentOurs:
		return fmt.Sprintf("Where are you with %s?", claim.Body),
			"Have the current state and the next date ready before the meeting."
	case kindObjection:
		return fmt.Sprintf("%q — expect them to raise it again.", claim.Body),
			"Answer it with a record they can check, not with reassurance."
	default:
		return "", ""
	}
}

func askBasis(claim ClaimIn) string {
	if claim.Kind == kindCommitmentOurs {
		return fmt.Sprintf("We promised %s this and it is still open.", claim.PersonName)
	}
	return fmt.Sprintf("%s raised this and it has not been closed out.", claim.PersonName)
}

func relevanceOf(claim ClaimIn) crmcontracts.MeetingPlanTier {
	if claim.Status == statusOpen {
		return crmcontracts.MeetingPlanTierHigh
	}
	return crmcontracts.MeetingPlanTierMedium
}

// questionsFor is what to ask THEM.
//
// Claims first: a question built on something a named person actually said is
// specific by construction. Then the unknowns, but ONLY those whose question
// can name a real fact from this account — see the rule at the top of this
// file. An unknown with nothing to anchor on stays in `unknowns`, where it
// reads as the gap it is, instead of being dressed up as preparation.
func questionsFor(in Input, unknowns []Unknown, ranked *rankedClaims) []Question {
	var out []Question
	for _, claim := range ranked.all(ofKind(kindPriority, kindSuccessCriterion, kindOpenQuestion)) {
		if len(out) == questionCap {
			return out
		}
		out = append(out, Question{
			Ask: fmt.Sprintf("You said %q — what has changed about that since?", claim.Body),
			Why: fmt.Sprintf(
				"It is the thing %s named, and the plan should be built on it rather than on what we assume.",
				claim.PersonName),
			ListenFor: "Whether it still matters, who owns it now, and what it is costing them.",
			Evidence:  []Evidence{{EntityType: citeActivity, EntityID: claim.SourceID}},
		})
	}
	anchor := accountAnchor(in)
	if anchor == "" {
		return out
	}
	for _, unknown := range unknowns {
		if len(out) == questionCap {
			return out
		}
		why := unknownWhy(unknown.Kind, anchor)
		if why == "" {
			continue
		}
		out = append(out, Question{
			Ask:       unknown.Question,
			Why:       why,
			ListenFor: unknownListenFor(unknown.Kind),
			Evidence:  []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}},
		})
	}
	return out
}

// accountAnchor is the one thing this plan can name that no other account
// would produce: the company, or failing that the person in the room.
func accountAnchor(in Input) string {
	if in.Company != "" {
		return in.Company
	}
	if len(in.Attendees) > 0 {
		return in.Attendees[0].FullName
	}
	return ""
}

func unknownWhy(kind crmcontracts.MeetingPlanUnknownKind, anchor string) string {
	switch kind {
	case crmcontracts.MeetingPlanUnknownIntentNotCaptured:
		return fmt.Sprintf("Nothing in the record says what %s wants from this meeting.", anchor)
	case crmcontracts.MeetingPlanUnknownDecisionRouteNotCaptured:
		return fmt.Sprintf("The record does not name who decides at %s.", anchor)
	case crmcontracts.MeetingPlanUnknownNoOpenDeal:
		return fmt.Sprintf("There is no open deal with %s, so what this is for is unstated.", anchor)
	default:
		// The rest are gaps worth stating and not worth spending one of five
		// questions on — a rep does not need prompting to ask whether anything
		// is outstanding.
		return ""
	}
}

func unknownListenFor(kind crmcontracts.MeetingPlanUnknownKind) string {
	switch kind {
	case crmcontracts.MeetingPlanUnknownIntentNotCaptured:
		return "The word they use for the problem, and whether a date is attached to it."
	case crmcontracts.MeetingPlanUnknownDecisionRouteNotCaptured:
		return "Names and roles: who approves, who pays, who can veto."
	default:
		return "Whether the answer names a person, a date, or neither."
	}
}
