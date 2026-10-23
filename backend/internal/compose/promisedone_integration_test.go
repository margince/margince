// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// promiseThread is one promise we made by mail, on its own thread.
type promiseThread struct {
	contact ids.ContactID
	address string
	claim   crmcontracts.ConversationClaim
	sentAt  time.Time
}

func seedPromiseThread(t *testing.T, e *integration.Env) promiseThread {
	t.Helper()
	const address = "lena@kunde.example"
	row, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Lena Kunde", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	contact := ids.From[ids.ContactKind](ids.UUID(row.Id))
	sentAt := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)
	promise := logThreadMail(e.Admin(), t, e, contact, "thread-listing", address, "outbound", sentAt)
	quote := "I'll prepare the listing details and get those over to you shortly."
	claim := e.RecordClaim(e.Admin(), t, contacts.ClaimInput{
		ContactID: contact, Kind: "commitment_ours", Body: "Send the listing details", ActivityID: promise, Quote: quote, Source: "manual",
	})
	return promiseThread{contact: contact, address: address, claim: claim, sentAt: sentAt}
}

// logThreadMail writes one email on a thread through the activity writer.
// Outbound mail is attested as sent by us.
func logThreadMail(as context.Context, t *testing.T, e *integration.Env, contact ids.ContactID, thread, counterparty, direction string, at time.Time) ids.UUID {
	t.Helper()
	subject := "Re: Listing details"
	from, to := "rep@margince.example", counterparty
	if direction == "inbound" {
		from, to = counterparty, "rep@margince.example"
	}
	row, _, err := e.Activities.LogActivity(as, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &at, Source: "manual",
		ThreadKey: thread, CounterpartyEmail: counterparty, EmailFrom: from, EmailTo: []string{to}, CounterpartyOutboundAttested: direction == "outbound",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact.UUID}},
	})
	if err != nil {
		t.Fatalf("logging a %s email on %s: %v", direction, thread, err)
	}
	return ids.UUID(row.Id)
}

func TestALaterReplyOnThePromiseThreadToItsRecipientAsksDone(t *testing.T) {
	e := integration.Setup(t)
	p := seedPromiseThread(t, e)
	reply := logThreadMail(e.Admin(), t, e, p.contact, "thread-listing", p.address, "outbound", p.sentAt.AddDate(0, 0, 6))
	page, err := pageAsOf(e, p.sentAt.AddDate(0, 0, 7)).Assemble(e.Admin(), p.contact)
	if err != nil || page.Moment == nil {
		t.Fatalf("assembling the contact page: %v", err)
	}
	got := *page.Moment
	if got.MayBeDone == nil {
		t.Fatalf("our later reply on the promise's thread to its recipient asked nothing: %+v", got)
	}
	if got.MayBeDone.PromiseType != crmcontracts.ContactMomentMayBeDonePromiseTypeClaim ||
		ids.UUID(got.MayBeDone.PromiseId) != ids.UUID(p.claim.Id) || ids.UUID(got.MayBeDone.EmailActivityId) != reply {
		t.Fatalf("may_be_done = %+v, want the claim and the reply behind it", got.MayBeDone)
	}
	if got.Headline != "You may have done this — you wrote to them on 2026-09-13" {
		t.Fatalf("headline = %q", got.Headline)
	}
	if page.Claims == nil || len(*page.Claims) != 1 || (*page.Claims)[0].Status != crmcontracts.ConversationClaimStatusOpen {
		t.Fatalf("the question settled the promise by itself: %+v", page.Claims)
	}
}

func TestOnlyOurReplyOnThePromiseThreadToItsRecipientAsksDone(t *testing.T) {
	for name, mail := range map[string]struct{ thread, counterparty, direction string }{
		"their acknowledgement on the thread":    {"thread-listing", "lena@kunde.example", "inbound"},
		"our mail on the thread to someone else": {"thread-listing", "kollege@kunde.example", "outbound"},
		"our unrelated mail to the recipient":    {"thread-invoice", "lena@kunde.example", "outbound"},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			p := seedPromiseThread(t, e)
			logThreadMail(e.Admin(), t, e, p.contact, mail.thread, mail.counterparty, mail.direction, p.sentAt.AddDate(0, 0, 2))
			got := momentOf(e.Admin(), t, pageAsOf(e, p.sentAt.AddDate(0, 0, 3)), p.contact)
			if got.MayBeDone != nil {
				t.Fatalf("%s raised Done?: %+v", name, got.MayBeDone)
			}
			if got.Rule != crmcontracts.ContactMomentRuleOpenPromise {
				t.Fatalf("the plain promise card is gone: rule %s", got.Rule)
			}
		})
	}
}

func TestDoneIsNotAskedOverAReplyTheReaderMayNotOpen(t *testing.T) {
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
	p := seedPromiseThread(t, e)
	reply := logThreadMail(author, t, e, p.contact, "thread-listing", p.address, "outbound", p.sentAt.AddDate(0, 0, 2))
	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](reply), activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the reply to its participants: %v", err)
	}
	svc := pageAsOf(e, p.sentAt.AddDate(0, 0, 3))
	if got := momentOf(author, t, svc, p.contact); got.MayBeDone == nil {
		t.Fatalf("the reply's author was not asked: %+v", got)
	}
	if got := momentOf(colleague, t, svc, p.contact); got.MayBeDone != nil {
		t.Fatalf("a colleague was shown a reply they may not open: %+v", got.MayBeDone)
	}
}

func TestDoneWithdrawsWhenTheReplyIsArchived(t *testing.T) {
	e := integration.Setup(t)
	p := seedPromiseThread(t, e)
	reply := logThreadMail(e.Admin(), t, e, p.contact, "thread-listing", p.address, "outbound", p.sentAt.AddDate(0, 0, 2))
	svc := pageAsOf(e, p.sentAt.AddDate(0, 0, 3))
	if got := momentOf(e.Admin(), t, svc, p.contact); got.MayBeDone == nil {
		t.Fatal("the reply asked nothing")
	}
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](reply), nil); err != nil {
		t.Fatalf("archiving the reply: %v", err)
	}
	if got := momentOf(e.Admin(), t, svc, p.contact); got.MayBeDone != nil {
		t.Fatalf("an archived reply still asks Done?: %+v", got.MayBeDone)
	}
}

func TestTheAccountCardAsksDoneAndNotYetBringsThePromiseBack(t *testing.T) {
	e := integration.Setup(t)
	p := seedPromiseThread(t, e)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Kunde GmbH", &e.AdminUser))
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &p.contact, CompanyID: &company, IsCurrentPrimary: new(true), Source: "manual"}); err != nil {
		t.Fatalf("employing the contact: %v", err)
	}
	reply := logThreadMail(e.Admin(), t, e, p.contact, "thread-listing", p.address, "outbound", p.sentAt.AddDate(0, 0, 6))
	now := p.sentAt.AddDate(0, 0, 7)
	svc := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, approvals.NewService(e.DB()), func() time.Time { return now })
	card := func() crmcontracts.ContactMoment {
		t.Helper()
		page, err := svc.Assemble(e.Admin(), company)
		if err != nil || page.Moment == nil {
			t.Fatalf("assembling the company page: %v", err)
		}
		return *page.Moment
	}
	asked := card()
	if asked.MayBeDone == nil || ids.UUID(asked.MayBeDone.PromiseId) != ids.UUID(p.claim.Id) || ids.UUID(asked.MayBeDone.EmailActivityId) != reply {
		t.Fatalf("the account card did not ask about the reply: %+v", asked)
	}
	if err := svc.DismissSuggestion(e.Admin(), company, asked.EvidenceFingerprint, nil); err != nil {
		t.Fatalf("answering Not yet: %v", err)
	}
	if plain := card(); plain.MayBeDone != nil || plain.Rule != crmcontracts.ContactMomentRuleOpenPromise {
		t.Fatalf("Not yet did not bring the plain promise back: %+v", plain)
	}
}
