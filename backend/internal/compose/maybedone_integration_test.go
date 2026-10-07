// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An overdue promise card asks "you may have done this" once we have written
// to the contact after the promise was made, and never closes it by itself.
// Seeded through the real activity writer, so the attestation, the links and
// the audience the read filters on are the ones capture and send produce.

import (
	"context"
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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnEmailAfterTheTaskProposesItDoneAndNotYetHoldsUntilTheNext(t *testing.T) {
	e := integration.Setup(t)
	const anna, bert = "anna@kunde.example", "bert@kunde.example"
	contact := seedLinkedContact(t, e, anna)
	other := seedLinkedContact(t, e, bert)
	contactID := ids.From[ids.ContactKind](contact)

	task := logTaskFor(t, e, contact, "Send demo email", at(time.Now().Add(-48*time.Hour)))
	filed := readTask(t, e, task).OccurredAt
	after := filed.Add(time.Hour)
	svc := pageAsOf(e, filed.Add(3*time.Hour))
	read := func() crmcontracts.ContactMoment { return momentOf(e.Admin(), t, svc, contactID) }

	logEmailFor(e.Admin(), t, e, contact, anna, "outbound", true, filed.Add(-time.Hour))
	logEmailFor(e.Admin(), t, e, contact, anna, "inbound", false, after)
	logEmailFor(e.Admin(), t, e, other, bert, "outbound", true, after)
	logEmailFor(e.Admin(), t, e, contact, anna, "outbound", false, after)
	if got := read(); got.MayBeDone != nil || got.Headline != "You owe them: Send demo email" {
		t.Fatalf("card = %q, want the overdue card: no attested email to them came after the task", got.Headline)
	}

	sent := logEmailFor(e.Admin(), t, e, contact, anna, "outbound", true, after)
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

	later := logEmailFor(e.Admin(), t, e, contact, anna, "outbound", true, after.Add(time.Hour))
	if got := read(); got.MayBeDone == nil || ids.UUID(got.MayBeDone.EmailActivityId) != later {
		t.Errorf("after a later email the card is %q, want the question asked again", got.Headline)
	}
}

// The card names the email, so it is asked only of a reader who may open it:
// the author of a participants-only email is asked, a colleague outside it is not.
func TestOnlyAReaderWhoMayOpenTheEmailIsAskedAboutIt(t *testing.T) {
	e := integration.Setup(t)
	perms := principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"contact":  {Read: true, Update: true},
			"activity": {Create: true, Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	}
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, perms)
	const carla = "carla@kunde.example"
	contact := seedLinkedContact(t, e, carla)
	contactID := ids.From[ids.ContactKind](contact)

	task := logTaskFor(t, e, contact, "Send the pricing sheet", at(time.Now().Add(-48*time.Hour)))
	filed := readTask(t, e, task).OccurredAt
	sent := logEmailFor(author, t, e, contact, carla, "outbound", true, filed.Add(time.Hour))
	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](sent),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the email to its participants: %v", err)
	}
	svc := pageAsOf(e, filed.Add(3*time.Hour))

	if got := momentOf(author, t, svc, contactID); got.MayBeDone == nil || ids.UUID(got.MayBeDone.EmailActivityId) != sent {
		t.Errorf("the author's card is %q, want the question naming their own email", got.Headline)
	}
	if got := momentOf(colleague, t, svc, contactID); got.MayBeDone != nil {
		t.Errorf("a colleague outside the email's audience was asked %q, citing mail they cannot open", got.Headline)
	}
}

// pageAsOf is the contact page service reading as of now, so emails dated
// relative to the task's own timestamps have all been sent.
func pageAsOf(e *integration.Env, now time.Time) *contact360.Service {
	return contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), func() time.Time { return now })
}

func momentOf(as context.Context, t *testing.T, svc *contact360.Service, contactID ids.ContactID) crmcontracts.ContactMoment {
	t.Helper()
	page, err := svc.Assemble(as, contactID)
	if err != nil || page.Moment == nil {
		t.Fatalf("assembling contact360: %v (moment %v)", err, page.Moment)
	}
	return *page.Moment
}

// logEmailFor writes one email linked to the contact through the activity
// writer. attested is the provider's filing of it as sent by us.
func logEmailFor(as context.Context, t *testing.T, e *integration.Env, contact ids.UUID, address, direction string, attested bool, occurred time.Time) ids.UUID {
	t.Helper()
	subject := "Your demo"
	row, _, err := e.Activities.LogActivity(as, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &occurred, Source: "manual",
		CounterpartyEmail: address, CounterpartyOutboundAttested: attested,
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
