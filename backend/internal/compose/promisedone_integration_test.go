// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestALaterReplyOnThePromiseThreadToItsRecipientAsksDone(t *testing.T) {
	e := integration.Setup(t)
	p := e.SeedPromiseThread(t)
	reply := e.LogThreadMail(e.Admin(), t, p.Contact, "thread-listing", p.Address, "outbound", p.SentAt.AddDate(0, 0, 6))
	page, err := pageAsOf(e, p.SentAt.AddDate(0, 0, 7)).Assemble(e.Admin(), p.Contact)
	if err != nil || page.Moment == nil {
		t.Fatalf("assembling the contact page: %v", err)
	}
	got := *page.Moment
	if got.MayBeDone == nil {
		t.Fatalf("our later reply on the promise's thread to its recipient asked nothing: %+v", got)
	}
	if got.MayBeDone.PromiseType != crmcontracts.ContactMomentMayBeDonePromiseTypeClaim ||
		ids.UUID(got.MayBeDone.PromiseId) != ids.UUID(p.Claim.Id) || ids.UUID(got.MayBeDone.EmailActivityId) != reply {
		t.Fatalf("may_be_done = %+v, want the claim and the reply behind it", got.MayBeDone)
	}
	if got.Headline != "This may be done: an email went to them on 2026-09-13" {
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
			p := e.SeedPromiseThread(t)
			e.LogThreadMail(e.Admin(), t, p.Contact, mail.thread, mail.counterparty, mail.direction, p.SentAt.AddDate(0, 0, 2))
			got := momentOf(e.Admin(), t, pageAsOf(e, p.SentAt.AddDate(0, 0, 3)), p.Contact)
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
	p := e.SeedPromiseThread(t)
	reply := e.LogThreadMail(author, t, p.Contact, "thread-listing", p.Address, "outbound", p.SentAt.AddDate(0, 0, 2))
	if _, err := e.Activities.SetAudience(author, ids.From[ids.ActivityKind](reply), activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the reply to its participants: %v", err)
	}
	svc := pageAsOf(e, p.SentAt.AddDate(0, 0, 3))
	if got := momentOf(author, t, svc, p.Contact); got.MayBeDone == nil {
		t.Fatalf("the reply's author was not asked: %+v", got)
	}
	if got := momentOf(colleague, t, svc, p.Contact); got.MayBeDone != nil {
		t.Fatalf("a colleague was shown a reply they may not open: %+v", got.MayBeDone)
	}
}

func TestDoneWithdrawsWhenTheReplyIsArchived(t *testing.T) {
	e := integration.Setup(t)
	p := e.SeedPromiseThread(t)
	reply := e.LogThreadMail(e.Admin(), t, p.Contact, "thread-listing", p.Address, "outbound", p.SentAt.AddDate(0, 0, 2))
	svc := pageAsOf(e, p.SentAt.AddDate(0, 0, 3))
	if got := momentOf(e.Admin(), t, svc, p.Contact); got.MayBeDone == nil {
		t.Fatal("the reply asked nothing")
	}
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](reply), nil); err != nil {
		t.Fatalf("archiving the reply: %v", err)
	}
	if got := momentOf(e.Admin(), t, svc, p.Contact); got.MayBeDone != nil {
		t.Fatalf("an archived reply still asks Done?: %+v", got.MayBeDone)
	}
}
