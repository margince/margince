// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The two rungs that speak for an open task: one for a promise past its date,
// one for a promise still ahead or carrying none. They share a card, differ
// only in where they sit on the ladder, and both attribute the promise from
// the reader's own seat.

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The claim keys the promise cards are stored under. A dismissal matches its
// key exactly, so these spellings are shipped data and never change.
const (
	openTaskKey      = "moment:open_promise"
	overdueTaskKey   = "moment:overdue_task"
	overdueClaimRung = "moment:overdue_promise"
	openClaimRung    = "moment:open_promise_claim"
)

// openPromiseMoment: something is owed and its date has not passed. Dated or
// not — the transcript reader files "I'll send you the whitepaper" without a
// date, and it is owed either way.
//
// Below gone_quiet on purpose: a promise with no clock on it can wait for a
// day, a contact who stopped answering is already costing something. Above
// missing_next_step and everything under it, because a thing we said we would
// do outranks a thing we might do next.
//
// BOTH SOURCES, like the overdue rung above it. A promise read out of a
// conversation and one somebody typed are the same debt, so a claim that is
// not yet due reaches this card exactly as a task does. Reading only tasks
// here was how a contact owing nothing but an extracted commitment showed
// "nothing needed" while the commitments card beneath said otherwise.
func (said momentCopy) openPromiseMoment(ctx context.Context, now time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	next, ok := owedwork.Soonest(owedPromises(page), now)
	if !ok {
		return crmcontracts.ContactMoment{}, false
	}
	switch promise := next.Ref.(type) {
	case crmcontracts.Activity:
		return said.openPromiseFrom(ctx, now, promise), true
	case crmcontracts.ConversationClaim:
		return said.openClaimCard(now, promise), true
	default:
		// owedPromises puts only those two types in a Ref; a third would be a
		// promise this card cannot render rather than one to show blank.
		return crmcontracts.ContactMoment{}, false
	}
}

// openPromiseFrom is the card both promise rungs show: same headline, same
// reason, same verb. Only the rung's name and its place on the ladder differ,
// and neither is a fact about the promise.
func (said momentCopy) openPromiseFrom(ctx context.Context, now time.Time, task crmcontracts.Activity) crmcontracts.ContactMoment {
	// A task filed without a subject is still owed; the card names it as the
	// task list does rather than printing an empty promise.
	subject := said.Say(momentWords.OpenTask)
	prefill := ""
	if task.Subject != nil && *task.Subject != "" {
		subject = *task.Subject
		prefill = *task.Subject
	}
	// The evidence is dated by when the promise was filed, not when it falls
	// due: a task due next month is not "updated today", and the due date is
	// the reason's to state.
	observed := task.OccurredAt
	evidence := []crmcontracts.ContactMomentEvidence{{
		Type:       crmcontracts.ContactMomentEvidenceTypeTask,
		Id:         new(task.Id),
		Label:      subject,
		ObservedAt: &observed,
	}}
	// The fingerprint carries WHOSE promise this is, not only which task it
	// is. A dismissal is keyed on the fingerprint so that the moment comes
	// back when its evidence moves; handing a colleague's task to the reader
	// changes the card from "your team owes them" to "you owe them", and
	// without the holder in the fingerprint an old dismissal would keep
	// suppressing it — hiding the promise at the moment it became theirs to
	// deliver.
	return crmcontracts.ContactMoment{
		ClaimKey:            openTaskKey,
		Rule:                crmcontracts.ContactMomentRuleOpenPromise,
		RuleVersion:         new(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence) + heldMarker(ctx, task) + dueMarker(task.DueAt),
		Headline:            said.openPromiseHeadline(ctx, task, subject),
		WhyNow:              said.openPromiseWhyNow(now, task),
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         &observed,
		// Writing to them is the one move every promise shares, whether it is
		// a document to send or a room to book they are waiting to hear about.
		// The composer opens on the promise itself, so the label says what the
		// button does rather than presuming the promise is a thing to send.
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindDraftReply,
			Label: said.Say(momentWords.Write),
			State: crmcontracts.ContactMomentActionStateWillConfirm,
			Destination: &crmcontracts.ContactMomentDestination{
				Surface: crmcontracts.ContactMomentDestinationSurfaceComposer,
				Prefill: new(map[string]string{prefillIntent: deliverCommitmentIntent, prefillSubject: prefill}),
			},
		},
	}
}

// openPromiseHeadline names who owes it, from the READER's seat.
//
// The comparison is against the reader rather than against nil: the activity
// writer assigns every human-written task to its author, so "has an assignee"
// is true of almost every task and would say "your team owes them" about the
// reader's own work. A task held by a colleague still belongs on this card —
// the record owes it either way — but it is not the reader's to deliver, and
// a card that says otherwise sends them to do somebody else's job. Unassigned
// work is the workspace's, and the reader is the workspace.
func (said momentCopy) openPromiseHeadline(ctx context.Context, task crmcontracts.Activity, subject string) string {
	if heldByAnother(ctx, task) {
		return fmt.Sprintf(said.Say(momentWords.TeamOwes), subject)
	}
	return fmt.Sprintf(said.Say(momentWords.YouOwe), subject)
}

// heldMarker distinguishes the two cards one task can produce, so a
// dismissal of one does not silence the other.
func heldMarker(ctx context.Context, task crmcontracts.Activity) string {
	if heldByAnother(ctx, task) {
		return ":theirs"
	}
	return ":ours"
}

// dueMarker carries the task's due date into the fingerprint. The evidence is
// dated by when the task was filed. Without the due date, a promise moved to
// today would stay hidden behind a set-aside made while it was far off.
func dueMarker(due *time.Time) string {
	if due == nil {
		return ":undated"
	}
	return ":due=" + due.UTC().Format(time.RFC3339)
}

// heldByAnother reports whether a named colleague, and not the reader, holds
// this task.
//
// An agent claims nobody's work as its own. A passport carries its granting
// human's UserID, so comparing ids would answer about that human and the
// phrasing would call their task "ours". auth.RequireHuman is what tells the
// two apart, which a check for a non-zero id does not.
func heldByAnother(ctx context.Context, task crmcontracts.Activity) bool {
	if task.AssigneeId == nil {
		return false
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return true
	}
	viewer, ok := principal.Actor(ctx)
	if !ok || viewer.UserID == (ids.UUID{}) {
		return true
	}
	return ids.UUID(*task.AssigneeId) != viewer.UserID
}

// openPromiseWhyNow says what the date on the promise is, in the reader's
// terms: a deadline still ahead, one already behind, or none at all.
func (said momentCopy) openPromiseWhyNow(now time.Time, task crmcontracts.Activity) string {
	if task.DueAt == nil {
		logged := occurredday.DateIn(task.OccurredAt, activitywhen.IsDay(task.OccurredPrecision), said.zone)
		return fmt.Sprintf(said.Say(momentWords.TaskUndated), logged.Format(time.DateOnly))
	}
	return owedwork.DeadlineWords(*task.DueAt, now, string(said.Lang()))
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
func (said momentCopy) overduePromiseMoment(ctx context.Context, now time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	slipped, ok := owedwork.MostRecentlySlipped(owedPromises(page), now)
	if !ok {
		return crmcontracts.ContactMoment{}, false
	}
	switch promise := slipped.Ref.(type) {
	case crmcontracts.Activity:
		return said.overdueTaskCard(ctx, now, promise), true
	case crmcontracts.ConversationClaim:
		return said.overdueClaimCard(now, promise), true
	default:
		// owedPromises puts only those two types in a Ref; a third would be a
		// promise this card cannot render rather than one to show blank.
		return crmcontracts.ContactMoment{}, false
	}
}

// owedPromises is the page's open promises, from both places one gets written
// down, in the shape the shared ranking reads.
//
// Held by: TestAnUpcomingCommitmentIsTheMomentWithNoTaskFiled and
// TestTheLatestOverduePromiseWinsWhicheverSourceHoldsIt
// (internal/compose/contact360/moments_test.go), which fail if either source
// stops reaching the card.
//
// A claim that became a task is left to its task, so the promise is counted
// once. A task a rep typed for a promise an extractor also read stays a second
// row: nothing links the two, and guessing which pairs mean one promise would
// be this card inventing a fact.
func owedPromises(page *crmcontracts.Contact360) []owedwork.Item {
	var items []owedwork.Item
	if page.NextSteps != nil {
		for _, task := range page.NextSteps.Data {
			// A task the system minted — a check-in or renewal reminder — is
			// the product nudging the reader, not a promise anybody made, so
			// it stays on the task list and off the promise rungs. The account
			// card and the review_commitments sweep exclude the same rows in
			// SQL (activities.NotSystemMinted, the predicate's other spelling);
			// this rung reads the already-assembled section, so it filters the
			// page instead. The section's cap therefore bounds what this rung
			// can see, which it did before this filter too — and a single
			// contact would need a full page of open reminders due ahead of a
			// promise before the difference showed.
			if task.CapturedBy != nil && principal.SystemMintedID(*task.CapturedBy) {
				continue
			}
			items = append(items, owedwork.Item{
				Ref: task, Source: owedwork.FromTask,
				DueAt: task.DueAt, FiledAt: task.OccurredAt,
			})
		}
	}
	if page.Claims != nil {
		for _, claim := range *page.Claims {
			if claim.Kind != crmcontracts.ConversationClaimKindCommitmentOurs ||
				claim.Status != crmcontracts.ConversationClaimStatusOpen || claim.NeedsReview || claim.TaskActivityId != nil {
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
func (said momentCopy) overdueTaskCard(ctx context.Context, now time.Time, task crmcontracts.Activity) crmcontracts.ContactMoment {
	moment := said.openPromiseFrom(ctx, now, task)
	moment.ClaimKey = overdueTaskKey
	moment.Rule = crmcontracts.ContactMomentRuleOverduePromise
	return moment
}

// overdueClaimCard is the card for a promise read out of a conversation. It
// carries the quote it was read from, which a task has nothing to match.
func (said momentCopy) overdueClaimCard(now time.Time, claim crmcontracts.ConversationClaim) crmcontracts.ContactMoment {
	// Deliberately not owedwork.LateWords: a claim names the date the
	// SPEAKER promised, so its sentence keeps that framing, and its count is
	// elapsed calendar days rather than whole days behind.
	whyNow := said.Say(momentWords.PromisedToday)
	if overdue := elapsed.Days(*claim.DueAt, now, said.zone); overdue > 0 {
		whyNow = fmt.Sprintf(said.Say(momentWords.PromisedAgo), overdue)
		if overdue == 1 {
			whyNow = said.Say(momentWords.PromisedYesterday)
		}
	}
	evidence := []crmcontracts.ContactMomentEvidence{{
		Type:       crmcontracts.ContactMomentEvidenceTypeActivity,
		Id:         &claim.SourceActivityId,
		Label:      claim.Body,
		Snippet:    &claim.SourceQuote,
		ObservedAt: claim.DueAt,
	}}
	return crmcontracts.ContactMoment{
		ClaimKey:            claimMomentKey(overdueClaimRung, claim),
		Rule:                crmcontracts.ContactMomentRuleOverduePromise,
		RuleVersion:         new(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            fmt.Sprintf(said.Say(momentWords.TeamOwes), claim.Body),
		WhyNow:              whyNow,
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         claim.DueAt,
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindDraftReply,
			Label: said.Say(momentWords.Send),
			State: crmcontracts.ContactMomentActionStateWillConfirm,
			Destination: &crmcontracts.ContactMomentDestination{
				Surface: crmcontracts.ContactMomentDestinationSurfaceComposer,
				Prefill: new(map[string]string{prefillIntent: deliverCommitmentIntent, prefillSubject: claim.Body}),
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

// A claim has no assignee, so its original quote describes a team promise.
func (said momentCopy) openClaimCard(now time.Time, claim crmcontracts.ConversationClaim) crmcontracts.ContactMoment {
	evidence := []crmcontracts.ContactMomentEvidence{{
		Type:       crmcontracts.ContactMomentEvidenceTypeActivity,
		Id:         &claim.SourceActivityId,
		Label:      claim.Body,
		Snippet:    &claim.SourceQuote,
		ObservedAt: claim.OccurredAt,
	}}
	return crmcontracts.ContactMoment{
		ClaimKey:            claimMomentKey(openClaimRung, claim),
		Rule:                crmcontracts.ContactMomentRuleOpenPromise,
		RuleVersion:         new(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            fmt.Sprintf(said.Say(momentWords.TeamOwes), claim.Body),
		WhyNow:              said.openClaimWhyNow(now, claim),
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         claim.OccurredAt,
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindDraftReply,
			Label: said.Say(momentWords.Write),
			State: crmcontracts.ContactMomentActionStateWillConfirm,
			Destination: &crmcontracts.ContactMomentDestination{
				Surface: crmcontracts.ContactMomentDestinationSurfaceComposer,
				Prefill: new(map[string]string{prefillIntent: deliverCommitmentIntent, prefillSubject: claim.Body}),
			},
		},
	}
}

// openClaimWhyNow says what the date on a promised commitment is. The task
// rung's sentence, for the source that has no task: a deadline still ahead,
// or none at all.
func (said momentCopy) openClaimWhyNow(now time.Time, claim crmcontracts.ConversationClaim) string {
	if claim.DueAt == nil {
		return said.Say(momentWords.ClaimUndated)
	}
	return owedwork.UpcomingWords(*claim.DueAt, now, string(said.Lang()))
}

// claimMomentKey names WHICH promise a card is about, not merely that it is a
// promise card.
//
// A dismissal is one row per (reader, contact, claim key), so a bare rung name
// would give every promise on a record one shared dismissal: an email that
// says "I'll send the NDA and the quote" produces two claims, and putting the
// first away would silence the second the moment it came up. The claim id is
// what tells them apart.
//
// The fingerprint cannot do this job. It hashes the evidence — type, source
// row and moment — and deliberately ignores the words, so that rewording a
// promise does not re-arm a dismissal a reader already made. Two claims read
// out of ONE message share all three, which is exactly the case that needs
// separating.
//
// THIS WIDENS A SHIPPED KEY, and the overdue rung's dismissals do not survive
// it: a reader who had put an overdue commitment away sees it once more. That
// is the cost, and it is the cheaper of the two. The collision it removes hides
// a promise the reader never dismissed, permanently and invisibly — they are
// never told the second promise exists. A card that comes back is visible and
// one click to put away again.
func claimMomentKey(rung string, claim crmcontracts.ConversationClaim) string {
	return rung + ":" + claim.Id.String()
}
