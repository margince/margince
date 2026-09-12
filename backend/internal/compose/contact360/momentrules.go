// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The lower rungs of the moment ladder and the machinery every rung shares.
//
// They live beside moments.go rather than inside it because the ladder's ORDER
// is the decision worth reading in one screen, and the individual conditions
// are detail. A reader asking "what does this page open on" should not have to
// scroll through seven rule bodies to find out.

import (
	"context"
	"fmt"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// roleChangeMoment: the relationship crossed a threshold. Derived from what the
// page already read, never from a fresh query.
//
// The rule id is role_change and the only change it reads is replied_after_gap,
// which is not a role change — relstrength emits four kinds and none of them is
// one. So the headline states what the evidence actually shows. Naming the rung
// for a signal the system does not produce is a contract question, tracked
// separately; what must not happen meanwhile is the page telling a rep that
// somebody's seat moved on the strength of a reply.
func roleChangeMoment(_ context.Context, _ time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	change, ok := findChange(page, relstrength.ChangeRepliedAfterGap)
	if !ok {
		return crmcontracts.ContactMoment{}, false
	}
	evidence := []crmcontracts.ContactMomentEvidence{{
		Type:       crmcontracts.ContactMomentEvidenceTypeRelationshipChange,
		Label:      "They replied after a long gap",
		ObservedAt: &change.At,
	}}
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:role_change",
		Rule:                crmcontracts.ContactMomentRuleRoleChange,
		RuleVersion:         ptr(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            "They answered after a long silence",
		WhyNow:              "A relationship that had gone quiet has moved. The window where a reply is expected is now.",
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         &change.At,
		RecommendedAction:   openDeal(page),
	}, true
}

// withheld reports whether any of these sections was kept from this reader.
//
// A section the caller may not read comes back NIL, exactly like a section that
// is genuinely empty, and assemble.go records the difference in SectionsOmitted
// instead. A rule that reads nil as "there is nothing here" therefore tells a
// reader without the grant that nothing is scheduled, that nothing has been
// captured, that nobody is waiting — three confident statements about data the
// page was not allowed to look at.
//
// So every rule whose FINDING IS AN ABSENCE asks this first. A rule that fires
// on something present (a meeting exists, a promise is overdue) does not need
// it: what it saw, it saw.
func withheld(page *crmcontracts.Contact360, sections ...crmcontracts.Contact360SectionsOmitted) bool {
	for _, omitted := range page.SectionsOmitted {
		for _, section := range sections {
			if omitted == section {
				return true
			}
		}
	}
	return false
}

// missingNextStepMoment: there is an open deal and nothing scheduled with the
// contact who sits on it. The gap is the finding.
func missingNextStepMoment(_ context.Context, _ time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	// "Nothing is scheduled" is only true if this reader could see the schedule.
	if withheld(page, crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedNextMeeting,
		crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedNextSteps,
		crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedCommercial) {
		return crmcontracts.ContactMoment{}, false
	}
	if page.Commercial == nil || page.Commercial.Deal == nil {
		return crmcontracts.ContactMoment{}, false
	}
	if page.NextMeeting != nil {
		return crmcontracts.ContactMoment{}, false
	}
	if page.NextSteps != nil && len(page.NextSteps.Data) > 0 {
		return crmcontracts.ContactMoment{}, false
	}
	deal := *page.Commercial.Deal
	evidence := []crmcontracts.ContactMomentEvidence{{
		Type:  crmcontracts.ContactMomentEvidenceTypeRelationshipChange,
		Label: fmt.Sprintf("%s has no next step with them", deal.Title),
	}}
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:missing_next_step",
		Rule:                crmcontracts.ContactMomentRuleMissingNextStep,
		RuleVersion:         ptr(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            "No next step with them on an open deal",
		// The seat this contact actually holds, named — not "the contact whose
		// seat decides it", which the record does not say. The rung fires on
		// ANY recorded stakeholder role, and the vocabulary distinguishes the
		// ones that decide (economic_buyer, decision_maker) from the ones that
		// do not (champion, influencer, user). Telling a rep the deal turns on
		// somebody who is recorded as an influencer is a claim the row refuses.
		//
		// The rung is not narrowed to the deciding roles instead, because a
		// deal with no next step is worth saying whoever the seat belongs to —
		// what was wrong was the sentence, not the trigger.
		WhyNow: fmt.Sprintf("The deal is live and nothing is scheduled with them. They are %s on it.",
			recordedSeat(page.Commercial.Role)),
		Confidence:        crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:          evidence,
		RecommendedAction: bookMeeting(),
		SecondaryActions: &[]crmcontracts.ContactMomentAction{{
			Kind:        crmcontracts.ContactMomentActionKindOpenRecord,
			Label:       "Open the deal",
			State:       crmcontracts.ContactMomentActionStateAvailable,
			Destination: dealRecord(deal.DealId),
		}},
	}, true
}

// thinRelationshipMoment: nothing has been captured and nobody here knows them.
//
// It is second to last because it is the least urgent thing that can be true,
// and because saying it too eagerly on a record whose activity section was
// simply withheld would be a lie. Both inputs must be READ and empty, not
// absent.
func thinRelationshipMoment(_ context.Context, _ time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	if page.Activities == nil || page.Network == nil {
		// A section the caller may not read contributes no moment. Absent is
		// not the same as empty, and only one of them is a fact about the
		// relationship.
		return crmcontracts.ContactMoment{}, false
	}
	if len(page.Activities.Data) > 0 || len(page.Network.Colleagues) > 0 {
		return crmcontracts.ContactMoment{}, false
	}
	evidence := []crmcontracts.ContactMomentEvidence{{
		Type:  crmcontracts.ContactMomentEvidenceTypeRelationshipChange,
		Label: "Nothing captured, nobody connected",
	}}
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:thin_relationship",
		Rule:                crmcontracts.ContactMomentRuleThinRelationship,
		RuleVersion:         ptr(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            "Nothing is captured about them yet",
		WhyNow:              "There is no correspondence and no colleague who knows them. Everything about this record is still to be learned.",
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		RecommendedAction:   logInteraction(),
	}, true
}

// dealRecord points an action at one deal's page.
//
// The entity id is the whole content of this destination: the frontend
// dispatcher navigates only when it has one, so a record surface without an id
// is a button that looks live and goes nowhere - which is the same defect as an
// action with no destination at all, wearing a destination.
func dealRecord(dealID openapi_types.UUID) *crmcontracts.ContactMomentDestination {
	entity := crmcontracts.ContactMomentDestinationEntityTypeDeal
	return &crmcontracts.ContactMomentDestination{
		Surface:    crmcontracts.ContactMomentDestinationSurfaceRecord,
		EntityType: &entity,
		EntityId:   &dealID,
	}
}

// openDeal offers the deal this record has open, when the reader can see one.
//
// The relationship change names no deal, so the destination comes from the
// commercial section - and that section is absent for a reader without the
// deal grant. Blocked there rather than available: an action pointing at a
// record this caller cannot open would navigate them to a 404, which is worse
// than a control that says why it is off.
func openDeal(page *crmcontracts.Contact360) crmcontracts.ContactMomentAction {
	action := crmcontracts.ContactMomentAction{
		Kind:  crmcontracts.ContactMomentActionKindOpenRecord,
		Label: "Open the deal",
		State: crmcontracts.ContactMomentActionStateAvailable,
	}
	if page.Commercial == nil || page.Commercial.Deal == nil {
		reason := "No open deal is visible on this record"
		action.State = crmcontracts.ContactMomentActionStateBlocked
		action.BlockedReason = &reason
		return action
	}
	action.Destination = dealRecord(page.Commercial.Deal.DealId)
	return action
}

// bookMeeting offers the move this rung is actually about, and blocks it.
//
// Pointing "Book a meeting" at the deal record would satisfy every check —
// a real surface, a real entity id, a client that navigates — and still lie.
// The reader presses a button that says it books a meeting and lands on a deal
// page, which is a worse kind of dead button than one that does nothing: it
// does something, and something else.
//
// Nothing in the destination vocabulary opens a scheduler, so blocked is the
// honest state. Opening the deal stays offered beside it, under its own label,
// where it is true.
func bookMeeting() crmcontracts.ContactMomentAction {
	reason := "Booking a meeting from this card is not available yet"
	return crmcontracts.ContactMomentAction{
		Kind:          crmcontracts.ContactMomentActionKindScheduleMeeting,
		Label:         "Book a meeting",
		State:         crmcontracts.ContactMomentActionStateBlocked,
		BlockedReason: &reason,
	}
}

// inboundEvidence names the actual message where the page is showing it, and
// falls back to the bare fact when the timeline is capped past it.
//
// The fallback is honest rather than silent: the claim is true either way, and
// pretending there is a row to open when the reader would land on nothing is
// worse than saying the message is older than this page shows.
func inboundEvidence(page *crmcontracts.Contact360, inbound time.Time) crmcontracts.ContactMomentEvidence {
	return directionEvidence(page, inbound, "Their last message")
}

// outboundEvidence is the same lookup for the message WE sent.
func outboundEvidence(page *crmcontracts.Contact360, outbound time.Time) []crmcontracts.ContactMomentEvidence {
	return []crmcontracts.ContactMomentEvidence{
		directionEvidence(page, outbound, "Your last message"),
	}
}

func directionEvidence(page *crmcontracts.Contact360, at time.Time, fallback string) crmcontracts.ContactMomentEvidence {
	evidence := crmcontracts.ContactMomentEvidence{
		Type:       crmcontracts.ContactMomentEvidenceTypeActivity,
		Label:      fallback,
		ObservedAt: &at,
	}
	if activity, ok := findActivityAt(page, at); ok {
		id := activity.Id
		evidence.Id = &id
		if activity.Subject != nil && *activity.Subject != "" {
			evidence.Label = *activity.Subject
		}
	}
	return evidence
}

// findChange looks up one derived relationship change on the page. It answers
// false when the section was omitted for want of a grant, which is what keeps
// a moment from disclosing something the page itself is withholding.
func findChange(page *crmcontracts.Contact360, kind string) (crmcontracts.ContactRelationshipChange, bool) {
	if page.RelationshipChanges == nil {
		return crmcontracts.ContactRelationshipChange{}, false
	}
	for _, c := range *page.RelationshipChanges {
		if string(c.Kind) == kind {
			return c, true
		}
	}
	return crmcontracts.ContactRelationshipChange{}, false
}

// findActivityAt finds the timeline row for an instant the page reported
// separately. The two come from the same transaction, so a match is exact
// rather than approximate.
func findActivityAt(page *crmcontracts.Contact360, at time.Time) (crmcontracts.Activity, bool) {
	if page.Activities == nil {
		return crmcontracts.Activity{}, false
	}
	for _, a := range page.Activities.Data {
		if a.OccurredAt.Equal(at) {
			return a, true
		}
	}
	return crmcontracts.Activity{}, false
}

// fingerprintOf digests what a moment fired on, so a dismissal can be held
// against the evidence rather than against the moment's name.
//
// The digest itself is kernel/owedwork's, shared with the company page's card:
// two spellings of one hash do not fail loudly when they drift, they silently
// stop matching, and every dismissal a reader ever made stops working at once.
// This maps the contract's evidence into the marks that hash reads.
func fingerprintOf(evidence []crmcontracts.ContactMomentEvidence) string {
	marks := make([]owedwork.Mark, 0, len(evidence))
	for _, e := range evidence {
		mark := owedwork.Mark{Kind: string(e.Type), At: e.ObservedAt}
		if e.Id != nil {
			mark.ID = e.Id.String()
		}
		marks = append(marks, mark)
	}
	return owedwork.Fingerprint(marks)
}

// entityType lifts a destination's entity type, which the contract models as a
// nullable enum and therefore a pointer.
func entityType(v crmcontracts.ContactMomentDestinationEntityType) *crmcontracts.ContactMomentDestinationEntityType {
	return &v
}

// prefill lifts the string map the contract carries as an optional object.
func prefill(v map[string]string) *map[string]string { return &v }

// recordedSeat names this contact's seat on the deal as the record spells it,
// falling back to what is true when no role was recorded.
//
// The fallback is the load-bearing half: a stakeholder edge may carry no role
// at all, and a sentence that named one anyway would be inventing the fact the
// rung exists to report.
func recordedSeat(role *string) string {
	if role == nil || strings.TrimSpace(*role) == "" {
		return "a stakeholder"
	}
	return "the recorded " + strings.ReplaceAll(*role, "_", " ")
}
