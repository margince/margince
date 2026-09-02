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
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// openPromiseMoment: something is owed and its date has not passed. Dated or
// not — the transcript reader files "I'll send you the whitepaper" without a
// date, and it is owed either way.
//
// Below gone_quiet on purpose: a promise with no clock on it can wait for a
// day, a contact who stopped answering is already costing something. Above
// role_change and everything under it, because a thing we said we would do
// outranks a thing we might do next.
//
// BOTH SOURCES, like the overdue rung above it. A promise read out of a
// conversation and one somebody typed are the same debt, so a claim that is
// not yet due reaches this card exactly as a task does. Reading only tasks
// here was how a person owing nothing but an extracted commitment showed
// "nothing needed" while the commitments card beneath said otherwise.
func openPromiseMoment(ctx context.Context, now time.Time, page *crmcontracts.Person360) (crmcontracts.PersonMoment, bool) {
	next, ok := owedwork.Soonest(owedPromises(page), now)
	if !ok {
		return crmcontracts.PersonMoment{}, false
	}
	switch promise := next.Ref.(type) {
	case crmcontracts.Activity:
		return openPromiseFrom(ctx, now, promise), true
	case crmcontracts.ConversationClaim:
		return openClaimCard(now, promise), true
	default:
		// owedPromises puts only those two types in a Ref; a third would be a
		// promise this card cannot render rather than one to show blank.
		return crmcontracts.PersonMoment{}, false
	}
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

// openPromiseWhyNow says what the date on the promise is, in the reader's
// terms: a deadline still ahead, one already behind, or none at all.
func openPromiseWhyNow(now time.Time, task crmcontracts.Activity) string {
	if task.DueAt == nil {
		return fmt.Sprintf("Promised on %s with no date set. It stays open until you do it or close it.", task.OccurredAt.Format("2 Jan"))
	}
	if past, ok := deadline.DaysPast(task.DueAt, now); ok {
		return fmt.Sprintf("Due %d days ago and still open.", past)
	}
	// Whole days of REMAINING TIME, not calendar boundaries crossed — see
	// elapsed.FullDaysUntil for why a deadline counts differently from a pair
	// of dates. A task logged from the record page is due at the end of
	// today, so this is the common case rather than a corner.
	if days := elapsed.FullDaysUntil(now, *task.DueAt); days > 0 {
		return fmt.Sprintf("Due in %d days.", days)
	}
	return "Due today."
}

// overduePromiseMoment: we are past the date on something we said we would
// do. ONE rung over both places a promise is written down — a commitment an
// extractor read out of a conversation, and a task somebody filed — because
// where a promise was recorded is a fact about this system, not about what
// the reader should do next. Ranking them apart made identical lateness
// outrank a silence from one source and lose to it from the other.
//
// The choice of WHICH late promise to name is kernel/owedwork's, so this card
// and every other surface answering "what do we owe them?" name the same one.
// That package states why the latest slip wins and why a tie goes to the claim.
func overduePromiseMoment(ctx context.Context, now time.Time, page *crmcontracts.Person360) (crmcontracts.PersonMoment, bool) {
	slipped, ok := owedwork.MostRecentlySlipped(owedPromises(page), now)
	if !ok {
		return crmcontracts.PersonMoment{}, false
	}
	switch promise := slipped.Ref.(type) {
	case crmcontracts.Activity:
		return overdueTaskCard(ctx, now, promise), true
	case crmcontracts.ConversationClaim:
		return overdueClaimCard(now, promise), true
	default:
		// owedPromises puts only those two types in a Ref; a third would be a
		// promise this card cannot render rather than one to show blank.
		return crmcontracts.PersonMoment{}, false
	}
}

// owedPromises is the page's open promises, from both places one gets written
// down, in the shape the shared ranking reads.
//
// Held by: TestAnUpcomingCommitmentIsTheMomentWithNoTaskFiled and
// TestTheLatestOverduePromiseWinsWhicheverSourceHoldsIt
// (internal/compose/person360/moments_test.go), which fail if either source
// stops reaching the card.
//
// A UNION, NOT A JOIN. Nothing writes conversation_claim.task_activity_id, so
// an extracted commitment and a task about the same thing are two unlinked
// rows here. A reader who filed a task for a promise an extractor also read
// may therefore see both, which is the honest answer until that link is
// written — the alternative is guessing which pairs mean one promise.
func owedPromises(page *crmcontracts.Person360) []owedwork.Item {
	var items []owedwork.Item
	if page.NextSteps != nil {
		for _, task := range page.NextSteps.Data {
			items = append(items, owedwork.Item{
				Ref: task, Source: owedwork.FromTask,
				DueAt: task.DueAt, FiledAt: task.OccurredAt,
			})
		}
	}
	if page.Claims != nil {
		for _, claim := range *page.Claims {
			if claim.Kind != crmcontracts.CommitmentOurs ||
				claim.Status != crmcontracts.ConversationClaimStatusOpen {
				continue
			}
			items = append(items, owedwork.Item{
				Ref: claim, Source: owedwork.FromClaim,
				DueAt: claim.DueAt, FiledAt: claimFiledAt(claim),
			})
		}
	}
	return items
}

// overdueTaskCard is the promise card for a late TASK.
//
// Its claim key stays "moment:overdue_task" even though the rule is now the
// merged overdue_promise. A dismissal is stored against the claim key and
// looked up by exact match, so renaming the key would resurrect every task a
// reader had already put away — and the key is also what keeps a dismissal of
// the late card from silencing the not-yet-due one, which shares the task but
// says something different about it.
func overdueTaskCard(ctx context.Context, now time.Time, task crmcontracts.Activity) crmcontracts.PersonMoment {
	moment := openPromiseFrom(ctx, now, task)
	moment.ClaimKey = "moment:overdue_task"
	moment.Rule = crmcontracts.PersonMomentRuleOverduePromise
	return moment
}

// overdueClaimCard is the card for a promise read out of a conversation. It
// carries the quote it was read from, which a task has nothing to match.
func overdueClaimCard(now time.Time, claim crmcontracts.ConversationClaim) crmcontracts.PersonMoment {
	overdue := elapsed.Days(*claim.DueAt, now)
	evidence := []crmcontracts.PersonMomentEvidence{{
		Type:       crmcontracts.PersonMomentEvidenceTypeActivity,
		Id:         &claim.SourceActivityId,
		Label:      claim.Body,
		Snippet:    &claim.SourceQuote,
		ObservedAt: claim.DueAt,
	}}
	return crmcontracts.PersonMoment{
		ClaimKey:            "moment:overdue_promise",
		Rule:                crmcontracts.PersonMomentRuleOverduePromise,
		RuleVersion:         ptr(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            fmt.Sprintf("You owe them: %s", claim.Body),
		WhyNow:              fmt.Sprintf("Promised for a date that passed %d days ago.", overdue),
		Confidence:          crmcontracts.PersonMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         claim.DueAt,
		RecommendedAction: crmcontracts.PersonMomentAction{
			Kind:  crmcontracts.PersonMomentActionKindDraftReply,
			Label: "Send it now",
			State: crmcontracts.PersonMomentActionStateWillConfirm,
			Destination: &crmcontracts.PersonMomentDestination{
				Surface: crmcontracts.PersonMomentDestinationSurfaceComposer,
				Prefill: prefill(map[string]string{prefillIntent: "deliver_commitment", "subject": claim.Body}),
			},
		},
	}
}

// claimFiledAt is when the promise was said, for the tie-break between two
// promises sharing a due date.
//
// A claim whose source message carries no moment gets the zero time, which
// loses every tie rather than inventing one. The alternative — reaching for
// "now" — would make an undated claim the newest promise on the record on
// every render, and it would move each time the page was drawn.
func claimFiledAt(claim crmcontracts.ConversationClaim) time.Time {
	if claim.OccurredAt == nil {
		return time.Time{}
	}
	return *claim.OccurredAt
}

// openClaimCard is the card for a promise read out of a conversation whose
// date has NOT passed, or which carries none.
//
// The sibling of overdueClaimCard, and it exists for the same reason
// openPromiseFrom does: the promise is the same either way, so the two cards
// differ only in what they say about the date. Both quote the sentence the
// promise was made in, which is the one thing a claim has and a task does not.
//
// No holder marker, unlike the task card. A claim carries no assignee — it
// records what was said, not who was given it — so it is always the
// workspace's promise and the reader is the workspace.
func openClaimCard(now time.Time, claim crmcontracts.ConversationClaim) crmcontracts.PersonMoment {
	evidence := []crmcontracts.PersonMomentEvidence{{
		Type:       crmcontracts.PersonMomentEvidenceTypeActivity,
		Id:         &claim.SourceActivityId,
		Label:      claim.Body,
		Snippet:    &claim.SourceQuote,
		ObservedAt: claim.OccurredAt,
	}}
	return crmcontracts.PersonMoment{
		ClaimKey:            "moment:open_promise_claim",
		Rule:                crmcontracts.PersonMomentRuleOpenPromise,
		RuleVersion:         ptr(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            fmt.Sprintf("You owe them: %s", claim.Body),
		WhyNow:              openClaimWhyNow(now, claim),
		Confidence:          crmcontracts.PersonMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         claim.OccurredAt,
		RecommendedAction: crmcontracts.PersonMomentAction{
			Kind:  crmcontracts.PersonMomentActionKindDraftReply,
			Label: "Write to them about it",
			State: crmcontracts.PersonMomentActionStateWillConfirm,
			Destination: &crmcontracts.PersonMomentDestination{
				Surface: crmcontracts.PersonMomentDestinationSurfaceComposer,
				Prefill: prefill(map[string]string{prefillIntent: "deliver_commitment", "subject": claim.Body}),
			},
		},
	}
}

// openClaimWhyNow says what the date on a promised commitment is. The task
// rung's sentence, for the source that has no task: a deadline still ahead,
// or none at all.
func openClaimWhyNow(now time.Time, claim crmcontracts.ConversationClaim) string {
	if claim.DueAt == nil {
		return "Promised in a conversation with no date set. It stays open until you do it or close it."
	}
	if days := elapsed.FullDaysUntil(now, *claim.DueAt); days > 0 {
		return fmt.Sprintf("Due in %d days.", days)
	}
	return "Due today."
}
