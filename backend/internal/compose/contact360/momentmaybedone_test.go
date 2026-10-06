// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// asIs is the ladder with no email to ask about.
func asIs(moment crmcontracts.ContactMoment) crmcontracts.ContactMoment { return moment }

// overdueTaskPage is a contact owing one task, filed three days ago and due
// yesterday.
func overdueTaskPage() (*crmcontracts.Contact360, crmcontracts.Activity) {
	filed := now.Add(-72 * time.Hour)
	task := crmcontracts.Activity{
		Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
		Subject:    ptr("Follow up with Anna: send demo email"),
		OccurredAt: filed, CreatedAt: filed, DueAt: ptr(now.Add(-24 * time.Hour)),
	}
	return &crmcontracts.Contact360{NextSteps: timelineOf(task)}, task
}

func sentAt(at time.Time) *wroteTo {
	return &wroteTo{id: openapi_types.UUID(ids.NewV7()), at: at, subject: "Your demo"}
}

func askedWith(page *crmcontracts.Contact360, sent *wroteTo, dismissed func(crmcontracts.ContactMoment) bool) crmcontracts.ContactMoment {
	return deriveMomentPast(readerCtx(), now, page, dismissed, proposer(page, sent, dismissed))
}

func neverDismissed(crmcontracts.ContactMoment) bool { return false }

func TestAnEmailAfterTheTaskAsksWhetherItWasDone(t *testing.T) {
	page, task := overdueTaskPage()
	sent := sentAt(now.Add(-6 * time.Hour))

	got := askedWith(page, sent, neverDismissed)
	if got.Headline != "You may have done this — you wrote to them on 4 Aug" {
		t.Fatalf("headline = %q, want the question naming the day we wrote", got.Headline)
	}
	if got.Rule != crmcontracts.ContactMomentRuleOverduePromise {
		t.Errorf("rule = %q, want the overdue rung the question replaced", got.Rule)
	}
	if got.MayBeDone == nil || got.MayBeDone.PromiseId != task.Id ||
		got.MayBeDone.PromiseType != crmcontracts.ContactMomentMayBeDonePromiseTypeTask ||
		got.MayBeDone.EmailActivityId != sent.id {
		t.Fatalf("may_be_done = %+v, want the task and the email behind the question", got.MayBeDone)
	}
	if !strings.HasPrefix(got.WhyNow, "You owe them: Follow up with Anna") {
		t.Errorf("why_now = %q, want the promise itself named under the question", got.WhyNow)
	}
	if got.RecommendedAction.Kind != crmcontracts.ContactMomentActionKindCompleteTask ||
		got.RecommendedAction.Label != "Done" {
		t.Errorf("verb = %+v, want Done", got.RecommendedAction)
	}
	assertActionsAreHonest(t, got)
}

// Only an email sent strictly after the promise was made can have kept it.
func TestAnEmailNotAfterThePromiseChangesNothing(t *testing.T) {
	page, task := overdueTaskPage()
	for name, sent := range map[string]*wroteTo{
		"no email":           nil,
		"before the task":    sentAt(task.CreatedAt.Add(-time.Hour)),
		"the task's instant": sentAt(task.CreatedAt),
	} {
		t.Run(name, func(t *testing.T) {
			got := askedWith(page, sent, neverDismissed)
			if got.MayBeDone != nil || got.Headline != "You owe them: Follow up with Anna: send demo email" {
				t.Errorf("card = %q, want the overdue card unchanged", got.Headline)
			}
		})
	}
}

// Not yet puts the question away and the overdue card comes back for the
// same task; a later email asks again.
func TestNotYetKeepsTheOverdueCardUntilALaterEmail(t *testing.T) {
	page, task := overdueTaskPage()
	first := sentAt(now.Add(-6 * time.Hour))
	question := askedWith(page, first, neverDismissed)

	notYet := func(m crmcontracts.ContactMoment) bool {
		return m.ClaimKey == question.ClaimKey && m.EvidenceFingerprint == question.EvidenceFingerprint
	}
	got := askedWith(page, first, notYet)
	if got.MayBeDone != nil || got.Rule != crmcontracts.ContactMomentRuleOverduePromise ||
		got.Evidence[0].Id == nil || *got.Evidence[0].Id != task.Id {
		t.Fatalf("after Not yet the card is %q, want the overdue card for the same task", got.Headline)
	}

	later := sentAt(now.Add(-time.Hour))
	again := askedWith(page, later, notYet)
	if again.MayBeDone == nil || again.MayBeDone.EmailActivityId != later.id {
		t.Errorf("after a later email the card is %q, want the question asked again", again.Headline)
	}
}

// A commitment read out of a conversation is asked about too, and Done on it
// settles the claim rather than a task it does not have.
func TestAnEmailAfterAPromisedCommitmentAsksAboutTheClaim(t *testing.T) {
	said := now.Add(-48 * time.Hour)
	claim := crmcontracts.ConversationClaim{
		Id:   openapi_types.UUID(ids.NewV7()),
		Kind: crmcontracts.ConversationClaimKindCommitmentOurs, Status: crmcontracts.ConversationClaimStatusOpen,
		Body: "Send the revised quote", SourceQuote: "Ich schicke dir das Angebot.",
		SourceActivityId: openapi_types.UUID(ids.NewV7()), DueAt: ptr(now.Add(-24 * time.Hour)), OccurredAt: &said,
	}
	page := &crmcontracts.Contact360{Claims: &[]crmcontracts.ConversationClaim{claim}}

	got := askedWith(page, sentAt(now.Add(-time.Hour)), neverDismissed)
	if got.MayBeDone == nil || got.MayBeDone.PromiseType != crmcontracts.ContactMomentMayBeDonePromiseTypeClaim ||
		got.MayBeDone.PromiseId != claim.Id {
		t.Fatalf("may_be_done = %+v, want the claim behind the question", got.MayBeDone)
	}
	if before := askedWith(page, sentAt(said.Add(-time.Hour)), neverDismissed); before.MayBeDone != nil {
		t.Errorf("an email before the promise was said asked %q", before.Headline)
	}
}

// Done writes, so a reader who may not log activity sees it blocked.
func TestDoneIsWithheldFromAReaderWhoCannotWrite(t *testing.T) {
	page, _ := overdueTaskPage()
	got := askedWith(page, sentAt(now.Add(-time.Hour)), neverDismissed)
	momentaction.Withhold(as(nil), &got)
	if got.RecommendedAction.State != crmcontracts.ContactMomentActionStateBlocked {
		t.Errorf("Done state = %q for a reader without activity.create, want blocked", got.RecommendedAction.State)
	}
}
