// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An overdue promise card asks "you may have done this" once we have written
// to the contact after the promise was made, and never closes it by itself.
// Seeded through the real activity writer, so the attestation, the links and
// the audience the read filters on are the ones capture and send produce.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnEmailAfterTheTaskProposesItDoneAndNotYetHoldsUntilTheNext(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "anna@kunde.example")
	other := seedLinkedContact(t, e, "bert@kunde.example")
	contactID := ids.From[ids.ContactKind](contact)

	task := logTaskFor(t, e, contact, "Send demo email", at(time.Now().Add(-48*time.Hour)))
	created := readTask(t, e, task).CreatedAt
	// Every email sits clear of the task's creation by an hour, so a drift
	// between the database clock and this one cannot move it across.
	after := created.Add(time.Hour)
	// The page reads as of three hours on, so every email below has been sent.
	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), func() time.Time { return created.Add(3 * time.Hour) })
	read := func() crmcontracts.ContactMoment {
		t.Helper()
		page, err := svc.Assemble(e.Admin(), contactID)
		if err != nil || page.Moment == nil {
			t.Fatalf("assembling contact360: %v (moment %v)", err, page.Moment)
		}
		return *page.Moment
	}

	logEmailFor(t, e, contact, "outbound", true, created.Add(-time.Hour))
	logEmailFor(t, e, contact, "inbound", false, after)
	logEmailFor(t, e, other, "outbound", true, after)
	logEmailFor(t, e, contact, "outbound", false, after)
	if got := read(); got.MayBeDone != nil || got.Headline != "You owe them: Send demo email" {
		t.Fatalf("card = %q, want the overdue card: no attested email to them came after the task", got.Headline)
	}

	sent := logEmailFor(t, e, contact, "outbound", true, after)
	question := read()
	if question.MayBeDone == nil || ids.UUID(question.MayBeDone.PromiseId) != task ||
		ids.UUID(question.MayBeDone.EmailActivityId) != sent {
		t.Fatalf("card = %q (%+v), want the question about the task naming the email", question.Headline, question.MayBeDone)
	}
	if done := readTask(t, e, task).IsDone; done != nil && *done {
		t.Fatal("the task was completed without anybody pressing Done")
	}

	if err := svc.DismissMoment(e.Admin(), contactID, crmcontracts.DismissContactMomentRequest{
		ClaimKey: question.ClaimKey, EvidenceFingerprint: question.EvidenceFingerprint,
	}); err != nil {
		t.Fatalf("Not yet: %v", err)
	}
	if got := read(); got.MayBeDone != nil || got.Headline != "You owe them: Send demo email" {
		t.Fatalf("after Not yet the card is %q, want the overdue card back", got.Headline)
	}

	later := logEmailFor(t, e, contact, "outbound", true, after.Add(time.Hour))
	if got := read(); got.MayBeDone == nil || ids.UUID(got.MayBeDone.EmailActivityId) != later {
		t.Errorf("after a later email the card is %q, want the question asked again", got.Headline)
	}
}

// logEmailFor writes one email linked to the contact through the activity
// writer. attested is the provider's filing of it as sent by us.
func logEmailFor(t *testing.T, e *integration.Env, contact ids.UUID, direction string, attested bool, occurred time.Time) ids.UUID {
	t.Helper()
	subject := "Your demo"
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &occurred, Source: "manual",
		CounterpartyEmail: "anna@kunde.example", CounterpartyOutboundAttested: attested,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging a %s email: %v", direction, err)
	}
	return ids.UUID(row.Id)
}

// readTask reads the task back through the activity store.
func readTask(t *testing.T, e *integration.Env, task ids.UUID) crmcontracts.Activity {
	t.Helper()
	row, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the task back: %v", err)
	}
	return row
}
