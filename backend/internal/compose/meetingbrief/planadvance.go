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
	said := briefCopyFor(in.Language)
	who := said.Say(advanceWords.Them)
	if len(in.Attendees) > 0 {
		who = in.Attendees[0].FullName
	}
	switch typ.Value {
	case crmcontracts.MeetingPlanTypeValueMeetingPlanTypeRelationship, crmcontracts.MeetingPlanTypeValueMeetingPlanTypeUnknown:
		return said.Say(advanceWords.RelationshipMinimum),
			fmt.Sprintf(said.Say(advanceWords.RelationshipBest), who),
			said.Say(advanceWords.RelationshipFallback)
	case crmcontracts.MeetingPlanTypeValueMeetingPlanTypeFirstDiscovery, crmcontracts.MeetingPlanTypeValueMeetingPlanTypeFollowupDiscovery:
		return said.Say(advanceWords.DiscoveryMinimum),
			said.Say(advanceWords.DiscoveryBest),
			said.Say(advanceWords.DiscoveryFallback)
	case crmcontracts.MeetingPlanTypeValueMeetingPlanTypeDemo:
		return said.Say(advanceWords.DemoMinimum),
			said.Say(advanceWords.DemoBest),
			said.Say(advanceWords.DemoFallback)
	case crmcontracts.MeetingPlanTypeValueMeetingPlanTypeCommercial:
		return said.Say(advanceWords.CommercialMinimum),
			said.Say(advanceWords.CommercialBest),
			said.Say(advanceWords.CommercialFallback)
	case crmcontracts.MeetingPlanTypeValueMeetingPlanTypeDecision:
		return said.Say(advanceWords.DecisionMinimum),
			said.Say(advanceWords.DecisionBest),
			said.Say(advanceWords.DecisionFallback)
	case crmcontracts.MeetingPlanTypeValueMeetingPlanTypeDelivery:
		return said.Say(advanceWords.DeliveryMinimum),
			said.Say(advanceWords.DeliveryBest),
			said.Say(advanceWords.DeliveryFallback)
	default:
		return said.Say(advanceWords.RenewalMinimum),
			said.Say(advanceWords.RenewalBest),
			said.Say(advanceWords.RenewalFallback)
	}
}

// scenariosFor names what the meeting may become.
//
// Two or three branches, not a decision tree: a rep who has thought once about
// "what if this turns into a pricing conversation" handles it, and one holding
// nine branches handles none of them.
func scenariosFor(in Input, typ MeetingType) []Scenario {
	cite := []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}}
	branches := briefCopyFor(in.Language).scenarioLines(typ)
	out := make([]Scenario, 0, len(branches))
	for _, branch := range branches {
		out = append(out, Scenario{Label: branch[0], Play: branch[1], Evidence: cite})
	}
	return out
}

func (said briefCopy) scenarioLines(typ MeetingType) [][2]string {
	switch typ.Value {
	case crmcontracts.MeetingPlanTypeValueMeetingPlanTypeRelationship, crmcontracts.MeetingPlanTypeValueMeetingPlanTypeUnknown:
		return [][2]string{
			{said.Say(advanceWords.Social), said.Say(advanceWords.SocialPlay)},
			{said.Say(advanceWords.Work), said.Say(advanceWords.WorkPlay)},
			{said.Say(advanceWords.Complaint), said.Say(advanceWords.ComplaintPlay)},
		}
	case crmcontracts.MeetingPlanTypeValueMeetingPlanTypeCommercial, crmcontracts.MeetingPlanTypeValueMeetingPlanTypeDecision:
		return [][2]string{
			{said.Say(advanceWords.Price), said.Say(advanceWords.PricePlay)},
			{said.Say(advanceWords.Stakeholder), said.Say(advanceWords.StakeholderPlay)},
			{said.Say(advanceWords.Slip), said.Say(advanceWords.SlipPlay)},
		}
	case crmcontracts.MeetingPlanTypeValueMeetingPlanTypeDelivery, crmcontracts.MeetingPlanTypeValueMeetingPlanTypeRenewalRisk:
		return [][2]string{
			{said.Say(advanceWords.Escalation), said.Say(advanceWords.EscalationPlay)},
			{said.Say(advanceWords.Scope), said.Say(advanceWords.ScopePlay)},
		}
	default:
		return [][2]string{
			{said.Say(advanceWords.EarlyDemo), said.Say(advanceWords.EarlyDemoPlay)},
			{said.Say(advanceWords.NoProblem), said.Say(advanceWords.NoProblemPlay)},
		}
	}
}
