// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// What KIND of meeting this is, which decides what a good plan for it looks
// like.
//
// A coffee and a contract review need opposite briefs: one wants permission and
// a light next step, the other wants the decision criteria and the unresolved
// objection. Preparing both the same way is how a brief becomes something a rep
// skims once and stops opening.
//
// Read from signals the records already carry rather than asked of a model: the
// subject a human typed, the deal's own stage, whether anyone in the room has
// met before. `unknown` is a real answer and the most useful one when it is
// true — it turns the first question of the meeting into "what would make this
// hour worth your time", which is the right question when the record does not
// say.

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// MeetingType is the classification and how much to trust it.
type MeetingType struct {
	Value      crmcontracts.MeetingPlanTypeValue
	Confidence crmcontracts.MeetingPlanTier
	// Signals names what decided it, as machine keys. Kept for the plan's own
	// reasoning and for a test that asks WHY a meeting was called a demo.
	Signals []string
}

// subjectFamily is one set of words that names a kind of meeting, in the two
// languages this product is sold in. Ordered: the first family that matches
// wins, so a subject naming both a demo and a price is read as the demo it is.
type subjectFamily struct {
	value crmcontracts.MeetingPlanTypeValue
	words []string
}

var subjectFamilies = []subjectFamily{
	{crmcontracts.MeetingPlanTypeRenewalRisk, []string{
		"renewal", "verlängerung", "churn", "kündigung", "escalation",
		"eskalation", "complaint", "beschwerde",
	}},
	{crmcontracts.MeetingPlanTypeDecision, []string{
		"decision", "entscheidung", "sign-off", "signoff", "board",
		"steering", "lenkungsausschuss", "freigabe",
	}},
	{crmcontracts.MeetingPlanTypeCommercial, []string{
		"proposal", "offer", "angebot", "pricing", "preis", "quote",
		"negotiation", "verhandlung", "contract", "vertrag",
	}},
	{crmcontracts.MeetingPlanTypeDemo, []string{
		"demo", "walkthrough", "präsentation", "presentation", "showcase",
	}},
	{crmcontracts.MeetingPlanTypeDelivery, []string{
		"kickoff", "kick-off", "onboarding", "status", "weekly", "jour fixe",
		"sprint", "review", "cutover", "go-live", "abnahme", "retro",
	}},
	{crmcontracts.MeetingPlanTypeFirstDiscovery, []string{
		"discovery", "intro", "kennenlernen", "erstgespräch", "qualification",
	}},
	{crmcontracts.MeetingPlanTypeRelationship, []string{
		"coffee", "kaffee", "lunch", "mittagessen", "catch-up", "catch up",
		"check-in", "networking", "dinner", "abendessen",
	}},
}

// classifyMeeting reads the signals and settles on one kind.
func classifyMeeting(in Input) MeetingType {
	fromSubject, subjectSignal := subjectSignalOf(in.Subject)
	fromStructure, structureSignals := structuralSignalsOf(in)

	signals := append([]string{}, structureSignals...)
	if subjectSignal != "" {
		signals = append([]string{subjectSignal}, signals...)
	}

	switch {
	case fromSubject == "" && fromStructure == "":
		return MeetingType{
			Value:      crmcontracts.MeetingPlanTypeUnknown,
			Confidence: crmcontracts.MeetingPlanTierLow,
			Signals:    signals,
		}
	case fromSubject == "":
		// The records say what stage this is at; nobody said what the meeting
		// is for. That is a reading, not a reading of a statement.
		return MeetingType{
			Value: fromStructure, Confidence: crmcontracts.MeetingPlanTierLow, Signals: signals,
		}
	case fromStructure == "" || fromStructure != fromSubject:
		// A typed subject with nothing agreeing, or with the deal's own stage
		// disagreeing. Believe the human who named the meeting, and say the
		// confidence is middling rather than pretending the conflict away.
		return MeetingType{
			Value:      refineDiscovery(fromSubject, in),
			Confidence: crmcontracts.MeetingPlanTierMedium, Signals: signals,
		}
	default:
		return MeetingType{
			Value:      refineDiscovery(fromSubject, in),
			Confidence: crmcontracts.MeetingPlanTierHigh, Signals: signals,
		}
	}
}

func subjectSignalOf(subject string) (crmcontracts.MeetingPlanTypeValue, string) {
	folded := strings.ToLower(subject)
	if strings.TrimSpace(folded) == "" {
		return "", ""
	}
	for _, family := range subjectFamilies {
		for _, word := range family.words {
			if strings.Contains(folded, word) {
				return family.value, "subject:" + word
			}
		}
	}
	return "", ""
}

// structuralSignalsOf reads what the records say about where this account is,
// independently of what anyone called the meeting.
func structuralSignalsOf(in Input) (crmcontracts.MeetingPlanTypeValue, []string) {
	var signals []string
	var value crmcontracts.MeetingPlanTypeValue

	if in.Deal != nil && in.Deal.Stage != "" {
		stage := strings.ToLower(in.Deal.Stage)
		switch {
		case containsAny(stage, "proposal", "angebot", "negotiation", "verhandlung", "commit"):
			value = crmcontracts.MeetingPlanTypeCommercial
			signals = append(signals, "stage:commercial")
		case containsAny(stage, "discovery", "qualif"):
			value = crmcontracts.MeetingPlanTypeFollowupDiscovery
			signals = append(signals, "stage:discovery")
		case containsAny(stage, "won", "closed"):
			value = crmcontracts.MeetingPlanTypeDelivery
			signals = append(signals, "stage:closed")
		}
	}
	if value == "" && in.Project != nil && in.Deal == nil {
		// Work in flight and nothing being sold: this is a delivery meeting,
		// which is the case the goal section fell silent on for months.
		value = crmcontracts.MeetingPlanTypeDelivery
		signals = append(signals, "project:no-open-deal")
	}
	if firstTimeRoom(in) {
		signals = append(signals, "room:first-time")
	}
	return value, signals
}

// refineDiscovery splits "discovery" by whether this room has met before.
func refineDiscovery(
	value crmcontracts.MeetingPlanTypeValue, in Input,
) crmcontracts.MeetingPlanTypeValue {
	if value != crmcontracts.MeetingPlanTypeFirstDiscovery {
		return value
	}
	if firstTimeRoom(in) {
		return crmcontracts.MeetingPlanTypeFirstDiscovery
	}
	return crmcontracts.MeetingPlanTypeFollowupDiscovery
}

// firstTimeRoom reports whether nobody in this room has met us before.
func firstTimeRoom(in Input) bool {
	if len(in.PriorMeetings) > 0 {
		return false
	}
	for _, attendee := range in.Attendees {
		if !attendee.FirstTime {
			return false
		}
	}
	return true
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
