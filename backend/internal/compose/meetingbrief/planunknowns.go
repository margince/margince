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
// would produce — a contact, a promise, a subject somebody actually wrote. A
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
	said := briefCopyFor(in.Language)
	var out []Unknown
	add := func(kind crmcontracts.MeetingPlanUnknownKind, question string) {
		out = append(out, Unknown{Kind: kind, Question: question})
	}
	if typ.Value == crmcontracts.MeetingPlanTypeValueMeetingPlanTypeUnknown {
		add(crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownIntentNotCaptured,
			said.Say(questionWords.Intent))
	}
	if in.Deal == nil {
		add(crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownNoOpenDeal,
			said.Say(questionWords.Funding))
	}
	if !hasClaimKind(in, kindDecisionProcess) {
		add(crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownDecisionRouteNotCaptured,
			said.Say(questionWords.Approvers))
	}
	if len(in.PriorMeetings) == 0 {
		add(crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownNoPriorMeeting,
			said.Say(questionWords.PriorContact))
	}
	if len(in.Commitments) == 0 {
		add(crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownNoCommitmentsCaptured,
			said.Say(questionWords.Outstanding))
	}
	if len(arc) == 0 {
		add(crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownNoHistory,
			said.Say(questionWords.Background))
	}
	if len(in.Attendees) == 0 {
		add(crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownAttendeesNotVisible,
			said.Say(questionWords.Joining))
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
	said := briefCopyFor(in.Language)
	var out []Ask
	for _, claim := range ranked.all(openOfKind(kindOpenQuestion, kindCommitmentOurs, kindObjection)) {
		if len(out) == askCap {
			break
		}
		question, prepare := said.askLines(claim)
		if question == "" {
			continue
		}
		out = append(out, Ask{
			Question: question,
			Basis: Sentence{
				Text:     said.askBasis(claim),
				Nature:   natureAssessment,
				Evidence: []Evidence{{EntityType: citeActivity, EntityID: claim.SourceID}},
			},
			Relevance: crmcontracts.MeetingPlanTierMeetingPlanTierHigh,
			Prepare:   prepare,
		})
	}
	return out
}

func (said briefCopy) askLines(claim ClaimIn) (string, string) {
	switch claim.Kind {
	case kindOpenQuestion:
		return fmt.Sprintf("%q", claim.Body),
			said.Say(questionWords.Answer)
	case kindCommitmentOurs:
		return fmt.Sprintf(said.Say(questionWords.Progress), claim.Body),
			said.Say(questionWords.Prepare)
	case kindObjection:
		return fmt.Sprintf(said.Say(questionWords.Objection), claim.Body),
			said.Say(questionWords.Evidence)
	default:
		return "", ""
	}
}

func (said briefCopy) askBasis(claim ClaimIn) string {
	if claim.Kind == kindCommitmentOurs {
		return fmt.Sprintf(said.Say(questionWords.PromiseOpen), claim.ContactName)
	}
	return fmt.Sprintf(said.Say(questionWords.RaisedOpen), claim.ContactName)
}

// questionsFor is what to ask THEM.
//
// Claims first: a question built on something a named contact actually said is
// specific by construction. Then the unknowns, but ONLY those whose question
// can name a real fact from this account — see the rule at the top of this
// file. An unknown with nothing to anchor on stays in `unknowns`, where it
// reads as the gap it is, instead of being dressed up as preparation.
func questionsFor(in Input, unknowns []Unknown, ranked *rankedClaims) []Question {
	said := briefCopyFor(in.Language)
	var out []Question
	for _, claim := range ranked.all(func(claim ClaimIn) bool {
		return ofKind(kindPriority, kindSuccessCriterion)(claim) || openOfKind(kindOpenQuestion)(claim)
	}) {
		if len(out) == questionCap {
			return out
		}
		out = append(out, Question{
			Ask: fmt.Sprintf(said.Say(questionWords.Changed), claim.Body),
			Why: fmt.Sprintf(
				said.Say(questionWords.Named),
				claim.ContactName),
			ListenFor: said.Say(questionWords.ListenPriority),
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
		why := said.unknownWhy(unknown.Kind, anchor)
		if why == "" {
			continue
		}
		out = append(out, Question{
			Ask:       unknown.Question,
			Why:       why,
			ListenFor: said.unknownListenFor(unknown.Kind),
			Evidence:  []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}},
		})
	}
	return out
}

// accountAnchor is the one thing this plan can name that no other account
// would produce: the company, or failing that the contact in the room.
func accountAnchor(in Input) string {
	if in.Company != "" {
		return in.Company
	}
	if len(in.Attendees) > 0 {
		return in.Attendees[0].FullName
	}
	return ""
}

func (said briefCopy) unknownWhy(kind crmcontracts.MeetingPlanUnknownKind, anchor string) string {
	switch kind {
	case crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownIntentNotCaptured:
		return fmt.Sprintf(said.Say(questionWords.WhyIntent), anchor)
	case crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownDecisionRouteNotCaptured:
		return fmt.Sprintf(said.Say(questionWords.WhyDecision), anchor)
	case crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownNoOpenDeal:
		return fmt.Sprintf(said.Say(questionWords.WhyDeal), anchor)
	default:
		// The rest are gaps worth stating and not worth spending one of five
		// questions on — a rep does not need prompting to ask whether anything
		// is outstanding.
		return ""
	}
}

func (said briefCopy) unknownListenFor(kind crmcontracts.MeetingPlanUnknownKind) string {
	switch kind {
	case crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownIntentNotCaptured:
		return said.Say(questionWords.ListenProblem)
	case crmcontracts.MeetingPlanUnknownKindMeetingPlanUnknownDecisionRouteNotCaptured:
		return said.Say(questionWords.ListenRoles)
	default:
		return said.Say(questionWords.ListenContact)
	}
}
