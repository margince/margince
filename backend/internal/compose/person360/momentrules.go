// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// The lower rungs of the moment ladder and the machinery every rung shares.
//
// They live beside moments.go rather than inside it because the ladder's ORDER
// is the decision worth reading in one screen, and the individual conditions
// are detail. A reader asking "what does this page open on" should not have to
// scroll through seven rule bodies to find out.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
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
func roleChangeMoment(_ time.Time, page *crmcontracts.Person360) (crmcontracts.PersonMoment, bool) {
	change, ok := findChange(page, relstrength.ChangeRepliedAfterGap)
	if !ok {
		return crmcontracts.PersonMoment{}, false
	}
	evidence := []crmcontracts.PersonMomentEvidence{{
		Type:       crmcontracts.PersonMomentEvidenceTypeRelationshipChange,
		Label:      "They replied after a long gap",
		ObservedAt: &change.At,
	}}
	return crmcontracts.PersonMoment{
		ClaimKey:            "moment:role_change",
		Rule:                crmcontracts.PersonMomentRuleRoleChange,
		RuleVersion:         ptr(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            "They answered after a long silence",
		WhyNow:              "A relationship that had gone quiet has moved. The window where a reply is expected is now.",
		Confidence:          crmcontracts.PersonMomentConfidenceObservedFact,
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
func withheld(page *crmcontracts.Person360, sections ...crmcontracts.Person360SectionsOmitted) bool {
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
// person who sits on it. The gap is the finding.
func missingNextStepMoment(_ time.Time, page *crmcontracts.Person360) (crmcontracts.PersonMoment, bool) {
	// "Nothing is scheduled" is only true if this reader could see the schedule.
	if withheld(page, crmcontracts.Person360SectionsOmittedNextMeeting,
		crmcontracts.Person360SectionsOmittedNextSteps,
		crmcontracts.Person360SectionsOmittedCommercial) {
		return crmcontracts.PersonMoment{}, false
	}
	if page.Commercial == nil || page.Commercial.Deal == nil {
		return crmcontracts.PersonMoment{}, false
	}
	if page.NextMeeting != nil {
		return crmcontracts.PersonMoment{}, false
	}
	if page.NextSteps != nil && len(page.NextSteps.Data) > 0 {
		return crmcontracts.PersonMoment{}, false
	}
	deal := *page.Commercial.Deal
	evidence := []crmcontracts.PersonMomentEvidence{{
		Type:  crmcontracts.PersonMomentEvidenceTypeRelationshipChange,
		Label: fmt.Sprintf("%s has no next step with them", deal.Title),
	}}
	return crmcontracts.PersonMoment{
		ClaimKey:            "moment:missing_next_step",
		Rule:                crmcontracts.PersonMomentRuleMissingNextStep,
		RuleVersion:         ptr(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            "No next step with them on an open deal",
		WhyNow:              "The deal is live and nothing is scheduled with the person whose seat decides it.",
		Confidence:          crmcontracts.PersonMomentConfidenceObservedFact,
		Evidence:            evidence,
		RecommendedAction:   bookMeeting(),
		SecondaryActions: &[]crmcontracts.PersonMomentAction{{
			Kind:        crmcontracts.PersonMomentActionKindOpenRecord,
			Label:       "Open the deal",
			State:       crmcontracts.PersonMomentActionStateAvailable,
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
func thinRelationshipMoment(_ time.Time, page *crmcontracts.Person360) (crmcontracts.PersonMoment, bool) {
	if page.Activities == nil || page.Network == nil {
		// A section the caller may not read contributes no moment. Absent is
		// not the same as empty, and only one of them is a fact about the
		// relationship.
		return crmcontracts.PersonMoment{}, false
	}
	if len(page.Activities.Data) > 0 || len(page.Network.Colleagues) > 0 {
		return crmcontracts.PersonMoment{}, false
	}
	evidence := []crmcontracts.PersonMomentEvidence{{
		Type:  crmcontracts.PersonMomentEvidenceTypeRelationshipChange,
		Label: "Nothing captured, nobody connected",
	}}
	return crmcontracts.PersonMoment{
		ClaimKey:            "moment:thin_relationship",
		Rule:                crmcontracts.PersonMomentRuleThinRelationship,
		RuleVersion:         ptr(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            "Nothing is captured about them yet",
		WhyNow:              "There is no correspondence and no colleague who knows them. Everything about this record is still to be learned.",
		Confidence:          crmcontracts.PersonMomentConfidenceObservedFact,
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
func dealRecord(dealID openapi_types.UUID) *crmcontracts.PersonMomentDestination {
	entity := crmcontracts.PersonMomentDestinationEntityTypeDeal
	return &crmcontracts.PersonMomentDestination{
		Surface:    crmcontracts.PersonMomentDestinationSurfaceRecord,
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
func openDeal(page *crmcontracts.Person360) crmcontracts.PersonMomentAction {
	action := crmcontracts.PersonMomentAction{
		Kind:  crmcontracts.PersonMomentActionKindOpenRecord,
		Label: "Open the deal",
		State: crmcontracts.PersonMomentActionStateAvailable,
	}
	if page.Commercial == nil || page.Commercial.Deal == nil {
		reason := "No open deal is visible on this record"
		action.State = crmcontracts.PersonMomentActionStateBlocked
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
func bookMeeting() crmcontracts.PersonMomentAction {
	reason := "Booking a meeting from this card is not available yet"
	return crmcontracts.PersonMomentAction{
		Kind:          crmcontracts.PersonMomentActionKindScheduleMeeting,
		Label:         "Book a meeting",
		State:         crmcontracts.PersonMomentActionStateBlocked,
		BlockedReason: &reason,
	}
}

// oldestOverdueCommitment finds the promise of OURS that has been late longest.
//
// Oldest rather than newest: the one that has been waiting longest is the one
// with the most damage already done, and the one a reader would be most
// embarrassed to discover from the other side.
func oldestOverdueCommitment(now time.Time, page *crmcontracts.Person360) (crmcontracts.ConversationClaim, bool) {
	if page.Claims == nil {
		return crmcontracts.ConversationClaim{}, false
	}
	var oldest *crmcontracts.ConversationClaim
	for i := range *page.Claims {
		claim := &(*page.Claims)[i]
		if claim.Kind != crmcontracts.CommitmentOurs || claim.Status != crmcontracts.ConversationClaimStatusOpen {
			continue
		}
		if !deadline.Passed(claim.DueAt, now) {
			continue
		}
		if oldest == nil || claim.DueAt.Before(*oldest.DueAt) {
			oldest = claim
		}
	}
	if oldest == nil {
		return crmcontracts.ConversationClaim{}, false
	}
	return *oldest, true
}

// inboundEvidence names the actual message where the page is showing it, and
// falls back to the bare fact when the timeline is capped past it.
//
// The fallback is honest rather than silent: the claim is true either way, and
// pretending there is a row to open when the reader would land on nothing is
// worse than saying the message is older than this page shows.
func inboundEvidence(page *crmcontracts.Person360, inbound time.Time) crmcontracts.PersonMomentEvidence {
	return directionEvidence(page, inbound, "Their last message")
}

// outboundEvidence is the same lookup for the message WE sent.
func outboundEvidence(page *crmcontracts.Person360, outbound time.Time) []crmcontracts.PersonMomentEvidence {
	return []crmcontracts.PersonMomentEvidence{
		directionEvidence(page, outbound, "Your last message"),
	}
}

func directionEvidence(page *crmcontracts.Person360, at time.Time, fallback string) crmcontracts.PersonMomentEvidence {
	evidence := crmcontracts.PersonMomentEvidence{
		Type:       crmcontracts.PersonMomentEvidenceTypeActivity,
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
func findChange(page *crmcontracts.Person360, kind string) (crmcontracts.PersonRelationshipChange, bool) {
	if page.RelationshipChanges == nil {
		return crmcontracts.PersonRelationshipChange{}, false
	}
	for _, c := range *page.RelationshipChanges {
		if string(c.Kind) == kind {
			return c, true
		}
	}
	return crmcontracts.PersonRelationshipChange{}, false
}

// findActivityAt finds the timeline row for an instant the page reported
// separately. The two come from the same transaction, so a match is exact
// rather than approximate.
func findActivityAt(page *crmcontracts.Person360, at time.Time) (crmcontracts.Activity, bool) {
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
// It hashes the identity and timing of each piece — not the label, which is
// prose this build may reword without the underlying fact having moved. A
// reworded headline must not silently un-dismiss a moment the reader put away.
func fingerprintOf(evidence []crmcontracts.PersonMomentEvidence) string {
	// Built as one string and hashed once. sha256's Write never returns an
	// error, but writing through it would still spread unchecked returns
	// across four calls to say something a single Sum says here.
	var b strings.Builder
	for _, e := range evidence {
		b.WriteString(string(e.Type))
		b.WriteByte(0)
		if e.Id != nil {
			b.WriteString(e.Id.String())
		}
		b.WriteByte(0)
		if e.ObservedAt != nil {
			b.WriteString(strconv.FormatInt(e.ObservedAt.UTC().UnixNano(), 10))
		}
		b.WriteByte(0)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// entityType lifts a destination's entity type, which the contract models as a
// nullable enum and therefore a pointer.
func entityType(v crmcontracts.PersonMomentDestinationEntityType) *crmcontracts.PersonMomentDestinationEntityType {
	return &v
}

// prefill lifts the string map the contract carries as an optional object.
func prefill(v map[string]string) *map[string]string { return &v }

// openPromiseMoment: an open task is filed against them and nobody has done
// it. Dated or not — the transcript reader files "I'll send you the
// whitepaper" without a date, and it is owed either way.
//
// Below gone_quiet on purpose: a promise with no clock on it can wait for a
// day, a contact who stopped answering is already costing something. Above
// role_change and everything under it, because a thing we said we would do
// outranks a thing we might do next. The overdue rung above reads claims
// from correspondence; this one reads the task list, so a promise the reader
// accepted from a transcript reaches the card the moment it becomes a task.
func openPromiseMoment(now time.Time, page *crmcontracts.Person360) (crmcontracts.PersonMoment, bool) {
	task, ok := nearestOpenTask(page)
	if !ok {
		return crmcontracts.PersonMoment{}, false
	}
	// A task filed without a subject is still owed; the card names it as the
	// task list does rather than printing an empty promise.
	subject := "an open task"
	if task.Subject != nil && *task.Subject != "" {
		subject = *task.Subject
	}
	// The evidence is dated by when the promise was filed, not when it falls
	// due: a task due next month is not "updated today", and the due date is
	// the reason's to state.
	observed := task.OccurredAt
	evidence := []crmcontracts.PersonMomentEvidence{{
		Type:       crmcontracts.PersonMomentEvidenceTypeTask,
		Id:         ptr(task.Id),
		Label:      subject,
		ObservedAt: &observed,
	}}
	return crmcontracts.PersonMoment{
		ClaimKey:            "moment:open_promise",
		Rule:                crmcontracts.PersonMomentRuleOpenPromise,
		RuleVersion:         ptr(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            openPromiseHeadline(task, subject),
		WhyNow:              openPromiseWhyNow(now, task),
		Confidence:          crmcontracts.PersonMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         &observed,
		// Writing to them is the one move every promise shares, whether it is
		// a document to send or a room to book they are waiting to hear about.
		// The composer opens on the promise itself, so the label says what the
		// button does rather than presuming the promise is a thing to send.
		RecommendedAction: crmcontracts.PersonMomentAction{
			Kind:  crmcontracts.PersonMomentActionKindDraftReply,
			Label: "Write to them about it",
			State: crmcontracts.PersonMomentActionStateWillConfirm,
			Destination: &crmcontracts.PersonMomentDestination{
				Surface: crmcontracts.PersonMomentDestinationSurfaceComposer,
				Prefill: prefill(map[string]string{prefillIntent: "deliver_commitment", "subject": subject}),
			},
		},
	}, true
}

// openPromiseHeadline names who owes it. A task with an assignee is that
// person's, and the card is read by the whole workspace — "you owe them" to a
// reader whose colleague holds the task attributes a promise to the wrong
// desk. Unassigned, it is the workspace's, and the reader is the workspace.
func openPromiseHeadline(task crmcontracts.Activity, subject string) string {
	if task.AssigneeId != nil {
		return fmt.Sprintf("Owed to them: %s", subject)
	}
	return fmt.Sprintf("You owe them: %s", subject)
}

// nearestOpenTask picks the one open task the card speaks for: the earliest
// due date first, and among undated ones the oldest filed. One promise on the
// card, not a list — the task list below it is where the rest live.
func nearestOpenTask(page *crmcontracts.Person360) (crmcontracts.Activity, bool) {
	if page.NextSteps == nil {
		return crmcontracts.Activity{}, false
	}
	var pick *crmcontracts.Activity
	for i := range page.NextSteps.Data {
		task := &page.NextSteps.Data[i]
		if pick == nil || openTaskSooner(task, pick) {
			pick = task
		}
	}
	if pick == nil {
		return crmcontracts.Activity{}, false
	}
	return *pick, true
}

// openTaskSooner orders two open tasks: a dated one before an undated one,
// then by due date, then by when it was filed. Two tasks due the same day
// fall through to the filing order on purpose: the section lists newest
// first, and without the fall-through the card switched to whichever task
// was logged last rather than the one that has waited longest.
func openTaskSooner(a, b *crmcontracts.Activity) bool {
	switch {
	case a.DueAt != nil && b.DueAt != nil && !a.DueAt.Equal(*b.DueAt):
		return a.DueAt.Before(*b.DueAt)
	case a.DueAt != nil && b.DueAt == nil:
		return true
	case a.DueAt == nil && b.DueAt != nil:
		return false
	default:
		return a.OccurredAt.Before(b.OccurredAt)
	}
}

// openPromiseWhyNow says what the date on the promise is, in the reader's
// terms: a deadline still ahead, one already behind, or none at all.
func openPromiseWhyNow(now time.Time, task crmcontracts.Activity) string {
	if task.DueAt == nil {
		return fmt.Sprintf("Promised on %s with no date set. It stays open until you do it or close it.", task.OccurredAt.Format("2 Jan"))
	}
	if past, ok := deadline.DaysPast(task.DueAt, now); ok {
		return fmt.Sprintf("Due %d days ago and still open.", past)
	}
	// A task logged from the record page carries today's date unless the
	// writer picks another, so "in 0 days" is the common case, not a corner.
	if days := elapsed.Days(now, *task.DueAt); days > 0 {
		return fmt.Sprintf("Due in %d days.", days)
	}
	return "Due today."
}
