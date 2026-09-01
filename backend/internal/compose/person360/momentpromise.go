// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// The two rungs that speak for an open task: one for a promise past its date,
// one for a promise still ahead or carrying none. They share a card, differ
// only in where they sit on the ladder, and both attribute the promise from
// the reader's own seat.

import (
	"context"
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

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
func openPromiseMoment(ctx context.Context, now time.Time, page *crmcontracts.Person360) (crmcontracts.PersonMoment, bool) {
	task, ok := nearestOpenTask(page)
	if !ok {
		return crmcontracts.PersonMoment{}, false
	}
	// A task already past its date is the rung above; this one speaks for the
	// promises that are undated or still ahead.
	if deadline.Passed(task.DueAt, now) {
		return crmcontracts.PersonMoment{}, false
	}
	moment := openPromiseFrom(ctx, now, task)
	return moment, true
}

// openPromiseFrom is the card both promise rungs show: same headline, same
// reason, same verb. Only the rung's name and its place on the ladder differ,
// and neither is a fact about the promise.
func openPromiseFrom(ctx context.Context, now time.Time, task crmcontracts.Activity) crmcontracts.PersonMoment {
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
	// The fingerprint carries WHOSE promise this is, not only which task it
	// is. A dismissal is keyed on the fingerprint so that the moment comes
	// back when its evidence moves; handing a colleague's task to the reader
	// changes the card from "owed to them" to "you owe them", and without the
	// holder in the fingerprint an old dismissal would keep suppressing it —
	// hiding the promise at the moment it became theirs to deliver.
	return crmcontracts.PersonMoment{
		ClaimKey:            "moment:open_promise",
		Rule:                crmcontracts.PersonMomentRuleOpenPromise,
		RuleVersion:         ptr(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence) + heldMarker(ctx, task),
		Headline:            openPromiseHeadline(ctx, task, subject),
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
	}
}

// openPromiseHeadline names who owes it, from the READER's seat.
//
// The comparison is against the reader rather than against nil: the activity
// writer assigns every human-written task to its author, so "has an assignee"
// is true of almost every task and would say "owed to them" about the
// reader's own work. A task held by a colleague still belongs on this card —
// the record owes it either way — but it is not the reader's to deliver, and
// a card that says otherwise sends them to do somebody else's job. Unassigned
// work is the workspace's, and the reader is the workspace.
func openPromiseHeadline(ctx context.Context, task crmcontracts.Activity, subject string) string {
	if heldByAnother(ctx, task) {
		return fmt.Sprintf("Owed to them: %s", subject)
	}
	return fmt.Sprintf("You owe them: %s", subject)
}

// heldMarker distinguishes the two cards one task can produce, so a
// dismissal of one does not silence the other.
func heldMarker(ctx context.Context, task crmcontracts.Activity) string {
	if heldByAnother(ctx, task) {
		return ":theirs"
	}
	return ":ours"
}

// heldByAnother reports whether a named colleague, and not the reader, holds
// this task. A call carrying no user (an agent reading through a passport)
// claims nobody's work as its own.
func heldByAnother(ctx context.Context, task crmcontracts.Activity) bool {
	if task.AssigneeId == nil {
		return false
	}
	viewer, ok := principal.Actor(ctx)
	if !ok || viewer.UserID == (ids.UUID{}) {
		return true
	}
	return ids.UUID(*task.AssigneeId) != viewer.UserID
}

// nearestOpenTask is the section's FIRST row, and the ordering is the
// section's own: the next-steps read sorts by due date, then filing order
// (byUrgency in sectionstimeline.go), so the row a reader sees at the top of
// their task list is the row the card speaks for. Picking a different one
// here would put the card and the list beneath it in disagreement, and
// re-sorting the 25 rows this page carries could not see the 26th anyway.
func nearestOpenTask(page *crmcontracts.Person360) (crmcontracts.Activity, bool) {
	if page.NextSteps == nil || len(page.NextSteps.Data) == 0 {
		return crmcontracts.Activity{}, false
	}
	return page.NextSteps.Data[0], true
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

// overdueTaskMoment: a task we owe them was due and is still open.
//
// It sits directly under the overdue-claim rung and ABOVE gone_quiet, which
// is the whole point of separating it from openPromiseMoment below. The two
// rungs read one fact — we are late on something we agreed — from the two
// places a promise gets written down: a claim an extractor read out of an
// email, and a task a person filed or confirmed. Ranking them apart made the
// same lateness outrank a silence when it came from mail and lose to one when
// it came from the task list, which is a rule about provenance rather than
// about what the reader should do next.
func overdueTaskMoment(ctx context.Context, now time.Time, page *crmcontracts.Person360) (crmcontracts.PersonMoment, bool) {
	task, ok := nearestOpenTask(page)
	if !ok || !deadline.Passed(task.DueAt, now) {
		return crmcontracts.PersonMoment{}, false
	}
	moment := openPromiseFrom(ctx, now, task)
	moment.ClaimKey = "moment:overdue_task"
	moment.Rule = crmcontracts.PersonMomentRuleOverdueTask
	return moment, true
}
