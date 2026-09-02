// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The three things at the top of a plan: what to earn, how to open, and the one
// thing that can change the conversation.
//
// Each is keyed on the meeting's kind, because the same account facts imply
// different moves in different rooms. The open promise that makes a commercial
// meeting's objective "close this out" makes a relationship coffee's objective
// "do not spend the hour on it".

import (
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
)

// The reminders. Fixed copy keyed to the meeting kind rather than read from
// records, which is why the contract keeps `caveat` out of the cited sentence:
// it is the product's advice, not a claim about this account.
const (
	caveatRelationship = "Do not force a commercial agenda onto an informal meeting. Ask permission first, then earn the next step."
	caveatDiscovery    = "Do not pitch. The meeting is worth more as questions than as answers."
	caveatCommercial   = "Do not concede on price before the value is agreed."
	caveatDelivery     = "Do not reopen scope that is already agreed; name what changed and what it costs."
	caveatUnknown      = "Do not assume what this meeting is for. Ask, then plan the rest of the hour."
	caveatDemo         = "Do not tour the product. Show the two things they asked about and stop."
)

func caveatFor(typ MeetingType) string {
	switch typ.Value {
	case crmcontracts.MeetingPlanTypeRelationship:
		return caveatRelationship
	case crmcontracts.MeetingPlanTypeFirstDiscovery, crmcontracts.MeetingPlanTypeFollowupDiscovery:
		return caveatDiscovery
	case crmcontracts.MeetingPlanTypeCommercial, crmcontracts.MeetingPlanTypeDecision:
		return caveatCommercial
	case crmcontracts.MeetingPlanTypeDelivery, crmcontracts.MeetingPlanTypeRenewalRisk:
		return caveatDelivery
	case crmcontracts.MeetingPlanTypeDemo:
		return caveatDemo
	default:
		return caveatUnknown
	}
}

// objectiveFor is the outcome to earn.
//
// It reads the SAME sharpest open claim the goal section names, through the
// same ranked set, so the two cannot aim the meeting at different things. Where
// there is no such claim the objective falls to what the meeting's own kind
// implies, cited to the meeting itself — the record that carries the filing the
// claim would have come from.
func objectiveFor(in Input, typ MeetingType, ranked *rankedClaims) *Objective {
	caveat := caveatFor(typ)
	if ask, ok := ranked.take(openOfKind(kindCommitmentOurs, kindOpenQuestion, kindDecision)); ok {
		return &Objective{
			Sentence: Sentence{
				Text:     objectiveLine(ask, typ, in.Now),
				Nature:   natureRecommendation,
				Evidence: []Evidence{{EntityType: citeActivity, EntityID: ask.SourceID}},
			},
			Caveat: caveat,
		}
	}
	text := objectiveByType(typ, in)
	if text == "" {
		return nil
	}
	return &Objective{
		Sentence: Sentence{
			Text:     text,
			Nature:   natureRecommendation,
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}},
		},
		Caveat: caveat,
	}
}

// objectiveLine turns the sharpest claim into an outcome, softened where the
// room is not the place to press it.
func objectiveLine(ask ClaimIn, typ MeetingType, now time.Time) string {
	if typ.Value == crmcontracts.MeetingPlanTypeRelationship {
		return fmt.Sprintf(
			"Confirm whether %s is still a priority, and leave with one dated next step on it: %s",
			ask.PersonName, ask.Body)
	}
	if deadline.Passed(ask.DueAt, now) {
		return fmt.Sprintf(
			"Close out what we owe %s, overdue since %s, and agree the next step: %s",
			ask.PersonName, ask.DueAt.UTC().Format("2 Jan"), ask.Body)
	}
	return fmt.Sprintf("Leave with %s's answer on: %s", ask.PersonName, ask.Body)
}

func objectiveByType(typ MeetingType, in Input) string {
	switch typ.Value {
	case crmcontracts.MeetingPlanTypeUnknown:
		return "Establish what this meeting is for before planning the rest of the hour."
	case crmcontracts.MeetingPlanTypeFirstDiscovery:
		return "Learn what prompted this conversation and who else decides, and earn a second meeting."
	case crmcontracts.MeetingPlanTypeRelationship:
		return "Keep the relationship warm and find out what has changed since you last spoke."
	case crmcontracts.MeetingPlanTypeDelivery:
		if in.Project != nil {
			return projectGoalLine(*in.Project)
		}
		return "Agree what is done, what is next, and who owns it."
	case crmcontracts.MeetingPlanTypeCommercial:
		return "Agree the shape of the commercial terms, or name what is blocking them."
	case crmcontracts.MeetingPlanTypeDecision:
		return "Get the decision, or the date and the name of whoever makes it."
	case crmcontracts.MeetingPlanTypeRenewalRisk:
		return "Understand what went wrong, and agree one thing that changes before the renewal."
	case crmcontracts.MeetingPlanTypeDemo:
		return "Show the two things they said they needed, and agree what happens after."
	default:
		return "Agree a dated next step before the meeting ends."
	}
}

// openingFor is the first thing to say.
//
// An opener that names something real — the last meeting, the promise
// outstanding — is the difference between an informed seller and a script. When
// there is nothing real to name, it opens by asking, which is the honest move
// and also the right one.
func openingFor(in Input, typ MeetingType) *Sentence {
	if len(in.PriorMeetings) > 0 {
		prior := in.PriorMeetings[0]
		return &Sentence{
			Text: fmt.Sprintf(
				"Open on what was agreed at %q on %s, and ask what has moved since.",
				prior.Subject, prior.StartsAt.UTC().Format("2 Jan")),
			Nature:   natureRecommendation,
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: prior.ID}},
		}
	}
	text := "Open by asking what prompted the meeting and what would make the hour worth their time."
	if typ.Value != crmcontracts.MeetingPlanTypeUnknown && in.LastTouchAt != nil {
		text = "Open on what has changed since you last spoke, before proposing anything."
	}
	return &Sentence{
		Text:     text,
		Nature:   natureRecommendation,
		Evidence: []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}},
	}
}

// topRiskFor is the one watch-out worth rehearsing, with what to do about it.
//
// One, not a list: a rep walking into a room can hold one thing they must not
// get wrong. The response is keyed on what KIND of risk it is, because an
// unanswered objection and a promise we broke need opposite openings — one
// needs answering, the other needs owning.
func topRiskFor(in Input, ranked *rankedClaims) *Risk {
	claim, ok := ranked.take(func(candidate ClaimIn) bool {
		_, isRisk := riskLine(candidate, in.Now)
		return isRisk
	})
	if !ok {
		return nil
	}
	text, _ := riskLine(claim, in.Now)
	return &Risk{
		Text: Sentence{
			Text:     text,
			Nature:   natureAssessment,
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: claim.SourceID}},
		},
		Response: responseFor(claim, in),
	}
}

func responseFor(claim ClaimIn, in Input) Response {
	if claim.Kind == kindObjection {
		return Response{
			Say: fmt.Sprintf(
				"Acknowledge it in their words before answering: %q is still open.", claim.Body),
			Show:  "The record that answers it, or the person who can.",
			Avoid: "Re-arguing a point they have already made twice.",
		}
	}
	return Response{
		Say: fmt.Sprintf(
			"Own the delay plainly and name a date: we owe %s on %q.",
			claim.PersonName, claim.Body),
		Show:  "What is actually ready, and what the remaining step is.",
		Avoid: "Promising a date nobody on our side has agreed to.",
	}
}
