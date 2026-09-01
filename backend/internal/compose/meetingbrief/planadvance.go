// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// How the meeting can end well, and what it may turn into on the way.
//
// Three advances rather than one goal, because "the meeting went fine" is what
// a rep says about a meeting that ended with nothing. A minimum that still
// counts makes the difference between a soft no and a wasted hour visible while
// there is still time to ask for it.

import (
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// advanceFor builds the three ways out, keyed on the kind of room.
//
// Every leg cites the meeting itself. These are moves rather than claims about
// the account — the record that supports "ask for a scoping session" is the
// fact that this meeting is happening, and citing a conversation would dress a
// recommendation as something somebody said.
func advanceFor(in Input, typ MeetingType) Advance {
	minimum, best, fallback := advanceLines(typ, in)
	cite := []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}}
	return Advance{
		Minimum:  Sentence{Text: minimum, Nature: natureRecommendation, Evidence: cite},
		Best:     Sentence{Text: best, Nature: natureRecommendation, Evidence: cite},
		Fallback: Sentence{Text: fallback, Nature: natureRecommendation, Evidence: cite},
	}
}

func advanceLines(typ MeetingType, in Input) (string, string, string) {
	who := "them"
	if len(in.Attendees) > 0 {
		who = in.Attendees[0].FullName
	}
	switch typ.Value {
	case crmcontracts.MeetingPlanTypeRelationship, crmcontracts.MeetingPlanTypeUnknown:
		return "A named next step with an owner and a date, even if the answer is 'not this quarter'.",
			fmt.Sprintf("A working session booked with %s and whoever else has to be in it.", who),
			"Agreement on what you will send, and who decides what happens with it."
	case crmcontracts.MeetingPlanTypeFirstDiscovery, crmcontracts.MeetingPlanTypeFollowupDiscovery:
		return "One quantified problem, and who owns it today.",
			"The two outcomes that matter most, and a booked session to scope them.",
			"A written summary they agree with, and the name of the person who has to approve it."
	case crmcontracts.MeetingPlanTypeDemo:
		return "Their reaction to the two things they said they needed.",
			"Agreement that it fits, and a date to talk commercials.",
			"The one objection that stopped it, in their own words."
	case crmcontracts.MeetingPlanTypeCommercial:
		return "The remaining gap between our number and theirs, stated.",
			"Agreed terms and a signature date.",
			"What has to be true for them to sign, and by when."
	case crmcontracts.MeetingPlanTypeDecision:
		return "The date the decision gets made, and who makes it.",
			"The decision itself.",
			"The one unresolved objection, and who has to answer it."
	case crmcontracts.MeetingPlanTypeDelivery:
		return "Agreement on what is done and what is next.",
			"The next milestone dated, with an owner on each side.",
			"A written list of what is blocked and who unblocks it."
	default:
		return "One thing that changes before the renewal date.",
			"Agreement to renew, or the date that decision gets made.",
			"What went wrong, in their words, and who else needs to hear it."
	}
}

// scenariosFor names what the meeting may become.
//
// Two or three branches, not a decision tree: a rep who has thought once about
// "what if this turns into a pricing conversation" handles it, and one holding
// nine branches handles none of them.
func scenariosFor(in Input, typ MeetingType) []Scenario {
	cite := []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}}
	branches := scenarioLines(typ)
	out := make([]Scenario, 0, len(branches))
	for _, branch := range branches {
		out = append(out, Scenario{Label: branch[0], Play: branch[1], Evidence: cite})
	}
	return out
}

func scenarioLines(typ MeetingType) [][2]string {
	switch typ.Value {
	case crmcontracts.MeetingPlanTypeRelationship, crmcontracts.MeetingPlanTypeUnknown:
		return [][2]string{
			{"It stays social", "Let it. Ask one 'what has changed' question and earn a separate working session."},
			{"They raise the work", "Follow it: prioritise two outcomes, then propose a scoping session."},
			{"They raise a complaint", "Hear it out fully before answering. Do not defend the internal history."},
		}
	case crmcontracts.MeetingPlanTypeCommercial, crmcontracts.MeetingPlanTypeDecision:
		return [][2]string{
			{"It becomes a price negotiation", "Anchor on the outcome agreed, not the list. Trade scope for price, never price alone."},
			{"A new stakeholder appears", "Stop selling and qualify them: what do they need to believe, and what do they decide?"},
			{"The decision slips", "Get the new date and the reason in the room, before it becomes an email."},
		}
	case crmcontracts.MeetingPlanTypeDelivery, crmcontracts.MeetingPlanTypeRenewalRisk:
		return [][2]string{
			{"It becomes an escalation", "Take it seriously in the room. Name what you will fix and by when; commit to nothing else."},
			{"They ask for more scope", "Welcome it and price it. Agreeing to it here is how a delivery goes red."},
		}
	default:
		return [][2]string{
			{"They want the demo early", "Show the two things they named, then return to what problem they were for."},
			{"They will not name a problem", "Ask what happens today, step by step, and listen for where it costs them."},
		}
	}
}
